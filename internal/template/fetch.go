package template

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DefaultRef 是没写 @<ref> 时取的东西：远端默认分支。
const DefaultRef = "HEAD"

// ParseSource 拆 "<url>[@<ref>]"。URL 里的 scheme 分隔符不算 ref 分隔符，
// 所以只认最后一个 @，且它必须在最后一个 / 之后——
// git@host:org/repo.git 这种写法里的 @ 不能被当成 ref。
func ParseSource(spec string) (source, ref string, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", fmt.Errorf("模板来源不能为空")
	}
	at := strings.LastIndex(spec, "@")
	slash := strings.LastIndex(spec, "/")
	if at > 0 && at > slash {
		source, ref = spec[:at], spec[at+1:]
		if ref == "" {
			return "", "", fmt.Errorf("模板来源 %q 的 @ 后面缺少 ref", spec)
		}
		return source, ref, nil
	}
	return spec, DefaultRef, nil
}

// Fetch 把 source 的 ref 取到一个临时目录，返回目录、解析出的 commit 和清理函数。
//
// 用 init+fetch 而不是 clone --branch：clone 的 --branch 不接受裸 commit sha，
// 而「钉住一个版本」正是这套东西的全部意义。
func Fetch(source, ref string) (dir, commit string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "keel-template-")
	if err != nil {
		return "", "", nil, err
	}
	cleanup = func() { os.RemoveAll(dir) }
	fail := func(e error) (string, string, func(), error) {
		cleanup()
		return "", "", nil, e
	}

	for _, args := range [][]string{
		{"init", "--quiet"},
		{"fetch", "--quiet", "--depth", "1", source, ref},
		{"checkout", "--quiet", "FETCH_HEAD"},
	} {
		if out, e := run(dir, args...); e != nil {
			return fail(fmt.Errorf("取模板 %s@%s 失败（git %s）：%s",
				source, ref, strings.Join(args, " "), firstLine(out, e)))
		}
	}
	out, e := run(dir, "rev-parse", "HEAD")
	if e != nil {
		return fail(fmt.Errorf("取模板 commit 失败：%s", firstLine(out, e)))
	}
	return dir, strings.TrimSpace(out), cleanup, nil
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// 模板取用是显式联网操作，但不该让宿主的 hook 或凭据助手弹交互。
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func firstLine(out string, err error) string {
	for _, l := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			return s
		}
	}
	return err.Error()
}
