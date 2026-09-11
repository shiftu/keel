package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/store"
)

// shell 补全的大脑。
//
// 为什么补全逻辑写在 Go 里、而不是写进各家 shell 的脚本：候选项有一半是活的——
// `keel promote` 能提哪几条规则、`keel verify` 能验哪几个对象，是这个仓库 .keel/ 里
// 当下有什么决定的。写死在 shell 脚本里，仓库一变补全就开始撒谎，而且用户不会发现
// （补全不报错，只是补错）。所以四家 shell 的脚本都只是十行胶水，每敲一次 Tab
// 就回头问一次二进制本身。

// completionLimit 是一次最多给多少候选。仓库大了 `--path` 能列出几千个条目，
// 那不叫补全，那叫刷屏。
const completionLimit = 200

// complete 算出补全候选。
//
// words 是 keel 后面的全部词，最后一个是光标所在的那个（可能是空串）。
// 约定「最后一个总是正在敲的词」，是因为四家 shell 取当前词的方式各不相同，
// 与其在每个脚本里各写一套，不如让脚本都按同一个形状把词递进来。
//
// dir 是当前目录，用来找 .keel/ 和补路径。
func complete(words []string, dir string) []valueSpec {
	words = normalizeWords(words)
	if len(words) == 0 {
		words = []string{""}
	}
	cur, done := words[len(words)-1], words[:len(words)-1]

	// `--` 之后是交给别人跑的命令（keel verify … -- go test），不是 keel 的词。
	for _, w := range done {
		if w == "--" {
			return nil
		}
	}

	list := specs()
	if len(done) == 0 {
		if strings.HasPrefix(cur, "-") {
			return matchPrefix([]valueSpec{
				{"--help", "命令总览"},
				{"-C", "指定仓库目录"},
			}, cur)
		}
		return matchPrefix(visibleCommands(list), cur)
	}

	c, ok := lookupSpec(list, done[0])
	if !ok {
		return nil
	}
	rest := done[1:]

	// 分发型命令（task / template）：先把子命令敲完，才谈得上选项。
	if len(c.Subs) > 0 {
		if len(rest) == 0 {
			return matchPrefix(visibleCommands(c.Subs), cur)
		}
		sub, ok := lookupSpec(c.Subs, rest[0])
		if !ok {
			return nil
		}
		c, rest = sub, rest[1:]
	}

	flags := append(c.Flags, commonSpec()...)

	// 上一个词是个要取值的选项，那现在敲的就是它的值。
	if len(rest) > 0 {
		if f, ok := flagByWord(flags, rest[len(rest)-1]); ok && f.Arg != "" && !strings.Contains(rest[len(rest)-1], "=") {
			return flagValues(f, cur, dir)
		}
	}

	if strings.HasPrefix(cur, "-") {
		// --tag=a 这种连写：等号左边决定候选，补出来的必须是整个词。
		if name, tail, found := strings.Cut(cur, "="); found {
			f, ok := flagByWord(flags, name)
			if !ok || f.Arg == "" {
				return nil
			}
			var out []valueSpec
			for _, v := range flagValues(f, tail, dir) {
				out = append(out, valueSpec{name + "=" + v.Name, v.Desc})
			}
			return out
		}
		var out []valueSpec
		for _, f := range flags {
			prefix := "--"
			if len(f.Name) == 1 {
				prefix = "-"
			}
			out = append(out, valueSpec{prefix + f.Name, f.Desc})
		}
		return matchPrefix(out, cur)
	}

	// 位置参数。已经给过一个、且这个命令只收一个，就不再提示。
	if !c.Repeat && hasPositional(flags, rest) {
		return nil
	}
	return dynValues(c.Arg, cur, dir)
}

// visibleCommands 把命令表转成候选，隐藏的不出现。
func visibleCommands(list []cmdSpec) []valueSpec {
	var out []valueSpec
	for _, c := range list {
		if c.Hidden {
			continue
		}
		out = append(out, valueSpec{c.Name, c.Summary})
	}
	return out
}

// flagValues 给一个选项的取值补全。逗号分隔的（--tag a,b）只补最后一段，
// 但补出来的要带上前面几段——否则 shell 会把已经敲好的部分吃掉。
func flagValues(f flagSpec, cur, dir string) []valueSpec {
	base := func(prefix string) []valueSpec {
		if len(f.Values) > 0 {
			return matchPrefix(f.Values, prefix)
		}
		return dynValues(f.Dyn, prefix, dir)
	}
	if !f.List {
		return base(cur)
	}
	i := strings.LastIndex(cur, ",")
	if i < 0 {
		return base(cur)
	}
	head, tail := cur[:i+1], cur[i+1:]
	used := map[string]bool{}
	for _, s := range strings.Split(cur[:i], ",") {
		used[s] = true
	}
	var out []valueSpec
	for _, v := range base(tail) {
		if used[v.Name] {
			continue
		}
		out = append(out, valueSpec{head + v.Name, v.Desc})
	}
	return out
}

// dynValues 现场算一类候选。
func dynValues(d dyn, prefix, dir string) []valueSpec {
	switch d {
	case dynPath:
		return completePaths(prefix, dir)
	case dynRule:
		return completeIDs(dir, prefix, store.KindRule)
	case dynMemory:
		return completeIDs(dir, prefix, store.KindMemory)
	case dynDecide:
		return completeIDs(dir, prefix, store.KindDecision)
	case dynObject:
		return completeIDs(dir, prefix, store.KindMemory, store.KindDecision)
	case dynShell:
		return matchPrefix(shellValues(), prefix)
	}
	return nil
}

// hasPositional 判断已经敲过位置参数没有。跳过选项和它们的取值。
func hasPositional(flags []flagSpec, rest []string) bool {
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if strings.HasPrefix(a, "-") {
			if f, ok := flagByWord(flags, a); ok && f.Arg != "" && !strings.Contains(a, "=") {
				i++ // 下一个词是它的值，不是位置参数
			}
			continue
		}
		return true
	}
	return false
}

// flagByWord 按词找选项，-x / --x / --x=v 都认。
func flagByWord(flags []flagSpec, word string) (flagSpec, bool) {
	if !strings.HasPrefix(word, "-") {
		return flagSpec{}, false
	}
	name, _, _ := strings.Cut(strings.TrimLeft(word, "-"), "=")
	for _, f := range flags {
		if f.Name == name {
			return f, true
		}
	}
	return flagSpec{}, false
}

// matchPrefix 按前缀筛候选。
func matchPrefix(items []valueSpec, prefix string) []valueSpec {
	var out []valueSpec
	for _, v := range items {
		if strings.HasPrefix(v.Name, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// normalizeWords 把 bash 拆碎的词拼回去。
//
// bash 按 COMP_WORDBREAKS 断词，其中含 '='，所以 `--tag=db` 递过来是三个词
// "--tag" "=" "db"。zsh / fish 不拆。在这里统一成一个词，后面的逻辑就只有一种形状。
func normalizeWords(words []string) []string {
	var out []string
	for _, w := range words {
		n := len(out)
		switch {
		case n > 0 && strings.HasPrefix(out[n-1], "-") && w == "=":
			out[n-1] += "="
		case n > 0 && strings.HasPrefix(out[n-1], "-") && strings.HasSuffix(out[n-1], "="):
			out[n-1] += w
		default:
			out = append(out, w)
		}
	}
	return out
}

// completePaths 补文件系统路径。自己补而不是让 shell 补，是因为四家 shell 里
// 只有 bash 能在「我没给候选」时干净地退回文件名补全；其余三家要么不退，
// 要么退了之后把已经给的候选一起吃掉。自己补，四家的行为才是同一个。
func completePaths(prefix, dir string) []valueSpec {
	sub, base := "", prefix
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		sub, base = prefix[:i+1], prefix[i+1:]
	}
	entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(sub)))
	if err != nil {
		return nil
	}
	var out []valueSpec
	for _, e := range entries {
		name := e.Name()
		// 隐藏文件只在用户明确敲了点号时才出现。.keel/ 是例外：它就是这个工具的家。
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") && name != store.DirName {
			continue
		}
		if !strings.HasPrefix(name, base) {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		out = append(out, valueSpec{Name: sub + name})
		if len(out) >= completionLimit {
			break
		}
	}
	return out
}

// completeIDs 列出 .keel/ 里某几类对象的 ID。
//
// 给的是短前缀（R-1a2b3c4d）：keel 允许在命令行用短前缀，人也只记得住这么长。
// 用户已经敲了超过短前缀长度的，说明他在写完整 ID，那就按完整 ID 给。
func completeIDs(dir, prefix string, kinds ...store.Kind) []valueSpec {
	st, err := store.Discover(dir)
	if err != nil {
		return nil
	}
	var out []valueSpec
	for _, k := range kinds {
		entries, err := os.ReadDir(st.SubDir(k))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			id, slug, ok := idFromFilename(e.Name())
			if !ok {
				continue
			}
			name := id.Short()
			if len(prefix) > len(name) {
				name = id.String()
			}
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			out = append(out, valueSpec{name, slug})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > completionLimit {
		out = out[:completionLimit]
	}
	return out
}

// idFromFilename 从 <ID>-<slug>.md 里拆出 ID 和 slug。
func idFromFilename(name string) (store.ID, string, bool) {
	base := strings.TrimSuffix(name, ".md")
	// <Kind>-<uuid> 固定 38 字符：1 + 1 + 36。
	if len(base) < 38 {
		return store.ID{}, "", false
	}
	id, err := store.ParseID(base[:38])
	if err != nil {
		return store.ID{}, "", false
	}
	slug := ""
	if len(base) > 39 && base[38] == '-' {
		slug = base[39:]
	}
	return id, slug, true
}
