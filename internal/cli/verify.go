package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/check"
	"github.com/shiftu/keel/internal/store"
	"github.com/shiftu/keel/internal/verify"
)

func cmdVerify(e *env, args []string) error {
	fs := newFlagSet("verify")
	jsonOut := commonFlags(fs, e)
	rule := fs.String("rule", "", "用这条规则的 check.argv 当验证器")
	id := fs.String("id", "", "验证器标识，默认取命令摘要")
	kind := fs.String("kind", "", "regression-test | check | review | self-reported")
	trust := fs.String("trust", "local", "local | ci")
	timeout := fs.Int("timeout", 0, "超时秒数，默认 30")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return usagef("用法：keel verify <M-…|D-…> --rule R-… 或 keel verify <M-…|D-…> -- <命令…>")
	}
	subjectRef, argv := rest[0], rest[1:]
	if *rule != "" && len(argv) > 0 {
		return usagef("--rule 和显式命令只能给一个：两个验证器同时跑，证据说不清是谁验的")
	}
	if *rule == "" && len(argv) == 0 {
		return usagef("要么给 --rule，要么在 -- 之后给命令。keel 不接受「登记一条我认为它通过了」")
	}
	switch *trust {
	case "local", "ci":
	default:
		return usagef("--trust 只能是 local 或 ci（self-reported 没有写入口）")
	}

	st, err := e.discover()
	if err != nil {
		return err
	}
	set, err := st.Load()
	if err != nil {
		return err
	}

	subject, err := resolveSubject(set, subjectRef)
	if err != nil {
		return err
	}
	req := verify.Request{
		Subject: subject,
		Trust:   *trust,
		Timeout: time.Duration(*timeout) * time.Second,
	}
	if *rule != "" {
		if err := fillFromRule(set, *rule, &req); err != nil {
			return err
		}
	} else {
		req.Argv = argv
		req.VerifierID = *id
		if req.VerifierID == "" {
			req.VerifierID = shortDigest(strings.Join(argv, "\x00"))
		}
		req.DefinitionDigest = digestOf(strings.Join(argv, "\x00"))
		req.Kind = "regression-test"
	}
	if *kind != "" {
		req.Kind = *kind
	}

	out, err := verify.Run(st, req)
	if err != nil {
		return err
	}
	return reportVerify(e, *jsonOut, subject, out)
}

// resolveSubject 只接受记忆和决策：规则由 check 直接执行，证据挂在结论上才有意义。
func resolveSubject(set *store.Set, ref string) (store.Object, error) {
	id, err := store.ParseRef(ref, set.IDs())
	if err != nil {
		return nil, usagef("%v", err)
	}
	o, ok := set.Lookup(id)
	if !ok {
		return nil, usagef("找不到 %s", id)
	}
	switch id.Kind {
	case store.KindMemory, store.KindDecision:
		return o, nil
	}
	return nil, usagef("只能验证记忆（M-）或决策（D-），%s 是 %s", id.Short(), id.Kind)
}

func fillFromRule(set *store.Set, ref string, req *verify.Request) error {
	id, err := store.ParseRef(ref, set.IDs())
	if err != nil {
		return usagef("%v", err)
	}
	o, ok := set.Lookup(id)
	if !ok || id.Kind != store.KindRule {
		return usagef("--rule 需要一个存在的规则 ID，收到 %q", ref)
	}
	r := o.(*store.Rule)
	if r.Check == nil || len(r.Check.Argv) == 0 {
		return usagef("规则 %s 没有 check.argv，不能当验证器", r.ID.Short())
	}
	req.Argv = r.Check.Argv
	req.VerifierID = r.ID.String()
	// 规则改了，旧证据的 definition_digest 就对不上——这是有意的。
	req.DefinitionDigest = digestOf(strings.Join(r.Check.Argv, "\x00") + "\x00" + strings.TrimSpace(r.Body()))
	req.Kind = "check"
	if req.Timeout <= 0 && r.Check.TimeoutSeconds > 0 {
		req.Timeout = time.Duration(r.Check.TimeoutSeconds) * time.Second
	}
	return nil
}

func digestOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func shortDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

type verifyReport struct {
	Schema      int    `json:"schema"`
	Subject     string `json:"subject"`
	Evidence    string `json:"evidence"`
	Path        string `json:"evidence_path"`
	Result      string `json:"result"`
	ExitCode    int    `json:"verifier_exit_code"`
	Transition  string `json:"transition,omitempty"`
	SubjectPath string `json:"subject_path,omitempty"`
}

func reportVerify(e *env, jsonOut bool, subject store.Object, out *verify.Outcome) error {
	rep := verifyReport{
		Schema: 1, Subject: subject.ObjectID().String(),
		Evidence: out.EvidenceID.String(), Path: out.EvidencePath,
		Result: string(out.Run.Outcome), ExitCode: out.Run.ExitCode,
		Transition: out.Transition, SubjectPath: out.SubjectPath,
	}
	if jsonOut {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		e.io.Println(string(data))
	} else {
		e.io.Println(out.EvidencePath)
		if !e.quiet {
			e.io.Errf("验证器结果：%s（退出码 %d）\n", out.Run.Outcome, out.Run.ExitCode)
			if out.Transition != "" {
				e.io.Errf("%s：%s\n", subject.ObjectID().Short(), out.Transition)
			} else {
				e.io.Errf("%s 状态不变，只追加了证据。\n", subject.ObjectID().Short())
			}
			if out.Run.Outcome == check.OutcomeError || out.Run.Outcome == check.OutcomeTimeout {
				e.io.Errln("跑不起来既不是通过也不是失败，所以没有做任何状态转换。")
			}
		}
	}
	if out.Run.Outcome != check.OutcomePass {
		return checkFailed
	}
	return nil
}
