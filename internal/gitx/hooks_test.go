package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newRepo 建一个隔离的临时 git 仓库。
func newRepo(t *testing.T) *Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("没有 git")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "keel@test"},
		{"config", "user.name", "keel test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "none"),
			"GIT_CONFIG_SYSTEM="+filepath.Join(dir, "none"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// runHook 用一个可控的 PATH 执行 hook 脚本，返回退出码。
func runHook(t *testing.T, script string, pathDir string, args ...string) int {
	t.Helper()
	cmd := exec.Command("/bin/sh", append([]string{script}, args...)...)
	cmd.Dir = filepath.Dir(script)
	path := "/usr/bin:/bin"
	if pathDir != "" {
		path = pathDir + ":" + path
	}
	cmd.Env = []string{"PATH=" + path}
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return ee.ExitCode()
	}
	t.Fatalf("执行 %s: %v", script, err)
	return -1
}

func asExitError(err error, out **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*out = ee
		return true
	}
	return false
}

// fakeKeel 造一个假的 keel，按给定退出码返回。
func fakeKeel(t *testing.T, code string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "keel")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit "+code+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestHookPropagatesFailure 锁定 docs/design/fixtures/git-hook-exit 的结论：
// keel 装好但检查失败时，hook 必须返回非零。旧写法的 `|| true` 会把它吞成 0。
func TestHookPropagatesFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook")
	}
	repo := newRepo(t)
	if _, err := repo.InstallHooks(false); err != nil {
		t.Fatal(err)
	}
	dir, err := repo.HooksDir()
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []HookName{PreCommit, CommitMsg} {
		path := filepath.Join(dir, string(h))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s 没装上: %v", h, err)
		}
		script := string(data)
		if strings.Contains(script, "|| true") {
			t.Errorf("%s 里还有 || true，会吞掉检查失败：\n%s", h, script)
		}
		if !strings.Contains(script, "exit $?") {
			t.Errorf("%s 没有传播退出码：\n%s", h, script)
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode()&0o111 == 0 {
			t.Errorf("%s 没有执行位", h)
		}

		msg := filepath.Join(t.TempDir(), "MSG")
		_ = os.WriteFile(msg, []byte("x\n"), 0o644)

		if got := runHook(t, path, fakeKeel(t, "1"), msg); got == 0 {
			t.Errorf("%s：keel 检查失败时 hook 却返回 0", h)
		}
		if got := runHook(t, path, fakeKeel(t, "0"), msg); got != 0 {
			t.Errorf("%s：keel 通过时 hook 返回 %d", h, got)
		}
		// 没装 keel 就放行：本地龙骨不是每个 clone 的门卫，合并门禁走 CI。
		if got := runHook(t, path, "", msg); got != 0 {
			t.Errorf("%s：没装 keel 时 hook 返回 %d，应放行", h, got)
		}
	}
}

// TestForeignHookIsNotClobbered 保证别人的 hook 不会被无条件追加或覆盖。
func TestForeignHookIsNotClobbered(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook")
	}
	repo := newRepo(t)
	dir, err := repo.HooksDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, string(PreCommit))
	// 提前 exit 的脚本：无条件在末尾追加会永远跑不到 keel 那几行。
	body := "#!/bin/sh\nif [ -n \"$SKIP\" ]; then exit 0; fi\nexit 3\n"
	if err := os.WriteFile(foreign, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.InstallHooks(false); err == nil {
		t.Fatal("已有自定义 hook 时应报出集成缺口，而不是静默跳过或覆盖")
	}
	got, err := os.ReadFile(foreign)
	if err != nil || string(got) != body {
		t.Fatal("原有 hook 被改动了")
	}

	// --adopt-hooks：串成明确的调用链，原脚本先跑且失败照样失败。
	if _, err := repo.InstallHooks(true); err != nil {
		t.Fatalf("adopt 失败: %v", err)
	}
	local := foreign + ".keel-local"
	if _, err := os.Stat(local); err != nil {
		t.Fatal("原脚本没有被保留成 .keel-local")
	}
	if code := runHook(t, foreign, fakeKeel(t, "0")); code != 3 {
		t.Errorf("原 hook 失败时应返回 3，得到 %d", code)
	}
}

// TestNonShellHookIsReported 非 shell hook 一律不碰，只报集成缺口。
func TestNonShellHookIsReported(t *testing.T) {
	repo := newRepo(t)
	dir, _ := repo.HooksDir()
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, string(CommitMsg))
	if err := os.WriteFile(p, []byte("#!/usr/bin/env python3\nimport sys\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plans, err := repo.PlanHooks()
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range plans {
		if pl.Name != CommitMsg {
			continue
		}
		if pl.Status != HookForeignOther || !pl.Blocked {
			t.Errorf("非 shell hook 的计划 = %+v", pl)
		}
	}
}

// TestHooksPathIsRespected 认 core.hooksPath。
func TestHooksPathIsRespected(t *testing.T) {
	repo := newRepo(t)
	custom := filepath.Join(repo.Root, "myhooks")
	cmd := exec.Command("git", "config", "core.hooksPath", "myhooks")
	cmd.Dir = repo.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	dir, err := repo.HooksDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != custom {
		t.Fatalf("HooksDir = %q，想要 %q", dir, custom)
	}
	if _, err := repo.InstallHooks(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(custom, "pre-commit")); err != nil {
		t.Error("没有装到 core.hooksPath 指定的目录")
	}
}

// TestIndexSnapshotSeesStagedNotWorktree 是 index 与 worktree 分离的底层保证。
func TestIndexSnapshotSeesStagedNotWorktree(t *testing.T) {
	repo := newRepo(t)
	f := filepath.Join(repo.Root, "a.txt")
	if err := os.WriteFile(f, []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "a.txt")
	cmd.Dir = repo.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := os.WriteFile(f, []byte("worktree fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := repo.SnapshotIndex()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "staged\n" {
		t.Errorf("快照拿到的是 %q，应该是暂存内容", got)
	}
}
