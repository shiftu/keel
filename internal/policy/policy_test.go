package policy

import (
	"testing"

	"github.com/shiftu/keel/internal/store"
)

func cfg() store.WorkflowConfig {
	return store.WorkflowConfig{
		DefaultLevel: 2,
		Actions: map[string]int{
			"dependency.add": 1,
			"data.delete":    0,
			"security":       0,
		},
		Paths: map[string]int{
			"internal/store/**": 1,
			"infra/**":          0,
		},
	}
}

func TestEvaluateTakesStrictest(t *testing.T) {
	cases := []struct {
		name string
		q    Query
		want Level
	}{
		{"没有命中就是默认值", Query{Action: "docs.edit", Paths: []string{"README.md"}}, LevelAutonomous},
		{"动作压低", Query{Action: "dependency.add", Paths: []string{"README.md"}}, LevelPrecedent},
		{"路径压低", Query{Action: "docs.edit", Paths: []string{"infra/main.tf"}}, LevelAsk},
		// 低风险动作不能抵消路径上的限制。
		{"多项命中取更严", Query{Action: "dependency.add", Paths: []string{"infra/main.tf"}}, LevelAsk},
		{"高风险动作压到 0", Query{Action: "data.delete", Paths: []string{"README.md"}}, LevelAsk},
	}
	for _, c := range cases {
		if got := Evaluate(cfg(), c.q); got.Level != c.want {
			t.Errorf("%s: Level = %d，想要 %d", c.name, got.Level, c.want)
		}
	}
}

// tag 只服务检索：同为 db 的成功先例不能给高风险数据操作放行。
func TestTagsDoNotRaiseLevel(t *testing.T) {
	c := cfg()
	got := Evaluate(c, Query{Action: "data.delete", Paths: []string{"internal/store/db.go"}})
	if got.Level != LevelAsk {
		t.Fatalf("Level = %d，想要 0", got.Level)
	}
	var sawAction bool
	for _, r := range got.Binding {
		if r.Source == "action" && r.Key == "data.delete" {
			sawAction = true
		}
	}
	if !sawAction {
		t.Error("应该说明是哪一条把上限压到 0 的")
	}
}

func TestPrecedentsRequireEvidenceToBeVerified(t *testing.T) {
	id, _ := store.ParseID("D-550e8400-e29b-41d4-a716-446655440000")
	set := &store.Set{Decisions: []*store.Decision{{
		DecisionFM: store.DecisionFM{
			ID:     id,
			Title:  "用 SQLite",
			Status: store.DecProven,
			Tags:   []string{"db"},
			Scope:  []string{"internal/store/**"},
		},
	}}}
	got := Precedents(set, PrecedentQuery{Tags: []string{"db"}, Paths: []string{"internal/store/db.go"}})
	if len(got) != 1 {
		t.Fatalf("找到 %d 条先例，想要 1", len(got))
	}
	// 手写的 proven 没有证据支撑，不能算已验证。
	if got[0].Verified {
		t.Error("没有 evidence 的 proven 不应标记为 Verified")
	}
}
