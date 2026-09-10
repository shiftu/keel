package store

import (
	"fmt"
	"strings"
)

// Object 是 .keel/ 里四类对象的共同接口。
type Object interface {
	ObjectID() ID
	ObjectTitle() string
	// StatusString 返回状态的文本形式，供输出与 finding 使用。
	StatusString() string
	// Slug 返回文件名中 ID 之后的部分。
	Slug() string
	// Refs 返回该对象引用的其他对象 ID（用于引用完整性检查）。
	Refs() []Ref
	// SourcePath 返回该对象来自哪个文件（仓库相对路径），未加载时为空。
	SourcePath() string
}

// Ref 是一次对外引用，Field 用于报错定位。
type Ref struct {
	Field string
	ID    ID
}

// RuleMigration 描述替代决策时旧规则的去向。To 为空表示 retired。
type RuleMigration struct {
	From ID  `yaml:"from"`
	To   *ID `yaml:"to,omitempty"`
}

type meta struct {
	path string
	body string
	slug string
}

func (m meta) SourcePath() string { return m.path }
func (m meta) Body() string       { return m.body }
func (m meta) Slug() string       { return m.slug }

// ---------- Decision ----------

type DecisionFM struct {
	Schema        int             `yaml:"schema"`
	ID            ID              `yaml:"id"`
	Title         string          `yaml:"title"`
	Status        DecisionStatus  `yaml:"status"`
	Date          Date            `yaml:"date"`
	By            string          `yaml:"by"`
	Tags          []string        `yaml:"tags"`
	Scope         []string        `yaml:"scope"`
	Supersedes    []ID            `yaml:"supersedes"`
	SupersededBy  []ID            `yaml:"superseded_by"`
	RuleMigration []RuleMigration `yaml:"rule_migration"`
	Confidence    *float64        `yaml:"confidence"`
	ReviewAfter   *Date           `yaml:"review_after"`
	Evidence      []ID            `yaml:"evidence"`
	Extensions    map[string]any  `yaml:"extensions,omitempty"`
}

type Decision struct {
	DecisionFM
	meta
}

func (d *Decision) ObjectID() ID        { return d.ID }
func (d *Decision) ObjectTitle() string { return d.Title }
func (d *Decision) StatusString() string {
	return string(d.Status)
}
func (d *Decision) Refs() []Ref {
	var refs []Ref
	for _, id := range d.Supersedes {
		refs = append(refs, Ref{"supersedes", id})
	}
	for _, id := range d.SupersededBy {
		refs = append(refs, Ref{"superseded_by", id})
	}
	for _, id := range d.Evidence {
		refs = append(refs, Ref{"evidence", id})
	}
	for _, m := range d.RuleMigration {
		refs = append(refs, Ref{"rule_migration.from", m.From})
		if m.To != nil {
			refs = append(refs, Ref{"rule_migration.to", *m.To})
		}
	}
	return refs
}

// 决策正文的四个固定小节。
var decisionSections = []string{"背景", "决定", "备选与理由", "后果与验证方式"}

// EmptySections 返回正文中缺失或为空的固定小节名。
func (d *Decision) EmptySections() []string {
	var missing []string
	for _, name := range decisionSections {
		if !hasNonEmptySection(d.body, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func hasNonEmptySection(body, name string) bool {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "#") {
			continue
		}
		if strings.TrimSpace(strings.TrimLeft(t, "# ")) != name {
			continue
		}
		for _, next := range lines[i+1:] {
			nt := strings.TrimSpace(next)
			if nt == "" {
				continue
			}
			if strings.HasPrefix(nt, "#") {
				break
			}
			return true
		}
		return false
	}
	return false
}

func (d *Decision) validate() error {
	if err := validateCommon(d.Schema, d.ID, KindDecision, d.Title); err != nil {
		return err
	}
	if !validDecisionStatus(d.Status) {
		return fmt.Errorf("status %q 非法（应为 proposed/accepted/proven/revisit/superseded/rejected）", d.Status)
	}
	if d.Date.IsZero() {
		return fmt.Errorf("date 必填")
	}
	if d.Confidence != nil && (*d.Confidence < 0 || *d.Confidence > 1) {
		return fmt.Errorf("confidence 应在 0 到 1 之间，实际 %v", *d.Confidence)
	}
	if d.Status == DecSuperseded && len(d.SupersededBy) == 0 {
		return fmt.Errorf("status superseded 必须有 superseded_by")
	}
	for _, m := range d.RuleMigration {
		if m.From.Kind != KindRule {
			return fmt.Errorf("rule_migration.from 必须是规则 ID，实际 %s", m.From)
		}
		if m.To != nil && m.To.Kind != KindRule {
			return fmt.Errorf("rule_migration.to 必须是规则 ID，实际 %s", *m.To)
		}
	}
	return refKindsMatch(d.Refs(), map[string]Kind{
		"supersedes": KindDecision, "superseded_by": KindDecision, "evidence": KindEvidence,
	})
}

// ---------- Rule ----------

type RuleCheck struct {
	Argv           []string `yaml:"argv"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
}

type RuleFM struct {
	Schema         int            `yaml:"schema"`
	ID             ID             `yaml:"id"`
	Title          string         `yaml:"title"`
	Status         RuleStatus     `yaml:"status"`
	Scope          []string       `yaml:"scope"`
	Severity       string         `yaml:"severity"`
	Check          *RuleCheck     `yaml:"check"`
	From           *ID            `yaml:"from"`
	FromTemplate   *string        `yaml:"from_template"`
	VerifierDigest string         `yaml:"verifier_digest"`
	Extensions     map[string]any `yaml:"extensions,omitempty"`
}

type Rule struct {
	RuleFM
	meta
}

func (r *Rule) ObjectID() ID         { return r.ID }
func (r *Rule) ObjectTitle() string  { return r.Title }
func (r *Rule) StatusString() string { return string(r.Status) }
func (r *Rule) IsExecutable() bool {
	return r.Status == RuleActive && r.Check != nil && len(r.Check.Argv) > 0
}
func (r *Rule) SeverityOrDefault() string {
	if r.Severity == "" {
		return "error"
	}
	return r.Severity
}
func (r *Rule) Refs() []Ref {
	if r.From == nil {
		return nil
	}
	return []Ref{{"from", *r.From}}
}

func (r *Rule) validate() error {
	if err := validateCommon(r.Schema, r.ID, KindRule, r.Title); err != nil {
		return err
	}
	if !validRuleStatus(r.Status) {
		return fmt.Errorf("status %q 非法（应为 candidate/active/retired）", r.Status)
	}
	switch r.Severity {
	case "", "error", "warn":
	default:
		return fmt.Errorf("severity %q 非法（应为 error 或 warn）", r.Severity)
	}
	if r.Check != nil {
		if len(r.Check.Argv) == 0 {
			return fmt.Errorf("check.argv 不能为空；keel 不接受 shell 字符串形式的 check")
		}
		if r.Check.TimeoutSeconds < 0 {
			return fmt.Errorf("check.timeout_seconds 不能为负")
		}
	}
	return refKindsMatch(r.Refs(), map[string]Kind{"from": KindDecision})
}

// ---------- Memory ----------

type MemoryKind string

const (
	MemKindGotcha         MemoryKind = "gotcha"
	MemKindFact           MemoryKind = "fact"
	MemKindPointer        MemoryKind = "pointer"
	MemKindCounterexample MemoryKind = "counterexample"
)

func validMemoryKind(k MemoryKind) bool {
	switch k {
	case MemKindGotcha, MemKindFact, MemKindPointer, MemKindCounterexample:
		return true
	}
	return false
}

type MemoryFM struct {
	Schema      int               `yaml:"schema"`
	ID          ID                `yaml:"id"`
	Kind        MemoryKind        `yaml:"kind"`
	Status      MemoryStatus      `yaml:"status"`
	Summary     string            `yaml:"summary"`
	Tags        []string          `yaml:"tags"`
	Scope       []string          `yaml:"scope"`
	Conditions  map[string]string `yaml:"conditions"`
	Evidence    []ID              `yaml:"evidence"`
	DerivedFrom []ID              `yaml:"derived_from"`
	Supersedes  []ID              `yaml:"supersedes"`
	VerifiedAt  *Date             `yaml:"verified_at"`
	ReviewAfter *Date             `yaml:"review_after"`
	By          string            `yaml:"by"`
	Date        Date              `yaml:"date"`
	Extensions  map[string]any    `yaml:"extensions,omitempty"`
}

type Memory struct {
	MemoryFM
	meta
}

func (m *Memory) ObjectID() ID         { return m.ID }
func (m *Memory) ObjectTitle() string  { return m.Summary }
func (m *Memory) StatusString() string { return string(m.Status) }
func (m *Memory) Refs() []Ref {
	var refs []Ref
	for _, id := range m.Evidence {
		refs = append(refs, Ref{"evidence", id})
	}
	for _, id := range m.DerivedFrom {
		refs = append(refs, Ref{"derived_from", id})
	}
	for _, id := range m.Supersedes {
		refs = append(refs, Ref{"supersedes", id})
	}
	return refs
}

func (m *Memory) validate() error {
	if err := validateCommon(m.Schema, m.ID, KindMemory, m.Summary); err != nil {
		return err
	}
	if !validMemoryKind(m.Kind) {
		return fmt.Errorf("kind %q 非法（应为 gotcha/fact/pointer/counterexample）", m.Kind)
	}
	if !validMemoryStatus(m.Status) {
		return fmt.Errorf("status %q 非法（应为 candidate/verified/disputed/stale/archived）", m.Status)
	}
	if m.Date.IsZero() {
		return fmt.Errorf("date 必填")
	}
	if m.Status == MemVerified && len(m.Evidence) == 0 {
		return fmt.Errorf("status verified 要求 evidence 非空")
	}
	return refKindsMatch(m.Refs(), map[string]Kind{
		"evidence": KindEvidence, "derived_from": KindMemory, "supersedes": KindMemory,
	})
}

// ---------- Evidence ----------

type EvidenceTarget struct {
	Commit        string   `yaml:"commit"`
	ContentDigest string   `yaml:"content_digest"`
	DirtyInputs   []string `yaml:"dirty_inputs"`
}

type EvidenceVerifier struct {
	ID               string   `yaml:"id"`
	DefinitionDigest string   `yaml:"definition_digest"`
	Command          []string `yaml:"command"`
}

type EvidenceFM struct {
	Schema        int              `yaml:"schema"`
	ID            ID               `yaml:"id"`
	Subject       ID               `yaml:"subject"`
	SubjectDigest string           `yaml:"subject_digest"`
	Kind          string           `yaml:"kind"`
	Trust         string           `yaml:"trust"`
	Target        EvidenceTarget   `yaml:"target"`
	Verifier      EvidenceVerifier `yaml:"verifier"`
	Result        string           `yaml:"result"`
	ObservedAt    string           `yaml:"observed_at"`
	Producer      string           `yaml:"producer"`
	Extensions    map[string]any   `yaml:"extensions,omitempty"`
}

type Evidence struct {
	EvidenceFM
	meta
}

func (e *Evidence) ObjectID() ID         { return e.ID }
func (e *Evidence) ObjectTitle() string  { return e.Verifier.ID }
func (e *Evidence) StatusString() string { return e.Result }
func (e *Evidence) Refs() []Ref          { return []Ref{{"subject", e.Subject}} }

func (e *Evidence) validate() error {
	if e.Schema != SchemaVersion {
		return fmt.Errorf("schema 应为 %d，实际 %d", SchemaVersion, e.Schema)
	}
	if e.ID.Kind != KindEvidence {
		return fmt.Errorf("id 应为 E- 开头的证据 ID，实际 %s", e.ID)
	}
	switch e.Result {
	case "pass", "fail", "error", "timeout", "skipped":
	default:
		return fmt.Errorf("result %q 非法（应为 pass/fail/error/timeout/skipped）", e.Result)
	}
	switch e.Trust {
	case "local", "ci", "self-reported":
	default:
		return fmt.Errorf("trust %q 非法（应为 local/ci/self-reported）", e.Trust)
	}
	if e.Subject.IsZero() {
		return fmt.Errorf("subject 必填")
	}
	return nil
}

// ---------- 共用校验 ----------

func validateCommon(schema int, id ID, want Kind, title string) error {
	if schema != SchemaVersion {
		return fmt.Errorf("schema 应为 %d，实际 %d", SchemaVersion, schema)
	}
	if id.IsZero() {
		return fmt.Errorf("id 必填")
	}
	if id.Kind != want {
		return fmt.Errorf("id 类型前缀应为 %s，实际 %s", want, id.Kind)
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("标题/摘要不能为空")
	}
	return nil
}

func refKindsMatch(refs []Ref, want map[string]Kind) error {
	for _, r := range refs {
		k, ok := want[r.Field]
		if !ok {
			continue
		}
		if r.ID.Kind != k {
			return fmt.Errorf("%s 应引用 %s 类型对象，实际 %s", r.Field, k, r.ID)
		}
	}
	return nil
}

// SetBodyForValidation 让 CLI 在写盘前用正文做校验（例如四段是否为空）。
func (d *Decision) SetBodyForValidation(body string) { d.meta.body = body }

// ValidMemoryKind 供 CLI 校验 --kind。
func ValidMemoryKind(k MemoryKind) bool { return validMemoryKind(k) }

// Body 返回对象正文，供渲染软规则等场景使用。
func (r *Rule) Body() string { return r.meta.body }

// Body 返回记忆正文。
func (m *Memory) Body() string { return m.meta.body }

// Body 返回决策正文。
func (d *Decision) Body() string { return d.meta.body }
