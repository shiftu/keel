//go:build !windows

package check

import (
	"os/exec"
	"syscall"
)

// setProcessGroup 让规则进程独立成一个进程组，超时时连它拉起的子进程一起杀掉。
// 只杀掉直接子进程的话，一个 `sh -c 'sleep 999 &'` 这样的规则会留下孤儿进程。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
