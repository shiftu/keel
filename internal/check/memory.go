package check

import (
	"fmt"
	"time"

	"github.com/shiftu/keel/internal/store"
)

// memoryLifecycle 推导记忆的可信度，但**不改任何文件**。
//
// 状态转换只有两个入口：keel verify 真的跑了一次验证器，或者人手工编辑。
// check 在这里只回答「文件自称的状态，现有证据撑不撑得住」。
func memoryLifecycle(st *store.Store, set *store.Set, res *Result) {
	today := time.Now()
	digests := store.NewDigester(st.Root, store.MemoryScopes(set))

	for _, m := range set.Memories {
		if m.Status == store.MemArchived {
			continue
		}
		if m.Status == store.MemVerified {
			verifiedMemory(m, set, digests, res)
		}
		if m.ReviewAfter != nil && !m.ReviewAfter.IsZero() && m.ReviewAfter.Time.Before(today) {
			switch m.Status {
			case store.MemCandidate, store.MemVerified:
				res.Add(Finding{
					Code:     CodeMemoryReviewDue,
					Severity: SeverityWarn,
					Path:     m.SourcePath(),
					Object:   m.ID.String(),
					Message:  fmt.Sprintf("复查日期 %s 已过，状态仍是 %s", m.ReviewAfter, m.Status),
					Fix:      "重新验证（keel verify）、改写、或归档；过期不等于错，但不该继续当现行结论",
				})
			}
		}
	}
	counterexampleConflicts(set, res)
}

func verifiedMemory(m *store.Memory, set *store.Set, digests *store.Digester, res *Result) {
	t := store.TrustOf(set, m, digests)
	if !t.Supported {
		res.Add(Finding{
			Code:    CodeMemoryStatusUnsupported,
			Path:    m.SourcePath(),
			Object:  m.ID.String(),
			Message: "状态是 verified，但没有一条 pass 证据的 subject_digest 对得上当前内容",
			Fix:     "跑 keel verify 重新验证；或把状态改回 candidate。自称 verified 会让它被当成项目规则用",
		})
		return
	}
	if !t.ContentStale {
		return
	}
	res.Add(Finding{
		Code:     CodeMemoryEvidenceStale,
		Severity: SeverityWarn,
		Path:     m.SourcePath(),
		Object:   m.ID.String(),
		Message:  fmt.Sprintf("证据 %s 验证的是当时的 scope 内容，之后代码变过了", t.Evidence.ID.Short()),
		Fix:      "重新跑一次 keel verify；在那之前它不作为现行结论呈现",
	})
}

// counterexampleConflicts 报告「反例指向的结论仍然是 verified」。
// 两边都留着，由人判断谁对——keel 不替谁做主。
func counterexampleConflicts(set *store.Set, res *Result) {
	for _, m := range set.Memories {
		if m.Kind != store.MemKindCounterexample || m.Status == store.MemArchived {
			continue
		}
		for _, id := range m.DerivedFrom {
			target, ok := set.MemoryByID(id)
			if !ok || target.Status != store.MemVerified {
				continue
			}
			res.Add(Finding{
				Code:     CodeMemoryConflict,
				Severity: SeverityWarn,
				Path:     m.SourcePath(),
				Object:   m.ID.String(),
				Message: fmt.Sprintf("反例 %s 指向的 %s 仍是 verified",
					m.ID.Short(), target.ID.Short()),
				Fix: "重新验证被反驳的那条，或把它改成 disputed / archived",
			})
		}
	}
}
