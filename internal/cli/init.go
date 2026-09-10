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
	"github.com/shiftu/keel/templates"
)

func cmdInit(e *env, args []string) error {
	fs_ := newFlagSet("init")
	_ = commonFlags(fs_, e)
	tools := fs_.String("tools", "", "要适配的工具，逗号分隔（默认探测 PATH）")
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

	// 2. 配置。
	cfgPath := st.ConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := store.DefaultConfig()
		cfg.Tools = splitList(*tools)
		if len(cfg.Tools) == 0 {
			cfg.Tools = adapter.Detect()
		}
		for _, t := range cfg.Tools {
			if _, ok := adapter.ByName(t); !ok {
				return usagef("未知工具 %q（支持 claude、codex）", t)
			}
		}
		data, err := cfg.Marshal()
		if err != nil {
			return err
		}
		if err := store.WriteFileAtomic(cfgPath, data, 0o644); err != nil {
			return err
		}
	} else if *tools != "" {
		return usagef("%s 已存在；要改工具列表请直接编辑它的 tools 字段", st.Rel(cfgPath))
	}

	// 3. 内置技能。
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

	// 4. git hooks。需要人介入时如实失败，不静默跳过。
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

	// 5. 生成产物。
	if err := runSync(e, st, false, false); err != nil {
		return err
	}

	// 6. 能力报告。没法确认的说 unknown，不拿「文件存在」当已启用。
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
