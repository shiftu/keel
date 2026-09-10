package check

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/store"
)

// Objects 做与 target 无关的对象层检查：解析、引用完整性、状态一致性、
// 规则依据、scope 冲突与失效。规则执行由 Rules 单独负责。
func Objects(st *store.Store, cfg store.Config, set *store.Set, res *Result) {
	loadErrors(set, res)
	duplicateIDs(st, set, res)
	refIntegrity(set, res)
	supersedeConsistency(set, res)
	ruleBasis(set, res)
	scopeOverlap(set, res)
	staleScope(st, set, res)
	memoryLifecycle(st, set, res)
	configPolicy(st, cfg, res)
}

func loadErrors(set *store.Set, res *Result) {
	for _, e := range set.Errors {
		res.Add(Finding{
			Code:    CodeFrontmatterInvalid,
			Path:    e.Path,
			Message: e.Err.Error(),
			Fix:     "按 docs/design/formats.md 修正 frontmatter 字段",
		})
	}
}

func duplicateIDs(st *store.Store, set *store.Set, res *Result) {
	seen := map[string][]string{}
	for _, o := range allObjects(set) {
		id := o.ObjectID().String()
		seen[id] = append(seen[id], o.SourcePath())
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		paths := seen[id]
		if len(paths) < 2 {
			continue
		}
		sort.Strings(paths)
		res.Add(Finding{
			Code:    CodeIDDuplicate,
			Object:  id,
			Path:    paths[0],
			Message: fmt.Sprintf("ID %s 出现在多个文件：%s", id, strings.Join(paths, "、")),
			Fix:     "canonical ID 永不重编号；删除或重建其中一个对象（新对象用新 ID）",
		})
	}
	_ = st
}

func refIntegrity(set *store.Set, res *Result) {
	for _, o := range allObjects(set) {
		for _, ref := range o.Refs() {
			if ref.ID.IsZero() {
				continue
			}
			if _, ok := set.Lookup(ref.ID); !ok {
				res.Add(Finding{
					Code:    CodeRefMissing,
					Path:    o.SourcePath(),
					Object:  o.ObjectID().String(),
					Message: fmt.Sprintf("%s 引用的 %s 不存在", ref.Field, ref.ID),
					Fix:     "修正引用，或先创建被引用对象",
				})
			}
		}
	}
}

// supersedeConsistency 保证替代关系两端一致，且 proposed 不会提前撤销旧决策。
func supersedeConsistency(set *store.Set, res *Result) {
	for _, d := range set.Decisions {
		for _, oldID := range d.Supersedes {
			old, ok := set.DecisionByID(oldID)
			if !ok {
				continue // 已由 refIntegrity 报告
			}
			if d.Status == store.DecAccepted || d.Status == store.DecProven {
				if old.Status != store.DecSuperseded {
					res.Add(Finding{
						Code:    CodeSupersedeIncomplete,
						Path:    d.SourcePath(),
						Object:  d.ID.String(),
						Message: fmt.Sprintf("%s 已 accepted 并声明替代 %s，但后者状态仍是 %s", d.ID.Short(), old.ID.Short(), old.Status),
						Fix:     "替代必须原子完成：旧决策转 superseded + superseded_by，并填写 rule_migration",
					})
					continue
				}
				if !containsID(old.SupersededBy, d.ID) {
					res.Add(Finding{
						Code:    CodeSupersedeIncomplete,
						Path:    old.SourcePath(),
						Object:  old.ID.String(),
						Message: fmt.Sprintf("%s 已 superseded，但 superseded_by 未指回 %s", old.ID.Short(), d.ID.Short()),
						Fix:     "补齐 superseded_by",
					})
				}
				if len(d.RuleMigration) == 0 && rulesFrom(set, old.ID) > 0 {
					res.Add(Finding{
						Code:    CodeSupersedeIncomplete,
						Path:    d.SourcePath(),
						Object:  d.ID.String(),
						Message: fmt.Sprintf("%s 替代了有派生规则的 %s，但没有填写 rule_migration", d.ID.Short(), old.ID.Short()),
						Fix:     "在 rule_migration 里说明每条旧规则迁移到哪里或 retired",
					})
				}
			}
			// proposed 只记录替代意图：旧决策必须仍然有效。
			if d.Status == store.DecProposed && old.Status == store.DecSuperseded && containsID(old.SupersededBy, d.ID) {
				res.Add(Finding{
					Code:    CodeSupersedeIncomplete,
					Path:    old.SourcePath(),
					Object:  old.ID.String(),
					Message: fmt.Sprintf("%s 被尚未 accepted 的 %s 提前置为 superseded", old.ID.Short(), d.ID.Short()),
					Fix:     "proposed 只写 supersedes 意图；旧决策在新决策 accepted 时才转换",
				})
			}
		}
	}
}

func rulesFrom(set *store.Set, dec store.ID) int {
	n := 0
	for _, r := range set.Rules {
		if r.From != nil && *r.From == dec && r.Status != store.RuleRetired {
			n++
		}
	}
	return n
}

// ruleBasis 报告依据已失效的规则。规则不因此自动 retired。
func ruleBasis(set *store.Set, res *Result) {
	for _, r := range set.Rules {
		if r.From == nil || r.Status == store.RuleRetired {
			continue
		}
		d, ok := set.DecisionByID(*r.From)
		if !ok {
			continue
		}
		if d.Status.IsActive() {
			continue
		}
		res.Add(Finding{
			Code:     CodeRuleBasisInvalid,
			Severity: SeverityWarn,
			Path:     r.SourcePath(),
			Object:   r.ID.String(),
			Message:  fmt.Sprintf("规则依据 %s 已是 %s，但规则仍为 %s", d.ID.Short(), d.Status, r.Status),
			Fix:      "由替代决策说明规则迁移；确认后再改规则状态",
		})
	}
}

// scopeOverlap 只在两条有效决策 scope 重叠且 tag 相交时提示，由人判断是否冲突。
func scopeOverlap(set *store.Set, res *Result) {
	active := set.ActiveDecisions()
	for i := 0; i < len(active); i++ {
		for j := i + 1; j < len(active); j++ {
			a, b := active[i], active[j]
			if !tagsIntersect(a.Tags, b.Tags) {
				continue
			}
			pat, ok := scopeOverlaps(a.Scope, b.Scope)
			if !ok {
				continue
			}
			res.Add(Finding{
				Code:     CodeDecisionScopeOverlap,
				Severity: SeverityWarn,
				Path:     a.SourcePath(),
				Object:   a.ID.String(),
				Message:  fmt.Sprintf("%s 与 %s 的 scope 在 %q 重叠且 tag 相交", a.ID.Short(), b.ID.Short(), pat),
				Fix:      "确认两条决策是否冲突；若是，用 accepted 决策替代其中一条",
			})
		}
	}
}

// scopeOverlaps 判断两组 glob 是否可能覆盖同一片路径。
// 只做保守判断：任一 pattern 互相匹配，或去掉通配后有前缀关系。
func scopeOverlaps(a, b []string) (string, bool) {
	for _, pa := range a {
		for _, pb := range b {
			if pa == pb {
				return pa, true
			}
			if store.MatchGlob(pa, literalPrefix(pb)) || store.MatchGlob(pb, literalPrefix(pa)) {
				return pa, true
			}
		}
	}
	return "", false
}

func literalPrefix(pattern string) string {
	segs := strings.Split(pattern, "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		if strings.ContainsAny(s, "*?") {
			break
		}
		out = append(out, s)
	}
	return strings.Join(out, "/")
}

func tagsIntersect(a, b []string) bool {
	m := make(map[string]bool, len(a))
	for _, t := range a {
		m[t] = true
	}
	for _, t := range b {
		if m[t] {
			return true
		}
	}
	return false
}

// staleScope 报告 scope 指向的路径在工作树里一个都不存在的对象。不删除任何东西。
func staleScope(st *store.Store, set *store.Set, res *Result) {
	report := func(path, id string, scope []string) {
		if len(scope) == 0 {
			return
		}
		for _, pat := range scope {
			if scopeHasMatch(st.Root, pat) {
				return
			}
		}
		res.Add(Finding{
			Code:     CodeObjectStale,
			Severity: SeverityWarn,
			Path:     path,
			Object:   id,
			Message:  fmt.Sprintf("scope %s 在工作树里没有任何匹配路径", strings.Join(scope, "、")),
			Fix:      "重定位 scope 或归档该对象；不要直接删除历史解释",
		})
	}
	for _, d := range set.Decisions {
		if d.Status.IsActive() {
			report(d.SourcePath(), d.ID.String(), d.Scope)
		}
	}
	for _, r := range set.Rules {
		if r.Status == store.RuleActive {
			report(r.SourcePath(), r.ID.String(), r.Scope)
		}
	}
	for _, m := range set.Memories {
		// archived 的记忆本来就不再声称适用，scope 失配不必再提醒。
		if m.Status != store.MemArchived {
			report(m.SourcePath(), m.ID.String(), m.Scope)
		}
	}
}

// scopeHasMatch 判断仓库里是否存在匹配该 glob 的文件或目录。
func scopeHasMatch(root, pattern string) bool {
	lit := literalPrefix(pattern)
	if lit != "" {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(lit))); err == nil {
			return true
		}
		if !strings.ContainsAny(pattern, "*?") {
			return false
		}
	}
	found := false
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "node_modules") {
			return filepath.SkipDir
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || rel == "." {
			return nil
		}
		if store.MatchGlob(pattern, filepath.ToSlash(rel)) {
			found = true
		}
		return nil
	})
	return found
}

// configPolicy 校验策略配置本身。auto_promote 在 MVP 必须关闭：
// 按计数自动扩大自决范围是审查里被否掉的自我确认循环。
func configPolicy(st *store.Store, cfg store.Config, res *Result) {
	if cfg.Workflow.AutoPromote {
		res.Add(Finding{
			Code:    CodeAutoPromoteEnabled,
			Path:    st.Rel(st.ConfigPath()),
			Message: "workflow.auto_promote 必须为 false：keel 不按 proven 计数自动扩大自决范围",
			Fix:     "改成 auto_promote: false；扩大上限走项目既有配置审查",
		})
	}
}

func allObjects(set *store.Set) []store.Object {
	var out []store.Object
	for _, d := range set.Decisions {
		out = append(out, d)
	}
	for _, r := range set.Rules {
		out = append(out, r)
	}
	for _, m := range set.Memories {
		out = append(out, m)
	}
	for _, e := range set.Evidence {
		out = append(out, e)
	}
	return out
}

func containsID(list []store.ID, id store.ID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// Rules 执行 active 硬规则。candidate 与 retired 不执行。
func Rules(st *store.Store, set *store.Set, res *Result) {
	for _, r := range set.Rules {
		if !r.IsExecutable() {
			continue
		}
		timeout := DefaultTimeout
		if r.Check.TimeoutSeconds > 0 {
			timeout = time.Duration(r.Check.TimeoutSeconds) * time.Second
		}
		run := RunRule(st.Root, r.Check.Argv, timeout)
		switch run.Outcome {
		case OutcomePass:
		case OutcomeFail:
			res.Add(Finding{
				Code:     CodeRuleFailed,
				Severity: r.SeverityOrDefault(),
				Path:     r.SourcePath(),
				Object:   r.ID.String(),
				Message:  fmt.Sprintf("规则未通过：%s%s", r.Title, indentOutput(run.Output)),
				Fix:      "修正代码，或用 accepted 决策替代该规则的依据",
			})
		case OutcomeTimeout:
			res.Add(Finding{
				Code:     CodeRuleTimeout,
				Severity: r.SeverityOrDefault(),
				Path:     r.SourcePath(),
				Object:   r.ID.String(),
				Message:  fmt.Sprintf("规则执行超时（%s）：%s", timeout, r.Title),
				Fix:      "缩小检查范围或调大 check.timeout_seconds",
			})
		default:
			res.Add(Finding{
				Code:     CodeRuleError,
				Severity: r.SeverityOrDefault(),
				Path:     r.SourcePath(),
				Object:   r.ID.String(),
				Message: fmt.Sprintf("规则执行出错（退出码 %d）：%s%s",
					run.ExitCode, r.Title, indentOutput(run.Output)),
				Fix: "检查 check.argv 指向的工具与路径是否存在；执行不了不等于通过",
			})
		}
	}
}

func indentOutput(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	if len(lines) > 10 {
		lines = append(lines[:10], "…")
	}
	return "\n    " + strings.Join(lines, "\n    ")
}
