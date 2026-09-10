package brief

import (
	"fmt"
	"strings"
)

// Text 渲染成给 agent 看的正文，并受 UTF-8 字节上限硬约束。
// 超预算时显示省略摘要和原文位置，不静默丢掉。
func (b *Brief) Text(maxBytes int) string {
	var sb strings.Builder
	sb.WriteString("# keel 上下文\n\n")
	fmt.Fprintf(&sb, "工作流建议上限：%s\n", b.Workflow.Meaning)
	if len(b.Workflow.Binding) > 0 {
		fmt.Fprintf(&sb, "  受这些约束限制：%s\n", strings.Join(b.Workflow.Binding, "；"))
	}
	sb.WriteString("  上限由项目策略给出，不看 tag；扩大上限走配置审查，不按记录条数自动升级。\n")
	if b.HasIntent {
		sb.WriteString("\n项目目标与约束见 .keel/intent.md\n")
	}

	section(&sb, "相关决策", b.Decisions)
	section(&sb, "生效中的规则", b.Rules)
	section(&sb, "相关记忆", b.Memories)

	if len(b.Omitted) > 0 {
		sb.WriteString("\n## 省略\n")
		for _, o := range b.Omitted {
			fmt.Fprintf(&sb, "- %s\n", o)
		}
	}
	if len(b.Notes) > 0 {
		sb.WriteString("\n## 说明\n")
		for _, n := range b.Notes {
			fmt.Fprintf(&sb, "- %s\n", n)
		}
	}
	sb.WriteString("\n查询：keel why --path <路径> / --query <关键词>；" +
		"记决策：keel decide；记经验：keel note；验证：keel check --target index\n")

	out := sb.String()
	if maxBytes <= 0 || len(out) <= maxBytes {
		return out
	}
	return truncateUTF8(out, maxBytes)
}

func section(sb *strings.Builder, title string, items []Item) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "\n## %s\n", title)
	for _, it := range items {
		fmt.Fprintf(sb, "- %s [%s] %s\n  %s · %s\n",
			shortID(it.ID), it.Status, it.Title, it.WhySelected, it.Path)
	}
}

func shortID(id string) string {
	if len(id) >= 10 {
		return id[:10]
	}
	return id
}

// truncateUTF8 在字节上限内截断，且不切碎多字节字符。
func truncateUTF8(s string, max int) string {
	const tail = "\n\n（上下文超出预算，已截断。完整内容见 .keel/，或缩小 --path 范围重查。）\n"
	budget := max - len(tail)
	if budget < 0 {
		budget = 0
	}
	cut := budget
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	if cut > len(s) {
		cut = len(s)
	}
	return s[:cut] + tail
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
