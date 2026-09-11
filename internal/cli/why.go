package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/store"
)

// whyHit 是一条命中。
type whyHit struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Path    string `json:"path"`
	Why     string `json:"why_selected"`
	History bool   `json:"history"`
}

func cmdWhy(e *env, args []string) error {
	fs := newFlagSet("why")
	jsonOut := commonFlags(fs, e)
	path := fs.String("path", "", "路径（可以是还不存在的新文件）")
	query := fs.String("query", "", "关键词：标题、标签、正文子串")
	history := fs.Bool("history", false, "同时列出被否决、被替代的结论")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("why", rest, 0); err != nil {
		return err
	}
	if *path == "" && *query == "" {
		return usagef("why 需要 --path 或 --query")
	}

	st, err := e.discover()
	if err != nil {
		return err
	}
	set, err := st.Load()
	if err != nil {
		return err
	}

	var trailerIDs map[store.ID]string
	if *path != "" {
		trailerIDs = trailerRefsForPath(st, *path)
	}

	var hits []whyHit
	for _, d := range set.Decisions {
		hist := d.Status.IsHistory()
		if hist && !*history {
			continue
		}
		why, ok := matchDecision(d, *path, *query, trailerIDs)
		if !ok {
			continue
		}
		hits = append(hits, whyHit{"decision", d.ID.String(), d.Title, string(d.Status),
			d.SourcePath(), why, hist})
	}
	for _, r := range set.Rules {
		hist := r.Status.IsHistory()
		if hist && !*history {
			continue
		}
		why, ok := matchGeneric(r.Scope, r.Title, nil, r.Body(), *path, *query)
		if !ok {
			continue
		}
		hits = append(hits, whyHit{"rule", r.ID.String(), r.Title, string(r.Status),
			r.SourcePath(), why, hist})
	}
	for _, m := range set.Memories {
		hist := m.Status.IsHistory()
		if hist && !*history {
			continue
		}
		why, ok := matchGeneric(m.Scope, m.Summary, m.Tags, m.Body(), *path, *query)
		if !ok {
			continue
		}
		hits = append(hits, whyHit{"memory", m.ID.String(), m.Summary, string(m.Status),
			m.SourcePath(), why, hist})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.History != b.History {
			return !a.History
		}
		// revisit / disputed 先看：它们是提醒。
		if ra, rb := isAlert(a.Status), isAlert(b.Status); ra != rb {
			return ra
		}
		if a.Kind != b.Kind {
			return kindOrder(a.Kind) < kindOrder(b.Kind)
		}
		return a.ID < b.ID
	})

	if *jsonOut {
		data, err := json.MarshalIndent(map[string]any{"schema": 1, "hits": hits}, "", "  ")
		if err != nil {
			return err
		}
		e.io.Println(string(data))
		return nil
	}
	if len(hits) == 0 {
		e.io.Println("没有找到相关的决策、规则或记忆。")
		if !*history {
			e.io.Println("加 --history 可以看被否决和被替代的结论。")
		}
		return nil
	}
	for _, h := range hits {
		mark := ""
		if h.History {
			mark = "（历史）"
		}
		e.io.Printf("%s %s [%s]%s %s\n    %s · %s\n",
			kindLabel(h.Kind), shortRef(h.ID), h.Status, mark, h.Title, h.Why, h.Path)
	}
	return nil
}

func isAlert(status string) bool {
	return status == string(store.DecRevisit) || status == string(store.MemDisputed)
}

func kindOrder(k string) int {
	switch k {
	case "decision":
		return 0
	case "rule":
		return 1
	}
	return 2
}

func kindLabel(k string) string {
	switch k {
	case "decision":
		return "决策"
	case "rule":
		return "规则"
	}
	return "记忆"
}

func shortRef(id string) string {
	if len(id) >= 10 {
		return id[:10]
	}
	return id
}

func matchDecision(d *store.Decision, path, query string, trailers map[store.ID]string) (string, bool) {
	if why, ok := matchGeneric(d.Scope, d.Title, d.Tags, d.Body(), path, query); ok {
		return why, true
	}
	if commit, ok := trailers[d.ID]; ok {
		return "被提交 " + commit + " 的 Decision: 引用", true
	}
	return "", false
}

func matchGeneric(scope []string, title string, tags []string, body, path, query string) (string, bool) {
	if path != "" {
		if pat, ok := store.MatchAny(scope, strings.TrimPrefix(path, "./")); ok {
			return "scope " + pat + " 覆盖该路径", true
		}
		return "", false
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(title), q) {
		return "标题包含 " + query, true
	}
	for _, t := range tags {
		if strings.EqualFold(t, query) {
			return "标签 " + t, true
		}
	}
	if strings.Contains(strings.ToLower(body), q) {
		return "正文包含 " + query, true
	}
	return "", false
}

// trailerRefsForPath 从 git 历史里找该路径的提交引用过哪些决策。
// git 是提交索引，.keel/ 是对象源，两边合起来才是完整答案。
func trailerRefsForPath(st *store.Store, path string) map[store.ID]string {
	out := map[store.ID]string{}
	repo, err := gitx.Open(st.Root)
	if err != nil || !repo.HasHead() {
		return out
	}
	logOut, err := repo.LogTrailers(path, 200)
	if err != nil {
		return out
	}
	for commit, refs := range logOut {
		for _, raw := range refs {
			if id, err := store.ParseID(raw); err == nil {
				out[id] = commit
			}
		}
	}
	return out
}

var _ = fmt.Sprintf
