package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesScriptAndRC(t *testing.T) {
	home := t.TempDir()
	res, err := installCompletion("zsh", home, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.ScriptWritten || !res.RCWritten {
		t.Fatalf("第一次装该同时写脚本和 rc：%+v", res)
	}
	got := readFile(t, res.ScriptPath)
	if got != zshScript {
		t.Error("落在磁盘上的不是 zsh 补全脚本")
	}
	rc := readFile(t, filepath.Join(home, ".zshrc"))
	if !strings.Contains(rc, rcBegin) || !strings.Contains(rc, rcEnd) {
		t.Errorf(".zshrc 里没有带标记的块：\n%s", rc)
	}
}

// 重装是常事（升级、换机器），跑第二遍不该多出一份。
func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := installCompletion("bash", home, ""); err != nil {
		t.Fatal(err)
	}
	res, err := installCompletion("bash", home, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ScriptWritten || res.RCWritten {
		t.Errorf("内容没变就不该动文件：%+v", res)
	}
	rc := readFile(t, filepath.Join(home, ".bashrc"))
	if n := strings.Count(rc, rcBegin); n != 1 {
		t.Errorf(".bashrc 里有 %d 个标记块，该只有 1 个", n)
	}
}

// 标记块是整段替换，用户写在外面的东西一个字都不能动。
func TestInstallKeepsUserRCContent(t *testing.T) {
	home := t.TempDir()
	rcPath := filepath.Join(home, ".bashrc")
	head := "export PATH=/my/bin:$PATH\n"
	tail := "\nalias ll='ls -l'\n"
	mustWrite(t, rcPath, head+rcBegin+"\n旧的内容\n"+rcEnd+tail)

	if _, err := installCompletion("bash", home, ""); err != nil {
		t.Fatal(err)
	}
	rc := readFile(t, rcPath)
	if !strings.HasPrefix(rc, head) || !strings.HasSuffix(rc, tail) {
		t.Errorf("标记块外的内容被动了：\n%s", rc)
	}
	if strings.Contains(rc, "旧的内容") {
		t.Error("标记块里的旧内容该被整段换掉")
	}
}

// 用户手改坏了（只剩开头标记）时不去猜块从哪结束，追加一份新的，别把他的配置切掉。
func TestInstallToleratesBrokenMarker(t *testing.T) {
	home := t.TempDir()
	rcPath := filepath.Join(home, ".bashrc")
	mustWrite(t, rcPath, rcBegin+"\n半截\nexport KEEP=1\n")
	if _, err := installCompletion("bash", home, ""); err != nil {
		t.Fatal(err)
	}
	rc := readFile(t, rcPath)
	if !strings.Contains(rc, "export KEEP=1") {
		t.Errorf("用户的配置被吃掉了：\n%s", rc)
	}
	if !strings.Contains(rc, rcEnd) {
		t.Errorf("新块没写进去：\n%s", rc)
	}
}

// fish 自己会发现 completions 目录，不该去动它的配置。
func TestInstallFishTouchesNoRC(t *testing.T) {
	home := t.TempDir()
	res, err := installCompletion("fish", home, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RCPath != "" || res.RCWritten {
		t.Errorf("fish 不该改配置：%+v", res)
	}
	if _, err := os.Stat(res.ScriptPath); err != nil {
		t.Errorf("fish 脚本没写到位：%v", err)
	}
}

func TestInstallUnknownShell(t *testing.T) {
	if _, err := installCompletion("nushell", t.TempDir(), ""); err == nil {
		t.Error("不认识的 shell 该报错")
	}
}

// PowerShell 的 $PROFILE 只能问它本人要；问不出来就没有安装位置可言。
func TestPlanPowerShellNeedsProfile(t *testing.T) {
	if _, ok := planInstall("powershell", t.TempDir(), ""); ok {
		t.Error("没有 $PROFILE 路径时不该给出安装计划")
	}
	home := t.TempDir()
	profile := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	plan, ok := planInstall("powershell", home, profile)
	if !ok || filepath.Dir(plan.ScriptPath) != filepath.Dir(profile) {
		t.Errorf("脚本该落在 $PROFILE 旁边：%+v", plan)
	}
}

func TestDetectShell(t *testing.T) {
	for in, want := range map[string]string{
		"/bin/zsh":            "zsh",
		"/usr/local/bin/fish": "fish",
		"/bin/bash":           "bash",
		"/usr/bin/pwsh":       "powershell",
		"/bin/tcsh":           "",
		"":                    "",
	} {
		t.Setenv("SHELL", in)
		if got := detectShell(); got != want {
			t.Errorf("SHELL=%q 认成 %q，该是 %q", in, got, want)
		}
	}
}

// 四家脚本都得把正在敲的词走 --cur=：空串当普通参数传会被 shell 整个吞掉。
func TestShellScriptsPassCurrentWordSafely(t *testing.T) {
	for _, sh := range shells {
		script, ok := shellScript(sh)
		if !ok {
			t.Fatalf("%s 没有脚本", sh)
		}
		if !strings.Contains(script, "--cur=") {
			t.Errorf("%s 的脚本没用 --cur=", sh)
		}
		if !strings.Contains(script, "keel __complete") && !strings.Contains(script, "(keel __complete") {
			t.Errorf("%s 的脚本没调 keel __complete", sh)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
