package cli

import (
	"fmt"
	"os"

	"github.com/shiftu/keel/internal/adapter"
	"github.com/shiftu/keel/internal/brief"
	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/render"
	"github.com/shiftu/keel/internal/store"
)

func cmdSync(e *env, args []string) error {
	fs := newFlagSet("sync")
	_ = commonFlags(fs, e)
	dryRun := fs.Bool("dry-run", false, "只打印计划与冲突，不写入")
	codemap := fs.Bool("codemap", false, "这一轮同时生成目录概览（长期开启用 knowledge.codemap）")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("sync", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return err
	}
	return runSyncOpts(e, st, syncOpts{dryRun: *dryRun, verbose: true, codemap: *codemap})
}

type syncOpts struct {
	dryRun  bool
	verbose bool
	codemap bool
}

// runSync 先完整规划 diff 和冲突，再写入。有冲突就一个都不写。
func runSync(e *env, st *store.Store, dryRun, verbose bool) error {
	return runSyncOpts(e, st, syncOpts{dryRun: dryRun, verbose: verbose})
}

func runSyncOpts(e *env, st *store.Store, opt syncOpts) error {
	dryRun, verbose := opt.dryRun, opt.verbose
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	set, err := st.Load()
	if err != nil {
		return err
	}
	if len(set.Errors) > 0 {
		return fmt.Errorf(".keel/ 里有 %d 个对象读不出来，先跑 keel check 修好再 sync", len(set.Errors))
	}

	in := adapter.Input{
		Protocol:  brief.Protocol(cfg),
		SoftRules: brief.SoftRules(set),
		Config:    cfg,
	}
	if _, err := os.Stat(st.SkillsDir()); err == nil {
		in.Skills = os.DirFS(st.SkillsDir())
	}

	// 知识索引不属于任何一家工具：clone 之后不装 keel 也能读。
	arts := []render.Artifact{{
		Path:    render.KnowledgeIndexPath,
		Mode:    render.ModeWholeFile,
		Content: render.KnowledgeIndex(set, cfg),
	}}
	if opt.codemap || cfg.Knowledge.Codemap {
		repo, err := gitx.Open(st.Root)
		if err != nil {
			return fmt.Errorf("--codemap 需要 git 仓库：%w", err)
		}
		files, err := repo.TrackedFiles()
		if err != nil {
			return err
		}
		arts = append(arts, render.Artifact{
			Path:    render.CodemapPath,
			Mode:    render.ModeWholeFile,
			Content: render.Codemap(files),
		})
	}
	schema := map[string]string{}
	for _, name := range cfg.Tools {
		a, ok := adapter.ByName(name)
		if !ok {
			return fmt.Errorf("keel.yaml 里的 tools 有未知工具 %q（支持 claude、codex）", name)
		}
		schema[name] = a.SchemaVersion()
		got, err := a.Artifacts(in)
		if err != nil {
			return err
		}
		arts = append(arts, got...)
	}

	prev, err := render.LoadGenerated(st.GeneratedPath())
	if err != nil {
		return err
	}
	plan, err := render.BuildPlan(st.Root, prev, arts, schema)
	if err != nil {
		return err
	}

	if len(plan.Conflicts) > 0 {
		e.io.Errln("sync 发现冲突，没有写入任何文件：")
		for _, c := range plan.Conflicts {
			e.io.Errf("✘ %s\n    %s\n    › %s\n", c.Path, c.Reason, c.Fix)
		}
		return checkFailed
	}

	if dryRun {
		for _, op := range plan.Ops {
			if op.Kind == render.OpUnchanged {
				continue
			}
			e.io.Printf("%-9s %s\n", op.Kind, op.Path)
		}
		if !plan.Changed() {
			e.io.Println("无变化")
		}
		return nil
	}

	if err := render.Apply(st.Root, plan); err != nil {
		return err
	}
	data, err := plan.Next.Marshal()
	if err != nil {
		return err
	}
	if err := store.WriteFileAtomic(st.GeneratedPath(), data, 0o644); err != nil {
		return err
	}

	if verbose && !e.quiet {
		changed := 0
		for _, op := range plan.Ops {
			if op.Kind == render.OpUnchanged {
				continue
			}
			changed++
			e.io.Printf("%-9s %s\n", op.Kind, op.Path)
		}
		if changed == 0 {
			e.io.Println("无变化")
		}
	}
	return nil
}
