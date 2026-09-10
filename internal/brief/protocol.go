package brief

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/store"
)

// Protocol 生成 CLAUDE.md / AGENTS.md 里的稳定协议块。
//
// 只放入口、验证命令和工作流上限。**不放会过期的 proven 计数表**：
// 静态文件里的动态状态在下一次 note/decide 之后就是错的，
// 动态状态一律从 keel brief 查。
func Protocol(cfg store.Config) string {
	var b strings.Builder
	b.WriteString("## keel 协议（由 keel sync 生成，勿手改；改 .keel/）\n\n")
	b.WriteString("本仓库用 keel 保存项目目标、开发约定、任务经验和验证证据。\n\n")
	b.WriteString("1. 目标与约束见 `.keel/intent.md`。\n")
	b.WriteString("2. 改相关路径前：`keel why --path <path>`；要任务上下文：`keel brief --task \"…\" --path <path>`。\n")
	b.WriteString("3. 这四类动作要记录决策或引用已有决策的完整 ID：加/换依赖、跨两个以上模块的结构选择、" +
		"改公开接口或数据格式、改构建或部署方式。空 ADR 和不相关 ID 不算覆盖。\n")
	b.WriteString("4. 能不能自决看 `keel brief` 里的工作流建议上限。上限由项目策略给出，**不看 tag**；" +
		"扩大上限走配置审查，不按记录条数自动升级。你自己把决策改成 proven 不算独立验证。\n")
	b.WriteString("5. 提交时用完整 trailer：`Decision: D-<uuid>`。短前缀只能在命令行输入时用。\n")
	b.WriteString("6. 有价值的经验用 `keel note` 记成 candidate；没有值得记的就不记。" +
		"未验证的记忆不要当成项目规则来执行。\n")
	b.WriteString("\n验证：`keel check --target index`（提交前）· `keel check --target worktree`（随时诊断）\n")

	if lv := cfg.Workflow.DefaultLevel; true {
		b.WriteString("\n工作流默认上限：" + levelWord(lv) + "。")
		if strict := strictActions(cfg.Workflow.Actions); len(strict) > 0 {
			b.WriteString("以下动作更严：" + strings.Join(strict, "、") + "。")
		}
		b.WriteString("具体到某次任务的有效上限以 `keel brief` 为准。\n")
	}
	return b.String()
}

func levelWord(l int) string {
	switch l {
	case 0:
		return "0（先问人）"
	case 1:
		return "1（有适用先例才自决）"
	default:
		return "2（在上限内自决并记录）"
	}
}

func strictActions(m map[string]int) []string {
	var out []string
	for k, v := range m {
		if v == 0 {
			out = append(out, fmt.Sprintf("%s→0", k))
		}
	}
	sort.Strings(out)
	return out
}

// SoftRules 汇总非执行的规则摘要，供 .claude/rules/keel.md 与 AGENTS.md 使用。
func SoftRules(set *store.Set) string {
	var soft []*store.Rule
	for _, r := range set.Rules {
		if r.Status == store.RuleActive && r.Check == nil {
			soft = append(soft, r)
		}
	}
	if len(soft) == 0 {
		return ""
	}
	sort.Slice(soft, func(i, j int) bool { return soft[i].ID.String() < soft[j].ID.String() })
	var b strings.Builder
	b.WriteString("## keel 软规则（无自动检查，靠人和 agent 遵守）\n\n")
	for _, r := range soft {
		fmt.Fprintf(&b, "- **%s** %s\n", r.ID.Short(), r.Title)
		if first := firstParagraph(r.Body()); first != "" {
			fmt.Fprintf(&b, "  %s\n", first)
		}
	}
	return b.String()
}

func firstParagraph(body string) string {
	for _, para := range strings.Split(strings.TrimSpace(body), "\n\n") {
		p := strings.TrimSpace(para)
		if p != "" && !strings.HasPrefix(p, "#") {
			return strings.ReplaceAll(p, "\n", " ")
		}
	}
	return ""
}
