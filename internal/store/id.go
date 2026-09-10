package store

import (
	"crypto/rand"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind 是对象类型前缀。canonical ID 形如 D-<uuid v4>。
type Kind string

const (
	KindDecision Kind = "D"
	KindRule     Kind = "R"
	KindMemory   Kind = "M"
	KindEvidence Kind = "E"
)

// KindOf 把前缀字符转成 Kind，第二个返回值表示是否合法。
func KindOf(s string) (Kind, bool) {
	switch Kind(s) {
	case KindDecision, KindRule, KindMemory, KindEvidence:
		return Kind(s), true
	}
	return "", false
}

// Dir 是该类对象在 .keel/ 下的目录名。
func (k Kind) Dir() string {
	switch k {
	case KindDecision:
		return "decisions"
	case KindRule:
		return "rules"
	case KindMemory:
		return "memory"
	case KindEvidence:
		return "evidence"
	}
	return ""
}

// ID 是 canonical 标识，永不重编号。零值表示未设置。
type ID struct {
	Kind Kind
	// UUID 是小写、带连字符的 UUID v4 文本。
	UUID string
}

func (id ID) IsZero() bool { return id.Kind == "" && id.UUID == "" }

func (id ID) String() string {
	if id.IsZero() {
		return ""
	}
	return string(id.Kind) + "-" + id.UUID
}

// Short 返回人类输入用的短前缀：类型 + UUID 前 8 位十六进制。
// 短前缀只在当前仓库无歧义时可作为输入；trailer 必须写完整 canonical ID。
func (id ID) Short() string {
	if id.IsZero() {
		return ""
	}
	return string(id.Kind) + "-" + id.UUID[:8]
}

// NewID 生成一个新的 canonical ID。
func NewID(k Kind) (ID, error) {
	u, err := newUUIDv4()
	if err != nil {
		return ID{}, err
	}
	return ID{Kind: k, UUID: u}, nil
}

func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("生成 UUID 失败: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	const hexd = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, x := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hexd[x>>4], hexd[x&0x0f])
	}
	return string(out), nil
}

// ParseID 严格解析 canonical ID：<Kind>-<uuid v4 小写>。
func ParseID(s string) (ID, error) {
	k, rest, ok := strings.Cut(s, "-")
	if !ok {
		return ID{}, fmt.Errorf("不是 canonical ID: %q", s)
	}
	kind, ok := KindOf(k)
	if !ok {
		return ID{}, fmt.Errorf("未知对象类型前缀 %q（应为 D/R/M/E）: %q", k, s)
	}
	if err := validUUIDv4(rest); err != nil {
		return ID{}, fmt.Errorf("%q: %w", s, err)
	}
	return ID{Kind: kind, UUID: rest}, nil
}

func validUUIDv4(s string) error {
	if len(s) != 36 {
		return fmt.Errorf("UUID 长度应为 36，实际 %d", len(s))
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return fmt.Errorf("UUID 第 %d 位应为连字符", i+1)
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return fmt.Errorf("UUID 只接受小写十六进制，第 %d 位是 %q", i+1, string(c))
		}
	}
	if s[14] != '4' {
		return fmt.Errorf("只接受 UUID v4（版本位应为 4，实际 %q）", string(s[14]))
	}
	switch s[19] {
	case '8', '9', 'a', 'b':
	default:
		return fmt.Errorf("只接受 UUID v4（变体位应为 8/9/a/b，实际 %q）", string(s[19]))
	}
	return nil
}

// ParseRef 解析人类或 agent 输入的引用：canonical、短前缀（D-xxxxxxxx）、
// 裸 UUID、裸短前缀。known 提供当前仓库已有的 ID 用于消歧。
// 有歧义时报错并要求完整 ID。
func ParseRef(s string, known []ID) (ID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ID{}, fmt.Errorf("空引用")
	}
	if id, err := ParseID(s); err == nil {
		return id, nil
	}
	var wantKind Kind
	body := s
	if k, rest, ok := strings.Cut(s, "-"); ok && len(k) == 1 {
		if kind, valid := KindOf(k); valid {
			wantKind = kind
			body = rest
		}
	}
	body = strings.ToLower(body)
	if body == "" {
		return ID{}, fmt.Errorf("引用 %q 缺少标识部分", s)
	}
	for _, c := range body {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c == '-') {
			return ID{}, fmt.Errorf("引用 %q 含非法字符", s)
		}
	}
	var hits []ID
	for _, id := range known {
		if wantKind != "" && id.Kind != wantKind {
			continue
		}
		if strings.HasPrefix(id.UUID, body) {
			hits = append(hits, id)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return ID{}, fmt.Errorf("引用 %q 在本仓库没有匹配对象", s)
	default:
		names := make([]string, 0, len(hits))
		for _, h := range hits {
			names = append(names, h.String())
		}
		return ID{}, fmt.Errorf("引用 %q 有歧义，请用完整 ID：%s", s, strings.Join(names, " "))
	}
}

func (id *ID) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		*id = ID{}
		return nil
	}
	parsed, err := ParseID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

func (id ID) MarshalYAML() (any, error) {
	if id.IsZero() {
		return nil, nil
	}
	return id.String(), nil
}
