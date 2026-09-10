package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/shiftu/keel/internal/store"
	"github.com/shiftu/keel/internal/template"
)

const templateUsage = `用法：
  keel template status [--json]
  keel template update [--to <ref>] [--dry-run] [--json]

首次导入用 keel init --from <git-url>[@<ref>]。`

func cmdTemplate(e *env, args []string) error {
	if len(args) == 0 {
		return usagef("%s", templateUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "status":
		return templateStatus(e, rest)
	case "update":
		return templateUpdate(e, rest)
	}
	return usagef("未知子命令 %q\n%s", sub, templateUsage)
}

func statePath(st *store.Store) string {
	return filepath.Join(st.KeelDir(), template.FileName)
}

// loadState 读状态文件；没导入过模板时如实报错，不假装有个空模板。
func loadState(st *store.Store) (template.State, error) {
	state, ok, err := template.LoadState(statePath(st))
	if err != nil {
		return state, err
	}
	if !ok {
		return state, fmt.Errorf("这个仓库没有从模板导入过（缺少 %s）；"+
			"首次导入用 keel init --from <git-url>", st.Rel(statePath(st)))
	}
	return state, nil
}

func templateStatus(e *env, args []string) error {
	fs := newFlagSet("template status")
	jsonOut := commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("template status", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return err
	}
	state, err := loadState(st)
	if err != nil {
		return err
	}

	// status 不联网：它回答的是「本地相对导入基线动过什么」，
	// 不是「模板那边有没有新版」。后者要 update 去问。
	drifted, missing := localDrift(st, state)

	if *jsonOut {
		return writeJSON(e, map[string]any{
			"source": state.Source, "ref": state.Ref, "commit": state.Commit,
			"files": len(state.Files), "drifted": drifted, "missing": missing,
		})
	}
	e.io.Printf("来源  %s\n", state.Source)
	e.io.Printf("ref   %s\n", state.Ref)
	e.io.Printf("钉住  %s\n", state.Commit)
	e.io.Printf("导入  %d 个文件\n", len(state.Files))
	if len(drifted) == 0 && len(missing) == 0 {
		e.io.Println("\n本地与导入基线一致。")
	}
	if len(drifted) > 0 {
		e.io.Printf("\n本地已改（%d）——update 时模板若也动了同一个文件就是冲突：\n", len(drifted))
		for _, p := range drifted {
			e.io.Printf("  ~ %s\n", p)
		}
	}
	if len(missing) > 0 {
		e.io.Printf("\n本地已删（%d）——update 不会写回：\n", len(missing))
		for _, p := range missing {
			e.io.Printf("  - %s\n", p)
		}
	}
	e.io.Println("\n下一步：keel template update --dry-run 看模板那边有没有新版")
	return nil
}

// localDrift 比对本地文件与导入基线。不联网。
func localDrift(st *store.Store, state template.State) (drifted, missing []string) {
	for _, f := range state.Files {
		data, err := readFileOrNil(filepath.Join(st.KeelDir(), filepath.FromSlash(f.Path)))
		if data == nil && err == nil {
			missing = append(missing, f.Path)
			continue
		}
		if err != nil {
			continue
		}
		if template.Digest(template.Normalize(f.Path, data)) != f.Digest {
			drifted = append(drifted, f.Path)
		}
	}
	sort.Strings(drifted)
	sort.Strings(missing)
	return drifted, missing
}

func templateUpdate(e *env, args []string) error {
	fs := newFlagSet("template update")
	jsonOut := commonFlags(fs, e)
	to := fs.String("to", "", "改用另一个 ref（默认沿用导入时记的那个）")
	dryRun := fs.Bool("dry-run", false, "只报告，不写入")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("template update", rest, 0); err != nil {
		return err
	}
	st, err := e.discover()
	if err != nil {
		return err
	}
	state, err := loadState(st)
	if err != nil {
		return err
	}
	ref := state.Ref
	if *to != "" {
		ref = *to
	}

	dir, commit, cleanup, err := template.Fetch(state.Source, ref)
	if err != nil {
		return err
	}
	defer cleanup()

	theirs, err := template.Surface(filepath.Join(dir, store.DirName),
		template.Provenance(state.Source, commit))
	if err != nil {
		return err
	}
	changes, err := template.Plan(state, theirs, st.KeelDir())
	if err != nil {
		return err
	}

	if !*dryRun {
		if err := template.Apply(&state, changes, theirs, st.KeelDir()); err != nil {
			return err
		}
		state.Ref, state.Commit = ref, commit
		data, err := state.Marshal()
		if err != nil {
			return err
		}
		if err := store.WriteFileAtomic(statePath(st), data, 0o644); err != nil {
			return err
		}
	}

	counts := template.Counts(changes)
	if *jsonOut {
		return writeJSON(e, map[string]any{
			"source": state.Source, "ref": ref, "commit": commit,
			"dry_run": *dryRun, "changes": changesJSON(changes),
		})
	}
	reportChanges(e, changes, commit, *dryRun)
	if counts[template.Conflict] > 0 {
		return checkFailed
	}
	return nil
}

func reportChanges(e *env, changes []template.Change, commit string, dryRun bool) {
	e.io.Printf("模板新版 %s\n\n", commit)
	quiet := map[template.Action]bool{template.Unchanged: true}
	shown := 0
	for _, c := range changes {
		if quiet[c.Action] {
			continue
		}
		shown++
		line := fmt.Sprintf("%-16s %s", c.Action, c.Path)
		if c.Note != "" {
			line += "\n                 " + c.Note
		}
		e.io.Println(line)
	}
	if shown == 0 {
		e.io.Println("无变化。")
		return
	}
	counts := template.Counts(changes)
	e.io.Println("")
	if dryRun {
		e.io.Println("--dry-run：什么都没写。")
	}
	if n := counts[template.Conflict]; n > 0 {
		e.io.Errf("%d 个文件本地和模板都改过，一个都没写。\n"+
			"  › 想要模板那一版：把本地改动挪走再 update；想留本地：不用管，下次还会报。\n", n)
	}
}

func changesJSON(changes []template.Change) []map[string]string {
	out := make([]map[string]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, map[string]string{
			"path": c.Path, "action": string(c.Action), "note": c.Note,
		})
	}
	return out
}

func writeJSON(e *env, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	e.io.Println(string(data))
	return nil
}

// readFileOrNil 把「文件不存在」和「读失败」区分开：前者返回 nil, nil。
func readFileOrNil(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}
