package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/store"
	"github.com/shiftu/keel/internal/verify"
)

func cmdPromote(e *env, args []string) error {
	fs := newFlagSet("promote")
	jsonOut := commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := exactlyArgs("promote", rest, 1); err != nil {
		return usagef("用法：keel promote <R-…>")
	}

	st, set, r, err := loadRule(e, rest[0])
	if err != nil {
		return err
	}

	out, err := verify.PromoteRule(st, set, r)
	if err != nil {
		var np *verify.ErrNotPromotable
		if errors.As(err, &np) {
			// 结构性前置条件没满足：一条用例都没跑，也没有证据可写。
			return usagef("%s 还不能 promote：%s\n  › %s", r.ID.Short(), np.Reason, np.Fix)
		}
		return err
	}

	if *jsonOut {
		data, jerr := json.MarshalIndent(out, "", "  ")
		if jerr != nil {
			return jerr
		}
		e.io.Println(string(data))
	} else {
		e.io.Println(out.EvidencePath)
		if !e.quiet {
			for _, c := range out.Cases {
				mark := "✘"
				if c.OK {
					mark = "✔"
				}
				e.io.Errf("%s %s 期望 %s，实际 %s（退出码 %d）\n", mark, c.Dir, c.Expect, c.Got, c.ExitCode)
			}
			switch {
			case out.Promoted:
				e.io.Errf("%s：candidate → active\n", r.ID.Short())
			case out.OK:
				e.io.Errf("%s 状态不变（本来就是 active），证据已更新。\n", r.ID.Short())
			default:
				e.io.Errf("%s 留在 candidate。失败证据已经写下来，下次 keel review 看得到。\n", r.ID.Short())
			}
		}
	}
	if !out.OK {
		return checkFailed
	}
	return nil
}

func cmdRetire(e *env, args []string) error {
	fs := newFlagSet("retire")
	jsonOut := commonFlags(fs, e)
	reason := fs.String("reason", "", "为什么撤回；会写进规则正文")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := exactlyArgs("retire", rest, 1); err != nil {
		return usagef("用法：keel retire <R-…> --reason \"<原因>\"")
	}
	if strings.TrimSpace(*reason) == "" {
		return usagef("retire 需要 --reason：撤回原因留在正文里，下一个人才知道这里发生过什么")
	}

	st, set, r, err := loadRule(e, rest[0])
	if err != nil {
		return err
	}
	if r.Status == store.RuleRetired {
		return usagef("%s 已经是 retired", r.ID.Short())
	}
	if err := store.CanTransitionRule(r.Status, store.RuleRetired); err != nil {
		return err
	}

	fm := r.RuleFM
	fm.Status = store.RuleRetired
	body := strings.TrimRight(r.Body(), "\n") +
		fmt.Sprintf("\n\n## 撤回\n\n%s：%s\n", store.NewDate(time.Now()), strings.TrimSpace(*reason))
	if err := st.ReplaceObject(r.SourcePath(), fm, body); err != nil {
		return err
	}

	restore := supersededPredecessors(set, r.ID)
	if *jsonOut {
		data, jerr := json.MarshalIndent(map[string]any{
			"schema": 1, "rule": r.ID.String(), "status": string(store.RuleRetired),
			"consider_restoring": restore,
		}, "", "  ")
		if jerr != nil {
			return jerr
		}
		e.io.Println(string(data))
		return nil
	}
	e.io.Println(r.SourcePath())
	if e.quiet {
		return nil
	}
	e.io.Errf("%s：%s → retired。撤回原因已写进正文，历史保留。\n", r.ID.Short(), r.Status)
	for _, old := range restore {
		e.io.Errf("提示：%s 曾被它替代，现在可以考虑恢复。keel 不替你改。\n", old)
	}
	return nil
}

// supersededPredecessors 找出「被这条规则替代掉」的旧规则：
// 某条决策的 rule_migration 里 to 指向它。撤回它之后，那些旧规则可能需要恢复。
func supersededPredecessors(set *store.Set, id store.ID) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range set.Decisions {
		for _, m := range d.RuleMigration {
			if m.To == nil || *m.To != id {
				continue
			}
			s := m.From.Short()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func loadRule(e *env, ref string) (*store.Store, *store.Set, *store.Rule, error) {
	st, err := e.discover()
	if err != nil {
		return nil, nil, nil, err
	}
	set, err := st.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	id, err := store.ParseRef(ref, set.IDs())
	if err != nil {
		return nil, nil, nil, usagef("%v", err)
	}
	if id.Kind != store.KindRule {
		return nil, nil, nil, usagef("需要一个规则 ID（R-…），%s 是 %s", id.Short(), id.Kind)
	}
	o, ok := set.Lookup(id)
	if !ok {
		return nil, nil, nil, usagef("找不到 %s", id)
	}
	return st, set, o.(*store.Rule), nil
}
