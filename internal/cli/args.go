package cli

import (
	"flag"
	"fmt"
	"strings"
)

// UsageError 表示用法错误，映射到退出码 2。
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

func usagef(format string, a ...any) error {
	return &UsageError{msg: fmt.Sprintf(format, a...)}
}

// parseArgs 支持选项与位置参数混排（`keel decide "标题" --tag db`），
// 同时严格拒绝未知选项和缺失的选项值——不静默丢掉任何 token。
func parseArgs(fs *flag.FlagSet, args []string) (positional []string, err error) {
	var flagArgs []string
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		name, _, hasValue := strings.Cut(name, "=")
		if name == "" {
			return nil, usagef("无法解析的参数 %q", a)
		}
		f := fs.Lookup(name)
		if f == nil {
			return nil, usagef("未知选项 --%s", name)
		}
		flagArgs = append(flagArgs, a)
		if hasValue || isBoolFlag(f) {
			continue
		}
		if i+1 >= len(args) {
			return nil, usagef("选项 --%s 缺少取值", name)
		}
		i++
		flagArgs = append(flagArgs, args[i])
	}
	if err := fs.Parse(flagArgs); err != nil {
		return nil, usagef("%v", err)
	}
	return positional, nil
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// atMostArgs 限制位置参数个数。多余参数报用法错误，不忽略。
func atMostArgs(cmd string, args []string, n int) error {
	if len(args) > n {
		return usagef("%s 最多接受 %d 个位置参数，收到 %d 个：%s",
			cmd, n, len(args), strings.Join(args, " "))
	}
	return nil
}

// exactlyArgs 要求恰好 n 个位置参数。
func exactlyArgs(cmd string, args []string, n int) error {
	if len(args) != n {
		return usagef("%s 需要 %d 个位置参数，收到 %d 个", cmd, n, len(args))
	}
	return nil
}

// splitList 把 --tag a,b 这样的逗号列表拆开并去空白。
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
