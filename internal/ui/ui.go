// Package ui 负责面向人的中文输出与机器输出的分流。
// stdout 只放结果，诊断一律走 stderr。
package ui

import (
	"fmt"
	"io"
)

// 业务 CLI 退出码。宿主 hook 协议不复用这套码，见 internal/cli/hook.go。
const (
	// ExitOK 检查通过 / 命令成功。
	ExitOK = 0
	// ExitCheckFailed 检查失败或内核出错。
	ExitCheckFailed = 1
	// ExitUsage 用法错误：未知选项、参数个数不对、缺少必填项。
	ExitUsage = 2
)

// IO 把标准流显式传进来，方便测试。
type IO struct {
	Out io.Writer
	Err io.Writer
}

func (io IO) Printf(format string, a ...any) {
	fmt.Fprintf(io.Out, format, a...)
}

func (io IO) Println(a ...any) {
	fmt.Fprintln(io.Out, a...)
}

func (io IO) Errf(format string, a ...any) {
	fmt.Fprintf(io.Err, format, a...)
}

func (io IO) Errln(a ...any) {
	fmt.Fprintln(io.Err, a...)
}
