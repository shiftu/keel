package store

import "testing"

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"internal/store/**", "internal/store/db.go", true},
		{"internal/store/**", "internal/store/sub/x.go", true},
		// scope 写目录就该覆盖目录本身，否则「改这个目录前先看什么」会漏。
		{"internal/store/**", "internal/store", true},
		{"internal/store/**", "internal/check/db.go", false},
		{"internal/store/**", "internal", false},
		{"go.mod", "go.mod", true},
		{"go.mod", "internal/go.mod", false},
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"**/*.go", "c.md", false},
		{"docs/**/*.md", "docs/design/design.md", true},
		{"docs/**/*.md", "docs/README.md", true},
		{"requirements*.txt", "requirements-dev.txt", true},
		{"requirements*.txt", "requirements.txt", true},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"./go.mod", "go.mod", true},
		{"infra/**", "infra/terraform/main.tf", true},
	}
	for _, c := range cases {
		if got := MatchGlob(c.pattern, c.name); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v，想要 %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	pats := []string{"a/**", "b/**"}
	if p, ok := MatchAny(pats, "b/x.go"); !ok || p != "b/**" {
		t.Errorf("MatchAny = %q, %v", p, ok)
	}
	if _, ok := MatchAny(pats, "c/x.go"); ok {
		t.Error("不该命中")
	}
}
