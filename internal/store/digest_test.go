package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func memoryFixture() *Memory {
	m := &Memory{}
	m.MemoryFM = MemoryFM{
		Schema: SchemaVersion, ID: ID{Kind: KindMemory, UUID: "11111111-2222-4333-8444-555555555555"},
		Kind: MemKindGotcha, Status: MemCandidate, Summary: "摘要",
		Tags: []string{"b", "a"}, Scope: []string{"src/**"},
		Conditions: map[string]string{"environment": "本地盘"},
	}
	m.meta = meta{body: "正文"}
	return m
}

// verify 会改 status、evidence、verified_at。摘要要是把它们算进去，
// 刚写下的证据在下一次 check 时就自己失效了。
func TestObjectDigestIgnoresFieldsVerifyMutates(t *testing.T) {
	before := ObjectDigest(memoryFixture())

	m := memoryFixture()
	m.Status = MemVerified
	m.Evidence = []ID{{Kind: KindEvidence, UUID: "22222222-2222-4333-8444-555555555555"}}
	d := NewDate(time.Now())
	m.VerifiedAt = &d

	if got := ObjectDigest(m); got != before {
		t.Fatalf("状态与证据变化不该改变对象摘要\nbefore=%s\nafter =%s", before, got)
	}
}

func TestObjectDigestTracksConclusionChanges(t *testing.T) {
	before := ObjectDigest(memoryFixture())
	for name, mutate := range map[string]func(*Memory){
		"正文":    func(m *Memory) { m.meta.body = "改过的正文" },
		"摘要":    func(m *Memory) { m.Summary = "别的摘要" },
		"scope": func(m *Memory) { m.Scope = []string{"other/**"} },
		"适用条件":  func(m *Memory) { m.Conditions = map[string]string{"environment": "NFS"} },
	} {
		m := memoryFixture()
		mutate(m)
		if ObjectDigest(m) == before {
			t.Errorf("%s 变了，摘要却没变——旧证据会被当成仍然有效", name)
		}
	}
}

// tag 顺序是书写习惯，不是结论变化。
func TestObjectDigestIsOrderIndependent(t *testing.T) {
	a := memoryFixture()
	b := memoryFixture()
	b.Tags = []string{"a", "b"}
	if ObjectDigest(a) != ObjectDigest(b) {
		t.Fatal("tag 顺序不该改变摘要")
	}
}

func TestDigesterMatchesScopeDigest(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/a.go", "package a")
	write(t, root, "src/sub/b.go", "package b")
	write(t, root, "docs/x.md", "无关")

	scope := []string{"src/**"}
	want, inputs, err := ScopeDigest(root, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 {
		t.Fatalf("应只收 src 下两个文件，实际 %v", inputs)
	}
	got, err := NewDigester(root, [][]string{scope}).Digest(scope)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Digester 与 ScopeDigest 不一致：%s vs %s", got, want)
	}

	write(t, root, "src/a.go", "package a // 改过")
	after, err := NewDigester(root, [][]string{scope}).Digest(scope)
	if err != nil {
		t.Fatal(err)
	}
	if after == want {
		t.Fatal("scope 内文件变了，摘要必须变")
	}
}

// 证据目录和缓存目录会被验证动作自己改动，不能进摘要——否则每验证一次就自己失效。
func TestScopeDigestSkipsEvidenceAndCache(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".keel/memory/m.md", "x")
	scope := []string{".keel/**"}
	before, _, err := ScopeDigest(root, scope)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, ".keel/evidence/E-1.md", "新证据")
	write(t, root, ".keel/cache/stop/x", "缓存")
	after, _, err := ScopeDigest(root, scope)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("evidence/ 与 cache/ 不该进内容摘要")
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
