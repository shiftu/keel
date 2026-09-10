// Package verify 产生证据：真的跑一次验证器，把结果写进 evidence/，
// 再按记忆的状态转换表更新被验证的对象。
//
// 这里没有「登记一条我认为它通过了」的入口。自述的通过既不能被别人重跑、
// 也不能被撤回，把它写进仓库等于把自我确认固化成事实。
package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/store"
)

// Request 是一次验证的输入。Argv 一定是 argv，不是 shell 字符串。
type Request struct {
	Subject          store.Object
	VerifierID       string
	DefinitionDigest string
	Argv             []string
	Kind             string
	Trust            string
	Timeout          time.Duration
}

// Outcome 是一次验证的结果。Evidence 一定被写下来，哪怕验证器报错。
type Outcome struct {
	EvidencePath string
	EvidenceID   store.ID
	Run          check.RunResult
	// Transition 形如 "candidate → verified"；没有状态转换时为空。
	Transition string
	// SubjectPath 是被更新的对象文件，没更新时为空。
	SubjectPath string
}

// maxEvidenceOutputLines 限制记进证据正文的输出行数：证据要能读，不是日志转储。
const maxEvidenceOutputLines = 40

// Run 执行一次验证并落盘。
//
// 顺序是「先写证据、再改 subject」，改 subject 失败就删掉刚写的证据：
// 不能留下一条没人认领的证据，也不能留下一个没有依据的 verified。
func Run(st *store.Store, req Request) (*Outcome, error) {
	if len(req.Argv) == 0 {
		return nil, fmt.Errorf("验证器命令为空")
	}
	subj := req.Subject
	scope := subjectScope(subj)

	contentDigest, inputs, err := store.ScopeDigest(st.Root, scope)
	if err != nil {
		return nil, fmt.Errorf("计算 scope 内容摘要：%w", err)
	}

	commit, dirty := targetCommit(st.Root)
	run := check.RunRule(st.Root, req.Argv, req.Timeout)

	id, err := store.NewID(store.KindEvidence)
	if err != nil {
		return nil, err
	}
	fm := store.EvidenceFM{
		Schema:        store.SchemaVersion,
		ID:            id,
		Subject:       subj.ObjectID(),
		SubjectDigest: store.ObjectDigest(subj),
		Kind:          req.Kind,
		Trust:         req.Trust,
		Target: store.EvidenceTarget{
			Commit:        commit,
			ContentDigest: contentDigest,
			DirtyInputs:   nil,
		},
		Verifier: store.EvidenceVerifier{
			ID:               req.VerifierID,
			DefinitionDigest: req.DefinitionDigest,
			Command:          req.Argv,
		},
		Result:     string(run.Outcome),
		ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Producer:   "keel-verify",
	}
	// 工作树是脏的就不能把结果挂在某个提交上，只能列出实际纳入摘要的路径。
	if dirty {
		fm.Target.DirtyInputs = inputs
	}

	body := evidenceBody(subj, run)
	path, err := st.CreateObject(id, "", fm, body)
	if err != nil {
		return nil, err
	}
	out := &Outcome{EvidencePath: path, EvidenceID: id, Run: run}

	if err := applyToSubject(st, subj, id, run.Outcome, out); err != nil {
		_ = os.Remove(filepath.Join(st.Root, filepath.FromSlash(path)))
		return nil, fmt.Errorf("更新 %s 失败，已回滚刚写的证据：%w", subj.ObjectID().Short(), err)
	}
	return out, nil
}

func subjectScope(o store.Object) []string {
	switch v := o.(type) {
	case *store.Memory:
		return v.Scope
	case *store.Decision:
		return v.Scope
	}
	return nil
}

// targetCommit 只在工作树干净时返回 HEAD。拿不准就留空，不猜。
func targetCommit(root string) (commit string, dirty bool) {
	repo, err := gitx.Open(root)
	if err != nil {
		return "", true
	}
	clean, err := repo.IsClean()
	if err != nil || !clean {
		return "", true
	}
	sha, err := repo.HeadSHA()
	if err != nil {
		return "", true
	}
	return sha, false
}

// applyToSubject 按 formats.md §5 的状态转换表更新被验证对象。
//
// 决策只追加证据不改状态：proven 是人的判断，不是跑通一条命令的自动结果。
// error / timeout 不做任何转换——跑不起来既不是通过也不是失败。
func applyToSubject(st *store.Store, subj store.Object, evID store.ID, outcome check.RuleOutcome, out *Outcome) error {
	switch v := subj.(type) {
	case *store.Memory:
		fm := v.MemoryFM
		fm.Evidence = appendID(fm.Evidence, evID)
		from := fm.Status
		switch outcome {
		case check.OutcomePass:
			if fm.Status == store.MemCandidate || fm.Status == store.MemDisputed {
				fm.Status = store.MemVerified
			}
			d := store.NewDate(time.Now())
			fm.VerifiedAt = &d
		case check.OutcomeFail:
			if fm.Status == store.MemCandidate || fm.Status == store.MemVerified {
				fm.Status = store.MemDisputed
			}
			fm.VerifiedAt = nil
		}
		if fm.Status != from {
			if err := store.CanTransitionMemory(from, fm.Status); err != nil {
				return err
			}
			out.Transition = fmt.Sprintf("%s → %s", from, fm.Status)
		}
		out.SubjectPath = v.SourcePath()
		return st.ReplaceObject(v.SourcePath(), fm, v.Body())

	case *store.Decision:
		fm := v.DecisionFM
		fm.Evidence = appendID(fm.Evidence, evID)
		out.SubjectPath = v.SourcePath()
		return st.ReplaceObject(v.SourcePath(), fm, v.Body())
	}
	return fmt.Errorf("只能验证记忆（M-）或决策（D-），拿到的是 %s", subj.ObjectID())
}

func appendID(list []store.ID, id store.ID) []store.ID {
	for _, x := range list {
		if x == id {
			return list
		}
	}
	return append(append([]store.ID{}, list...), id)
}

func evidenceBody(subj store.Object, run check.RunResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "验证对象：%s %s\n\n", subj.ObjectID(), subj.ObjectTitle())
	fmt.Fprintf(&sb, "结果：%s（退出码 %d）\n", run.Outcome, run.ExitCode)
	if run.Err != nil {
		fmt.Fprintf(&sb, "错误：%s\n", run.Err)
	}
	output := strings.TrimSpace(run.Output)
	if output == "" {
		return sb.String()
	}
	lines := strings.Split(output, "\n")
	if len(lines) > maxEvidenceOutputLines {
		lines = append(lines[:maxEvidenceOutputLines],
			fmt.Sprintf("…（还有 %d 行未记录）", len(lines)-maxEvidenceOutputLines))
	}
	sb.WriteString("\n```\n")
	sb.WriteString(strings.Join(lines, "\n"))
	sb.WriteString("\n```\n")
	return sb.String()
}
