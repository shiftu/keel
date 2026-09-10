package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ObjectDigest 是「这条结论本身」的摘要，用来回答：证据写下之后，结论有没有被改过。
//
// 它**不包含** verify 自己会改的字段（status、evidence、verified_at），
// 否则验证动作会立刻让刚写下的证据失效，成了永远对不上的死循环。
func ObjectDigest(o Object) string {
	h := sha256.New()
	switch v := o.(type) {
	case *Memory:
		fmt.Fprintf(h, "kind\x00%s\n", v.Kind)
		fmt.Fprintf(h, "summary\x00%s\n", v.Summary)
		writeList(h, "tags", v.Tags)
		writeList(h, "scope", v.Scope)
		writeMap(h, "conditions", v.Conditions)
	case *Decision:
		fmt.Fprintf(h, "title\x00%s\n", v.Title)
		writeList(h, "tags", v.Tags)
		writeList(h, "scope", v.Scope)
	default:
		fmt.Fprintf(h, "title\x00%s\n", o.ObjectTitle())
	}
	fmt.Fprintf(h, "body\x00%s\n", strings.TrimSpace(bodyOf(o)))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func bodyOf(o Object) string {
	type bodied interface{ Body() string }
	if b, ok := o.(bodied); ok {
		return b.Body()
	}
	return ""
}

func writeList(h io.Writer, name string, xs []string) {
	sorted := append([]string{}, xs...)
	sort.Strings(sorted)
	fmt.Fprintf(h, "%s\x00%s\n", name, strings.Join(sorted, "\x01"))
}

func writeMap(h io.Writer, name string, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(h, "%s\x00", name)
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s\x01", k, m[k])
	}
	fmt.Fprint(h, "\n")
}

// ScopeDigest 是「被验证的代码」的摘要：按 scope 收集文件，路径排序后逐个摘要。
// 返回的 inputs 是纳入摘要的仓库相对路径，供证据里的 dirty_inputs 使用。
//
// scope 为空时返回空摘要与空列表：没有声明范围就没什么可比对的，
// 调用方据此知道这条证据只能证明「命令跑通了」，不能证明代码没变。
func ScopeDigest(root string, scope []string) (digest string, inputs []string, err error) {
	if len(scope) == 0 {
		return "", nil, nil
	}
	files, err := filesUnderScope(root, scope)
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	for _, rel := range files {
		data, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if rerr != nil {
			return "", nil, rerr
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s\x00%s\n", rel, hex.EncodeToString(sum[:]))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), files, nil
}

// filesUnderScope 返回匹配任一 glob 的普通文件，仓库相对路径、已排序。
func filesUnderScope(root string, scope []string) ([]string, error) {
	var out []string
	err := walkRepo(root, func(rel string) {
		if _, ok := MatchAny(scope, rel); ok {
			out = append(out, rel)
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// walkRepo 遍历仓库里的普通文件，回调收到仓库相对路径（/ 分隔）。
func walkRepo(root string, visit func(rel string)) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir(d.Name(), rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			visit(rel)
		}
		return nil
	})
}

// Digester 为多组 scope 一次性算内容摘要：只遍历一遍仓库，
// 且只对至少命中一组 scope 的文件做摘要。
//
// check 和 brief 都要对若干条记忆做同样的比对，各自走一遍文件树太贵，
// 尤其 brief 挂在 SessionStart 上。
type Digester struct {
	root   string
	scopes [][]string
	files  map[string]string
	err    error
	walked bool
}

// NewDigester 收下全部要比对的 scope。遍历推迟到第一次 Digest。
func NewDigester(root string, scopes [][]string) *Digester {
	return &Digester{root: root, scopes: scopes}
}

// Digest 返回该 scope 的内容摘要；scope 为空返回空串。
func (d *Digester) Digest(scope []string) (string, error) {
	if len(scope) == 0 {
		return "", nil
	}
	d.walk()
	if d.err != nil {
		return "", d.err
	}
	rels := make([]string, 0, len(d.files))
	for rel := range d.files {
		if _, ok := MatchAny(scope, rel); ok {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		fmt.Fprintf(h, "%s\x00%s\n", rel, d.files[rel])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func (d *Digester) walk() {
	if d.walked {
		return
	}
	d.walked = true
	d.files = map[string]string{}
	d.err = walkRepo(d.root, func(rel string) {
		hit := false
		for _, sc := range d.scopes {
			if _, ok := MatchAny(sc, rel); ok {
				hit = true
				break
			}
		}
		if !hit {
			return
		}
		data, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(rel)))
		if err != nil {
			return
		}
		sum := sha256.Sum256(data)
		d.files[rel] = hex.EncodeToString(sum[:])
	})
}

// skipDir 挡掉不该进摘要的目录：版本库内部、依赖目录，以及 .keel 里
// 会被验证动作自己改动的两块（证据与缓存）。
func skipDir(name, rel string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	}
	switch rel {
	case DirName + "/evidence", DirName + "/cache":
		return true
	}
	return false
}

// RuleDefinitionDigest 覆盖 check.argv、对照用例集合、正文，
// 以及 argv[0] 指向的仓库内脚本的内容。
//
// 最后一项不能少：改检查脚本和改 argv 是同一件事——现在生效的这条规则
// 和当初通过对照验证的那条不再是同一个东西。
func RuleDefinitionDigest(root string, r *Rule) string {
	h := sha256.New()
	if r.Check != nil {
		fmt.Fprintf(h, "argv\x00%s\n", strings.Join(r.Check.Argv, "\x01"))
		if script, ok := RepoScript(root, r.Check.Argv); ok {
			if data, err := os.ReadFile(script); err == nil {
				sum := sha256.Sum256(data)
				fmt.Fprintf(h, "script\x00%s\n", hex.EncodeToString(sum[:]))
			}
		}
	}
	for _, c := range r.Cases {
		fmt.Fprintf(h, "case\x00%s=%s\n", c.Dir, c.Expect)
	}
	fmt.Fprintf(h, "body\x00%s\n", strings.TrimSpace(r.Body()))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// RepoScript 把 argv[0] 解析成仓库内的脚本绝对路径。
// argv[0] 是绝对路径、或不含 / 的命令名（走 PATH）时返回 false。
func RepoScript(root string, argv []string) (string, bool) {
	if len(argv) == 0 {
		return "", false
	}
	a0 := argv[0]
	if filepath.IsAbs(a0) || !strings.Contains(a0, "/") {
		return "", false
	}
	p := filepath.Join(root, filepath.FromSlash(a0))
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return "", false
	}
	return p, true
}
