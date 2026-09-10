package cli

import (
	"strings"
	"time"

	"github.com/shiftu/keel/internal/store"
)

// maxNoteLines 是一条记忆正文的行数上限。超过说明它其实是决策或该拆开。
const maxNoteLines = 20

func cmdNote(e *env, args []string) error {
	fs := newFlagSet("note")
	_ = commonFlags(fs, e)
	tag := fs.String("tag", "", "标签，逗号分隔")
	path := fs.String("path", "", "相关路径，逗号分隔")
	kind := fs.String("kind", string(store.MemKindGotcha), "gotcha | fact | pointer | counterexample")
	by := fs.String("by", "human", "作者声明，如 agent:codex")
	body := fs.String("body", "", "正文来源：- 表示标准输入，否则为文件路径")
	condition := fs.String("condition", "", "适用条件，形如 environment=仅本地盘，逗号分隔")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("note", rest, 1); err != nil {
		return err
	}
	if len(rest) == 1 && *body != "" {
		return usagef("正文只能来自位置参数或 --body 之一，不能同时给")
	}
	if len(rest) == 0 && *body == "" {
		return usagef("note 需要正文：位置参数或 --body -")
	}

	text := ""
	if len(rest) == 1 {
		text = rest[0]
	} else {
		text, err = readBody(e, *body)
		if err != nil {
			return err
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return usagef("正文不能为空")
	}
	if n := countLines(text); n > maxNoteLines {
		return usagef("正文 %d 行，超过 %d 行上限。够得上一次选择就用 keel decide；否则拆成多条 note",
			n, maxNoteLines)
	}

	tags := splitList(*tag)
	if len(tags) == 0 {
		return usagef("note 需要 --tag")
	}
	k := store.MemoryKind(*kind)
	if !store.ValidMemoryKind(k) {
		return usagef("--kind %q 非法（应为 gotcha/fact/pointer/counterexample）", *kind)
	}
	conds, err := parseConditions(*condition)
	if err != nil {
		return err
	}

	st, err := e.discover()
	if err != nil {
		return err
	}
	id, err := store.NewID(store.KindMemory)
	if err != nil {
		return err
	}
	now := time.Now()
	summary := firstLine(text)
	ra := store.NewDate(now.AddDate(0, 0, 90))
	fm := store.MemoryFM{
		Schema: store.SchemaVersion,
		ID:     id,
		Kind:   k,
		// 新记忆一律是 candidate：可检索，但不以命令式口吻注入为项目规则。
		Status:      store.MemCandidate,
		Summary:     summary,
		Tags:        tags,
		Scope:       splitList(*path),
		Conditions:  conds,
		Evidence:    []store.ID{},
		DerivedFrom: []store.ID{},
		Supersedes:  []store.ID{},
		ReviewAfter: &ra,
		By:          *by,
		Date:        store.NewDate(now),
	}
	p, err := st.CreateObject(id, store.Slugify(summary), fm, text)
	if err != nil {
		return err
	}
	e.io.Println(p)
	if !e.quiet {
		e.io.Errln("已记为 candidate。它可被检索，但在有证据之前不会被当成项目规则。")
	}
	return nil
}

func firstLine(s string) string {
	line := s
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		line = s[:i]
	}
	line = strings.TrimSpace(line)
	runes := []rune(line)
	if len(runes) > 80 {
		line = string(runes[:80])
	}
	return line
}

func parseConditions(spec string) (map[string]string, error) {
	items := splitList(spec)
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, usagef("--condition 每项应形如 key=value，实际 %q", it)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}
