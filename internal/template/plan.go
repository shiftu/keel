package template

import (
	"os"
	"path/filepath"
	"sort"
)

// Action 是一个文件在这次导入/更新里的处置。
type Action string

const (
	Unchanged       Action = "unchanged"
	Add             Action = "add"
	FastForward     Action = "fast-forward"
	KeepLocal       Action = "keep-local"
	Conflict        Action = "conflict"
	LocalDeleted    Action = "local-deleted"
	TemplateDeleted Action = "template-deleted"
)

// Writes 报告这个动作是否要落盘。
func (a Action) Writes() bool { return a == Add || a == FastForward }

// Change 是一个文件的处置结果。Path 相对 .keel/。
type Change struct {
	Path   string
	Action Action
	Data   []byte
	Note   string
}

// Plan 做三方比较：base 是 state 里记的导入基线，theirs 是模板新版，ours 是本地磁盘。
//
// 只有一种情况会写盘之外的判断：两边都相对基线变了，就一个字节都不写。
// 这是「模板更新不覆盖本地演化」的全部实现——没有自动合并，没有择一覆盖。
func Plan(state State, theirs map[string][]byte, keelDir string) ([]Change, error) {
	seen := map[string]bool{}
	var paths []string
	for p := range theirs {
		paths = append(paths, p)
		seen[p] = true
	}
	for _, f := range state.Files {
		if !seen[f.Path] {
			paths = append(paths, f.Path)
			seen[f.Path] = true
		}
	}
	sort.Strings(paths)

	var out []Change
	for _, p := range paths {
		theirData, inTheirs := theirs[p]
		base, hasBase := state.Base(p)

		ourData, err := os.ReadFile(filepath.Join(keelDir, filepath.FromSlash(p)))
		ourExists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}

		if !inTheirs {
			out = append(out, Change{Path: p, Action: TemplateDeleted,
				Note: "模板新版里没有了；本地这份保留不动"})
			continue
		}
		theirDigest := Digest(Normalize(p, theirData))

		if !ourExists {
			if hasBase {
				out = append(out, Change{Path: p, Action: LocalDeleted,
					Note: "本地已删除；不写回"})
			} else {
				out = append(out, Change{Path: p, Action: Add, Data: theirData})
			}
			continue
		}

		ourDigest := Digest(Normalize(p, ourData))
		if ourDigest == theirDigest {
			out = append(out, Change{Path: p, Action: Unchanged})
			continue
		}
		if !hasBase {
			out = append(out, Change{Path: p, Action: KeepLocal,
				Note: "本地已有同名文件，内容不同；未覆盖"})
			continue
		}
		switch {
		case ourDigest == base:
			data := theirData
			if p == ConfigPath {
				data = MergeConfig(theirData, ourData)
			}
			out = append(out, Change{Path: p, Action: FastForward, Data: data})
		case theirDigest == base:
			out = append(out, Change{Path: p, Action: KeepLocal,
				Note: "本地演化；模板这一版没动它"})
		default:
			out = append(out, Change{Path: p, Action: Conflict,
				Note: "本地和模板都相对上次导入改过；未写入"})
		}
	}
	return out, nil
}

// Apply 落盘可以写的那些，并把基线推进到模板新版。
// 冲突的文件基线不动——它得一直报到人处理为止。
func Apply(state *State, changes []Change, theirs map[string][]byte, keelDir string) error {
	for _, c := range changes {
		if c.Action.Writes() {
			dst := filepath.Join(keelDir, filepath.FromSlash(c.Path))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := writeAtomic(dst, c.Data); err != nil {
				return err
			}
		}
		if c.Action == Conflict || c.Action == TemplateDeleted {
			continue
		}
		if data, ok := theirs[c.Path]; ok {
			state.SetBase(c.Path, Digest(Normalize(c.Path, data)))
		}
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".keel-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Counts 汇总各类动作的条数，供输出与退出码判断。
func Counts(changes []Change) map[Action]int {
	m := map[Action]int{}
	for _, c := range changes {
		m[c.Action]++
	}
	return m
}
