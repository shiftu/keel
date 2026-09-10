// Package brief 按任务和路径确定性地挑选上下文，并受字节预算约束。
// 挑选顺序固定，每条都带被选中的理由；候选事实以引用呈现，不提升成执行指令。
package brief

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/policy"
	"github.com/shiftu/keel/internal/store"
)

// Query 描述这次要做什么。SessionStart 还不知道任务时两者都为空。
type Query struct {
	Task   string
	Paths  []string
	Action string
}

// HasFocus 报告是否已经有明确任务或路径。
func (q Query) HasFocus() bool { return strings.TrimSpace(q.Task) != "" || len(q.Paths) > 0 }

// Item 是一条被选中的材料。
type Item struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Path        string `json:"path"`
	WhySelected string `json:"why_selected"`
	rank        int
	date        string
}

// Brief 是一次上下文包。
type Brief struct {
	Schema    int      `json:"schema"`
	HasIntent bool     `json:"has_intent"`
	Workflow  Workflow `json:"workflow"`
	Decisions []Item   `json:"decisions"`
	Memories  []Item   `json:"memories"`
	Rules     []Item   `json:"rules"`
	Omitted   []string `json:"omitted,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

// Workflow 是这次任务的工作流建议。
type Workflow struct {
	Level   int      `json:"level"`
	Meaning string   `json:"meaning"`
	Binding []string `json:"binding"`
}

// Build 组装上下文包。
func Build(cfg store.Config, set *store.Set, q Query, hasIntent bool) *Brief {
	d := policy.Evaluate(cfg.Workflow, policy.Query{Action: q.Action, Paths: q.Paths})
	b := &Brief{Schema: 1, HasIntent: hasIntent}
	b.Workflow = Workflow{Level: int(d.Level), Meaning: d.Level.String()}
	for _, r := range d.Binding {
		b.Workflow.Binding = append(b.Workflow.Binding, fmt.Sprintf("%s %s = %d", r.Source, r.Key, int(r.Level)))
	}

	for _, dec := range set.ActiveDecisions() {
		it := Item{Kind: "decision", ID: dec.ID.String(), Title: dec.Title,
			Status: string(dec.Status), Path: dec.SourcePath(), date: dec.Date.String()}
		it.rank, it.WhySelected = rankDecision(dec, q)
		b.Decisions = append(b.Decisions, it)
	}
	for _, m := range set.Memories {
		if !m.Status.IsRecallable() {
			continue
		}
		it := Item{Kind: "memory", ID: m.ID.String(), Title: m.Summary,
			Status: string(m.Status), Path: m.SourcePath(), date: m.Date.String()}
		it.rank, it.WhySelected = rankMemory(m, q)
		b.Memories = append(b.Memories, it)
	}
	for _, r := range set.Rules {
		if r.Status != store.RuleActive {
			continue
		}
		it := Item{Kind: "rule", ID: r.ID.String(), Title: r.Title,
			Status: string(r.Status), Path: r.SourcePath()}
		it.rank, it.WhySelected = rankRule(r, q)
		b.Rules = append(b.Rules, it)
	}

	sortItems(b.Decisions)
	sortItems(b.Memories)
	sortItems(b.Rules)
	b.Decisions = trim(b.Decisions, cfg.Brief.MaxDecisions, &b.Omitted, "决策")
	b.Memories = trim(b.Memories, cfg.Brief.MaxNotes, &b.Omitted, "记忆")

	if !q.HasFocus() {
		b.Notes = append(b.Notes,
			"还没有具体任务，这里只给基础材料。理解用户请求后用 keel brief --task \"…\" --path <路径> 再查一次。")
	}
	if !hasIntent {
		b.Notes = append(b.Notes, "本仓库还没有填 .keel/intent.md，缺一层项目背景。")
	}
	return b
}

// 挑选顺序：显式路径命中 > 冲突与失败提醒 > 主题匹配 > 兜底。
const (
	rankPath     = 4
	rankConflict = 3
	rankTopic    = 2
	rankBase     = 1
)

func rankDecision(d *store.Decision, q Query) (int, string) {
	if pat, ok := matchPaths(d.Scope, q.Paths); ok {
		return rankPath, "scope " + pat + " 覆盖了本次路径"
	}
	if d.Status == store.DecRevisit {
		return rankConflict, "状态是 revisit，结论正在被重新考虑"
	}
	if hit, ok := topicHit(q.Task, d.Title, d.Tags); ok {
		return rankTopic, "主题匹配 " + hit
	}
	return rankBase, "本仓库的有效决策"
}

func rankMemory(m *store.Memory, q Query) (int, string) {
	if pat, ok := matchPaths(m.Scope, q.Paths); ok {
		return rankPath, prefixStatus(m.Status) + "scope " + pat + " 覆盖了本次路径"
	}
	if m.Status == store.MemDisputed {
		return rankConflict, "有冲突的记录，召回时要同时看来源"
	}
	if hit, ok := topicHit(q.Task, m.Summary, m.Tags); ok {
		return rankTopic, prefixStatus(m.Status) + "主题匹配 " + hit
	}
	return rankBase, prefixStatus(m.Status) + "本仓库的记忆"
}

// prefixStatus 让候选记忆在被引用时始终带上「未验证」的标签。
func prefixStatus(s store.MemoryStatus) string {
	if s == store.MemCandidate {
		return "候选（未验证）："
	}
	return ""
}

func rankRule(r *store.Rule, q Query) (int, string) {
	if pat, ok := matchPaths(r.Scope, q.Paths); ok {
		return rankPath, "scope " + pat + " 覆盖了本次路径"
	}
	if hit, ok := topicHit(q.Task, r.Title, nil); ok {
		return rankTopic, "主题匹配 " + hit
	}
	return rankBase, "生效中的规则"
}

func matchPaths(scope, paths []string) (string, bool) {
	for _, p := range paths {
		if pat, ok := store.MatchAny(scope, p); ok {
			return pat, true
		}
	}
	return "", false
}

func topicHit(task, title string, tags []string) (string, bool) {
	task = strings.ToLower(strings.TrimSpace(task))
	if task == "" {
		return "", false
	}
	for _, t := range tags {
		if strings.Contains(task, strings.ToLower(t)) {
			return t, true
		}
	}
	for _, w := range strings.Fields(title) {
		w = strings.ToLower(strings.Trim(w, "，。,.:：、"))
		if len([]rune(w)) >= 2 && strings.Contains(task, w) {
			return w, true
		}
	}
	return "", false
}

func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.rank != b.rank {
			return a.rank > b.rank
		}
		if a.date != b.date {
			return a.date > b.date
		}
		return a.ID < b.ID
	})
}

func trim(items []Item, max int, omitted *[]string, label string) []Item {
	if max <= 0 || len(items) <= max {
		return items
	}
	*omitted = append(*omitted, fmt.Sprintf("%s还有 %d 条未显示（按相关性截断）；用 keel why 查全部",
		label, len(items)-max))
	return items[:max]
}
