//go:build windows

package check

import "os/exec"

// Windows 没有 POSIX 的进程组信号语义，回收整棵进程树要靠 job object。
// 这里保留 CommandContext 的默认行为：超时只杀掉规则进程本身。
// 超时判定和退出码约定不受影响，差别只在规则自己拉起的后台子进程可能残留。
func setProcessGroup(cmd *exec.Cmd) {}
