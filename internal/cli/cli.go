// Package cli 是面向用户的命令层：参数校验、业务退出码、中文输出。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/shiftu/keel/internal/store"
	"github.com/shiftu/keel/internal/ui"
)

// Version 由 ldflags 覆盖。
var Version = "0.1.0-dev"

type env struct {
	io    ui.IO
	stdin io.Reader
	dir   string
	quiet bool
}

// discover 找到当前仓库的 .keel/。
func (e *env) discover() (*store.Store, error) {
	return store.Discover(e.dir)
}

type command struct {
	name    string
	summary string
	run     func(e *env, args []string) error
	hidden  bool // 内部入口，不在帮助里推荐
}

func commands() []command {
	return []command{
		{"init", "建 .keel/，探测工具，安装 git hooks，然后 sync", cmdInit, false},
		{"sync", ".keel/ → 工具原生文件；先规划再写入", cmdSync, false},
		{"decide", "记录一次架构决策", cmdDecide, false},
		{"why", "查某路径或主题下的决策、规则、记忆", cmdWhy, false},
		{"note", "记一条候选记忆", cmdNote, false},
		{"check", "按 target 验证对象、规则与语义变化", cmdCheck, false},
		{"verify", "跑一次验证器，把结果记成证据", cmdVerify, false},
		{"promote", "规则对照验证：candidate → active", cmdPromote, false},
		{"retire", "撤回一条规则，保留原因与历史", cmdRetire, false},
		{"archive", "归档一条记忆，保留原因与历史", cmdArchive, false},
		{"task", "任务接续摘要：set / show / clear", cmdTask, false},
		{"template", "模板来源：status / update", cmdTemplate, false},
		{"brief", "输出任务相关的上下文包", cmdBrief, false},
		{"review", "进化报告：到期、候选、工作流建议", cmdReview, false},
		{"completion", "装 shell 补全（不带参数会认一下当前 shell）", cmdCompletion, false},
		{"update", "把 keel 自己换成 GitHub 上的新版", cmdUpdate, false},
		{"hook", "内部：宿主 hook 事件编解码", cmdHook, true},
		{"__complete", "内部：给补全脚本算候选", cmdComplete, true},
		{"version", "打印版本", cmdVersion, false},
	}
}

const rootUsage = `keel — 给 vibe coding 装一根龙骨

用法：keel [-C <dir>] <命令> [选项]

命令：
%s
通用选项：
  -C <dir>     指定仓库目录（默认当前目录）
  --json       输出机器可读结果
  --quiet      只输出必要内容

退出码：0 通过 / 1 检查失败 / 2 用法错误
`

// Run 执行一次命令，返回进程退出码。
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	e := &env{io: ui.IO{Out: stdout, Err: stderr}, stdin: stdin, dir: "."}

	rest, err := takeGlobalFlags(e, args)
	if err != nil {
		return fail(e, err)
	}
	if len(rest) == 0 {
		e.io.Errf(rootUsage, commandList())
		return ui.ExitUsage
	}
	name := rest[0]
	if name == "-h" || name == "--help" || name == "help" {
		e.io.Printf(rootUsage, commandList())
		return ui.ExitOK
	}
	for _, c := range commands() {
		if c.name != name {
			continue
		}
		if err := c.run(e, rest[1:]); err != nil {
			return fail(e, err)
		}
		return ui.ExitOK
	}
	e.io.Errf("未知命令 %q\n\n", name)
	e.io.Errf(rootUsage, commandList())
	return ui.ExitUsage
}

func commandList() string {
	var b strings.Builder
	for _, c := range commands() {
		if c.hidden {
			continue
		}
		fmt.Fprintf(&b, "  %-10s %s\n", c.name, c.summary)
	}
	return b.String()
}

// takeGlobalFlags 允许 -C 出现在子命令之前。
func takeGlobalFlags(e *env, args []string) ([]string, error) {
	for len(args) > 0 {
		switch {
		case args[0] == "-C" || args[0] == "--chdir":
			if len(args) < 2 {
				return nil, usagef("选项 %s 缺少取值", args[0])
			}
			e.dir = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "-C="):
			e.dir = strings.TrimPrefix(args[0], "-C=")
			args = args[1:]
		default:
			return args, nil
		}
	}
	return args, nil
}

// checkFailed 表示检查未通过，映射到退出码 1（不是错误信息）。
var checkFailed = errors.New("check failed")

func fail(e *env, err error) int {
	var ue *UsageError
	if errors.As(err, &ue) {
		e.io.Errln("用法错误：" + ue.Error())
		return ui.ExitUsage
	}
	if errors.Is(err, checkFailed) {
		return ui.ExitCheckFailed
	}
	e.io.Errln("keel: " + err.Error())
	return ui.ExitCheckFailed
}

// newFlagSet 建一个不自动打印用法、不自动退出的 FlagSet。
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// commonFlags 注册每个命令都接受的通用选项。
func commonFlags(fs *flag.FlagSet, e *env) (jsonOut *bool) {
	fs.StringVar(&e.dir, "C", e.dir, "仓库目录")
	fs.BoolVar(&e.quiet, "quiet", e.quiet, "只输出必要内容")
	return fs.Bool("json", false, "输出机器可读结果")
}

func cmdVersion(e *env, args []string) error {
	fs := newFlagSet("version")
	_ = commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("version", rest, 0); err != nil {
		return err
	}
	e.io.Println("keel " + Version)
	return nil
}
