package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/review"
)

// revertScanLimit 是往回翻多少条提交找 revert。够用就行，review 不做全历史扫描。
const revertScanLimit = 200

func cmdReview(e *env, args []string) error {
	fs := newFlagSet("review")
	jsonOut := commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("review", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return err
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	set, err := st.Load()
	if err != nil {
		return err
	}

	in := review.Input{Store: st, Config: cfg, Set: set, Now: time.Now()}
	if repo, rerr := gitx.Open(st.Root); rerr == nil {
		if rev, gerr := repo.RevertedDecisions(revertScanLimit); gerr == nil {
			in.Reverted = rev
		}
	}
	rep := review.Build(in)

	if *jsonOut {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		e.io.Println(string(data))
		return nil
	}
	e.io.Printf("%s", reviewText(rep))
	return nil
}

func reviewText(r *review.Report) string {
	var sb strings.Builder
	sb.WriteString("# keel review\n")

	if len(r.Due) > 0 {
		sb.WriteString("\n## 到期复查\n")
		for _, d := range r.Due {
			fmt.Fprintf(&sb, "- %s [%s] %s\n  复查日期 %s · %s · %s\n",
				shortRef(d.ID), d.Status, d.Title, d.ReviewAfter, d.Evidence, d.Path)
		}
	}
	if len(r.Issues) > 0 {
		sb.WriteString("\n## 失效与冲突\n")
		for _, i := range r.Issues {
			fmt.Fprintf(&sb, "- [%s] %s\n", i.Code, i.Message)
			if i.Path != "" {
				fmt.Fprintf(&sb, "  %s\n", i.Path)
			}
		}
	}
	if len(r.Candidates) > 0 {
		sb.WriteString("\n## 学习候选\n")
		for _, c := range r.Candidates {
			fmt.Fprintf(&sb, "- [%s] %s\n", c.Code, c.Reason)
			if len(c.Members) > 0 {
				fmt.Fprintf(&sb, "  依据：%s\n", strings.Join(c.Members, "、"))
			}
			fmt.Fprintf(&sb, "  › %s\n", c.Next)
		}
	}
	if len(r.Due) == 0 && len(r.Issues) == 0 && len(r.Candidates) == 0 {
		sb.WriteString("\n没有到期、失效或候选项。\n")
	}

	s := r.Stats
	sb.WriteString("\n## 统计\n")
	fmt.Fprintf(&sb, "决策 %d（有效 %d，有证据 %d）· 规则 %d（生效 %d，候选 %d）· 记忆 %d（已验证 %d）· 证据 %d\n",
		s.Decisions, s.ActiveDecisions, s.DecisionsWithEv,
		s.Rules, s.ActiveRules, s.CandidateRules,
		s.Memories, s.VerifiedMemories, s.Evidence)

	for _, n := range r.Notes {
		fmt.Fprintf(&sb, "\n%s\n", n)
	}
	return sb.String()
}
