package cli_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/shiftu/keel/internal/cli"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		// keel 按真实退出码退出，供 exec / ! exec 使用。
		"keel": func() int {
			return cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin)
		},
		// keelrc 把退出码打到 stdout 并自身返回 0，
		// 让脚本能精确断言 0/1/2 —— 退出码本身就是契约。
		"keelrc": func() int {
			code := cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin)
			fmt.Printf("\nkeel-exit=%d\n", code)
			return 0
		},
	}))
}

func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
	})
}
