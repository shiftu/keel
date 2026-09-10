package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shiftu/keel/internal/gitx"
	"github.com/shiftu/keel/internal/store"
)

const taskUsage = `用法：
  keel task set --goal "<目标>" [--done "<已完成>"]… [--next "<下一步>"]…
                [--failing "<失败的验证>"]… [--ref <ID>]… [--by agent:claude]
  keel task show [--json]
  keel task clear`

func cmdTask(e *env, args []string) error {
	if len(args) == 0 {
		return usagef("%s", taskUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "set":
		return taskSet(e, rest)
	case "show":
		return taskShow(e, rest)
	case "clear":
		return taskClear(e, rest)
	}
	return usagef("未知子命令 %q\n%s", sub, taskUsage)
}

func taskSet(e *env, args []string) error {
	fs := newFlagSet("task set")
	_ = commonFlags(fs, e)
	goal := fs.String("goal", "", "这次任务的目标")
	by := fs.String("by", "", "谁写的，如 agent:claude")
	var done, next, failing, refs stringList
	fs.Var(&done, "done", "已完成事项（可重复；追加）")
	fs.Var(&next, "next", "下一步（可重复；整体替换）")
	fs.Var(&failing, "failing", "还在失败的验证（可重复；整体替换）")
	fs.Var(&refs, "ref", "相关对象 ID（可重复；整体替换）")

	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("task set", rest, 0); err != nil {
		return err
	}

	st, err := e.discover()
	if err != nil {
		return err
	}
	t, err := st.LoadTask()
	if err != nil {
		return err
	}
	if t == nil {
		t = &store.Task{}
	}
	if *goal == "" && t.Goal == "" {
		return usagef("第一次写接续摘要要给 --goal：没有目标的清单接不上任务")
	}

	if *goal != "" {
		t.Goal = *goal
	}
	// done 是流水，追加；其余三项描述当前状态，给了就整体替换。
	t.Done = append(t.Done, done...)
	if next != nil {
		t.Next = next
	}
	if failing != nil {
		t.Failing = failing
	}
	if refs != nil {
		set, lerr := st.Load()
		if lerr != nil {
			return lerr
		}
		checked, rerr := resolveRefs(set, refs)
		if rerr != nil {
			return rerr
		}
		t.Refs = checked
	}
	t.BaseCommit = headOf(st.Root)
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	t.UpdatedBy = *by
	if t.UpdatedBy == "" {
		t.UpdatedBy = "human"
	}
	if err := st.SaveTask(t); err != nil {
		return err
	}
	e.io.Println(st.Rel(st.TaskPath()))
	if !e.quiet {
		e.io.Errln("接续摘要在 cache/ 里，不进 git。要跨 clone 留下的经验用 keel note / keel decide。")
	}
	return nil
}

// resolveRefs 把短前缀补成完整 ID，顺便确认它们真的存在。
// 接不上的引用留在摘要里，下一个工具只会白找一遍。
func resolveRefs(set *store.Set, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	ids := set.IDs()
	for _, r := range refs {
		id, err := store.ParseRef(r, ids)
		if err != nil {
			return nil, usagef("--ref %s：%v", r, err)
		}
		if _, ok := set.Lookup(id); !ok {
			return nil, usagef("--ref %s 指向的对象不存在", r)
		}
		out = append(out, id.String())
	}
	return out, nil
}

func headOf(root string) string {
	repo, err := gitx.Open(root)
	if err != nil {
		return ""
	}
	sha, err := repo.HeadSHA()
	if err != nil {
		return ""
	}
	return sha
}

func taskShow(e *env, args []string) error {
	fs := newFlagSet("task show")
	jsonOut := commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("task show", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return err
	}
	t, err := st.LoadTask()
	if err != nil {
		return err
	}
	if *jsonOut {
		if t == nil {
			e.io.Println("null")
			return nil
		}
		data, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			return err
		}
		e.io.Println(string(data))
		return nil
	}
	if t == nil {
		e.io.Println("没有接续摘要。用 keel task set --goal \"…\" 开一个。")
		return nil
	}
	e.io.Printf("%s", TaskText(t, headOf(st.Root)))
	return nil
}

func taskClear(e *env, args []string) error {
	fs := newFlagSet("task clear")
	_ = commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("task clear", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return err
	}
	if err := st.ClearTask(); err != nil {
		return err
	}
	if !e.quiet {
		e.io.Errln("已清空接续摘要。已提交的决策、记忆和证据不受影响。")
	}
	return nil
}

// TaskText 把接续摘要渲染成人和 agent 都能读的段落。
// HEAD 变了就照实说明，不假装摘要还准确。
func TaskText(t *store.Task, head string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "目标：%s\n", t.Goal)
	list(&sb, "已完成", t.Done)
	list(&sb, "下一步", t.Next)
	list(&sb, "还在失败", t.Failing)
	list(&sb, "相关", t.Refs)
	fmt.Fprintf(&sb, "更新：%s by %s\n", t.UpdatedAt, t.UpdatedBy)
	switch {
	case t.BaseCommit == "":
		sb.WriteString("记录时仓库还没有提交。\n")
	case head != "" && head != t.BaseCommit:
		fmt.Fprintf(&sb, "记录于 %s，当前 HEAD 是 %s——中间有新提交，摘要可能已经落后。\n",
			shortSHA(t.BaseCommit), shortSHA(head))
	default:
		fmt.Fprintf(&sb, "记录于 %s（与当前 HEAD 一致）。\n", shortSHA(t.BaseCommit))
	}
	return sb.String()
}

func list(sb *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "%s：\n", label)
	for _, it := range items {
		fmt.Fprintf(sb, "  - %s\n", it)
	}
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
