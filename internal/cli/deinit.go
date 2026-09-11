package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/render"
	"github.com/shiftu/keel/internal/store"
)

// cmdDeinit 是 init 的逆操作：只带走 keel 自己写过的东西。见 design.md §12 M7。
//
// 严格倒序，而且先撤依赖 .keel/ 的部分，最后才碰 .keel/：
//  1. 产物——generated.yaml 是唯一清单，工具侧的产物集合当空集跑一次规划；
//     .keel/ 内的产物（knowledge/INDEX.md）不在集合里，随 .keel/ 一起留下。
//  2. git hook——含标记的才是 keel 的；串联过的把 .keel-local 换回去。
//  3. .keel/cache/——一律清。
//  4. .keel/——默认保留；--purge 才删，且要求它在 git 里干净。
//
// 和 sync 一样：有任何冲突就一个文件都不写。跑两次的结果和跑一次相同。
func cmdDeinit(e *env, args []string) error {
	fs := newFlagSet("deinit")
	jsonOut := commonFlags(fs, e)
	purge := fs.Bool("purge", false, "连 .keel/ 一起删（要求它在 git 里是干净的）")
	dryRun := fs.Bool("dry-run", false, "只打印计划与冲突，不写入")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("deinit", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return usagef("这里没有 .keel/，没什么可退出的")
	}

	// git 不是硬要求：仓库的 .git 没了，产物照样能清；只是 hook 和 --purge 的检查做不了。
	repo, gerr := gitx.Open(st.Root)
	if gerr != nil {
		repo = nil
	}

	// 1. 产物计划。.keel/ 内的记录留给保留模式，工具侧的全部清掉。
	prev, err := render.LoadGenerated(st.GeneratedPath())
	if err != nil {
		return err
	}
	var keep, drop []render.GenFile
	for _, rec := range prev.Files {
		if strings.HasPrefix(rec.Path, store.DirName+"/") {
			keep = append(keep, rec)
		} else {
			drop = append(drop, rec)
		}
	}
	plan, err := render.BuildPlan(st.Root, render.Generated{Files: drop}, nil, map[string]string{})
	if err != nil {
		return err
	}
	plan.Next.Files = keep

	// 2. hook 计划。
	var hooks []gitx.HookRemoval
	if repo != nil {
		hooks, err = repo.PlanRemoveHooks()
		if err != nil {
			return err
		}
	}
	for _, h := range hooks {
		if h.Blocked {
			plan.Conflicts = append(plan.Conflicts, render.Conflict{
				Path: st.Rel(h.Path), Reason: h.Detail, Fix: "处理好这个 hook 再 deinit",
			})
		}
	}

	// 3. --purge 的前提：.keel/ 在 git 里干净。没提交的删了就真没了。
	if *purge {
		if repo == nil {
			return usagef("--purge 需要 git 仓库：没提交过的 .keel/ 删了就找不回来")
		}
		dirty, err := repo.StatusPorcelain(store.DirName)
		if err != nil {
			return err
		}
		if dirty != "" {
			e.io.Errln(".keel/ 里有没提交的改动，--purge 拒绝删除：")
			for _, line := range strings.Split(dirty, "\n") {
				e.io.Errln("  " + line)
			}
			e.io.Errln("先提交它们（删了还能从历史里捞），或者不带 --purge 只退出集成。")
			return checkFailed
		}
	}

	if len(plan.Conflicts) > 0 {
		e.io.Errln("deinit 发现冲突，没有写入任何文件：")
		for _, c := range plan.Conflicts {
			e.io.Errf("✘ %s\n    %s\n    › %s\n", c.Path, c.Reason, c.Fix)
		}
		return checkFailed
	}

	report := deinitReport{Schema: 1, DryRun: *dryRun, KeelDir: "kept"}
	if *purge {
		report.KeelDir = "purged"
	}
	for _, op := range plan.Ops {
		if op.Kind == render.OpUnchanged {
			continue
		}
		report.Ops = append(report.Ops, deinitOp{Kind: string(op.Kind), Path: op.Path})
	}
	for _, h := range hooks {
		if h.Action == "none" {
			continue
		}
		report.Hooks = append(report.Hooks, deinitHook{
			Name: string(h.Name), Path: st.Rel(h.Path), Action: h.Action, Detail: h.Detail,
		})
	}
	report.Notes = deinitNotes(st, repo, *purge)

	if !*dryRun {
		if err := render.Apply(st.Root, plan); err != nil {
			return err
		}
		if repo != nil {
			if err := repo.RemoveHooks(hooks); err != nil {
				return err
			}
		}
		if *purge {
			if err := os.RemoveAll(st.KeelDir()); err != nil {
				return err
			}
		} else {
			if err := os.RemoveAll(st.CacheDir()); err != nil {
				return err
			}
			data, err := plan.Next.Marshal()
			if err != nil {
				return err
			}
			if err := store.WriteFileAtomic(st.GeneratedPath(), data, 0o644); err != nil {
				return err
			}
		}
	}

	if *jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		e.io.Println(string(data))
		return nil
	}
	printDeinitReport(e, report, *purge)
	return nil
}

type deinitReport struct {
	Schema  int          `json:"schema"`
	DryRun  bool         `json:"dry_run"`
	Ops     []deinitOp   `json:"ops"`
	Hooks   []deinitHook `json:"hooks"`
	KeelDir string       `json:"keel_dir"`
	Notes   []string     `json:"notes"`
}

type deinitOp struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type deinitHook struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

func printDeinitReport(e *env, r deinitReport, purge bool) {
	changed := len(r.Ops)
	for _, op := range r.Ops {
		e.io.Printf("%-9s %s\n", op.Kind, op.Path)
	}
	for _, h := range r.Hooks {
		if h.Action != "keep" {
			changed++
		}
		line := "hook " + padRight(h.Name, 11) + " " + h.Action + "（" + h.Path + "）"
		if h.Detail != "" {
			line += " " + h.Detail
		}
		e.io.Println(line)
	}
	switch {
	case purge:
		e.io.Printf("%-9s %s/\n", "purge", store.DirName)
	case changed == 0:
		e.io.Println("无变化")
	}
	if e.quiet {
		return
	}
	for _, n := range r.Notes {
		e.io.Println(n)
	}
}

func padRight(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// deinitNotes 列出退出后需要人知道的事：留下了什么、别处还有什么要接。
func deinitNotes(st *store.Store, repo *gitx.Repo, purge bool) []string {
	var notes []string
	if purge {
		notes = append(notes, store.DirName+"/ 已删除；提交过的内容仍在 git 历史里。")
	} else {
		notes = append(notes, "保留了 "+store.DirName+"/：决策、规则、记忆和证据都还在，不装 keel 也能读。"+
			"要连它一起删：keel deinit --purge；要接回来：keel init。")
	}
	if repo != nil {
		if n := repo.WorktreeCount(); n > 1 {
			notes = append(notes, "git hook 是仓库级的：其他工作树也一并没有提交检查了。")
		}
	}
	if files := ciFilesMentioningKeel(st.Root); len(files) > 0 {
		notes = append(notes, "这些 CI 配置里还提到 keel，自己删掉那一步：\n  "+strings.Join(files, "\n  "))
	}
	notes = append(notes, "改动都在工作树里，没有提交。")
	return notes
}

// ciFilesMentioningKeel 找出仍然调用 keel 的 CI 配置。keel 不改它们，只指出来。
func ciFilesMentioningKeel(root string) []string {
	var patterns = []string{
		".github/workflows/*.yml", ".github/workflows/*.yaml",
		".gitea/workflows/*.yml", ".gitea/workflows/*.yaml",
		".gitlab-ci.yml",
	}
	var out []string
	for _, pat := range patterns {
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
		for _, m := range matches {
			data, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			if strings.Contains(string(data), "keel ") {
				if rel, err := filepath.Rel(root, m); err == nil {
					out = append(out, filepath.ToSlash(rel))
				}
			}
		}
	}
	return out
}
