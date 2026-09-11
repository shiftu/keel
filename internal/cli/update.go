package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/shiftu/keel/internal/selfupdate"
	"github.com/shiftu/keel/internal/ui"
)

// updateEnv 是「跟谁要、换掉哪个文件、当自己是什么平台」。
//
// 单拿出来是为了能测：这段代码的失败模式（校验和对不上、这个版本没发你的平台、
// 目录不可写）恰恰是最需要测、又最不能拿真 GitHub 和真二进制去试的那几种。
type updateEnv struct {
	api    string
	repo   string
	target string // 要被换掉的那个文件；空 = 问 selfupdate.Target()
	goos   string
	goarch string
}

func liveUpdateEnv() updateEnv {
	return updateEnv{
		api: selfupdate.DefaultAPI, repo: selfupdate.DefaultRepo,
		goos: runtime.GOOS, goarch: runtime.GOARCH,
	}
}

// updateJSON 是 --json 的输出。给 agent 看的：它得能判断「要不要提醒人升级」。
type updateJSON struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"update_available"`
	Comparab  bool   `json:"comparable"`
	Updated   bool   `json:"updated"`
	Target    string `json:"target,omitempty"`
}

// cmdUpdate 把 keel 自己换成 GitHub 上的新版。
//
// 顺序是刻意的：先问清楚要装哪个版本、需不需要装，再动手下载；下完先对校验和，
// 对上了才碰目标文件。也就是说，任何一步失败，用户手里那个 keel 还是完好的。
func cmdUpdate(e *env, args []string) error {
	return update(context.Background(), e, args, liveUpdateEnv())
}

func update(ctx context.Context, e *env, args []string, up updateEnv) error {
	fs := newFlagSet("update")
	jsonOut := commonFlags(fs, e)
	check := fs.Bool("check", false, "只看有没有新版，不下载")
	force := fs.Bool("force", false, "版本一样也重装")
	want := fs.String("version", "", "指定版本，如 v0.5.0")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("update", rest, 0); err != nil {
		return err
	}

	out := updateJSON{Current: Version}
	c := selfupdate.Client()
	rel, err := selfupdate.Fetch(ctx, c, up.api, up.repo, *want)
	if err != nil {
		return err
	}
	out.Latest = rel.Tag

	// 版本号比不出来（自己编的 dev 构建）时不能默认「已经最新」——
	// 那等于把一次明确的更新请求悄悄吃掉。说清楚比不了，然后照装。
	cmp, comparable := selfupdate.Compare(Version, rel.Tag)
	out.Comparab = comparable
	out.Available = comparable && cmp < 0

	say := func(s string) {
		if !*jsonOut {
			e.io.Println(s)
		}
	}
	say("当前 " + Version + "  ·  " + up.goos + "/" + up.goarch)
	switch {
	case *want != "":
		say("要装 " + rel.Tag)
	case !comparable:
		say("当前版本号比不了（多半是自己编的构建），没法跟 " + rel.Tag + " 比。")
	case cmp >= 0 && !*force:
		say("已经是最新的了（" + rel.Tag + "）。")
		if *jsonOut {
			return writeJSON(e, out)
		}
		return nil
	default:
		say("有新版 " + rel.Tag)
	}
	if *check {
		if out.Available {
			say("装上它：keel update")
		}
		if *jsonOut {
			return writeJSON(e, out)
		}
		return nil
	}
	if comparable && cmp >= 0 && *force {
		say("--force：版本一样也重装一次")
	}

	asset := selfupdate.AssetName(up.goos, up.goarch)
	binURL, ok := rel.Assets[asset]
	if !ok {
		return fmt.Errorf("%s 里没有 %s —— 这个版本没发你这个平台的包", rel.Tag, asset)
	}
	sumsURL, ok := rel.Assets[selfupdate.SumsAsset]
	if !ok {
		// 有二进制没校验和，就没法确认下到的是不是发布的那个。宁可不装。
		return fmt.Errorf("%s 没有 %s，校验不了。不装", rel.Tag, selfupdate.SumsAsset)
	}

	target := up.target
	if target == "" {
		t, err := selfupdate.Target()
		if err != nil {
			return err
		}
		target = t
	}
	out.Target = target
	dir := filepath.Dir(target)
	if err := writable(dir); err != nil {
		return fmt.Errorf("没有写 %s 的权限。换个方式：sudo keel update，或者重跑安装脚本装到 ~/.local/bin", dir)
	}

	sums, err := fetchSums(ctx, c, sumsURL, dir)
	if err != nil {
		return err
	}
	wantSum, ok := sums[asset]
	if !ok {
		return fmt.Errorf("%s 里没有 %s 那一行，校验不了。不装", selfupdate.SumsAsset, asset)
	}

	// 进度条走 stderr：stdout 只放结果，管道那头等的是 --json，不是动画。
	bar := &ui.Progress{
		W: e.io.Err, Live: !*jsonOut && !e.quiet && ui.IsTTY(e.io.Err),
		Label: asset,
		Total: selfupdate.Size(ctx, c, binURL),
	}
	tmp, gotSum, err := selfupdate.Download(ctx, c, binURL, dir, bar)
	if err != nil {
		bar.Abort()
		return err
	}
	bar.Done()

	if gotSum != wantSum {
		_ = os.Remove(tmp)
		return fmt.Errorf("校验和对不上，下到的不是发布的那个文件。已经删掉，没有安装\n  期望 %s…  实际 %s…", wantSum[:16], gotSum[:16])
	}
	if err := selfupdate.Replace(target, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	out.Updated = true
	if *jsonOut {
		return writeJSON(e, out)
	}
	e.io.Println("已更新 " + Version + " → " + rel.Tag + "  ·  " + target)
	if !e.quiet {
		e.io.Println("改动看这里：https://github.com/" + up.repo + "/releases/tag/" + rel.Tag)
	}
	return nil
}

// fetchSums 把校验和文件读进来。它只有几百字节，不值得给进度条。
func fetchSums(ctx context.Context, c *http.Client, url, dir string) (map[string]string, error) {
	path, _, err := selfupdate.Download(ctx, c, url, dir, nil)
	if err != nil {
		return nil, fmt.Errorf("拿不到 %s：%w", selfupdate.SumsAsset, err)
	}
	defer func() { _ = os.Remove(path) }()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return selfupdate.ParseSums(string(b)), nil
}

// writable 试着在目录里建个文件——判断「能不能写」唯一靠得住的办法就是真去写一下。
// 光看权限位会在 root 拥有的目录、只读挂载、Windows ACL 上给出错的答案。
func writable(dir string) error {
	f, err := os.CreateTemp(dir, ".keel-perm-*")
	if err != nil {
		return errors.New("不可写")
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}
