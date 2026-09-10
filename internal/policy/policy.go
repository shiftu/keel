// Package policy 计算工作流建议上限。
//
// 这是「建议」而不是授权：keel 不能授予宿主沙箱、网络或发布权限，
// 也不按 proven 计数自动扩大范围。tag 只服务检索，不参与本计算。
package policy

import (
	"fmt"
	"sort"

	"github.com/shiftu/keel/internal/store"
)

// Level 是工作流建议级别。
type Level int

const (
	// LevelAsk 先问人。
	LevelAsk Level = 0
	// LevelPrecedent 有适用先例且无失效证据时才自决。
	LevelPrecedent Level = 1
	// LevelAutonomous 在上限内自决并记录。
	LevelAutonomous Level = 2
)

func (l Level) String() string {
	switch l {
	case LevelAsk:
		return "0 先问人"
	case LevelPrecedent:
		return "1 有适用先例才自决"
	case LevelAutonomous:
		return "2 在上限内自决并记录"
	}
	return fmt.Sprintf("%d", int(l))
}

// Reason 说明某一项约束为什么把上限压到这个值。
type Reason struct {
	Source string // "default" | "action" | "path"
	Key    string
	Level  Level
}

// Decision 是一次策略计算的结果。
type Decision struct {
	Level   Level
	Reasons []Reason
	// Binding 是真正决定了结果的那些约束（等于 Level 的项）。
	Binding []Reason
}

// Query 描述要做的事：一个动作类别和若干相关路径。
type Query struct {
	Action string
	Paths  []string
}

// Evaluate 取 default、命中的 action、命中的 path 三者中最严格的一项。
// 多项命中取更严；低风险标签不能抵消路径或动作上的限制。
func Evaluate(cfg store.WorkflowConfig, q Query) Decision {
	d := Decision{Level: Level(cfg.DefaultLevel)}
	d.Reasons = append(d.Reasons, Reason{Source: "default", Key: "workflow.default_level", Level: Level(cfg.DefaultLevel)})

	if q.Action != "" {
		if lv, ok := cfg.Actions[q.Action]; ok {
			d.Reasons = append(d.Reasons, Reason{Source: "action", Key: q.Action, Level: Level(lv)})
			if Level(lv) < d.Level {
				d.Level = Level(lv)
			}
		}
	}

	patterns := make([]string, 0, len(cfg.Paths))
	for p := range cfg.Paths {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	for _, pat := range patterns {
		lv := Level(cfg.Paths[pat])
		hit := false
		for _, p := range q.Paths {
			if store.MatchGlob(pat, p) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		d.Reasons = append(d.Reasons, Reason{Source: "path", Key: pat, Level: lv})
		if lv < d.Level {
			d.Level = lv
		}
	}

	for _, r := range d.Reasons {
		if r.Level == d.Level {
			d.Binding = append(d.Binding, r)
		}
	}
	return d
}

// PrecedentQuery 是级别 1 下判断「有没有可复用先例」的输入。
type PrecedentQuery struct {
	Tags  []string
	Paths []string
}

// Precedent 是一条被认为可复用的先例。
type Precedent struct {
	ID     store.ID
	Title  string
	Status store.DecisionStatus
	// Verified 表示有一条 pass 证据，且它的 subject_digest 对得上决策现在的内容。
	// 手写 proven 不会让它变 true——那是自述，不是验证。
	Verified bool
}

// Precedents 找出适用范围内、状态有效的决策。
//
// 重要：agent 自己把决策改成 proven 不构成独立验证。本函数只回答
// 「有没有覆盖该路径的有效决策」以及「它有没有跑出来的证据」，
// 不回答「能不能因此自决」——上限只由项目策略给出，证据不会把它抬高。
func Precedents(set *store.Set, q PrecedentQuery) []Precedent {
	var out []Precedent
	for _, d := range set.ActiveDecisions() {
		if len(q.Tags) > 0 && !anyTag(d.Tags, q.Tags) {
			continue
		}
		if len(q.Paths) > 0 && !anyScope(d.Scope, q.Paths) {
			continue
		}
		_, verified := set.SupportingEvidence(d)
		out = append(out, Precedent{
			ID:       d.ID,
			Title:    d.Title,
			Status:   d.Status,
			Verified: verified,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}

func anyTag(have, want []string) bool {
	m := make(map[string]bool, len(have))
	for _, t := range have {
		m[t] = true
	}
	for _, t := range want {
		if m[t] {
			return true
		}
	}
	return false
}

func anyScope(scope, paths []string) bool {
	for _, pat := range scope {
		for _, p := range paths {
			if store.MatchGlob(pat, p) {
				return true
			}
		}
	}
	return false
}
