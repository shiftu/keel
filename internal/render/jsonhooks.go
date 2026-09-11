package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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

// removeOwnedHookEntries 从 hooks 数组里撤掉 keel 写过的那几条。
// 认领仍靠 owned_json 的内容比对：只删内容完全相同的条目，别人的一律保留。
// 事件数组空了删键，hooks 空了删键，根对象空了返回空串让调用方删文件。
func removeOwnedHookEntries(rec GenFile, current string) (cleaned, ownedNow string, err error) {
	root, err := parseJSONObject(current)
	if err != nil {
		return "", "", err
	}
	ours := map[string][]string{}
	if err := json.Unmarshal([]byte(rec.OwnedJSON), &ours); err != nil {
		return "", "", fmt.Errorf("generated.yaml 里的 owned_json 坏了: %w", err)
	}
	hooks, _ := root["hooks"].(map[string]any)

	found := map[string][]string{}
	for event, wantList := range ours {
		want := map[string]bool{}
		for _, s := range wantList {
			want[s] = true
		}
		list, _ := hooks[event].([]any)
		var foreign []any
		for _, item := range list {
			canon, cerr := canonical(item)
			if cerr != nil {
				return "", "", cerr
			}
			if want[canon] {
				found[event] = append(found[event], canon)
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

	ownedNow = rec.OwnedJSON
	if !sameEntrySets(ours, found) {
		b, _ := json.Marshal(found)
		ownedNow = string(b)
	}
	cleaned, err = marshalJSONObject(root)
	return cleaned, ownedNow, err
}

// removeOwnedObjectKeys 从某个对象（如 mcpServers）下撤掉 keel 写过的键。
// 值和记录不一样的键已经不完全是 keel 的了——留下并让摘要对不上，由调用方报冲突。
// 人已经删掉的键当作不再需要，不算冲突，和 sync 的口径一致。
func removeOwnedObjectKeys(rec GenFile, current string) (cleaned, ownedNow string, err error) {
	root, err := parseJSONObject(current)
	if err != nil {
		return "", "", err
	}
	ours := map[string]string{}
	if err := json.Unmarshal([]byte(rec.OwnedJSON), &ours); err != nil {
		return "", "", fmt.Errorf("generated.yaml 里的 owned_json 坏了: %w", err)
	}
	container, _ := root[rec.Owned].(map[string]any)

	found := map[string]string{}
	for name, want := range ours {
		cur, ok := container[name]
		if !ok {
			found[name] = want
			continue
		}
		canon, cerr := canonical(cur)
		if cerr != nil {
			return "", "", cerr
		}
		found[name] = canon
		if canon == want {
			delete(container, name)
		}
	}
	if len(container) == 0 {
		delete(root, rec.Owned)
	} else {
		root[rec.Owned] = container
	}

	ownedNow = rec.OwnedJSON
	for name, want := range ours {
		if found[name] != want {
			b, _ := json.Marshal(found)
			ownedNow = string(b)
			break
		}
	}
	cleaned, err = marshalJSONObject(root)
	return cleaned, ownedNow, err
}

func parseJSONObject(current string) (map[string]any, error) {
	root := map[string]any{}
	if strings.TrimSpace(current) == "" {
		return root, nil
	}
	if err := json.Unmarshal([]byte(current), &root); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	return root, nil
}

// marshalJSONObject 序列化；根对象空了返回空串，表示这个文件已经没有别人的内容。
func marshalJSONObject(root map[string]any) (string, error) {
	if len(root) == 0 {
		return "", nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

// sameEntrySets 比较两组 hook 条目是否完全一致（顺序无关）。
func sameEntrySets(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for event, la := range a {
		lb := b[event]
		if len(la) != len(lb) {
			return false
		}
		sa := append([]string{}, la...)
		sb := append([]string{}, lb...)
		sort.Strings(sa)
		sort.Strings(sb)
		for i := range sa {
			if sa[i] != sb[i] {
				return false
			}
		}
	}
	return true
}
