package cli

import (
	"os"
	"strings"

	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/gitx"
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
	// --quiet 是给 git hook 用的：通过时一个字都不输出，失败时照常说清楚。
	if e.quiet && res.Status == check.StatusPass {
		return
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
	if !e.quiet {
		for _, n := range res.Notes {
			e.io.Printf("· %s\n", n)
		}
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

// runTargetChecks 按 target 决定在哪一份快照上验证。
//
// worktree / index / commit-msg / range 看的是不同内容，绝不互相替代：
// 工作树里改好但没暂存，不能让 index 通过。
func runTargetChecks(e *env, st *store.Store, cfg store.Config, set *store.Set,
	res *check.Result, t check.Target, commitMsgFile, base, head string) error {
	if t == check.TargetWorktree {
		return nil // 只读诊断：对象检查与规则已经跑过
	}

	repo, err := gitx.Open(st.Root)
	if err != nil {
		res.AddError(check.Finding{
			Code:    check.CodeIndexSnapshotFailed,
			Message: "提交验证需要 git 仓库：" + err.Error(),
		})
		return nil
	}

	switch t {
	case check.TargetIndex, check.TargetCommitMsg:
		cov := check.Coverage{}
		if t == check.TargetCommitMsg {
			msg, err := os.ReadFile(commitMsgFile)
			if err != nil {
				return usagef("读取 --commit-msg 文件失败：%v", err)
			}
			cov.Trailers = check.TrailerDecisions(repo, set, string(msg), res)
		}
		return checkIndex(e, st, cfg, res, repo, cov)

	case check.TargetRange:
		files, err := repo.RangeFiles(base, head)
		if err != nil {
			res.AddError(check.Finding{Code: check.CodeIndexSnapshotFailed, Message: err.Error()})
			return nil
		}
		cs := check.ChangeSet{Repo: repo, Files: files, BeforeRev: base, AfterRev: head}
		check.Signals(st, cfg, set, res, cs, coverageFromFiles(st, set, files, nil))
		return nil
	}
	return nil
}

// checkIndex 在暂存区快照上验证。绝不 stash / reset 用户工作树。
func checkIndex(e *env, st *store.Store, cfg store.Config, res *check.Result,
	repo *gitx.Repo, cov check.Coverage) error {

	dir, cleanup, err := repo.SnapshotIndex()
	if err != nil {
		// 拿不到可靠的暂存快照就明说，不拿工作树结果冒充。
		res.AddError(check.Finding{
			Code:    check.CodeIndexSnapshotFailed,
			Message: "无法导出暂存区快照：" + err.Error(),
			Fix:     "先解决 git 报的问题；keel 不会用工作树结果代替 index 验证",
		})
		return nil
	}
	defer cleanup()

	files, err := repo.StagedFiles()
	if err != nil {
		res.AddError(check.Finding{Code: check.CodeIndexSnapshotFailed, Message: err.Error()})
		return nil
	}
	if cov.ChangedDecisionPaths == nil {
		cov = coverageFromFiles(st, nil, files, cov.Trailers)
	}

	// 对象与规则都用同一份快照，避免工作树里的修复替暂存的坏代码背书。
	snap := &store.Store{Root: dir}
	if _, err := os.Stat(snap.KeelDir()); err == nil {
		snapSet, err := snap.Load()
		if err != nil {
			return err
		}
		snapCfg := cfg
		if c, err := snap.LoadConfig(); err == nil {
			snapCfg = c
		}
		res.Notes = append(res.Notes, "对象与规则在暂存区快照上验证")
		check.Objects(snap, snapCfg, snapSet, res)
		check.Rules(snap, snapSet, res)
		cov.ChangedDecisionPaths = changedDecisionPaths(files)
		check.Signals(st, snapCfg, snapSet, res, check.ChangeSet{
			Repo: repo, Files: files, BeforeRev: headRev(repo), AfterRev: ":",
		}, cov)
		return nil
	}

	set, err := st.Load()
	if err != nil {
		return err
	}
	check.Signals(st, cfg, set, res, check.ChangeSet{
		Repo: repo, Files: files, BeforeRev: headRev(repo), AfterRev: ":",
	}, cov)
	return nil
}

func headRev(repo *gitx.Repo) string {
	if repo.HasHead() {
		return "HEAD"
	}
	return ""
}

func coverageFromFiles(st *store.Store, set *store.Set, files []string, trailers []store.ID) check.Coverage {
	return check.Coverage{Trailers: trailers, ChangedDecisionPaths: changedDecisionPaths(files)}
}

// changedDecisionPaths 找出本次变化里新增或修改的决策文件。
func changedDecisionPaths(files []string) map[string]bool {
	out := map[string]bool{}
	prefix := store.DirName + "/decisions/"
	for _, f := range files {
		if strings.HasPrefix(f, prefix) && strings.HasSuffix(f, ".md") {
			out[f] = true
		}
	}
	return out
}
