package cli

import (
	"fmt"
	"strings"

	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/store"
)

func cmdCheck(e *env, args []string) error {
	fs := newFlagSet("check")
	jsonOut := commonFlags(fs, e)
	target := fs.String("target", "", "worktree | index | commit-msg | range")
	commitMsg := fs.String("commit-msg", "", "提交信息文件（--target commit-msg 时必填）")
	base := fs.String("base", "", "--target range 的起点")
	head := fs.String("head", "", "--target range 的终点")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("check", rest, 0); err != nil {
		return err
	}

	t, err := resolveTarget(*target, *commitMsg, *base, *head)
	if err != nil {
		return err
	}

	st, err := e.discover()
	if err != nil {
		return err
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	set, err := st.Load()
	if err != nil {
		return err
	}

	res := check.NewResult(t)
	check.Objects(st, cfg, set, res)
	if t == check.TargetIndex || t == check.TargetRange || t == check.TargetWorktree {
		check.Rules(st, set, res)
	}
	if err := runTargetChecks(e, st, cfg, set, res, t, *commitMsg, *base, *head); err != nil {
		return err
	}

	return emitResult(e, res, *jsonOut)
}

func resolveTarget(target, commitMsg, base, head string) (check.Target, error) {
	t := check.Target(target)
	if target == "" {
		t = check.TargetWorktree
	}
	if !check.ValidTarget(t) {
		return "", usagef("--target %q 非法（worktree | index | commit-msg | range）", target)
	}
	if t == check.TargetCommitMsg && commitMsg == "" {
		return "", usagef("--target commit-msg 需要 --commit-msg <文件>")
	}
	if t != check.TargetCommitMsg && commitMsg != "" {
		return "", usagef("--commit-msg 只在 --target commit-msg 时有意义")
	}
	if t == check.TargetRange && (base == "" || head == "") {
		return "", usagef("--target range 需要显式的 --base 与 --head")
	}
	if t != check.TargetRange && (base != "" || head != "") {
		return "", usagef("--base/--head 只在 --target range 时有意义")
	}
	return t, nil
}

// emitResult 输出结果并把 fail/error 映射成退出码 1。
func emitResult(e *env, res *check.Result, jsonOut bool) error {
	res.Sort()
	if jsonOut {
		data, err := res.JSON()
		if err != nil {
			return err
		}
		e.io.Println(string(data))
	} else {
		printHuman(e, res)
	}
	if res.Status != check.StatusPass {
		return checkFailed
	}
	return nil
}

func printHuman(e *env, res *check.Result) {
	errs, warns := 0, 0
	for _, f := range res.Findings {
		if f.Severity == check.SeverityError {
			errs++
		} else {
			warns++
		}
	}
	if !e.quiet {
		e.io.Printf("keel check · target=%s\n\n", res.Target)
	}
	for _, f := range res.Findings {
		mark := "✘"
		if f.Severity == check.SeverityWarn {
			mark = "!"
		}
		where := f.Path
		if f.Object != "" {
			where = strings.TrimSpace(f.Object + " " + f.Path)
		}
		e.io.Printf("%s [%s] %s\n", mark, f.Code, where)
		for _, line := range strings.Split(f.Message, "\n") {
			e.io.Printf("    %s\n", line)
		}
		if f.Fix != "" {
			e.io.Printf("    › %s\n", f.Fix)
		}
	}
	for _, n := range res.Notes {
		e.io.Printf("· %s\n", n)
	}
	if e.quiet {
		return
	}
	if len(res.Findings) > 0 {
		e.io.Println("")
	}
	switch res.Status {
	case check.StatusPass:
		if warns > 0 {
			e.io.Printf("通过（%d 条提醒）\n", warns)
		} else {
			e.io.Println("通过")
		}
	case check.StatusFail:
		e.io.Printf("未通过：%d 个错误，%d 条提醒\n", errs, warns)
	default:
		e.io.Printf("检查出错：%d 个错误，%d 条提醒\n", errs, warns)
	}
}

// unimplemented 是 M0 阶段还没有实现的命令。它不是用法错误。
func unimplemented(name, milestone string) error {
	return fmt.Errorf("%s 尚未实现（计划在 %s）；当前可用：decide、note、check、version", name, milestone)
}

func cmdInit(e *env, args []string) error   { return unimplemented("keel init", "M1") }
func cmdSync(e *env, args []string) error   { return unimplemented("keel sync", "M1") }
func cmdWhy(e *env, args []string) error    { return unimplemented("keel why", "M1") }
func cmdBrief(e *env, args []string) error  { return unimplemented("keel brief", "M1") }
func cmdReview(e *env, args []string) error { return unimplemented("keel review", "M3") }
func cmdHook(e *env, args []string) error   { return unimplemented("keel hook", "M1") }

// runTargetChecks 在 M0 只支持 worktree；index/commit-msg/range 属于 M1。
func runTargetChecks(e *env, st *store.Store, cfg store.Config, set *store.Set,
	res *check.Result, t check.Target, commitMsg, base, head string) error {
	if t != check.TargetWorktree {
		return unimplemented("keel check --target "+string(t), "M1")
	}
	return nil
}
