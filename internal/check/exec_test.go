package check

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestRunRuleOutcomes 把 docs/design/fixtures/shell-grep-negation 的结论固定下来：
// 「检查跑不起来」必须是 error，绝不能变成 pass。
func TestRunRuleOutcomes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("夹具用 POSIX sh")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "no-pg.sh")
	body := `#!/bin/sh
if [ ! -d internal/store ]; then
  exit 2
fi
grep -rlnE 'pgx|lib/pq' internal/store >/dev/null 2>&1
case $? in
  0) exit 1 ;;
  1) exit 0 ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	argv := []string{"sh", script}

	// 目录不存在：旧写法 `! grep ...` 会返回成功，这里必须是 error。
	if got := RunRule(dir, argv, 0); got.Outcome != OutcomeError {
		t.Errorf("目录缺失时 Outcome = %s，想要 error（退出码 %d）", got.Outcome, got.ExitCode)
	}

	store := filepath.Join(dir, "internal", "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "db.go"), []byte("package store\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RunRule(dir, argv, 0); got.Outcome != OutcomePass {
		t.Errorf("干净时 Outcome = %s，想要 pass", got.Outcome)
	}

	bad := "package store\n\nimport _ \"github.com/jackc/pgx/v5\"\n"
	if err := os.WriteFile(filepath.Join(store, "bad.go"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RunRule(dir, argv, 0); got.Outcome != OutcomeFail {
		t.Errorf("命中禁用导入时 Outcome = %s，想要 fail", got.Outcome)
	}
}

func TestRunRuleMissingCommand(t *testing.T) {
	got := RunRule(t.TempDir(), []string{"keel-no-such-tool-xyz"}, 0)
	if got.Outcome != OutcomeError {
		t.Errorf("命令不存在时 Outcome = %s，想要 error", got.Outcome)
	}
}

func TestRunRuleTimeoutKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("夹具用 POSIX sh")
	}
	start := time.Now()
	got := RunRule(t.TempDir(), []string{"sh", "-c", "sleep 30"}, 300*time.Millisecond)
	if got.Outcome != OutcomeTimeout {
		t.Errorf("Outcome = %s，想要 timeout", got.Outcome)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("超时后没有及时收敛，用了 %s", elapsed)
	}
}

func TestRunRuleEmptyArgv(t *testing.T) {
	if got := RunRule(t.TempDir(), nil, 0); got.Outcome != OutcomeError {
		t.Errorf("空 argv 应为 error，得到 %s", got.Outcome)
	}
}
