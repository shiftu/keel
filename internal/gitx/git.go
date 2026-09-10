// Package gitx 是 keel 用到的 git 能力：仓库定位、结构化 diff、
// trailer 解析、index 快照与 hook 安装。全部通过 git 命令行，不解析 .git 内部格式。
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoGit 表示当前目录不在 git 仓库里。
var ErrNoGit = errors.New("不在 git 仓库里")

// Repo 是一个 git 工作树。
type Repo struct {
	// Root 是工作树根目录。
	Root string
	// GitDir 是该工作树的 .git 目录（worktree 时是 .git/worktrees/<name>）。
	GitDir string
	// CommonDir 是主仓库的 .git 目录，hooks 默认装在这里。
	CommonDir string
}

// Open 定位包含 dir 的 git 工作树。
func Open(dir string) (*Repo, error) {
	root, err := runIn(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNoGit
	}
	r := &Repo{Root: strings.TrimSpace(root)}
	if g, err := runIn(dir, "rev-parse", "--absolute-git-dir"); err == nil {
		r.GitDir = strings.TrimSpace(g)
	}
	if c, err := runIn(dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		r.CommonDir = strings.TrimSpace(c)
	}
	if r.CommonDir == "" {
		r.CommonDir = r.GitDir
	}
	return r, nil
}

// IsWorktree 报告这是不是链接工作树（不是主仓库）。
func (r *Repo) IsWorktree() bool {
	return r.GitDir != "" && r.CommonDir != "" && r.GitDir != r.CommonDir
}

func (r *Repo) git(args ...string) (string, error) { return runIn(r.Root, args...) }

func runIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

// HasHead 报告仓库是否已有提交。首次提交时 HEAD 还不存在。
func (r *Repo) HasHead() bool {
	_, err := r.git("rev-parse", "--verify", "HEAD")
	return err == nil
}

// StagedFiles 返回暂存区相对 HEAD 变化的文件（新仓库则是全部暂存文件）。
func (r *Repo) StagedFiles() ([]string, error) {
	args := []string{"diff", "--cached", "--name-only", "-z"}
	if !r.HasHead() {
		args = []string{"diff", "--cached", "--name-only", "-z", "--no-renames",
			"4b825dc642cb6eb9a060e54bf8d69288fbee4904"} // 空树
	}
	out, err := r.git(args...)
	if err != nil {
		return nil, err
	}
	return splitZ(out), nil
}

// WorktreeFiles 返回工作树相对 HEAD 的变化，含未跟踪文件。
func (r *Repo) WorktreeFiles() ([]string, error) {
	out, err := r.git("status", "--porcelain=1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, rec := range strings.Split(out, "\x00") {
		if len(rec) < 4 {
			continue
		}
		files = append(files, rec[3:])
	}
	sort.Strings(files)
	return files, nil
}

// RangeFiles 返回 base..head 的变化文件。
func (r *Repo) RangeFiles(base, head string) ([]string, error) {
	out, err := r.git("diff", "--name-only", "-z", base+"..."+head)
	if err != nil {
		return nil, err
	}
	return splitZ(out), nil
}

func splitZ(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// FileAt 读取某个 revision 下的文件内容。rev 为 ":" 表示暂存区。
// 文件不存在时返回 nil, nil —— 「这一版没有这个文件」是正常情况。
func (r *Repo) FileAt(rev, path string) ([]byte, error) {
	spec := rev + ":" + path
	if rev == ":" {
		spec = ":" + path
	}
	cmd := exec.Command("git", "show", spec)
	cmd.Dir = r.Root
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := errb.String()
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in") ||
			strings.Contains(msg, "unknown revision") || strings.Contains(msg, "path") {
			return nil, nil
		}
		return nil, fmt.Errorf("git show %s: %s", spec, strings.TrimSpace(msg))
	}
	return out.Bytes(), nil
}

// TopLevelDirs 列出某 revision 下的顶层目录。
func (r *Repo) TopLevelDirs(rev string) (map[string]bool, error) {
	dirs := map[string]bool{}
	var out string
	var err error
	if rev == ":" {
		out, err = r.git("ls-files", "-z")
	} else {
		out, err = r.git("ls-tree", "-r", "--name-only", "-z", rev)
	}
	if err != nil {
		return nil, err
	}
	for _, p := range splitZ(out) {
		if i := strings.IndexByte(p, '/'); i > 0 {
			dirs[p[:i]] = true
		}
	}
	return dirs, nil
}

// SnapshotIndex 把暂存区导出到一个临时目录，用于在「真正要提交的内容」上跑检查。
// 绝不 stash / reset 用户工作树。
func (r *Repo) SnapshotIndex() (dir string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "keel-index")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	prefix := tmp + string(filepath.Separator)
	if _, err := r.git("checkout-index", "-a", "-f", "--prefix="+prefix); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp, cleanup, nil
}

// Trailers 用 git interpret-trailers --parse 解析提交信息，
// 避免把正文里长得像 trailer 的字符串当成引用。
func (r *Repo) Trailers(message string) (map[string][]string, error) {
	cmd := exec.Command("git", "interpret-trailers", "--parse")
	cmd.Dir = r.Root
	cmd.Stdin = strings.NewReader(message)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git interpret-trailers: %s", strings.TrimSpace(errb.String()))
	}
	res := map[string][]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		res[k] = append(res[k], v)
	}
	return res, nil
}

// LogTrailers 返回改动过 path 的提交里出现的 Decision: 引用。
// 用 %(trailers) 让 git 自己解析，避免把正文里的相似字符串当成引用。
func (r *Repo) LogTrailers(path string, limit int) (map[string][]string, error) {
	out, err := r.git("log", fmt.Sprintf("-n%d", limit),
		"--format=%h%x1f%(trailers:key=Decision,valueonly,separator=%x1e)%x1d",
		"--", path)
	if err != nil {
		return nil, err
	}
	res := map[string][]string{}
	for _, rec := range strings.Split(out, "\x1d") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		commit, values, ok := strings.Cut(rec, "\x1f")
		if !ok {
			continue
		}
		for _, v := range strings.Split(values, "\x1e") {
			v = strings.TrimSpace(v)
			if v != "" {
				res[strings.TrimSpace(commit)] = append(res[strings.TrimSpace(commit)], v)
			}
		}
	}
	return res, nil
}
