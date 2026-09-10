// Package check 定义验证内核的稳定结果契约。
// 普通 CLI、Git hook 与宿主 hook 分别解释同一份 Result，互不共用退出码。
package check

import (
	"encoding/json"
	"sort"
)

// ResultSchema 是机器输出的 schema 版本。
const ResultSchema = 1

// Target 是本次检查的对象。不同 target 看的是不同快照，不能互相替代。
type Target string

const (
	// TargetWorktree 只读诊断当前工作树。
	TargetWorktree Target = "worktree"
	// TargetIndex 验证实际将提交的内容（暂存区快照）。
	TargetIndex Target = "index"
	// TargetCommitMsg 校验提交信息里的 trailer 与 index 的语义变化。
	TargetCommitMsg Target = "commit-msg"
	// TargetRange 在 CI 上按 base..head 重算。
	TargetRange Target = "range"
)

func ValidTarget(t Target) bool {
	switch t {
	case TargetWorktree, TargetIndex, TargetCommitMsg, TargetRange:
		return true
	}
	return false
}

// Status 是整体结论。
type Status string

const (
	StatusPass  Status = "pass"
	StatusFail  Status = "fail"
	StatusError Status = "error"
)

// Severity 决定 finding 是否让整体失败。
const (
	SeverityError = "error"
	SeverityWarn  = "warn"
)

// 稳定 finding code。agent 按 code 判断，不解析自然语言。
const (
	CodeFrontmatterInvalid   = "frontmatter_invalid"
	CodeIDDuplicate          = "id_duplicate"
	CodeFieldInvalid         = "field_invalid"
	CodeRefMissing           = "ref_missing"
	CodeRuleFailed           = "rule_failed"
	CodeRuleError            = "rule_error"
	CodeRuleTimeout          = "rule_timeout"
	CodeRuleBasisInvalid     = "rule_basis_invalid"
	CodeDecisionScopeOverlap = "decision_scope_overlap"
	CodeObjectStale          = "object_stale"
	CodeConfigInvalid        = "config_invalid"
	CodeAutoPromoteEnabled   = "auto_promote_enabled"
	CodeSupersedeIncomplete  = "supersede_incomplete"

	CodeUnexplainedDependency = "unexplained_dependency"
	CodeUnexplainedTopLevel   = "unexplained_top_level_dir"
	CodeDependencyVersion     = "dependency_version_change"
	CodeManifestUnparsed      = "manifest_unparsed"
	CodeTrailerUnknownRef     = "trailer_unknown_ref"
	CodeTrailerNotUsable      = "trailer_not_usable"
	CodeIndexSnapshotFailed   = "index_snapshot_unavailable"
	CodeSyncDrift             = "sync_drift"

	CodeMemoryStatusUnsupported = "memory_status_unsupported"
	CodeMemoryEvidenceStale     = "memory_evidence_stale"
	CodeMemoryReviewDue         = "memory_review_due"
	CodeMemoryConflict          = "memory_conflict"
)

// Finding 是一条检查结论。Code 是契约，Message 只给人看。
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Object   string `json:"object,omitempty"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
}

// Result 是内核返回给所有调用者的统一结果。
type Result struct {
	Schema   int       `json:"schema"`
	Status   Status    `json:"status"`
	Target   Target    `json:"target"`
	Findings []Finding `json:"findings"`
	// Evidence 列出本次产生的证据 ID（M2 起使用）。
	Evidence []string `json:"evidence,omitempty"`
	// Notes 是不影响结论的说明，例如降级原因。
	Notes []string `json:"notes,omitempty"`
}

// NewResult 建一个空结果。
func NewResult(t Target) *Result {
	return &Result{Schema: ResultSchema, Status: StatusPass, Target: t, Findings: []Finding{}}
}

// Add 追加 finding 并按 severity 更新整体状态。
func (r *Result) Add(f Finding) {
	if f.Severity == "" {
		f.Severity = SeverityError
	}
	r.Findings = append(r.Findings, f)
	if f.Severity == SeverityError && r.Status != StatusError {
		r.Status = StatusFail
	}
}

// AddError 记录内核自身的运行错误（不是被检查对象的问题）。
func (r *Result) AddError(f Finding) {
	f.Severity = SeverityError
	r.Findings = append(r.Findings, f)
	r.Status = StatusError
}

// HasCode 报告结果里是否有该 code。
func (r *Result) HasCode(code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

// Sort 让输出稳定：error 在前，然后按 code、path、object。
func (r *Result) Sort() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if (a.Severity == SeverityError) != (b.Severity == SeverityError) {
			return a.Severity == SeverityError
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Object < b.Object
	})
}

// JSON 渲染成稳定的机器输出。
func (r *Result) JSON() ([]byte, error) {
	r.Sort()
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	return json.MarshalIndent(r, "", "  ")
}

// ExitCode 把结果映射成业务 CLI 退出码：0 通过，1 检查失败或内核错误。
// 用法错误（2）由 CLI 层负责，内核不产生。
func (r *Result) ExitCode() int {
	if r.Status == StatusPass {
		return 0
	}
	return 1
}
