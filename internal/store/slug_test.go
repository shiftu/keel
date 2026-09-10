package store

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"用 SQLite 而不是 Postgres":     "用-sqlite-而不是-postgres",
		"Use SQLite, not Postgres!": "use-sqlite-not-postgres",
		"   ":                       "",
		"a---b":                     "a-b",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q，想要 %q", in, got, want)
		}
	}
	long := Slugify("aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeee")
	if len([]rune(long)) > 40 {
		t.Errorf("slug 超过 40 字符：%q", long)
	}
}

func TestFileNameAndSlugRoundTrip(t *testing.T) {
	id, _ := ParseID("D-550e8400-e29b-41d4-a716-446655440000")
	name := FileName(id, "用-sqlite")
	if name != "D-550e8400-e29b-41d4-a716-446655440000-用-sqlite.md" {
		t.Fatalf("FileName = %q", name)
	}
	if got := slugFromFilename(name); got != "用-sqlite" {
		t.Errorf("slugFromFilename = %q", got)
	}
	if got := slugFromFilename(FileName(id, "")); got != "" {
		t.Errorf("无 slug 时应为空，得到 %q", got)
	}
}
