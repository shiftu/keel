package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/store"
)

// CaseResult 是一条对照用例的执行结果。
type CaseResult struct {
	Dir      string            `json:"dir"`
	Expect   string            `json:"expect"`
	Got      check.RuleOutcome `json:"got"`
	ExitCode int               `json:"exit_code"`
	OK       bool              `json:"ok"`
	Output   string            `json:"-"`
}

// PromoteResult 是一次对照验证的完整结果。证据无论过没过都会写下来。
type PromoteResult struct {
	Cases        []CaseResult `json:"cases"`
	OK           bool         `json:"ok"`
	EvidenceID   store.ID     `json:"evidence"`
	EvidencePath string       `json:"evidence_path"`
	Promoted     bool         `json:"promoted"`
}

// ErrNotPromotable 表示还没到能跑对照验证的程度：结构性前置条件没满足。
type ErrNotPromotable struct{ Reason, Fix string }

func (e *ErrNotPromotable) Error() string { return e.Reason }

// PromoteRule 跑规则的对照验证，写证据，通过则把 candidate 转 active。
//
// 前置条件任一不满足就直接拒绝，一条用例都不跑：候选不能一边放宽自己的检查
// 一边宣告通过，也不能在没有决策依据的情况下变成拦住所有人的硬规则。
func PromoteRule(st *store.Store, set *store.Set, r *store.Rule) (*PromoteResult, error) {
	if err := promotable(st, set, r); err != nil {
		return nil, err
	}

	timeout := DefaultRuleTimeout
	if r.Check.TimeoutSeconds > 0 {
		timeout = time.Duration(r.Check.TimeoutSeconds) * time.Second
	}

	out := &PromoteResult{OK: true}
	for _, c := range r.Cases {
		dir := filepath.Join(st.Root, filepath.FromSlash(c.Dir))
		cr := CaseResult{Dir: c.Dir, Expect: c.Expect}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			cr.Got = check.OutcomeError
			cr.ExitCode = -1
			cr.Output = "夹具目录不存在：" + c.Dir
		} else {
			run := check.RunRule(dir, caseArgv(st.Root, r.Check.Argv), timeout)
			cr.Got, cr.ExitCode, cr.Output = run.Outcome, run.ExitCode, run.Output
		}
		// 只有恰好等于预期才算过。error / timeout 永远不算——跑不起来不是任何结论。
		cr.OK = string(cr.Got) == cr.Expect
		if !cr.OK {
			out.OK = false
		}
		out.Cases = append(out.Cases, cr)
	}

	if err := writePromoteEvidence(st, r, out); err != nil {
		return nil, err
	}
	if !out.OK {
		return out, nil
	}
	wasCandidate := r.Status == store.RuleCandidate
	if err := activate(st, r, out.EvidenceID); err != nil {
		return nil, fmt.Errorf("对照验证通过但写回规则失败：%w", err)
	}
	out.Promoted = wasCandidate
	return out, nil
}

// DefaultRuleTimeout 与规则执行的默认超时一致。
const DefaultRuleTimeout = check.DefaultTimeout

func promotable(st *store.Store, set *store.Set, r *store.Rule) error {
	switch r.Status {
	case store.RuleCandidate:
	case store.RuleActive:
		// 已生效但定义改过：那是重新验证，不是状态变更，应该允许。
		if store.RuleEvidenceCurrent(st.Root, set, r) {
			return &ErrNotPromotable{
				Reason: "规则已经生效，且现有证据对得上它现在的样子",
				Fix:    "改了 argv、对照用例、正文或检查脚本之后再跑 promote；要撤下用 keel retire",
			}
		}
	default:
		return &ErrNotPromotable{
			Reason: fmt.Sprintf("规则状态是 %s", r.Status),
			Fix:    "retired 的规则不能直接 promote；确认要恢复就先改回 candidate",
		}
	}
	if r.Check == nil || len(r.Check.Argv) == 0 {
		return &ErrNotPromotable{
			Reason: "规则没有 check.argv",
			Fix:    "没有可执行检查的是软规则，靠人和 agent 遵守，不走 promote",
		}
	}
	if r.From == nil {
		return &ErrNotPromotable{
			Reason: "规则没有 from（依据决策）",
			Fix:    "会拦住所有人提交的硬规则必须有一条说明「为什么」的 accepted 决策做依据",
		}
	}
	d, ok := set.DecisionByID(*r.From)
	if !ok {
		return &ErrNotPromotable{
			Reason: fmt.Sprintf("依据决策 %s 不存在", r.From.Short()),
			Fix:    "修正 from，或先建立依据决策",
		}
	}
	if !d.Status.IsActive() {
		return &ErrNotPromotable{
			Reason: fmt.Sprintf("依据决策 %s 已经是 %s", d.ID.Short(), d.Status),
			Fix:    "先让依据成立：用 accepted 决策替代它，再 promote",
		}
	}
	if !r.CasesComplete() {
		return &ErrNotPromotable{
			Reason: "cases 缺少 pass 或 fail 方向",
			Fix:    "两个方向都要有。只证明「该过的过了」不算对照验证——一条永远 exit 0 的检查也能满足",
		}
	}
	return nil
}

// caseArgv 让同一条规则能指着夹具目录跑：argv[0] 是仓库内的相对路径时按仓库根解析，
// 其余参数原样传。工作目录由调用方设成夹具目录。
func caseArgv(root string, argv []string) []string {
	a0 := argv[0]
	if filepath.IsAbs(a0) || !strings.Contains(a0, "/") {
		return argv
	}
	p := filepath.Join(root, filepath.FromSlash(a0))
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return argv
	}
	return append([]string{p}, argv[1:]...)
}

func writePromoteEvidence(st *store.Store, r *store.Rule, out *PromoteResult) error {
	id, err := store.NewID(store.KindEvidence)
	if err != nil {
		return err
	}
	result := "pass"
	if !out.OK {
		result = "fail"
	}
	commit, dirty := targetCommit(st.Root)
	contentDigest, inputs, err := store.ScopeDigest(st.Root, caseDirs(r))
	if err != nil {
		return err
	}
	fm := store.EvidenceFM{
		Schema:        store.SchemaVersion,
		ID:            id,
		Subject:       r.ID,
		SubjectDigest: store.ObjectDigest(r),
		Kind:          "check",
		Trust:         "local",
		Target: store.EvidenceTarget{
			Commit:        commit,
			ContentDigest: contentDigest,
		},
		Verifier: store.EvidenceVerifier{
			ID:               r.ID.String(),
			DefinitionDigest: store.RuleDefinitionDigest(st.Root, r),
			Command:          r.Check.Argv,
		},
		Result:     result,
		ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Producer:   "keel-promote",
	}
	if dirty {
		fm.Target.DirtyInputs = inputs
	}
	path, err := st.CreateObject(id, "", fm, promoteBody(r, out))
	if err != nil {
		return err
	}
	out.EvidenceID = id
	out.EvidencePath = path
	return nil
}

func caseDirs(r *store.Rule) []string {
	var out []string
	for _, c := range r.Cases {
		out = append(out, strings.TrimSuffix(c.Dir, "/")+"/**")
	}
	return out
}

func promoteBody(r *store.Rule, out *PromoteResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "对照验证：%s %s\n\n", r.ID, r.Title)
	fmt.Fprintf(&sb, "| 夹具 | 期望 | 实际 | 退出码 | |\n|---|---|---|---|---|\n")
	for _, c := range out.Cases {
		mark := "✘"
		if c.OK {
			mark = "✔"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %d | %s |\n", c.Dir, c.Expect, c.Got, c.ExitCode, mark)
	}
	if out.OK {
		sb.WriteString("\n每个方向都符合预期。\n")
	} else {
		sb.WriteString("\n有用例不符合预期，规则留在 candidate。\n")
		for _, c := range out.Cases {
			if c.OK || strings.TrimSpace(c.Output) == "" {
				continue
			}
			fmt.Fprintf(&sb, "\n%s：\n```\n%s\n```\n", c.Dir, trimOutput(c.Output))
		}
	}
	return sb.String()
}

func trimOutput(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > maxEvidenceOutputLines {
		lines = append(lines[:maxEvidenceOutputLines],
			fmt.Sprintf("…（还有 %d 行未记录）", len(lines)-maxEvidenceOutputLines))
	}
	return strings.Join(lines, "\n")
}

func activate(st *store.Store, r *store.Rule, evID store.ID) error {
	fm := r.RuleFM
	fm.Evidence = appendID(fm.Evidence, evID)
	if fm.Status != store.RuleActive {
		if err := store.CanTransitionRule(fm.Status, store.RuleActive); err != nil {
			return err
		}
		fm.Status = store.RuleActive
	}
	return st.ReplaceObject(r.SourcePath(), fm, r.Body())
}
