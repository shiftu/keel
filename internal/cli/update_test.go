package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiftu/keel/internal/selfupdate"
)

// 自更新的那几种失败，恰恰是最需要测、又最不能拿真 GitHub 和真二进制去试的：
// 校验和对不上、这个版本没发你的平台、发布里干脆没有校验和文件。
// 共同的要求只有一条：任何一步失败，用户手里那个 keel 都还是完好的。

const oldBinary = "我是旧的 keel"

type fakeRelease struct {
	tag      string
	binary   string // 服务器发的二进制内容
	sums     string // SHA256SUMS 的内容；空 = 按 binary 算
	noBinary bool
	noSums   bool
}

// serve 起一个假 GitHub，返回 updateEnv 和被替换的目标文件路径。
func serve(t *testing.T, r fakeRelease) (updateEnv, string) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "keel")
	if err := os.WriteFile(target, []byte(oldBinary), 0o755); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	asset := assetName("linux", "amd64")
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, r.binary)
	})
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		if r.sums != "" {
			fmt.Fprint(w, r.sums)
			return
		}
		h := sha256.Sum256([]byte(r.binary))
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(h[:]), asset)
	})
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		type a struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		}
		var assets []a
		if !r.noBinary {
			assets = append(assets, a{asset, srv.URL + "/bin"})
		}
		if !r.noSums {
			assets = append(assets, a{"SHA256SUMS", srv.URL + "/sums"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": r.tag, "assets": assets})
	})

	return updateEnv{api: srv.URL, repo: "o/r", target: target, goos: "linux", goarch: "amd64"}, target
}

func runUpdate(t *testing.T, up updateEnv, args ...string) (*capture, error) {
	t.Helper()
	c, io := newCapture()
	return c, update(context.Background(), &env{io: io, dir: "."}, args, up)
}

func TestUpdateReplacesBinary(t *testing.T) {
	up, target := serve(t, fakeRelease{tag: "v9.9.9", binary: "我是新的 keel"})
	c, err := runUpdate(t, up)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "我是新的 keel" {
		t.Errorf("目标文件 = %q，该被换成新版", got)
	}
	if !strings.Contains(c.out.String(), "v9.9.9") {
		t.Errorf("该说清楚换成了哪个版本：%q", c.out.String())
	}
}

// 校验和对不上就是下到了别的东西。宁可不装，也不能把它放到 PATH 上去执行。
func TestUpdateAbortsOnChecksumMismatch(t *testing.T) {
	up, target := serve(t, fakeRelease{
		tag: "v9.9.9", binary: "冒牌货",
		sums: "0000000000000000000000000000000000000000000000000000000000000000  " + assetName("linux", "amd64") + "\n",
	})
	_, err := runUpdate(t, up)
	if err == nil || !strings.Contains(err.Error(), "校验和对不上") {
		t.Fatalf("校验和对不上该报错，实际 %v", err)
	}
	if got := readFile(t, target); got != oldBinary {
		t.Errorf("失败时不该动目标文件，实际 %q", got)
	}
	assertNoLeftovers(t, filepath.Dir(target))
}

// 有二进制没校验和，就没法确认下到的是不是发布的那个。
func TestUpdateRefusesWithoutSums(t *testing.T) {
	up, target := serve(t, fakeRelease{tag: "v9.9.9", binary: "新", noSums: true})
	_, err := runUpdate(t, up)
	if err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("没有校验和文件该拒绝安装，实际 %v", err)
	}
	if got := readFile(t, target); got != oldBinary {
		t.Errorf("不该动目标文件，实际 %q", got)
	}
}

func TestUpdateSumsMissingThisAsset(t *testing.T) {
	up, _ := serve(t, fakeRelease{tag: "v9.9.9", binary: "新", sums: "abc  keel_darwin_arm64\n"})
	_, err := runUpdate(t, up)
	if err == nil || !strings.Contains(err.Error(), "校验不了") {
		t.Fatalf("校验和文件里没有本平台那一行该拒绝，实际 %v", err)
	}
}

func TestUpdateNoAssetForPlatform(t *testing.T) {
	up, target := serve(t, fakeRelease{tag: "v9.9.9", binary: "新", noBinary: true})
	_, err := runUpdate(t, up)
	if err == nil || !strings.Contains(err.Error(), "没发你这个平台") {
		t.Fatalf("这个版本没发本平台该说清楚，实际 %v", err)
	}
	if got := readFile(t, target); got != oldBinary {
		t.Errorf("不该动目标文件，实际 %q", got)
	}
}

// --check 只是问一句，不该下载、更不该替换。
func TestUpdateCheckDoesNotDownload(t *testing.T) {
	up, target := serve(t, fakeRelease{tag: "v9.9.9", binary: "新"})
	c, err := runUpdate(t, up, "--check")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != oldBinary {
		t.Errorf("--check 不该替换文件，实际 %q", got)
	}
	if !strings.Contains(c.out.String(), "v9.9.9") {
		t.Errorf("--check 该报出最新版本：%q", c.out.String())
	}
	assertNoLeftovers(t, filepath.Dir(target))
}

func TestUpdateCheckJSON(t *testing.T) {
	up, _ := serve(t, fakeRelease{tag: "v9.9.9", binary: "新"})
	c, err := runUpdate(t, up, "--check", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got updateJSON
	if err := json.Unmarshal([]byte(c.out.String()), &got); err != nil {
		t.Fatalf("--json 该只吐 JSON：%q（%v）", c.out.String(), err)
	}
	if got.Latest != "v9.9.9" || got.Updated {
		t.Errorf("--check --json = %+v", got)
	}
}

// 已经是最新的就什么都不做——但 dev 构建比不出版本，那是另一回事（见下一个测试）。
func TestUpdateAlreadyLatest(t *testing.T) {
	old := Version
	Version = "v9.9.9"
	defer func() { Version = old }()
	up, target := serve(t, fakeRelease{tag: "v9.9.9", binary: "新"})
	c, err := runUpdate(t, up)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != oldBinary {
		t.Errorf("已经最新就不该重下，实际 %q", got)
	}
	if !strings.Contains(c.out.String(), "已经是最新的") {
		t.Errorf("该说已经最新：%q", c.out.String())
	}
}

// 版本号比不出来时不能默认「已经最新」——那等于把一次明确的更新请求悄悄吃掉。
func TestUpdateIncomparableVersionStillInstalls(t *testing.T) {
	old := Version
	Version = "v0.5.0-3-gabcdef"
	defer func() { Version = old }()
	up, target := serve(t, fakeRelease{tag: "v0.5.0", binary: "新"})
	c, err := runUpdate(t, up)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "新" {
		t.Errorf("比不了就该照装，实际 %q", got)
	}
	if !strings.Contains(c.out.String(), "比不了") {
		t.Errorf("该说清楚为什么比不了：%q", c.out.String())
	}
}

func TestUpdateForceReinstalls(t *testing.T) {
	old := Version
	Version = "v9.9.9"
	defer func() { Version = old }()
	up, target := serve(t, fakeRelease{tag: "v9.9.9", binary: "新"})
	if _, err := runUpdate(t, up, "--force"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "新" {
		t.Errorf("--force 该重装一次，实际 %q", got)
	}
}

func TestUpdateRejectsPositionalArgs(t *testing.T) {
	up, _ := serve(t, fakeRelease{tag: "v9.9.9", binary: "新"})
	_, err := runUpdate(t, up, "v1.2.3")
	var ue *UsageError
	if err == nil || !asUsage(err, &ue) {
		t.Errorf("版本号得走 --version，位置参数该报用法错误，实际 %v", err)
	}
}

// 下载失败、校验失败之后，不该在目标目录留下半截临时文件。
func assertNoLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".keel-") {
			t.Errorf("留下了临时文件 %s", e.Name())
		}
	}
}

// assetName 让测试和实现用同一份平台命名规则。
func assetName(goos, goarch string) string { return selfupdate.AssetName(goos, goarch) }

func asUsage(err error, target **UsageError) bool { return errors.As(err, target) }
