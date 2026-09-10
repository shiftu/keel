package check

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// RuleOutcome 是单条规则的执行结果。它不是布尔值：
// 「检查不了」和「检查通过」必须区分开，否则目录不存在会被当成合规。
type RuleOutcome string

const (
	OutcomePass    RuleOutcome = "pass"
	OutcomeFail    RuleOutcome = "fail"
	OutcomeError   RuleOutcome = "error"
	OutcomeTimeout RuleOutcome = "timeout"
	OutcomeSkipped RuleOutcome = "skipped"
)

// DefaultTimeout 是规则未指定时的超时。
const DefaultTimeout = 30 * time.Second

// RunResult 是一次规则执行的完整记录。
type RunResult struct {
	Outcome  RuleOutcome
	ExitCode int
	Output   string
	Err      error
}

// RunRule 在 dir 下执行 argv，按约定翻译退出码：
//
//	0    -> pass
//	1    -> fail（检查跑通了，发现了违规）
//	其他 -> error（工具缺失、路径不存在、用法错误等，不得当成通过）
//
// 这条约定就是 docs/design/fixtures/shell-grep-negation 锁定的契约：
// `! grep` 会把「目录不存在」翻转成成功，所以 keel 不接受 shell 取反，
// 只接受 argv，并且只有退出码恰好为 1 才算 fail。
func RunRule(dir string, argv []string, timeout time.Duration) RunResult {
	if len(argv) == 0 {
		return RunResult{Outcome: OutcomeError, ExitCode: -1, Err: errors.New("check.argv 为空")}
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	// 超时时连规则拉起的子进程一起回收；具体做法按平台分（procgroup_*.go）。
	setProcessGroup(cmd)
	out, err := cmd.CombinedOutput()
	res := RunResult{Output: strings.TrimRight(string(out), "\n")}

	if ctx.Err() == context.DeadlineExceeded {
		res.Outcome = OutcomeTimeout
		res.ExitCode = -1
		res.Err = ctx.Err()
		return res
	}
	if err == nil {
		res.Outcome = OutcomePass
		res.ExitCode = 0
		return res
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		if res.ExitCode == 1 {
			res.Outcome = OutcomeFail
		} else {
			res.Outcome = OutcomeError
			res.Err = err
		}
		return res
	}
	// 启动失败：命令不存在、没有执行位等。
	res.Outcome = OutcomeError
	res.ExitCode = -1
	res.Err = err
	return res
}
