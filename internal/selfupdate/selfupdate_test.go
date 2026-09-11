package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssetNameMatchesMakefile(t *testing.T) {
	// Makefile 的 TARGETS 决定发布时叫什么名字。这里对不上的表现是 404，
	// 而不是「装错了」—— 所以拿 Makefile 本身钉住。
	b, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	var targets string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "TARGETS") {
			_, targets, _ = strings.Cut(line, ":=")
		}
	}
	if strings.TrimSpace(targets) == "" {
		t.Fatal("Makefile 里没有 TARGETS")
	}
	for _, tgt := range strings.Fields(targets) {
		goos, goarch, _ := strings.Cut(tgt, "/")
		want := "keel_" + goos + "_" + goarch
		if goos == "windows" {
			want += ".exe"
		}
		if got := AssetName(goos, goarch); got != want {
			t.Errorf("AssetName(%q, %q) = %q，Makefile 发出来的是 %q", goos, goarch, got, want)
		}
	}
}

func TestParseSums(t *testing.T) {
	got := ParseSums("abc123  keel_linux_amd64\ndef456 *keel_windows_amd64.exe\n\n坏行\n")
	if got["keel_linux_amd64"] != "abc123" {
		t.Errorf("普通行没解析出来：%v", got)
	}
	// shasum 二进制模式会在文件名前加个 *。
	if got["keel_windows_amd64.exe"] != "def456" {
		t.Errorf("带星号的行没解析出来：%v", got)
	}
	if len(got) != 2 {
		t.Errorf("坏行该被跳过，实际 %v", got)
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b       string
		want       int
		comparable bool
	}{
		{"v0.5.0", "v0.6.0", -1, true},
		{"0.5.0", "v0.5.0", 0, true},
		{"v0.6.0", "v0.5.9", 1, true},
		{"v0.10.0", "v0.9.0", 1, true}, // 不是字符串比较
		{"v1.0.0", "v0.99.99", 1, true},
		// git describe 给自编版本打的是 v0.5.0-3-gabcdef：数字和 v0.5.0 一样但其实更新。
		// 与其猜，不如说清楚比不了。
		{"v0.5.0-3-gabcdef", "v0.5.0", 0, false},
		{"0.1.0-dev", "v0.5.0", 0, false},
		{"v0.5", "v0.5.0", 0, false},
		{"", "v0.5.0", 0, false},
	}
	for _, c := range cases {
		got, ok := Compare(c.a, c.b)
		if ok != c.comparable || (ok && got != c.want) {
			t.Errorf("Compare(%q, %q) = (%d, %v)，想要 (%d, %v)", c.a, c.b, got, ok, c.want, c.comparable)
		}
	}
}

func TestFetchLatestAndTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			w.Write([]byte(`{"tag_name":"v9.9.9","assets":[{"name":"keel_linux_amd64","browser_download_url":"http://x/bin"}]}`))
		case "/repos/o/r/releases/tags/v1.0.0":
			w.Write([]byte(`{"tag_name":"v1.0.0","assets":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	rel, err := Fetch(context.Background(), srv.Client(), srv.URL, "o/r", "")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v9.9.9" || rel.Assets["keel_linux_amd64"] != "http://x/bin" {
		t.Errorf("latest 解析错了：%+v", rel)
	}
	if rel, err := Fetch(context.Background(), srv.Client(), srv.URL, "o/r", "v1.0.0"); err != nil || rel.Tag != "v1.0.0" {
		t.Errorf("按 tag 查失败：%+v %v", rel, err)
	}
	// 说清楚是「没有这个版本」，不是「网络坏了」。
	_, err = Fetch(context.Background(), srv.Client(), srv.URL, "o/r", "v0.0.1")
	if err == nil || !strings.Contains(err.Error(), "v0.0.1") {
		t.Errorf("查不存在的版本该点名它，实际 %v", err)
	}
}

// 匿名调 GitHub API 是 60 次/小时。被限流时看着像网络坏了，得说清楚。
func TestFetchRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := Fetch(context.Background(), srv.Client(), srv.URL, "o/r", "")
	if err == nil || !strings.Contains(err.Error(), "限流") {
		t.Errorf("403 该提到限流，实际 %v", err)
	}
}

func TestDownloadComputesSumInPlace(t *testing.T) {
	body := strings.Repeat("keel", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	var counted int64
	path, sum, err := Download(context.Background(), srv.Client(), srv.URL+"/bin", dir, writerFunc(func(p []byte) (int, error) {
		counted += int64(len(p))
		return len(p), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	// 临时文件必须和目标同目录：跨盘 rename 不是原子的，有些系统上直接失败。
	if filepath.Dir(path) != dir {
		t.Errorf("临时文件落在 %s，该在 %s", filepath.Dir(path), dir)
	}
	h := sha256.Sum256([]byte(body))
	if sum != hex.EncodeToString(h[:]) {
		t.Error("边下边算的校验和不对")
	}
	if counted != int64(len(body)) {
		t.Errorf("进度条只收到 %d 字节，实际下了 %d", counted, len(body))
	}
}

func TestDownloadHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	dir := t.TempDir()
	if _, _, err := Download(context.Background(), srv.Client(), srv.URL+"/bin", dir, nil); err == nil {
		t.Fatal("404 该报错")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("失败时不该留下临时文件：%v", entries)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "keel")
	if err := os.WriteFile(dst, []byte("旧"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "new")
	if err := os.WriteFile(src, []byte("新"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Replace(dst, src); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "新" {
		t.Errorf("没换成新的：%q %v", b, err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(dst)
		if err != nil || fi.Mode()&0o111 == 0 {
			t.Errorf("换上去的文件得能执行：%v %v", fi.Mode(), err)
		}
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
