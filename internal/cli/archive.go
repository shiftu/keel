package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/store"
)

// cmdArchive 把一条记忆转成 archived，与 cmdRetire 对称。
//
// 归档不是删除：文件留在 .keel/memory/，keel why --history 查得到，
// 只是退出知识索引主表和默认召回。见 design.md §12 M5。
//
// 这是记忆状态的**显式**入口。keel 不会因为「过期太久」自己归档任何东西——
// 时间不是证据（§8.1）。要找该归档的，跑 keel review 看 memory_archive_candidate。
func cmdArchive(e *env, args []string) error {
	fs := newFlagSet("archive")
	jsonOut := commonFlags(fs, e)
	reason := fs.String("reason", "", "为什么归档；会写进记忆正文")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := exactlyArgs("archive", rest, 1); err != nil {
		return usagef("用法：keel archive <M-…> --reason \"<原因>\"")
	}
	if strings.TrimSpace(*reason) == "" {
		return usagef("archive 需要 --reason：归档原因留在正文里，下一个人才知道这条为什么不再算数")
	}

	st, set, m, err := loadMemory(e, rest[0])
	if err != nil {
		return err
	}
	if m.Status == store.MemArchived {
		return usagef("%s 已经是 archived", m.ID.Short())
	}
	if err := store.CanTransitionMemory(m.Status, store.MemArchived); err != nil {
		return err
	}

	fm := m.MemoryFM
	fm.Status = store.MemArchived
	body := strings.TrimRight(m.Body(), "\n") +
		fmt.Sprintf("\n\n## 归档\n\n%s：%s\n", store.NewDate(time.Now()), strings.TrimSpace(*reason))
	if err := st.ReplaceObject(m.SourcePath(), fm, body); err != nil {
		return err
	}

	dependents := memoriesDerivedFrom(set, m.ID)
	if *jsonOut {
		data, jerr := json.MarshalIndent(map[string]any{
			"schema": 1, "memory": m.ID.String(), "status": string(store.MemArchived),
			"dependents": dependents,
		}, "", "  ")
		if jerr != nil {
			return jerr
		}
		e.io.Println(string(data))
		return nil
	}
	e.io.Println(m.SourcePath())
	if e.quiet {
		return nil
	}
	e.io.Errf("%s：%s → archived。归档原因已写进正文，历史保留。\n", m.ID.Short(), m.Status)
	for _, dep := range dependents {
		e.io.Errf("提示：%s 的 derived_from 指向它，转述可能也该一并处理。keel 不替你改。\n", dep)
	}
	return nil
}

// memoriesDerivedFrom 找出把这条记忆当来源的其他记忆。
// 归档一条被转述过的结论时，转述往往也失去依据——但那是人的判断，不是自动动作。
func memoriesDerivedFrom(set *store.Set, id store.ID) []string {
	var out []string
	for _, m := range set.Memories {
		if m.ID == id || m.Status == store.MemArchived {
			continue
		}
		for _, src := range m.DerivedFrom {
			if src == id {
				out = append(out, m.ID.Short())
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func loadMemory(e *env, ref string) (*store.Store, *store.Set, *store.Memory, error) {
	st, err := e.discover()
	if err != nil {
		return nil, nil, nil, err
	}
	set, err := st.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	id, err := store.ParseRef(ref, set.IDs())
	if err != nil {
		return nil, nil, nil, usagef("%v", err)
	}
	if id.Kind != store.KindMemory {
		return nil, nil, nil, usagef("需要一个记忆 ID（M-…），%s 是 %s", id.Short(), id.Kind)
	}
	o, ok := set.Lookup(id)
	if !ok {
		return nil, nil, nil, usagef("找不到 %s", id)
	}
	return st, set, o.(*store.Memory), nil
}
