package render

import (
	"encoding/json"
	"fmt"
	"sort"
)

// renderHookEntries 把 keel 的 hook 条目合并进 .claude/settings.json 这类文件。
//
// 不按 command 前缀猜所有权：上次写进去的条目原样记在 generated.yaml 的
// owned_json 里，靠内容比对认领。用户自己加的 hook 一律原样保留；
// keel 写过的条目被人改过或删掉，就报冲突而不是默默重写。
func renderHookEntries(a Artifact, current string, existed bool, rec GenFile, hasRec bool) (
	newContent, ownedJSON string, conflict *Conflict) {

	root := map[string]any{}
	if existed && len(current) > 0 {
		if err := json.Unmarshal([]byte(current), &root); err != nil {
			return "", "", &Conflict{Path: a.Path, Reason: "JSON 解析失败: " + err.Error(),
				Fix: "修好 JSON 语法再 sync"}
		}
	}

	prevOurs := map[string][]string{}
	if hasRec && rec.OwnedJSON != "" {
		if err := json.Unmarshal([]byte(rec.OwnedJSON), &prevOurs); err != nil {
			return "", "", &Conflict{Path: a.Path, Reason: "generated.yaml 里的 owned_json 坏了",
				Fix: "删掉该记录后重新 sync"}
		}
	}

	hooks := map[string]any{}
	if h, ok := root["hooks"].(map[string]any); ok {
		hooks = h
	}

	events := make([]string, 0, len(a.Hooks))
	for e := range a.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)

	nextOurs := map[string][]string{}
	for _, event := range events {
		var currentList []any
		if l, ok := hooks[event].([]any); ok {
			currentList = l
		}
		want := map[string]bool{}
		for _, s := range prevOurs[event] {
			want[s] = true
		}
		var foreign []any
		found := 0
		for _, item := range currentList {
			canon, err := canonical(item)
			if err != nil {
				return "", "", &Conflict{Path: a.Path, Reason: err.Error(), Fix: "修好 JSON 再 sync"}
			}
			if want[canon] {
				found++
				continue
			}
			foreign = append(foreign, item)
		}
		if hasRec && found != len(prevOurs[event]) {
			return "", "", &Conflict{
				Path:   a.Path,
				Reason: fmt.Sprintf("hooks.%s 里 keel 写过的条目被改过或删掉了", event),
				Fix:    "还原成生成内容，或删掉 .keel/generated.yaml 里该文件的记录后重新 sync",
			}
		}
		merged := append([]any{}, foreign...)
		for _, item := range a.Hooks[event] {
			canon, err := canonical(item)
			if err != nil {
				return "", "", &Conflict{Path: a.Path, Reason: err.Error()}
			}
			nextOurs[event] = append(nextOurs[event], canon)
			merged = append(merged, item)
		}
		hooks[event] = merged
	}

	// keel 不再生成的事件：把自己那几条撤掉，别人的留下。
	for event, prev := range prevOurs {
		if _, still := a.Hooks[event]; still {
			continue
		}
		l, ok := hooks[event].([]any)
		if !ok {
			continue
		}
		want := map[string]bool{}
		for _, s := range prev {
			want[s] = true
		}
		var foreign []any
		for _, item := range l {
			if canon, err := canonical(item); err == nil && want[canon] {
				continue
			}
			foreign = append(foreign, item)
		}
		if len(foreign) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = foreign
		}
	}

	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", "", &Conflict{Path: a.Path, Reason: err.Error()}
	}
	ownedBytes, err := json.Marshal(nextOurs)
	if err != nil {
		return "", "", &Conflict{Path: a.Path, Reason: err.Error()}
	}
	return string(out) + "\n", string(ownedBytes), nil
}

// canonical 把一个 JSON 值序列化成稳定文本（Go 的 map 键会排序）。
func canonical(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("无法序列化 hook 条目: %w", err)
	}
	return string(b), nil
}

// renderJSONObjectKeys 合并 keel 拥有的对象键（如 .mcp.json 的 mcpServers.<name>）。
// 同样靠 owned_json 内容比对认领，不按名字猜；用户自己加的服务器原样保留。
func renderJSONObjectKeys(a Artifact, current string, existed bool, rec GenFile, hasRec bool) (
	newContent, ownedJSON string, conflict *Conflict) {

	root := map[string]any{}
	if existed && len(current) > 0 {
		if err := json.Unmarshal([]byte(current), &root); err != nil {
			return "", "", &Conflict{Path: a.Path, Reason: "JSON 解析失败: " + err.Error(),
				Fix: "修好 JSON 语法再 sync"}
		}
	}
	prevOurs := map[string]string{}
	if hasRec && rec.OwnedJSON != "" {
		if err := json.Unmarshal([]byte(rec.OwnedJSON), &prevOurs); err != nil {
			return "", "", &Conflict{Path: a.Path, Reason: "generated.yaml 里的 owned_json 坏了",
				Fix: "删掉该记录后重新 sync"}
		}
	}

	container := map[string]any{}
	if c, ok := root[a.JSONKey].(map[string]any); ok {
		container = c
	}

	// 先确认上次写进去的键还是原样，被改过就报冲突。
	for name, want := range prevOurs {
		cur, ok := container[name]
		if !ok {
			continue // 用户删掉了：视为不再需要，下面重建
		}
		canon, err := canonical(cur)
		if err != nil || canon != want {
			return "", "", &Conflict{
				Path:   a.Path,
				Reason: fmt.Sprintf("%s.%s 在上次生成后被改过", a.JSONKey, name),
				Fix:    "把改动搬回 .keel/keel.yaml 的 mcp.servers，或还原后再 sync",
			}
		}
	}
	// keel 不再生成的键，撤掉自己那部分。
	for name := range prevOurs {
		if _, still := a.Entries[name]; !still {
			delete(container, name)
		}
	}

	names := make([]string, 0, len(a.Entries))
	for n := range a.Entries {
		names = append(names, n)
	}
	sort.Strings(names)
	nextOurs := map[string]string{}
	for _, name := range names {
		if _, wasOurs := prevOurs[name]; !wasOurs {
			if _, taken := container[name]; taken {
				return "", "", &Conflict{
					Path:   a.Path,
					Reason: fmt.Sprintf("%s.%s 已被非 keel 管理的内容占用", a.JSONKey, name),
					Fix:    "改名，或把它挪进 .keel/keel.yaml 的 mcp.servers 交给 keel 管理",
				}
			}
		}
		container[name] = a.Entries[name]
		canon, err := canonical(a.Entries[name])
		if err != nil {
			return "", "", &Conflict{Path: a.Path, Reason: err.Error()}
		}
		nextOurs[name] = canon
	}

	if len(container) == 0 {
		delete(root, a.JSONKey)
	} else {
		root[a.JSONKey] = container
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", "", &Conflict{Path: a.Path, Reason: err.Error()}
	}
	ownedBytes, err := json.Marshal(nextOurs)
	if err != nil {
		return "", "", &Conflict{Path: a.Path, Reason: err.Error()}
	}
	return string(out) + "\n", string(ownedBytes), nil
}
