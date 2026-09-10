package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shiftu/keel/internal/adapter"
	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/store"
	"github.com/shiftu/keel/internal/template"
	"github.com/shiftu/keel/templates"
)

func cmdInit(e *env, args []string) error {
	fs_ := newFlagSet("init")
	_ = commonFlags(fs_, e)
	tools := fs_.String("tools", "", "要适配的工具，逗号分隔（默认探测 PATH）")
	from := fs_.String("from", "", "从模板仓库导入：<git-url>[@<ref>]")
	noHooks := fs_.Bool("no-hooks", false, "不安装 git hooks")
	adoptHooks := fs_.Bool("adopt-hooks", false, "已有 shell hook 时串联而不是放弃")

	rest, err := parseArgs(fs_, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("init", rest, 0); err != nil {
		return err
	}

	repo, err := gitx.Open(e.dir)
	if err != nil {
		return usagef("keel init 需要在 git 仓库里运行（%v）", err)
	}
	st := &store.Store{Root: repo.Root}

	// 1. 目录骨架。已有的一律不覆盖。
	for _, d := range []string{"decisions", "rules", "memory", "evidence", "skills", "knowledge", "cache"} {
		if err := os.MkdirAll(filepath.Join(st.KeelDir(), d), 0o755); err != nil {
			return err
		}
	}
	if err := writeIfMissing(filepath.Join(st.KeelDir(), ".gitignore"), []byte("cache/\n")); err != nil {
		return err
	}
	if err := writeIfMissing(st.IntentPath(), templates.Intent()); err != nil {
		return err
	}

	// 2. 模板导入。必须排在写配置之前：模板的 keel.yaml 就是这个仓库的起点。
	//    这一步和 template update 是仅有的两个联网入口。
	importedConfig := false
	if *from != "" {
		var err error
		importedConfig, err = importTemplate(e, st, *from)
		if err != nil {
			return err
		}
	}

	// 3. 配置。模板带来的那份只换掉 tools —— tools 是本机探测结果，不跨仓库。
	cfgPath := st.ConfigPath()
	_, statErr := os.Stat(cfgPath)
	switch {
	case os.IsNotExist(statErr):
		cfg := store.DefaultConfig()
		if err := setTools(&cfg, *tools); err != nil {
			return err
		}
		data, err := cfg.Marshal()
		if err != nil {
			return err
		}
		if err := store.WriteFileAtomic(cfgPath, data, 0o644); err != nil {
			return err
		}
	case importedConfig:
		cfg, err := st.LoadConfig()
		if err != nil {
			return err
		}
		if err := setTools(&cfg, *tools); err != nil {
			return err
		}
		data, err := cfg.Marshal()
		if err != nil {
			return err
		}
		if err := store.WriteFileAtomic(cfgPath, data, 0o644); err != nil {
			return err
		}
	case *tools != "":
		return usagef("%s 已存在；要改工具列表请直接编辑它的 tools 字段", st.Rel(cfgPath))
	}

	// 4. 内置技能。模板已经带来的那份不会被盖掉。
	if err := copyFS(templates.Skills(), st.SkillsDir()); err != nil {
		return err
	}

	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Tools) == 0 {
		e.io.Errln("提示：PATH 里没探测到 claude 或 codex。已建好 .keel/，" +
			"要生成工具文件请在 .keel/keel.yaml 的 tools 里写上工具名再 keel sync。")
	}

	// 5. git hooks。需要人介入时如实失败，不静默跳过。
	if !*noHooks {
		plans, err := repo.InstallHooks(*adoptHooks)
		for _, p := range plans {
			if p.Blocked {
				continue
			}
			e.io.Printf("hook %-11s %s（%s）\n", p.Name, p.Action, st.Rel(p.Path))
		}
		if err != nil {
			return err
		}
	}

	// 6. 生成产物。
	if err := runSync(e, st, false, false); err != nil {
		return err
	}

	// 7. 能力报告。没法确认的说 unknown，不拿「文件存在」当已启用。
	e.io.Println("")
	for _, name := range cfg.Tools {
		a, ok := adapter.ByName(name)
		if !ok {
			continue
		}
		e.io.Printf("%s（schema %s）\n", name, a.SchemaVersion())
		for _, c := range a.Capabilities(st.Root) {
			e.io.Printf("  %-14s %-11s %s\n", c.Capability, c.Support, c.Detail)
		}
	}
	e.io.Println("")
	e.io.Println("下一步：keel brief 看上下文 · keel decide 记决策 · keel check --target index 验证要提交的内容")
	return nil
}

// setTools 定下要适配的工具：显式给的优先，否则探测 PATH。
func setTools(cfg *store.Config, tools string) error {
	cfg.Tools = splitList(tools)
	if len(cfg.Tools) == 0 {
		cfg.Tools = adapter.Detect()
	}
	for _, t := range cfg.Tools {
		if _, ok := adapter.ByName(t); !ok {
			return usagef("未知工具 %q（支持 claude、codex）", t)
		}
	}
	return nil
}

// importTemplate 首次导入：取模板、按导入面写入、把来源钉进 template.yaml。
// 返回 keel.yaml 是不是这次写进来的。
//
// 和 init 的其余步骤一样，已存在的文件一律不覆盖——但基线照记，
// 这样后面 template update 能如实报出「本地和模板都改过」。
func importTemplate(e *env, st *store.Store, spec string) (wroteConfig bool, err error) {
	if prev, ok, perr := template.LoadState(statePath(st)); perr != nil {
		return false, perr
	} else if ok {
		return false, usagef("这个仓库已经从 %s 导入过模板了；要更新用 keel template update", prev.Source)
	}
	source, ref, err := template.ParseSource(spec)
	if err != nil {
		return false, err
	}
	dir, commit, cleanup, err := template.Fetch(source, ref)
	if err != nil {
		return false, err
	}
	defer cleanup()

	theirs, err := template.Surface(filepath.Join(dir, store.DirName),
		template.Provenance(source, commit))
	if err != nil {
		return false, err
	}
	state := template.State{Schema: template.StateSchema, Source: source, Ref: ref, Commit: commit}
	changes, err := template.Plan(state, theirs, st.KeelDir())
	if err != nil {
		return false, err
	}
	if err := template.Apply(&state, changes, theirs, st.KeelDir()); err != nil {
		return false, err
	}
	data, err := state.Marshal()
	if err != nil {
		return false, err
	}
	if err := store.WriteFileAtomic(statePath(st), data, 0o644); err != nil {
		return false, err
	}

	counts := template.Counts(changes)
	e.io.Printf("模板 %s@%s\n", source, commit)
	e.io.Printf("  导入 %d 个文件", counts[template.Add])
	if n := counts[template.KeepLocal]; n > 0 {
		e.io.Printf("；本地已有 %d 个，未覆盖", n)
	}
	e.io.Println("")
	if n := countRules(changes); n > 0 {
		e.io.Printf("  %d 条规则落成 candidate：要生效得先 keel decide 记依据，再 keel promote 跑对照验证\n", n)
	}
	for _, c := range changes {
		if c.Action == template.KeepLocal {
			e.io.Printf("  保留本地 %s\n", c.Path)
		}
	}
	e.io.Println("")

	for _, c := range changes {
		if c.Action.Writes() && c.Path == template.ConfigPath {
			wroteConfig = true
		}
	}
	return wroteConfig, nil
}

func countRules(changes []template.Change) int {
	n := 0
	for _, c := range changes {
		if c.Action.Writes() && strings.HasPrefix(c.Path, "rules/") {
			n++
		}
	}
	return n
}

func writeIfMissing(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return store.WriteFileAtomic(path, data, 0o644)
}

// copyFS 把内嵌技能写进 .keel/skills/，已存在的文件不覆盖（用户可以改）。
func copyFS(src fs.FS, dst string) error {
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := fs.ReadFile(src, p)
		if rerr != nil {
			return rerr
		}
		return writeIfMissing(target, data)
	})
}

func toolList(cfg store.Config) string { return strings.Join(cfg.Tools, "、") }

var _ = fmt.Sprintf
