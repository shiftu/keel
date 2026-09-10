package template

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/store"
)

// ConfigPath 是导入面里配置文件的位置（相对 .keel/）。
const ConfigPath = "keel.yaml"

// 可复用的目录。决策、记忆、证据不在其中：它们是源仓库的项目事实，
// 证据的 target.content_digest 指向源仓库的代码，在这边永远对不上。
// intent.md 也不导入——那是本项目自己要回答的问题。
var importDirs = []string{"skills", "rules", "cases"}

// Surface 读出模板仓库 .keel/ 下的可复用文件，键是相对 .keel/ 的斜杠路径。
// 规则在这里就被改写成候选形态：导入面交出去的已经是本地可接受的内容。
func Surface(keelDir, provenance string) (map[string][]byte, error) {
	if st, err := os.Stat(keelDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("模板仓库里没有 %s/ 目录，它看起来不是一个 keel 仓库", store.DirName)
	}
	out := map[string][]byte{}

	if data, err := os.ReadFile(filepath.Join(keelDir, ConfigPath)); err == nil {
		out[ConfigPath] = data
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	for _, dir := range importDirs {
		root := filepath.Join(keelDir, dir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, rerr := filepath.Rel(keelDir, p)
			if rerr != nil {
				return rerr
			}
			key := filepath.ToSlash(rel)
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			if dir == "rules" && strings.HasSuffix(key, ".md") {
				data, rerr = asCandidate(data, provenance)
				if rerr != nil {
					return fmt.Errorf("%s：%w", key, rerr)
				}
			}
			out[key] = data
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// asCandidate 把模板里的规则改写成本地可接受的候选形态。
//
// M3 定了 promote 不能自我批准：规则要过 cases 双向对照验证，且要有一条有效的本地决策做依据。
// 模板导入不能绕开这条，否则「装个模板」就等于「让别人的仓库决定我这边执行什么代码」。
// 所以：
//
//   - status 一律 candidate，模板里写的是什么都一样。
//   - evidence 与 verifier_digest 清空：那些是对着源仓库的代码算的，在这边永远对不上。
//   - from 清空。模板的依据决策不导入（它是源仓库的项目事实），留一个指向本地不存在的
//     决策的 from 只会让 keel check 报 ref_missing——刚导完模板就满仓库红字，
//     连提交都过不去。本地没有依据就是没有依据，promote 会因此拒绝，这才是实情。
//   - from_template 记 <source>@<commit>。模板那条规则原本的 from 没有丢：
//     commit 是钉死的，去源仓库那个版本上一看就有。
func asCandidate(data []byte, provenance string) ([]byte, error) {
	fm, body, err := store.SplitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	var r store.RuleFM
	if err := store.DecodeFrontmatter(fm, &r); err != nil {
		return nil, err
	}
	r.Status = store.RuleCandidate
	r.Evidence = nil
	r.VerifierDigest = ""
	r.From = nil
	r.FromTemplate = &provenance
	return store.EncodeFrontmatter(r, body)
}

// SortedPaths 让所有输出顺序确定。
func SortedPaths(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Provenance 是写进规则 from_template 的出处串。
func Provenance(source, commit string) string { return source + "@" + commit }
