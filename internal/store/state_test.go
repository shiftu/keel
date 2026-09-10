package store

import "testing"

func TestDecisionTransitions(t *testing.T) {
	ok := [][2]DecisionStatus{
		{DecProposed, DecAccepted},
		{DecProposed, DecRejected},
		{DecAccepted, DecProven},
		{DecAccepted, DecRevisit},
		{DecAccepted, DecSuperseded},
		{DecProven, DecRevisit},
		{DecProven, DecSuperseded},
		{DecRevisit, DecAccepted},
		{DecRevisit, DecSuperseded},
	}
	for _, c := range ok {
		if err := CanTransitionDecision(c[0], c[1]); err != nil {
			t.Errorf("%s → %s 应该允许：%v", c[0], c[1], err)
		}
	}
	bad := [][2]DecisionStatus{
		// proposed 不能直接成为结论，也不能直接撤销别人。
		{DecProposed, DecProven},
		{DecProposed, DecSuperseded},
		{DecRejected, DecAccepted},
		{DecSuperseded, DecAccepted},
	}
	for _, c := range bad {
		if err := CanTransitionDecision(c[0], c[1]); err == nil {
			t.Errorf("%s → %s 应该被拒绝", c[0], c[1])
		}
	}
}

func TestActiveStatuses(t *testing.T) {
	for _, s := range []DecisionStatus{DecAccepted, DecProven, DecRevisit} {
		if !s.IsActive() {
			t.Errorf("%s 应该算有效结论", s)
		}
	}
	for _, s := range []DecisionStatus{DecProposed, DecRejected, DecSuperseded} {
		if s.IsActive() {
			t.Errorf("%s 不该算有效结论", s)
		}
	}
}

func TestRuleAndMemoryTransitions(t *testing.T) {
	if err := CanTransitionRule(RuleCandidate, RuleActive); err != nil {
		t.Errorf("candidate → active 应该允许：%v", err)
	}
	// 回归时要能恢复旧规则。
	if err := CanTransitionRule(RuleRetired, RuleActive); err != nil {
		t.Errorf("retired → active 应该允许：%v", err)
	}
	if err := CanTransitionMemory(MemArchived, MemVerified); err == nil {
		t.Error("archived 是终态")
	}
	if err := CanTransitionMemory(MemCandidate, MemVerified); err != nil {
		t.Errorf("candidate → verified 应该允许：%v", err)
	}
}
