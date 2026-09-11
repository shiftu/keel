// Package review 组装进化报告。
//
// 它**只读**：不改任何文件，也不执行任何规则。一次「看看有什么要维护的」
// 不该顺带执行仓库里的代码——规则过不过，跑 keel check --target worktree。
//
// 候选是确定性聚类，不是自动写出来的修订。keel 给簇和依据，
// 判断它们是不是同一条不变量、要不要提炼成规则，是宿主 agent 的活。
package review

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/store"
)

// Schema 是机器输出的版本。
const Schema = 1

// Due 是一条到期需要复查的对象。
type Due struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Path        string `json:"path"`
	ReviewAfter string `json:"review_after"`
	Evidence    string `json:"evidence"`
}

// Issue 是一条失效或冲突。code 与 check 的 finding 同一套契约。
type Issue struct {
	Code    string `json:"code"`
	ID      string `json:"object,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Candidate 是一个学习候选簇。Members 是依据，不是结论。
type Candidate struct {
	Code    string   `json:"code"`
	Subject string   `json:"subject,omitempty"`
	Reason  string   `json:"reason"`
	Members []string `json:"members,omitempty"`
	Next    string   `json:"next"`
}

// Stats 只用来排审阅优先级，不决定可信或生效。
type Stats struct {
	Decisions        int `json:"decisions"`
	ActiveDecisions  int `json:"active_decisions"`
	DecisionsWithEv  int `json:"decisions_with_evidence"`
	Rules            int `json:"rules"`
	ActiveRules      int `json:"active_rules"`
	CandidateRules   int `json:"candidate_rules"`
	Memories         int `json:"memories"`
	VerifiedMemories int `json:"verified_memories"`
	Evidence         int `json:"evidence"`
	// Current / History 是知识索引的两层，口径与 render.KnowledgeIndex 一致。
	Current int `json:"current_objects"`
	History int `json:"history_objects"`
}

// Report 是一次 keel review 的完整输出。
type Report struct {
	Schema     int         `json:"schema"`
	Due        []Due       `json:"due"`
	Issues     []Issue     `json:"issues"`
	Candidates []Candidate `json:"candidates"`
	Stats      Stats       `json:"stats"`
	Notes      []string    `json:"notes,omitempty"`
}

// 候选 code。agent 按 code 判断，不解析自然语言。
const (
	CodeRuleReady        = "rule_candidate_ready"
	CodeRuleIncomplete   = "rule_candidate_incomplete"
	CodeMemoryCluster    = "memory_cluster"
	CodeMemoryDisputed   = "memory_disputed"
	CodeMemoryArchive    = "memory_archive_candidate"
	CodeIndexPressure    = "index_pressure"
	CodeDecisionReverted = "decision_reverted"
)

// 报告里保留的知识层 finding。规则执行类的 code 不在这里——review 不跑规则。
var issueCodes = map[string]bool{
	check.CodeRuleBasisInvalid:        true,
	check.CodeMemoryStatusUnsupported: true,
	check.CodeMemoryEvidenceStale:     true,
	check.CodeMemoryConflict:          true,
	check.CodeMemoryReviewDue:         true,
	check.CodeDecisionScopeOverlap:    true,
	check.CodeObjectStale:             true,
	check.CodeSupersedeIncomplete:     true,
	check.CodeRefMissing:              true,
	check.CodeFrontmatterInvalid:      true,
}

// Input 是组装报告需要的材料。Reverted 由调用方从 git log 取，可为 nil。
type Input struct {
	Store    *store.Store
	Config   store.Config
	Set      *store.Set
	Reverted map[string][]string
	Now      time.Time
}

// Build 组装报告。
func Build(in Input) *Report {
	set := in.Set
	r := &Report{Schema: Schema, Due: []Due{}, Issues: []Issue{}, Candidates: []Candidate{}}

	res := check.NewResult(check.TargetWorktree)
	check.Objects(in.Store, in.Config, set, res)
	res.Sort()
	for _, f := range res.Findings {
		if issueCodes[f.Code] {
			r.Issues = append(r.Issues, Issue{Code: f.Code, ID: f.Object, Path: f.Path, Message: f.Message})
		}
	}

	dg := store.NewDigester(in.Store.Root, store.MemoryScopes(set))
	collectDue(r, set, dg, in.Now)
	ruleCandidates(r, set)
	memoryCandidates(r, set)
	archiveCandidates(r, set, in.Now)
	revertedDecisions(r, set, in.Reverted)
	r.Stats = stats(set, dg)
	indexPressure(r, in.Config)

	sort.SliceStable(r.Candidates, func(i, j int) bool {
		if r.Candidates[i].Code != r.Candidates[j].Code {
			return r.Candidates[i].Code < r.Candidates[j].Code
		}
		return r.Candidates[i].Subject < r.Candidates[j].Subject
	})
	r.Notes = append(r.Notes,
		"条数只排审阅优先级，不决定可信或生效。同一来源的重复转述计一条。",
		"review 不执行任何规则。规则当前过不过，跑 keel check --target worktree。")
	return r
}

func collectDue(r *Report, set *store.Set, dg *store.Digester, now time.Time) {
	for _, d := range set.Decisions {
		if d.ReviewAfter == nil || d.ReviewAfter.IsZero() || !d.ReviewAfter.Time.Before(now) {
			continue
		}
		if !d.Status.IsActive() {
			continue
		}
		ev := "无证据"
		if _, ok := set.SupportingEvidence(d); ok {
			ev = "有对得上的 pass 证据"
		}
		r.Due = append(r.Due, Due{Kind: "decision", ID: d.ID.String(), Title: d.Title,
			Status: string(d.Status), Path: d.SourcePath(),
			ReviewAfter: d.ReviewAfter.String(), Evidence: ev})
	}
	for _, m := range set.Memories {
		if m.ReviewAfter == nil || m.ReviewAfter.IsZero() || !m.ReviewAfter.Time.Before(now) {
			continue
		}
		if m.Status != store.MemCandidate && m.Status != store.MemVerified {
			continue
		}
		t := store.TrustOf(set, m, dg)
		ev := "无证据"
		switch {
		case t.Trustworthy():
			ev = "有对得上的 pass 证据"
		case t.Supported:
			ev = "有证据但已过期"
		}
		r.Due = append(r.Due, Due{Kind: "memory", ID: m.ID.String(), Title: m.Summary,
			Status: string(m.Status), Path: m.SourcePath(),
			ReviewAfter: m.ReviewAfter.String(), Evidence: ev})
	}
	sort.SliceStable(r.Due, func(i, j int) bool {
		if r.Due[i].ReviewAfter != r.Due[j].ReviewAfter {
			return r.Due[i].ReviewAfter < r.Due[j].ReviewAfter
		}
		return r.Due[i].ID < r.Due[j].ID
	})
}

func ruleCandidates(r *Report, set *store.Set) {
	for _, rule := range set.Rules {
		if rule.Status != store.RuleCandidate {
			continue
		}
		var missing []string
		if rule.Check == nil || len(rule.Check.Argv) == 0 {
			missing = append(missing, "check.argv")
		}
		if rule.From == nil {
			missing = append(missing, "from（依据决策）")
		} else if d, ok := set.DecisionByID(*rule.From); !ok || !d.Status.IsActive() {
			missing = append(missing, "有效的 from（依据决策已失效或不存在）")
		}
		if len(rule.CaseDirs("pass")) == 0 {
			missing = append(missing, "expect: pass 的对照用例")
		}
		if len(rule.CaseDirs("fail")) == 0 {
			missing = append(missing, "expect: fail 的对照用例")
		}
		if len(missing) == 0 {
			r.Candidates = append(r.Candidates, Candidate{
				Code: CodeRuleReady, Subject: rule.ID.String(),
				Reason: fmt.Sprintf("候选规则「%s」前置条件齐全", rule.Title),
				Next:   "keel promote " + rule.ID.Short(),
			})
			continue
		}
		// 缺什么已经写在 Reason 里了，不再用 Members 重复一遍。
		r.Candidates = append(r.Candidates, Candidate{
			Code:    CodeRuleIncomplete,
			Subject: rule.ID.String(),
			Reason:  fmt.Sprintf("候选规则「%s」还缺：%s", rule.Title, strings.Join(missing, "、")),
			Next:    "补齐之后再 keel promote " + rule.ID.Short(),
		})
	}
}

// memoryCandidates 按 scope + tag 聚簇。同一 derived_from 的重复转述算一条，
// 不让「同一件事换个说法记三遍」变成三份依据。
func memoryCandidates(r *Report, set *store.Set) {
	groups := map[string][]*store.Memory{}
	for _, m := range set.Memories {
		if m.Status != store.MemCandidate || len(m.Scope) == 0 || len(m.Tags) == 0 {
			continue
		}
		// 反例是对某条结论的反驳，不是它的同类。它单独报成 memory_disputed。
		if m.Kind == store.MemKindCounterexample {
			continue
		}
		for _, key := range clusterKeys(m) {
			groups[key] = append(groups[key], m)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		members := dedupBySource(groups[k])
		if len(members) < 2 {
			continue
		}
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.ID.Short())
		}
		r.Candidates = append(r.Candidates, Candidate{
			Code: CodeMemoryCluster, Subject: k,
			Reason:  fmt.Sprintf("%d 条 candidate 记忆落在同一个 scope 和 tag 上", len(members)),
			Members: ids,
			Next:    "看看它们是不是同一条不变量；是的话写成 candidate 规则，配 from 和 cases，再 keel promote",
		})
	}

	for _, m := range set.Memories {
		if m.Kind != store.MemKindCounterexample || m.Status == store.MemArchived {
			continue
		}
		for _, id := range m.DerivedFrom {
			target, ok := set.MemoryByID(id)
			if !ok {
				continue
			}
			r.Candidates = append(r.Candidates, Candidate{
				Code: CodeMemoryDisputed, Subject: target.ID.String(),
				Reason:  fmt.Sprintf("反例 %s 指向它：「%s」", m.ID.Short(), m.Summary),
				Members: []string{m.ID.Short()},
				Next:    "先处理冲突，再谈提炼。重新验证被反驳的那条，或把它改成 disputed / archived",
			})
		}
	}
}

// clusterKeys 用「每个 scope × 每个 tag」组键：一条记忆可以落进多个簇。
func clusterKeys(m *store.Memory) []string {
	scopes := append([]string{}, m.Scope...)
	tags := append([]string{}, m.Tags...)
	sort.Strings(scopes)
	sort.Strings(tags)
	var out []string
	for _, s := range scopes {
		for _, t := range tags {
			out = append(out, t+" @ "+s)
		}
	}
	return out
}

// dedupBySource 把同源转述折成一条。两种同源都要认出来：
// 共享同一个 derived_from，或者直接 derived_from 簇里已有的那条。
// 「同一件事换个说法记三遍」不该变成三份依据。
func dedupBySource(ms []*store.Memory) []*store.Memory {
	seen := map[string]bool{}
	var out []*store.Memory
	for _, m := range ms {
		if seen[m.ID.String()] {
			continue
		}
		dup := false
		for _, src := range m.DerivedFrom {
			if seen[src.String()] {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		// 自己的 ID 也记进去：后面 derived_from 指向它的都是转述。
		seen[m.ID.String()] = true
		for _, src := range m.DerivedFrom {
			seen[src.String()] = true
		}
		out = append(out, m)
	}
	return out
}

func revertedDecisions(r *Report, set *store.Set, reverted map[string][]string) {
	ids := make([]string, 0, len(reverted))
	for id := range reverted {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, raw := range ids {
		id, err := store.ParseID(raw)
		if err != nil {
			continue
		}
		d, ok := set.DecisionByID(id)
		if !ok || !d.Status.IsActive() {
			continue
		}
		commits := append([]string{}, reverted[raw]...)
		sort.Strings(commits)
		r.Candidates = append(r.Candidates, Candidate{
			Code: CodeDecisionReverted, Subject: d.ID.String(),
			Reason:  fmt.Sprintf("「%s」关联的 %d 个提交后来被 revert 过", d.Title, len(commits)),
			Members: shortSHAs(commits),
			Next:    "确认结论还成不成立；不成立就用 accepted 决策替代它，别直接删",
		})
	}
}

func shortSHAs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if len(s) > 8 {
			s = s[:8]
		}
		out = append(out, s)
	}
	return out
}

func stats(set *store.Set, dg *store.Digester) Stats {
	s := Stats{
		Decisions: len(set.Decisions), Rules: len(set.Rules),
		Memories: len(set.Memories), Evidence: len(set.Evidence),
	}
	for _, d := range set.Decisions {
		if d.Status.IsActive() {
			s.ActiveDecisions++
		}
		if _, ok := set.SupportingEvidence(d); ok {
			s.DecisionsWithEv++
		}
	}
	for _, r := range set.Rules {
		switch r.Status {
		case store.RuleActive:
			s.ActiveRules++
		case store.RuleCandidate:
			s.CandidateRules++
		}
	}
	for _, m := range set.Memories {
		if m.Status == store.MemVerified && store.TrustOf(set, m, dg).Trustworthy() {
			s.VerifiedMemories++
		}
	}
	for _, d := range set.Decisions {
		countLayer(&s, d.Status.IsHistory())
	}
	for _, r := range set.Rules {
		countLayer(&s, r.Status.IsHistory())
	}
	for _, m := range set.Memories {
		countLayer(&s, m.Status.IsHistory())
	}
	return s
}

func countLayer(s *Stats, history bool) {
	if history {
		s.History++
		return
	}
	s.Current++
}

// archiveCandidates 挑出「过了复查日期，而且一条证据都没有」的记忆。
//
// 条件卡得比 memory_review_due 更紧是有意的：有证据但过期的那些该重新 keel verify，
// 不该归档——两者出路不同，合成一条就等于把「再验一次」和「不再算数」混为一谈。
//
// 这里只给候选。keel 不会自己归档任何东西：时间不是证据（design.md §12 M5）。
func archiveCandidates(r *Report, set *store.Set, now time.Time) {
	for _, m := range set.Memories {
		if m.Status.IsHistory() || m.Kind == store.MemKindCounterexample {
			continue
		}
		if m.ReviewAfter == nil || m.ReviewAfter.IsZero() || !m.ReviewAfter.Time.Before(now) {
			continue
		}
		if len(set.EvidenceFor(m.ID)) > 0 {
			continue
		}
		r.Candidates = append(r.Candidates, Candidate{
			Code: CodeMemoryArchive, Subject: m.ID.String(),
			Reason: fmt.Sprintf("「%s」复查日期 %s 已过，且从来没有过证据",
				m.Summary, m.ReviewAfter),
			Next: fmt.Sprintf("还成立就 keel verify 补证据；不成立就 keel archive %s --reason \"…\"",
				m.ID.Short()),
		})
	}
}

// indexPressure 在现行对象超过软上限时把归档和提炼的优先级顶上来。
//
// 它只出现在 review 里，不进 check，更不进 pre-commit。现行对象太多不是错误，
// 是体检结果；做成门禁会产生「为了过门禁而删记忆」的反向激励。
func indexPressure(r *Report, cfg store.Config) {
	limit := cfg.Knowledge.IndexSoftLimit
	if limit <= 0 || r.Stats.Current <= limit {
		return
	}
	r.Candidates = append(r.Candidates, Candidate{
		Code: CodeIndexPressure,
		Reason: fmt.Sprintf("现行对象 %d 条，超过 knowledge.index_soft_limit（%d）；历史 %d 条不计在内",
			r.Stats.Current, limit, r.Stats.History),
		Next: "看本报告里的 memory_archive_candidate 和 memory_cluster：要么归档一批，要么把一簇记忆提炼成规则",
	})
}
