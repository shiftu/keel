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

// Input 是组装一次上下文包需要的全部材料。
type Input struct {
	Root      string
	Config    store.Config
	Set       *store.Set
	Query     Query
	HasIntent bool
	// Task 是接续摘要，可为 nil。Head 用来判断摘要有没有落后。
	Task *store.Task
	Head string
}

// Item 是一条被选中的材料。
type Item struct {
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	StatusNote  string            `json:"status_note,omitempty"`
	Path        string            `json:"path"`
	Conditions  map[string]string `json:"conditions,omitempty"`
	WhySelected string            `json:"why_selected"`
	rank        int
	date        string
}

// Precedent 是级别 1 下可复用的先例，以及它有没有跑出来的证据。
type Precedent struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Verified bool   `json:"verified"`
}

// TaskBlock 是渲染用的接续摘要。
type TaskBlock struct {
	Goal      string   `json:"goal"`
	Done      []string `json:"done,omitempty"`
	Next      []string `json:"next,omitempty"`
	Failing   []string `json:"failing,omitempty"`
	Refs      []string `json:"refs,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	UpdatedBy string   `json:"updated_by,omitempty"`
	// BaseNote 说明摘要记录时的提交与当前 HEAD 的关系。
	BaseNote string `json:"base_note,omitempty"`
}

// Brief 是一次上下文包。
type Brief struct {
	Schema     int         `json:"schema"`
	HasIntent  bool        `json:"has_intent"`
	Task       *TaskBlock  `json:"task,omitempty"`
	Workflow   Workflow    `json:"workflow"`
	Precedents []Precedent `json:"precedents,omitempty"`
	Decisions  []Item      `json:"decisions"`
	Memories   []Item      `json:"memories"`
	Rules      []Item      `json:"rules"`
	Omitted    []string    `json:"omitted,omitempty"`
	Notes      []string    `json:"notes,omitempty"`
}

// Workflow 是这次任务的工作流建议。
type Workflow struct {
	Level   int      `json:"level"`
	Meaning string   `json:"meaning"`
	Binding []string `json:"binding"`
}

// Build 组装上下文包。
func Build(in Input) *Brief {
	cfg, set, q := in.Config, in.Set, in.Query
	d := policy.Evaluate(cfg.Workflow, policy.Query{Action: q.Action, Paths: q.Paths})
	b := &Brief{Schema: 1, HasIntent: in.HasIntent}
	b.Workflow = Workflow{Level: int(d.Level), Meaning: d.Level.String()}
	for _, r := range d.Binding {
		b.Workflow.Binding = append(b.Workflow.Binding, fmt.Sprintf("%s %s = %d", r.Source, r.Key, int(r.Level)))
	}
	b.Task = taskBlock(in)

	// 记忆的可信度要看代码有没有变，所以按需算一次内容摘要——整棵树只走一遍。
	dg := store.NewDigester(in.Root, store.MemoryScopes(set))

	for _, dec := range set.ActiveDecisions() {
		it := Item{Kind: "decision", ID: dec.ID.String(), Title: dec.Title,
			Status: string(dec.Status), Path: dec.SourcePath(), date: dec.Date.String()}
		it.rank, it.WhySelected = rankDecision(set, dec, q)
		b.Decisions = append(b.Decisions, it)
	}
	for _, m := range set.Memories {
		if !m.Status.IsRecallable() {
			continue
		}
		t := store.TrustOf(set, m, dg)
		it := Item{Kind: "memory", ID: m.ID.String(), Title: m.Summary,
			Status: string(m.Status), StatusNote: trustNote(m, t),
			Path: m.SourcePath(), Conditions: m.Conditions, date: m.Date.String()}
		it.rank, it.WhySelected = rankMemory(m, t, q)
		b.Memories = append(b.Memories, it)
	}
	for _, r := range set.Rules {
		if r.Status != store.RuleActive {
			continue
		}
		it := Item{Kind: "rule", ID: r.ID.String(), Title: r.Title,
			Status: string(r.Status), Path: r.SourcePath()}
		it.rank, it.WhySelected = rankRule(set, r, q)
		b.Rules = append(b.Rules, it)
	}

	sortItems(b.Decisions)
	sortItems(b.Memories)
	sortItems(b.Rules)
	b.Decisions = trim(b.Decisions, cfg.Brief.MaxDecisions, &b.Omitted, "决策")
	b.Memories = trim(b.Memories, cfg.Brief.MaxNotes, &b.Omitted, "记忆")

	addPrecedents(b, set, d.Level, q)
	if !q.HasFocus() {
		b.Notes = append(b.Notes,
			"还没有具体任务，这里只给基础材料。理解用户请求后用 keel brief --task \"…\" --path <路径> 再查一次。")
	}
	if !in.HasIntent {
		b.Notes = append(b.Notes, "本仓库还没有填 .keel/intent.md，缺一层项目背景。")
	}
	return b
}

func taskBlock(in Input) *TaskBlock {
	t := in.Task
	if t == nil || strings.TrimSpace(t.Goal) == "" {
		return nil
	}
	tb := &TaskBlock{Goal: t.Goal, Done: t.Done, Next: t.Next, Failing: t.Failing,
		Refs: t.Refs, UpdatedAt: t.UpdatedAt, UpdatedBy: t.UpdatedBy}
	switch {
	case t.BaseCommit == "":
		tb.BaseNote = "记录时仓库还没有提交"
	case in.Head != "" && in.Head != t.BaseCommit:
		tb.BaseNote = fmt.Sprintf("记录于 %s，当前 HEAD 是 %s——中间有新提交，摘要可能已经落后",
			short8(t.BaseCommit), short8(in.Head))
	default:
		tb.BaseNote = fmt.Sprintf("记录于 %s（与当前 HEAD 一致）", short8(t.BaseCommit))
	}
	return tb
}

// addPrecedents 在上限为 1 时列出适用先例。没有路径就说明判断不了，
// 不拿「本仓库所有有效决策」冒充先例。
func addPrecedents(b *Brief, set *store.Set, lv policy.Level, q Query) {
	if lv != policy.LevelPrecedent {
		return
	}
	if len(q.Paths) == 0 {
		// 没有任务时已经在提示「先给任务再查」，这里不重复念一遍。
		if q.HasFocus() {
			b.Notes = append(b.Notes,
				"上限是 1（有适用先例才自决），但这次没给 --path，先例判断不了。带上相关路径再查一次。")
		}
		return
	}
	ps := policy.Precedents(set, policy.PrecedentQuery{Paths: q.Paths})
	if len(ps) == 0 {
		b.Notes = append(b.Notes, "上限是 1，但这些路径上没有适用先例。按 0 处理：先问人。")
		return
	}
	withEvidence := 0
	for _, p := range ps {
		b.Precedents = append(b.Precedents, Precedent{
			ID: p.ID.String(), Title: p.Title, Status: string(p.Status), Verified: p.Verified})
		if p.Verified {
			withEvidence++
		}
	}
	if withEvidence == 0 {
		b.Notes = append(b.Notes,
			"这些先例都没有跑出来的证据（手写 proven 不算）。可以参考，但不构成「验证过的做法」。")
	}
}

// 挑选顺序。数字大的排前面，同分按日期降序再按 ID 升序。
const (
	rankExplicit = 6 // 任务文本里点名了这条
	rankPath     = 5 // --path 命中 scope
	rankConflict = 4 // 冲突、失败、证据过期
	rankEvidence = 3 // 有对得上的 pass 证据
	rankTopic    = 2 // 标题词或 tag 出现在任务里
	rankBase     = 1
)

func rankDecision(set *store.Set, d *store.Decision, q Query) (int, string) {
	if mentioned(q.Task, d.ID) {
		return rankExplicit, "任务里点名了这条决策"
	}
	if pat, ok := matchPaths(d.Scope, q.Paths); ok {
		return rankPath, "scope " + pat + " 覆盖了本次路径"
	}
	if d.Status == store.DecRevisit {
		return rankConflict, "状态是 revisit，结论正在被重新考虑"
	}
	if _, ok := set.SupportingEvidence(d); ok {
		return rankEvidence, "有对得上当前内容的 pass 证据"
	}
	if hit, ok := topicHit(q.Task, d.Title, d.Tags); ok {
		return rankTopic, "主题匹配 " + hit
	}
	return rankBase, "本仓库的有效决策"
}

func rankMemory(m *store.Memory, t store.MemoryTrust, q Query) (int, string) {
	prefix := statusPrefix(m, t)
	if mentioned(q.Task, m.ID) {
		return rankExplicit, prefix + "任务里点名了这条记忆"
	}
	if pat, ok := matchPaths(m.Scope, q.Paths); ok {
		return rankPath, prefix + "scope " + pat + " 覆盖了本次路径"
	}
	if m.Status == store.MemDisputed {
		return rankConflict, "有冲突的记录，召回时要同时看来源"
	}
	// 自称 verified 却撑不住的，比普通记忆更需要被看见——它正被当成规则用。
	if m.Status == store.MemVerified && !t.Trustworthy() {
		return rankConflict, prefix + "它自称已验证，但证据撑不住"
	}
	if t.Trustworthy() {
		return rankEvidence, "有对得上当前代码的 pass 证据"
	}
	if hit, ok := topicHit(q.Task, m.Summary, m.Tags); ok {
		return rankTopic, prefix + "主题匹配 " + hit
	}
	return rankBase, prefix + "本仓库的记忆"
}

func rankRule(set *store.Set, r *store.Rule, q Query) (int, string) {
	if mentioned(q.Task, r.ID) {
		return rankExplicit, "任务里点名了这条规则"
	}
	if pat, ok := matchPaths(r.Scope, q.Paths); ok {
		return rankPath, "scope " + pat + " 覆盖了本次路径"
	}
	if r.From != nil {
		if d, ok := set.DecisionByID(*r.From); ok && !d.Status.IsActive() {
			return rankConflict, "规则依据 " + d.ID.Short() + " 已经是 " + string(d.Status)
		}
	}
	if hit, ok := topicHit(q.Task, r.Title, nil); ok {
		return rankTopic, "主题匹配 " + hit
	}
	return rankBase, "生效中的规则"
}

// statusPrefix 让未验证或已失效的记忆在被引用时始终带标签。
func statusPrefix(m *store.Memory, t store.MemoryTrust) string {
	switch {
	case m.Status == store.MemCandidate:
		return "候选（未验证）："
	case m.Status == store.MemVerified && !t.Supported:
		return "自称已验证但无有效证据："
	case m.Status == store.MemVerified && t.ContentStale:
		return "已验证但证据过期："
	}
	return ""
}

// trustNote 是给人看的状态补充，JSON 消费者按 status + status_note 判断。
func trustNote(m *store.Memory, t store.MemoryTrust) string {
	if m.Status != store.MemVerified {
		return ""
	}
	switch {
	case !t.Supported:
		return "无有效证据"
	case t.ContentStale:
		return "证据已过期"
	}
	return ""
}

// mentioned 判断任务文本里有没有点名这个 ID（完整或短前缀）。
func mentioned(task string, id store.ID) bool {
	if strings.TrimSpace(task) == "" {
		return false
	}
	lower := strings.ToLower(task)
	return strings.Contains(lower, strings.ToLower(id.String())) ||
		strings.Contains(lower, strings.ToLower(id.Short()))
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

func short8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
