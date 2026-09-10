package cli

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/shiftu/keel/internal/brief"
	"github.com/shiftu/keel/internal/store"
)

func cmdBrief(e *env, args []string) error {
	fs := newFlagSet("brief")
	jsonOut := commonFlags(fs, e)
	task := fs.String("task", "", "这次要做什么")
	path := listFlag(fs, "path", "相关路径，可重复或逗号分隔")
	action := fs.String("action", "", "动作类别，如 dependency.add")
	budget := fs.Int("budget", 0, "正文 UTF-8 字节上限（默认取 keel.yaml 的 brief.max_bytes）")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("brief", rest, 0); err != nil {
		return err
	}

	b, cfg, err := buildBrief(e, brief.Query{
		Task:   *task,
		Paths:  []string(*path),
		Action: *action,
	})
	if err != nil {
		return err
	}
	max := cfg.Brief.MaxBytes
	if *budget > 0 {
		max = *budget
	}
	if *jsonOut {
		data, err := json.MarshalIndent(b, "", "  ")
		if err != nil {
			return err
		}
		e.io.Println(string(data))
		return nil
	}
	e.io.Printf("%s", b.Text(max))
	return nil
}

func buildBrief(e *env, q brief.Query) (*brief.Brief, store.Config, error) {
	st, err := e.discover()
	if err != nil {
		return nil, store.Config{}, err
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return nil, store.Config{}, err
	}
	set, err := st.Load()
	if err != nil {
		return nil, store.Config{}, err
	}
	task, err := st.LoadTask()
	if err != nil {
		// 坏掉的接续摘要不该挡住整个 brief：它是缓存，不是事实来源。
		task = nil
		e.io.Errf("接续摘要读不出来，已忽略：%v\n", err)
	}
	in := brief.Input{
		Root:      st.Root,
		Config:    cfg,
		Set:       set,
		Query:     q,
		HasIntent: intentFilled(st),
		Task:      task,
		Head:      headOf(st.Root),
	}
	return brief.Build(in), cfg, nil
}

// intentFilled 报告 intent.md 是否已经填过内容（不只是骨架）。
func intentFilled(st *store.Store) bool {
	data, err := os.ReadFile(st.IntentPath())
	if err != nil {
		return false
	}
	// 骨架里每一项都是「待补充」。全都还在就当没填。
	return countOccurrences(string(data), "待补充") < 4
}

func countOccurrences(s, sub string) int { return strings.Count(s, sub) }
