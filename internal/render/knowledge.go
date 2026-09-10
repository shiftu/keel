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
func KnowledgeIndex(set *store.Set) string {
	var sb strings.Builder
	sb.WriteString("# 知识索引\n\n")
	sb.WriteString("由 `keel sync` 生成，勿手改；改 `.keel/` 下的源对象。\n")
	sb.WriteString("状态含义与验证方式见 `docs/design/formats.md`。\n")

	decisionTable(&sb, set)
	ruleTable(&sb, set)
	memoryTable(&sb, set)
	return sb.String()
}

func decisionTable(sb *strings.Builder, set *store.Set) {
	rows := append([]*store.Decision{}, set.Decisions...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID.String() < rows[j].ID.String() })
	sb.WriteString("\n## 决策\n\n")
	if len(rows) == 0 {
		sb.WriteString("（还没有记录任何决策。）\n")
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

func ruleTable(sb *strings.Builder, set *store.Set) {
	rows := append([]*store.Rule{}, set.Rules...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID.String() < rows[j].ID.String() })
	sb.WriteString("\n## 规则\n\n")
	if len(rows) == 0 {
		sb.WriteString("（还没有规则。）\n")
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

func memoryTable(sb *strings.Builder, set *store.Set) {
	rows := append([]*store.Memory{}, set.Memories...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID.String() < rows[j].ID.String() })
	sb.WriteString("\n## 记忆\n\n")
	if len(rows) == 0 {
		sb.WriteString("（还没有记忆。）\n")
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
