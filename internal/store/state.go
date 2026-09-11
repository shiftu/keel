package store

import (
	"fmt"
	"sort"
	"strings"
)

// 决策状态。状态机见 docs/design/formats.md §3。
type DecisionStatus string

const (
	DecProposed   DecisionStatus = "proposed"
	DecAccepted   DecisionStatus = "accepted"
	DecProven     DecisionStatus = "proven"
	DecRevisit    DecisionStatus = "revisit"
	DecSuperseded DecisionStatus = "superseded"
	DecRejected   DecisionStatus = "rejected"
)

var decisionTransitions = map[DecisionStatus][]DecisionStatus{
	DecProposed:   {DecAccepted, DecRejected},
	DecAccepted:   {DecProven, DecRevisit, DecSuperseded},
	DecProven:     {DecRevisit, DecSuperseded},
	DecRevisit:    {DecAccepted, DecSuperseded},
	DecSuperseded: nil,
	DecRejected:   nil,
}

// IsActive 报告该状态是否属于「有效结论」：参与 brief、默认 why、门禁覆盖判断。
func (s DecisionStatus) IsActive() bool {
	return s == DecAccepted || s == DecProven || s == DecRevisit
}

// IsHistory 报告该状态是否「不再声称适用」：退出知识索引主表，why 要 --history 才列出。
//
// proposed 不算历史——它是待办。一个正在等人拍板的方案比大多数已生效结论更该被看见。
func (s DecisionStatus) IsHistory() bool { return s == DecSuperseded || s == DecRejected }

// 规则状态。新增可执行规则默认 candidate，只有 active 才会被执行。
type RuleStatus string

const (
	RuleCandidate RuleStatus = "candidate"
	RuleActive    RuleStatus = "active"
	RuleRetired   RuleStatus = "retired"
)

var ruleTransitions = map[RuleStatus][]RuleStatus{
	RuleCandidate: {RuleActive, RuleRetired},
	RuleActive:    {RuleRetired},
	RuleRetired:   {RuleActive}, // 回归时恢复旧规则
}

// IsHistory 同 DecisionStatus.IsHistory。retired 不是终态（回归时可恢复），
// 但它当下不声称适用，所以按历史呈现。
func (s RuleStatus) IsHistory() bool { return s == RuleRetired }

// 记忆状态。未验证的记录可检索但不得以命令式口吻注入。
type MemoryStatus string

const (
	MemCandidate MemoryStatus = "candidate"
	MemVerified  MemoryStatus = "verified"
	MemDisputed  MemoryStatus = "disputed"
	MemStale     MemoryStatus = "stale"
	MemArchived  MemoryStatus = "archived"
)

var memoryTransitions = map[MemoryStatus][]MemoryStatus{
	MemCandidate: {MemVerified, MemDisputed, MemStale, MemArchived},
	MemVerified:  {MemDisputed, MemStale, MemArchived},
	MemDisputed:  {MemVerified, MemStale, MemArchived},
	MemStale:     {MemVerified, MemArchived},
	MemArchived:  nil,
}

// IsRecallable 报告该记忆是否进入检索结果（archived 只在 --history 出现）。
func (s MemoryStatus) IsRecallable() bool { return s != MemArchived }

// IsHistory 同 DecisionStatus.IsHistory。
//
// stale 和 disputed 不算历史：它们是提醒（「这条过期了」「这条被反驳了」），
// 把提醒藏进历史区等于替人做了「不用管」的判断。
func (s MemoryStatus) IsHistory() bool { return s == MemArchived }

type transitionTable[T ~string] map[T][]T

func checkTransition[T ~string](tbl transitionTable[T], kind string, from, to T) error {
	if from == to {
		return nil
	}
	allowed, known := tbl[from]
	if !known {
		return fmt.Errorf("%s 状态 %q 未定义", kind, string(from))
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	if len(allowed) == 0 {
		return fmt.Errorf("%s 状态 %q 是终态，不能转到 %q", kind, string(from), string(to))
	}
	names := make([]string, 0, len(allowed))
	for _, a := range allowed {
		names = append(names, string(a))
	}
	sort.Strings(names)
	return fmt.Errorf("%s 不允许从 %q 转到 %q（可转：%s）", kind, string(from), string(to), strings.Join(names, "、"))
}

// CanTransitionDecision 校验决策状态转换是否合法。
func CanTransitionDecision(from, to DecisionStatus) error {
	return checkTransition(transitionTable[DecisionStatus](decisionTransitions), "决策", from, to)
}

// CanTransitionRule 校验规则状态转换是否合法。
func CanTransitionRule(from, to RuleStatus) error {
	return checkTransition(transitionTable[RuleStatus](ruleTransitions), "规则", from, to)
}

// CanTransitionMemory 校验记忆状态转换是否合法。
func CanTransitionMemory(from, to MemoryStatus) error {
	return checkTransition(transitionTable[MemoryStatus](memoryTransitions), "记忆", from, to)
}

func validDecisionStatus(s DecisionStatus) bool { _, ok := decisionTransitions[s]; return ok }
func validRuleStatus(s RuleStatus) bool         { _, ok := ruleTransitions[s]; return ok }
func validMemoryStatus(s MemoryStatus) bool     { _, ok := memoryTransitions[s]; return ok }
