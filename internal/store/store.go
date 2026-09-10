package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// DirName 是仓库内数据目录名。
const DirName = ".keel"

// ErrNotFound 表示从当前目录向上没有找到 .keel/。
var ErrNotFound = errors.New("未找到 .keel/（先运行 keel init）")

// Store 指向一个仓库的 .keel/ 目录。
type Store struct {
	// Root 是包含 .keel/ 的目录绝对路径。
	Root string
}

// Discover 从 dir 向上查找含 .keel/ 的目录。
func Discover(dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		if st, err := os.Stat(filepath.Join(abs, DirName)); err == nil && st.IsDir() {
			return &Store{Root: abs}, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return nil, ErrNotFound
		}
		abs = parent
	}
}

func (s *Store) KeelDir() string       { return filepath.Join(s.Root, DirName) }
func (s *Store) SubDir(k Kind) string  { return filepath.Join(s.KeelDir(), k.Dir()) }
func (s *Store) ConfigPath() string    { return filepath.Join(s.KeelDir(), "keel.yaml") }
func (s *Store) IntentPath() string    { return filepath.Join(s.KeelDir(), "intent.md") }
func (s *Store) GeneratedPath() string { return filepath.Join(s.KeelDir(), "generated.yaml") }
func (s *Store) CacheDir() string      { return filepath.Join(s.KeelDir(), "cache") }
func (s *Store) SkillsDir() string     { return filepath.Join(s.KeelDir(), "skills") }
func (s *Store) KnowledgeDir() string  { return filepath.Join(s.KeelDir(), "knowledge") }

// Rel 把绝对路径转成仓库相对路径（用 / 分隔）。
func (s *Store) Rel(p string) string {
	r, err := filepath.Rel(s.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// LoadError 是单个文件的加载失败，保留路径以便报 finding。
type LoadError struct {
	Path string
	Err  error
}

func (e *LoadError) Error() string { return e.Path + ": " + e.Err.Error() }
func (e *LoadError) Unwrap() error { return e.Err }

// Set 是一次加载得到的全部对象，以及加载过程中的错误。
type Set struct {
	Decisions []*Decision
	Rules     []*Rule
	Memories  []*Memory
	Evidence  []*Evidence
	Errors    []*LoadError
}

// IDs 返回集合中所有对象的 ID，用于短前缀消歧和引用检查。
func (s *Set) IDs() []ID {
	var out []ID
	for _, d := range s.Decisions {
		out = append(out, d.ID)
	}
	for _, r := range s.Rules {
		out = append(out, r.ID)
	}
	for _, m := range s.Memories {
		out = append(out, m.ID)
	}
	for _, e := range s.Evidence {
		out = append(out, e.ID)
	}
	return out
}

// Lookup 按 ID 找对象。
func (s *Set) Lookup(id ID) (Object, bool) {
	switch id.Kind {
	case KindDecision:
		for _, d := range s.Decisions {
			if d.ID == id {
				return d, true
			}
		}
	case KindRule:
		for _, r := range s.Rules {
			if r.ID == id {
				return r, true
			}
		}
	case KindMemory:
		for _, m := range s.Memories {
			if m.ID == id {
				return m, true
			}
		}
	case KindEvidence:
		for _, e := range s.Evidence {
			if e.ID == id {
				return e, true
			}
		}
	}
	return nil, false
}

// DecisionByID 是 Lookup 的类型化版本。
func (s *Set) DecisionByID(id ID) (*Decision, bool) {
	o, ok := s.Lookup(id)
	if !ok {
		return nil, false
	}
	d, ok := o.(*Decision)
	return d, ok
}

// ActiveDecisions 返回有效结论（accepted/proven/revisit）。
func (s *Set) ActiveDecisions() []*Decision {
	var out []*Decision
	for _, d := range s.Decisions {
		if d.Status.IsActive() {
			out = append(out, d)
		}
	}
	return out
}

// Load 读取 .keel/ 下的全部对象。单个文件失败记入 Errors，不中断加载。
func (s *Store) Load() (*Set, error) {
	set := &Set{}
	for _, k := range []Kind{KindDecision, KindRule, KindMemory, KindEvidence} {
		dir := s.SubDir(k)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("读取 %s: %w", s.Rel(dir), err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			full := filepath.Join(dir, name)
			if err := s.loadOne(set, k, full, name); err != nil {
				set.Errors = append(set.Errors, &LoadError{Path: s.Rel(full), Err: err})
			}
		}
	}
	return set, nil
}

func (s *Store) loadOne(set *Set, k Kind, full, name string) error {
	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return err
	}
	rel := s.Rel(full)
	slug := slugFromFilename(name)
	switch k {
	case KindDecision:
		var o Decision
		if err := decodeFrontmatter(fm, &o.DecisionFM); err != nil {
			return err
		}
		o.meta = meta{path: rel, body: body, slug: slug}
		if err := o.validate(); err != nil {
			return err
		}
		set.Decisions = append(set.Decisions, &o)
	case KindRule:
		var o Rule
		if err := decodeFrontmatter(fm, &o.RuleFM); err != nil {
			return err
		}
		o.meta = meta{path: rel, body: body, slug: slug}
		if err := o.validate(); err != nil {
			return err
		}
		set.Rules = append(set.Rules, &o)
	case KindMemory:
		var o Memory
		if err := decodeFrontmatter(fm, &o.MemoryFM); err != nil {
			return err
		}
		o.meta = meta{path: rel, body: body, slug: slug}
		if err := o.validate(); err != nil {
			return err
		}
		set.Memories = append(set.Memories, &o)
	case KindEvidence:
		var o Evidence
		if err := decodeFrontmatter(fm, &o.EvidenceFM); err != nil {
			return err
		}
		o.meta = meta{path: rel, body: body, slug: slug}
		if err := o.validate(); err != nil {
			return err
		}
		set.Evidence = append(set.Evidence, &o)
	}
	return nil
}

// slugFromFilename 去掉 ID 前缀和 .md 后缀。
func slugFromFilename(name string) string {
	base := strings.TrimSuffix(name, ".md")
	if _, rest, ok := strings.Cut(base, "-"); ok {
		// base 形如 D-<uuid>-<slug>；uuid 本身含 4 个连字符。
		parts := strings.SplitN(rest, "-", 6)
		if len(parts) == 6 {
			return parts[5]
		}
	}
	return ""
}

// FileName 返回对象的文件名：<ID>-<slug>.md（slug 为空时省略）。
func FileName(id ID, slug string) string {
	if slug == "" {
		return id.String() + ".md"
	}
	return id.String() + "-" + slug + ".md"
}

// Slugify 把标题转成文件名可用的 slug：小写、非字母数字换 -、截到 40 字符。
// 中文等非 ASCII 字符原样保留。
func Slugify(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		// unicode.IsLetter 认汉字，同时把中英文标点挡在文件名之外。
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	runes := []rune(s)
	if len(runes) > 40 {
		s = strings.Trim(string(runes[:40]), "-")
	}
	return s
}

// MemoryByID 是 Lookup 的类型化版本。
func (s *Set) MemoryByID(id ID) (*Memory, bool) {
	o, ok := s.Lookup(id)
	if !ok {
		return nil, false
	}
	m, ok := o.(*Memory)
	return m, ok
}

// EvidenceFor 返回指向该 subject 的全部证据，按 ID 排序。
func (s *Set) EvidenceFor(subject ID) []*Evidence {
	var out []*Evidence
	for _, e := range s.Evidence {
		if e.Subject == subject {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}

// SupportingEvidences 找出「确实支持这条结论现在这个样子」的全部证据：
// 结果是 pass，且 subject_digest 等于对象当前内容摘要。按观测时间从新到旧。
//
// 改了结论正文就会让旧证据对不上——这是有意的：结论变了就要重新验证。
// 返回全部而不是一条，是因为同一条结论可以被验证多次，
// 判断「验证有没有过期」必须看最新那次，不能撞上排在前面的老证据。
func (s *Set) SupportingEvidences(o Object) []*Evidence {
	want := ObjectDigest(o)
	var out []*Evidence
	for _, e := range s.EvidenceFor(o.ObjectID()) {
		if e.Result == "pass" && e.SubjectDigest == want {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ObservedAt != out[j].ObservedAt {
			return out[i].ObservedAt > out[j].ObservedAt
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}

// SupportingEvidence 返回最新的一条支持证据。
func (s *Set) SupportingEvidence(o Object) (*Evidence, bool) {
	all := s.SupportingEvidences(o)
	if len(all) == 0 {
		return nil, false
	}
	return all[0], true
}
