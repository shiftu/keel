package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/store"
)

// KnowledgeIndexPath 是索引产物的位置，进 git。
const KnowledgeIndexPath = store.DirName + "/knowledge/INDEX.md"

// KnowledgeIndex 渲染 clone 之后不装 keel 也能读的入口。
//
// 内容只来自仓库里的对象：不放本机路径、生成时间或探测结果，
// 否则两台机器 sync 出来的文件会不一样，每次都是一个假 diff。
//
// 索引分两层（design.md §3.6）：现行结论出全表，历史结论只出计数。
// 历史 = 状态机里「不再声称适用」的那些，见 store.*Status.IsHistory。
// 它们一条都没删，只是退出主表——索引是视图，不是仓库。
func KnowledgeIndex(set *store.Set, cfg store.Config) string {
	cur, hist := partition(set)

	var sb strings.Builder
	sb.WriteString("# 知识索引\n\n")
	sb.WriteString("由 `keel sync` 生成，勿手改；改 `.keel/` 下的源对象。\n")
	sb.WriteString("状态含义与验证方式见 `docs/design/formats.md`。\n\n")
	fmt.Fprintf(&sb, "现行：决策 %d · 规则 %d · 记忆 %d。历史 %d 条见文末。\n",
		len(cur.decisions), len(cur.rules), len(cur.memories), hist.total())

	decisionTable(&sb, set, cur.decisions)
	ruleTable(&sb, set, cur.rules)
	memoryTable(&sb, set, cur.memories)
	historySection(&sb, hist, cfg.Knowledge.IndexHistory)
	return sb.String()
}

// buckets 把三类对象按现行/历史分开。两个方向都按 ID 稳定排序。
type buckets struct {
	decisions []*store.Decision
	rules     []*store.Rule
	memories  []*store.Memory
}

func (b buckets) total() int { return len(b.decisions) + len(b.rules) + len(b.memories) }

func partition(set *store.Set) (cur, hist buckets) {
	for _, d := range set.Decisions {
		if d.Status.IsHistory() {
			hist.decisions = append(hist.decisions, d)
		} else {
			cur.decisions = append(cur.decisions, d)
		}
	}
	for _, r := range set.Rules {
		if r.Status.IsHistory() {
			hist.rules = append(hist.rules, r)
		} else {
			cur.rules = append(cur.rules, r)
		}
	}
	for _, m := range set.Memories {
		if m.Status.IsHistory() {
			hist.memories = append(hist.memories, m)
		} else {
			cur.memories = append(cur.memories, m)
		}
	}
	for _, b := range []*buckets{&cur, &hist} {
		sort.Slice(b.decisions, func(i, j int) bool {
			return b.decisions[i].ID.String() < b.decisions[j].ID.String()
		})
		sort.Slice(b.rules, func(i, j int) bool {
			return b.rules[i].ID.String() < b.rules[j].ID.String()
		})
		sort.Slice(b.memories, func(i, j int) bool {
			return b.memories[i].ID.String() < b.memories[j].ID.String()
		})
	}
	return cur, hist
}

func decisionTable(sb *strings.Builder, set *store.Set, rows []*store.Decision) {
	sb.WriteString("\n## 决策\n\n")
	if len(rows) == 0 {
		sb.WriteString("（还没有记录任何现行决策。）\n")
		return
	}
	sb.WriteString("| ID | 状态 | 标题 | scope | 证据 |\n|---|---|---|---|---|\n")
	for _, d := range rows {
		_, verified := set.SupportingEvidence(d)
		fmt.Fprintf(sb, "| %s | %s | %s | %s | %s |\n",
			d.ID.Short(), d.Status, cell(d.Title), cell(strings.Join(d.Scope, "、")),
			evidenceCell(verified, len(set.EvidenceFor(d.ID))))
	}
}

func ruleTable(sb *strings.Builder, set *store.Set, rows []*store.Rule) {
	sb.WriteString("\n## 规则\n\n")
	if len(rows) == 0 {
		sb.WriteString("（还没有现行规则。）\n")
		return
	}
	sb.WriteString("| ID | 状态 | 标题 | scope | 依据 | 对照验证 |\n|---|---|---|---|---|---|\n")
	for _, r := range rows {
		basis := "—"
		if r.From != nil {
			basis = r.From.Short()
		}
		fmt.Fprintf(sb, "| %s | %s | %s | %s | %s | %s |\n",
			r.ID.Short(), r.Status, cell(r.Title), cell(strings.Join(r.Scope, "、")),
			basis, casesCell(r, set))
	}
}

func memoryTable(sb *strings.Builder, set *store.Set, rows []*store.Memory) {
	sb.WriteString("\n## 记忆\n\n")
	if len(rows) == 0 {
		sb.WriteString("（还没有现行记忆。）\n")
		return
	}
	// 这里只比对结论本身有没有变（subject_digest），不做代码内容比对：
	// 索引要在任何机器上渲染出同一份内容，而工作树是会变的。
	sb.WriteString("| ID | 状态 | 摘要 | scope | 验证于 | 证据 |\n|---|---|---|---|---|---|\n")
	for _, m := range rows {
		_, supported := set.SupportingEvidence(m)
		verifiedAt := "—"
		if m.VerifiedAt != nil && !m.VerifiedAt.IsZero() {
			verifiedAt = m.VerifiedAt.String()
		}
		fmt.Fprintf(sb, "| %s | %s | %s | %s | %s | %s |\n",
			m.ID.Short(), m.Status, cell(m.Summary), cell(strings.Join(m.Scope, "、")),
			verifiedAt, evidenceCell(supported, len(set.EvidenceFor(m.ID))))
	}
}

// historySection 是索引不随对象数无限变长的地方：默认只给计数。
//
// 计数表的行数由状态机的状态数封顶（最多 4 行），跟历史对象有多少条无关。
// 要看清单就打开 knowledge.index_history，或者跑 keel why --history。
func historySection(sb *strings.Builder, hist buckets, detail bool) {
	sb.WriteString("\n## 历史\n\n")
	if hist.total() == 0 {
		sb.WriteString("（还没有被替代、撤回或归档的结论。）\n")
		return
	}
	sb.WriteString("不再声称适用的结论。一条都没删，`keel why --history` 查得到。\n\n")
	sb.WriteString("| 类别 | 状态 | 条数 |\n|---|---|---:|\n")
	for _, row := range historyCounts(hist) {
		fmt.Fprintf(sb, "| %s | %s | %d |\n", row.kind, row.status, row.n)
	}
	if !detail {
		sb.WriteString("\n展开清单：`keel.yaml` 里设 `knowledge.index_history: true`。\n")
		return
	}
	historyDetail(sb, hist)
}

type countRow struct {
	kind, status string
	n            int
}

// historyCounts 按固定的状态顺序出行，只出非零的。
// 顺序写死而不是从 map 里排序，是为了让索引的行序不依赖遍历顺序。
func historyCounts(hist buckets) []countRow {
	rows := []countRow{
		{"决策", string(store.DecSuperseded), 0},
		{"决策", string(store.DecRejected), 0},
		{"规则", string(store.RuleRetired), 0},
		{"记忆", string(store.MemArchived), 0},
	}
	for _, d := range hist.decisions {
		for i := range rows {
			if rows[i].kind == "决策" && rows[i].status == string(d.Status) {
				rows[i].n++
			}
		}
	}
	rows[2].n = len(hist.rules)
	rows[3].n = len(hist.memories)

	out := rows[:0:0]
	for _, r := range rows {
		if r.n > 0 {
			out = append(out, r)
		}
	}
	return out
}

func historyDetail(sb *strings.Builder, hist buckets) {
	if len(hist.decisions) > 0 {
		sb.WriteString("\n### 历史 · 决策\n\n")
		sb.WriteString("| ID | 状态 | 标题 | 去向 |\n|---|---|---|---|\n")
		for _, d := range hist.decisions {
			fmt.Fprintf(sb, "| %s | %s | %s | %s |\n",
				d.ID.Short(), d.Status, cell(d.Title), shortIDs(d.SupersededBy))
		}
	}
	if len(hist.rules) > 0 {
		sb.WriteString("\n### 历史 · 规则\n\n")
		sb.WriteString("| ID | 状态 | 标题 | scope |\n|---|---|---|---|\n")
		for _, r := range hist.rules {
			fmt.Fprintf(sb, "| %s | %s | %s | %s |\n",
				r.ID.Short(), r.Status, cell(r.Title), cell(strings.Join(r.Scope, "、")))
		}
	}
	if len(hist.memories) > 0 {
		sb.WriteString("\n### 历史 · 记忆\n\n")
		sb.WriteString("| ID | 状态 | 摘要 | scope |\n|---|---|---|---|\n")
		for _, m := range hist.memories {
			fmt.Fprintf(sb, "| %s | %s | %s | %s |\n",
				m.ID.Short(), m.Status, cell(m.Summary), cell(strings.Join(m.Scope, "、")))
		}
	}
}

func shortIDs(ids []store.ID) string {
	if len(ids) == 0 {
		return "—"
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.Short())
	}
	sort.Strings(out)
	return strings.Join(out, "、")
}

// casesCell 说明这条规则的对照用例齐不齐、跑没跑过。
func casesCell(r *store.Rule, set *store.Set) string {
	if len(r.Cases) == 0 {
		return "无用例"
	}
	state := fmt.Sprintf("pass×%d fail×%d", len(r.CaseDirs("pass")), len(r.CaseDirs("fail")))
	if !r.CasesComplete() {
		return state + "（方向不全）"
	}
	if _, ok := set.SupportingEvidence(r); ok {
		return state + " · 已通过"
	}
	if len(set.EvidenceFor(r.ID)) > 0 {
		return state + " · 证据对不上当前内容"
	}
	return state + " · 未跑过"
}

func evidenceCell(supported bool, total int) string {
	switch {
	case total == 0:
		return "无"
	case supported:
		return fmt.Sprintf("%d 条（有对得上的 pass）", total)
	default:
		return fmt.Sprintf("%d 条（都对不上当前内容）", total)
	}
}

// cell 让内容不会撑破表格：竖线转义，换行压成空格。
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "—"
	}
	return s
}
