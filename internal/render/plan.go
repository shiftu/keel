package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/store"
)

// Mode 决定 keel 在一个文件里拥有什么。
type Mode string

const (
	// ModeMarkdownBlock 只拥有 <!-- keel:begin --> 与 <!-- keel:end --> 之间。
	ModeMarkdownBlock Mode = "markdown-block"
	// ModeTOMLBlock 只拥有 # keel:begin 与 # keel:end 之间。
	ModeTOMLBlock Mode = "toml-block"
	// ModeWholeFile 整个文件归 keel（技能副本、生成的索引）。
	ModeWholeFile Mode = "whole-file"
	// ModeJSONHookEntries 只拥有 JSON hooks 数组里 keel 自己写过的那几条。
	ModeJSONHookEntries Mode = "json-hook-entries"
	// ModeJSONObjectKeys 只拥有某个 JSON 对象下 keel 写过的那几个键。
	ModeJSONObjectKeys Mode = "json-object-keys"
)

// Artifact 是一个待渲染的产物。
type Artifact struct {
	Path    string
	Mode    Mode
	Content string
	// Hooks 只在 ModeJSONHookEntries 使用：事件名 → keel 的条目。
	Hooks map[string][]any
	// JSONKey 与 Entries 只在 ModeJSONObjectKeys 使用：顶层对象名 → keel 的键值。
	JSONKey string
	Entries map[string]any
	// Guard 列出不允许出现在标记块之外的字符串（同名内容冲突检测）。
	Guard   []string
	Source  string
	Adapter string
}

// OpKind 是一次写入动作。
type OpKind string

const (
	OpCreate    OpKind = "create"
	OpUpdate    OpKind = "update"
	OpUnchanged OpKind = "unchanged"
	OpDelete    OpKind = "delete"
)

// Op 是一次具体的文件写入。
type Op struct {
	Kind OpKind
	Path string
	Data []byte
	Note string
}

// Conflict 是不能安全写入的地方。sync 会全部报出来，一个都不写。
type Conflict struct {
	Path   string
	Reason string
	Fix    string
}

// Plan 是一次 sync 的完整计划。
type Plan struct {
	Ops       []Op
	Conflicts []Conflict
	Next      Generated
}

// Changed 报告计划里是否有实际写入。
func (p *Plan) Changed() bool {
	for _, op := range p.Ops {
		if op.Kind != OpUnchanged {
			return true
		}
	}
	return false
}

// BuildPlan 先完整规划 diff 和冲突，再交给 Apply 写入。
func BuildPlan(root string, prev Generated, arts []Artifact, adapterSchema map[string]string) (*Plan, error) {
	p := &Plan{Next: Generated{Schema: GeneratedSchema, AdapterSchema: adapterSchema}}
	kept := map[string]bool{}

	sort.Slice(arts, func(i, j int) bool { return arts[i].Path < arts[j].Path })
	for _, a := range arts {
		full := filepath.Join(root, filepath.FromSlash(a.Path))
		current, existed, err := readFile(full)
		if err != nil {
			return nil, err
		}
		owned := ownedKey(a)
		kept[a.Path+"\x00"+owned] = true
		rec, hasRec := prev.Record(a.Path, owned)

		newContent, newOwned, conflict, err := renderOne(a, current, existed, rec, hasRec)
		if err != nil {
			return nil, err
		}
		if conflict != nil {
			p.Conflicts = append(p.Conflicts, *conflict)
			continue
		}
		kind := OpUpdate
		if !existed {
			kind = OpCreate
		} else if newContent == current {
			kind = OpUnchanged
		}
		p.Ops = append(p.Ops, Op{Kind: kind, Path: a.Path, Data: []byte(newContent)})
		gf := GenFile{
			Path: a.Path, Owned: owned, Digest: Digest(newOwned),
			Source: a.Source, Adapter: a.Adapter,
		}
		if a.Mode == ModeJSONHookEntries || a.Mode == ModeJSONObjectKeys {
			gf.OwnedJSON = newOwned
		}
		p.Next.Files = append(p.Next.Files, gf)
	}

	// 源对象被删：只清理仍与上次生成一致的产物，改过的保留并报冲突。
	for _, rec := range prev.Files {
		if kept[rec.Path+"\x00"+rec.Owned] {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(rec.Path))
		current, existed, err := readFile(full)
		if err != nil {
			return nil, err
		}
		if !existed {
			continue
		}
		cleaned, ownedNow, err := removeOwned(rec, current)
		if err != nil {
			p.Conflicts = append(p.Conflicts, Conflict{Path: rec.Path, Reason: err.Error(),
				Fix: "手工清理后再 sync"})
			continue
		}
		if Digest(ownedNow) != rec.Digest {
			p.Conflicts = append(p.Conflicts, Conflict{
				Path:   rec.Path,
				Reason: "源已删除，但产物在上次生成后被改过",
				Fix:    "确认要保留就把它移出托管范围；要删除就先还原成生成内容",
			})
			continue
		}
		if strings.TrimSpace(cleaned) == "" {
			p.Ops = append(p.Ops, Op{Kind: OpDelete, Path: rec.Path})
		} else {
			p.Ops = append(p.Ops, Op{Kind: OpUpdate, Path: rec.Path, Data: []byte(cleaned),
				Note: "移除已删除来源的 keel 块"})
		}
	}
	return p, nil
}

func ownedKey(a Artifact) string {
	switch a.Mode {
	case ModeMarkdownBlock, ModeTOMLBlock:
		return "keel:begin..keel:end"
	case ModeJSONHookEntries:
		return "hooks"
	case ModeJSONObjectKeys:
		return a.JSONKey
	default:
		return "*"
	}
}

func renderOne(a Artifact, current string, existed bool, rec GenFile, hasRec bool) (
	newContent, newOwned string, conflict *Conflict, err error) {
	switch a.Mode {
	case ModeWholeFile:
		if existed && !hasRec {
			return "", "", &Conflict{Path: a.Path,
				Reason: "已存在同名的非托管文件",
				Fix:    "改名或删除后再 sync；keel 不覆盖不是自己写的文件"}, nil
		}
		if existed && hasRec && Digest(current) != rec.Digest {
			return "", "", &Conflict{Path: a.Path,
				Reason: "托管文件在上次生成后被改过",
				Fix:    "把改动搬回 .keel/ 里的源，或删掉本地修改再 sync"}, nil
		}
		return a.Content, a.Content, nil, nil

	case ModeMarkdownBlock, ModeTOMLBlock:
		begin, end := MarkdownBegin, MarkdownEnd
		if a.Mode == ModeTOMLBlock {
			begin, end = TOMLBegin, TOMLEnd
		}
		b, ferr := findBlock(current, begin, end)
		if ferr != nil {
			return "", "", &Conflict{Path: a.Path, Reason: ferr.Error(),
				Fix: "修好标记块，或整段删掉让 keel 重写"}, nil
		}
		for _, g := range a.Guard {
			if strings.Contains(b.before, g) || strings.Contains(b.after, g) {
				return "", "", &Conflict{Path: a.Path,
					Reason: fmt.Sprintf("keel 块之外已经有 %s", g),
					Fix:    "把它挪进 .keel/keel.yaml 交给 keel 管理，或改名避开"}, nil
			}
		}
		if hasRec {
			if !b.found {
				return "", "", &Conflict{Path: a.Path,
					Reason: "托管的 keel 块被删掉了",
					Fix:    "确认要停止托管就删掉 .keel/generated.yaml 里的对应记录"}, nil
			}
			if Digest(b.inner) != rec.Digest {
				return "", "", &Conflict{Path: a.Path,
					Reason: "keel 块在上次生成后被手工改过",
					Fix:    "把改动搬回 .keel/，或还原块内容再 sync"}, nil
			}
		}
		out, rerr := renderBlock(current, begin, end, a.Content)
		if rerr != nil {
			return "", "", &Conflict{Path: a.Path, Reason: rerr.Error(), Fix: "修好标记块再 sync"}, nil
		}
		return out, strings.Trim(a.Content, "\n"), nil, nil

	case ModeJSONHookEntries:
		out, ownedJSON, c := renderHookEntries(a, current, existed, rec, hasRec)
		return out, ownedJSON, c, nil

	case ModeJSONObjectKeys:
		out, ownedJSON, c := renderJSONObjectKeys(a, current, existed, rec, hasRec)
		return out, ownedJSON, c, nil
	}
	return "", "", nil, fmt.Errorf("未知渲染模式 %q", a.Mode)
}

// removeOwned 在源被删除（或 keel 整个退出）时，从文件里去掉 keel 拥有的部分。
//
// 规则只有一条：去掉 keel 拥有的范围，剩下什么留什么；一个字节都不剩才删文件
// （cleaned 为空时由调用方删）。ownedNow 是现在文件里实际属于 keel 的那部分，
// 调用方拿它和上次生成的摘要比——对不上说明被人改过，报冲突而不是删。
func removeOwned(rec GenFile, current string) (cleaned, ownedNow string, err error) {
	switch {
	case rec.Owned == "*":
		return "", current, nil
	case rec.Owned == "keel:begin..keel:end":
		for _, pair := range [][2]string{{MarkdownBegin, MarkdownEnd}, {TOMLBegin, TOMLEnd}} {
			b, ferr := findBlock(current, pair[0], pair[1])
			if ferr != nil {
				return "", "", ferr
			}
			if b.found {
				return joinAroundBlock(b.before, b.after), b.inner, nil
			}
		}
		return current, "", nil
	case rec.Owned == "hooks" && rec.OwnedJSON != "":
		return removeOwnedHookEntries(rec, current)
	case rec.OwnedJSON != "":
		return removeOwnedObjectKeys(rec, current)
	}
	return current, "", fmt.Errorf("不知道怎么清理 %q 范围", rec.Owned)
}

// joinAroundBlock 把标记块两侧的内容接回去：去掉块和紧邻的空行，两侧都有内容时
// 中间留一个空行。直接拼接会把块前最后一行和块后第一行粘成一行。
func joinAroundBlock(before, after string) string {
	before = strings.TrimRight(before, "\n")
	after = strings.TrimLeft(after, "\n")
	out := before
	switch {
	case before != "" && after != "":
		out += "\n\n" + after
	default:
		out += after
	}
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func readFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

// Apply 落盘。调用前必须确认 Conflicts 为空。
func Apply(root string, p *Plan) error {
	if len(p.Conflicts) > 0 {
		return fmt.Errorf("还有 %d 处冲突，不写入", len(p.Conflicts))
	}
	for _, op := range p.Ops {
		full := filepath.Join(root, filepath.FromSlash(op.Path))
		switch op.Kind {
		case OpDelete:
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return err
			}
			pruneEmptyDirs(root, filepath.Dir(full))
		case OpCreate, OpUpdate:
			if err := store.WriteFileAtomic(full, op.Data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// pruneEmptyDirs 删掉产物走后留下的空目录，一路向上，止于仓库根或非空目录。
// 只会删空目录，所以不需要知道哪些目录是 keel 建的：有别人的东西就停。
func pruneEmptyDirs(root, dir string) {
	root = filepath.Clean(root)
	for {
		dir = filepath.Clean(dir)
		if dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
