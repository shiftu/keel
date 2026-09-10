package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/store"
)

const decisionTemplate = `## 背景

## 决定

## 备选与理由

## 后果与验证方式
`

func cmdDecide(e *env, args []string) error {
	fs := newFlagSet("decide")
	_ = commonFlags(fs, e)
	tag := listFlag(fs, "tag", "标签，可重复或逗号分隔（只服务检索，不决定风险）")
	scope := listFlag(fs, "scope", "管辖路径 glob，可重复或逗号分隔")
	supersedes := fs.String("supersedes", "", "替代的决策 ID")
	ruleMigration := listFlag(fs, "rule-migration", "规则迁移，形如 R-xxx:R-yyy 或 R-xxx:retired，可重复或逗号分隔")
	confidence := fs.Float64("confidence", -1, "作者自评 0-1（不授予可信状态）")
	by := fs.String("by", "human", "作者声明，如 agent:claude")
	status := fs.String("status", string(store.DecProposed), "proposed 或 accepted")
	body := fs.String("body", "", "正文来源：- 表示标准输入，否则为文件路径")
	edit := fs.Bool("edit", false, "打开 $EDITOR 写正文")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := exactlyArgs("decide", rest, 1); err != nil {
		return err
	}
	title := strings.TrimSpace(rest[0])
	if title == "" {
		return usagef("标题不能为空")
	}
	tags := []string(*tag)
	scopes := []string(*scope)
	if len(tags) == 0 {
		return usagef("decide 需要 --tag")
	}
	if len(scopes) == 0 {
		return usagef("decide 需要 --scope（没有 scope 的决策无法被 keel why --path 找到）")
	}
	st, err := e.discover()
	if err != nil {
		return err
	}

	want := store.DecisionStatus(*status)
	if want != store.DecProposed && want != store.DecAccepted {
		return usagef("--status 只接受 proposed 或 accepted，实际 %q", *status)
	}
	if *body != "" && *edit {
		return usagef("--body 与 --edit 不能同时使用")
	}

	text := decisionTemplate
	switch {
	case *body != "":
		text, err = readBody(e, *body)
		if err != nil {
			return err
		}
	case *edit:
		text, err = editBody(decisionTemplate)
		if err != nil {
			return err
		}
	}

	set, err := st.Load()
	if err != nil {
		return err
	}
	known := set.IDs()

	var supersedesIDs []store.ID
	var old *store.Decision
	if strings.TrimSpace(*supersedes) != "" {
		id, err := store.ParseRef(*supersedes, known)
		if err != nil {
			return usagef("--supersedes %v", err)
		}
		if id.Kind != store.KindDecision {
			return usagef("--supersedes 必须是决策 ID，实际 %s", id)
		}
		d, ok := set.DecisionByID(id)
		if !ok {
			return usagef("--supersedes 指向的 %s 不存在", id)
		}
		old = d
		supersedesIDs = []store.ID{id}
	}

	migrations, err := parseRuleMigrations([]string(*ruleMigration), known)
	if err != nil {
		return err
	}
	if len(migrations) > 0 && old == nil {
		return usagef("--rule-migration 只在 --supersedes 时有意义")
	}

	id, err := store.NewID(store.KindDecision)
	if err != nil {
		return err
	}
	now := time.Now()
	fm := store.DecisionFM{
		Schema:        store.SchemaVersion,
		ID:            id,
		Title:         title,
		Status:        want,
		Date:          store.NewDate(now),
		By:            *by,
		Tags:          tags,
		Scope:         scopes,
		Supersedes:    supersedesIDs,
		SupersededBy:  []store.ID{},
		RuleMigration: migrations,
		Evidence:      []store.ID{},
	}
	if *confidence >= 0 {
		if *confidence > 1 {
			return usagef("--confidence 应在 0 到 1 之间")
		}
		c := *confidence
		fm.Confidence = &c
	}
	ra := store.NewDate(now.AddDate(0, 0, 90))
	fm.ReviewAfter = &ra

	probe := &store.Decision{DecisionFM: fm}
	probe.SetBodyForValidation(text)
	if want == store.DecAccepted {
		if missing := probe.EmptySections(); len(missing) > 0 {
			return usagef("--status accepted 要求四段正文都非空，缺：%s。先写正文（--body -）或用默认的 proposed",
				strings.Join(missing, "、"))
		}
	}

	// 替代是原子操作：只有 accepted 才转换旧决策；proposed 仅登记意图。
	migrate := old != nil && want == store.DecAccepted
	if migrate {
		if err := store.CanTransitionDecision(old.Status, store.DecSuperseded); err != nil {
			return fmt.Errorf("不能替代 %s：%w", old.ID.Short(), err)
		}
		if err := requireRuleMigration(set, old, migrations); err != nil {
			return err
		}
	}

	path, err := st.CreateObject(id, store.Slugify(title), fm, text)
	if err != nil {
		return err
	}
	if migrate {
		if err := supersedeOld(st, old, id); err != nil {
			_ = os.Remove(filepath.Join(st.Root, filepath.FromSlash(path)))
			return fmt.Errorf("替代 %s 失败，已回滚新决策：%w", old.ID.Short(), err)
		}
	}

	e.io.Println(path)
	if old != nil && !migrate {
		e.io.Errln(fmt.Sprintf("提示：%s 仍然有效。新决策转 accepted 时才会完成替代。", old.ID.Short()))
	}
	e.io.Println("")
	e.io.Println("Decision: " + id.String())
	return nil
}

// supersedeOld 把旧决策转成 superseded 并指回新决策。
func supersedeOld(st *store.Store, old *store.Decision, newID store.ID) error {
	fm := old.DecisionFM
	fm.Status = store.DecSuperseded
	if !containsID(fm.SupersededBy, newID) {
		fm.SupersededBy = append(fm.SupersededBy, newID)
	}
	return st.ReplaceObject(old.SourcePath(), fm, old.Body())
}

func containsID(list []store.ID, id store.ID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// requireRuleMigration 拒绝「替代了有派生规则的决策却不说明规则去向」。
func requireRuleMigration(set *store.Set, old *store.Decision, migrations []store.RuleMigration) error {
	var pending []string
	covered := map[store.ID]bool{}
	for _, m := range migrations {
		covered[m.From] = true
	}
	for _, r := range set.Rules {
		if r.From == nil || *r.From != old.ID || r.Status == store.RuleRetired {
			continue
		}
		if !covered[r.ID] {
			pending = append(pending, r.ID.Short()+"（"+r.Title+"）")
		}
	}
	if len(pending) == 0 {
		return nil
	}
	return usagef("替代 %s 前必须说明这些派生规则的去向：%s\n  用 --rule-migration %s:retired 或 %s:R-<新规则>",
		old.ID.Short(), strings.Join(pending, "、"),
		strings.SplitN(pending[0], "（", 2)[0], strings.SplitN(pending[0], "（", 2)[0])
}

func parseRuleMigrations(items []string, known []store.ID) ([]store.RuleMigration, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]store.RuleMigration, 0, len(items))
	for _, it := range items {
		fromRaw, toRaw, ok := strings.Cut(it, ":")
		if !ok {
			return nil, usagef("--rule-migration 每项应形如 R-xxx:R-yyy 或 R-xxx:retired，实际 %q", it)
		}
		from, err := store.ParseRef(strings.TrimSpace(fromRaw), known)
		if err != nil {
			return nil, usagef("--rule-migration %v", err)
		}
		if from.Kind != store.KindRule {
			return nil, usagef("--rule-migration 的 from 必须是规则 ID，实际 %s", from)
		}
		m := store.RuleMigration{From: from}
		if strings.TrimSpace(toRaw) != "retired" {
			to, err := store.ParseRef(strings.TrimSpace(toRaw), known)
			if err != nil {
				return nil, usagef("--rule-migration %v", err)
			}
			if to.Kind != store.KindRule {
				return nil, usagef("--rule-migration 的 to 必须是规则 ID 或 retired，实际 %s", to)
			}
			m.To = &to
		}
		out = append(out, m)
	}
	return out, nil
}
