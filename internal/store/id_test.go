package store

import "testing"

func TestParseIDStrict(t *testing.T) {
	ok := []string{
		"D-550e8400-e29b-41d4-a716-446655440000",
		"R-7c9e6679-7425-40de-944b-e07fc1f90ae7",
		"M-0dbdf12b-5e8c-4a0a-9c1a-2f3e4d5c6b7a",
		"E-11111111-2222-4333-8444-555555555555",
	}
	for _, s := range ok {
		if _, err := ParseID(s); err != nil {
			t.Errorf("ParseID(%q) 应该通过，得到 %v", s, err)
		}
	}
	bad := map[string]string{
		"D0012":                                  "旧的单调 ID 不再接受",
		"X-550e8400-e29b-41d4-a716-446655440000": "未知类型前缀",
		"D-550E8400-E29B-41D4-A716-446655440000": "大写十六进制",
		"D-550e8400-e29b-11d4-a716-446655440000": "不是 v4",
		"D-550e8400-e29b-41d4-7716-446655440000": "变体位非法",
		"D-550e8400":                             "短前缀不是 canonical",
		"":                                       "空串",
	}
	for s, why := range bad {
		if _, err := ParseID(s); err == nil {
			t.Errorf("ParseID(%q) 应该失败（%s）", s, why)
		}
	}
}

func TestNewIDIsValidV4(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id, err := NewID(KindDecision)
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		if _, err := ParseID(id.String()); err != nil {
			t.Fatalf("生成的 ID 无法解析: %v", err)
		}
		if seen[id.String()] {
			t.Fatalf("生成了重复 ID: %s", id)
		}
		seen[id.String()] = true
	}
}

func TestShortPrefix(t *testing.T) {
	id, _ := ParseID("D-550e8400-e29b-41d4-a716-446655440000")
	if got := id.Short(); got != "D-550e8400" {
		t.Errorf("Short() = %q，想要 D-550e8400", got)
	}
}

func TestParseRef(t *testing.T) {
	a, _ := ParseID("D-550e8400-e29b-41d4-a716-446655440000")
	b, _ := ParseID("D-550e8401-e29b-41d4-a716-446655440000")
	m, _ := ParseID("M-550e8400-e29b-41d4-a716-446655440000")
	known := []ID{a, b, m}

	cases := []struct {
		in   string
		want ID
	}{
		{"D-550e8400-e29b-41d4-a716-446655440000", a},
		{"D-550e8400", a},
		{"D-550e8401", b},
		{"M-550e8400", m},
	}
	for _, c := range cases {
		got, err := ParseRef(c.in, known)
		if err != nil {
			t.Errorf("ParseRef(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRef(%q) = %s，想要 %s", c.in, got, c.want)
		}
	}

	// 裸短前缀跨类型有歧义时必须要求完整 ID，而不是猜一个。
	if _, err := ParseRef("550e8400", known); err == nil {
		t.Error("跨类型歧义应该报错")
	}
	if _, err := ParseRef("D-ffffffff", known); err == nil {
		t.Error("无匹配应该报错")
	}
}
