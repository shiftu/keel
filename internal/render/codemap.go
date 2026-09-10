package render

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/store"
)

// CodemapPath 是目录概览产物的位置，进 git。
const CodemapPath = store.DirName + "/knowledge/CODEMAP.md"

// Codemap 渲染仓库的目录概览。
//
// 只列目录，不做 AST、符号表或调用图——那是 §10 明确不做的东西。
// 输入是 git 认的文件列表，不是工作树遍历：两台机器上 sync 出来的必须是同一份，
// 否则每次都是一个假 diff（和 KnowledgeIndex 同一个约束）。
func Codemap(files []string) string {
	dirs := map[string]map[string]int{}
	for _, f := range files {
		d := path.Dir(f)
		if d == "" {
			d = "."
		}
		if dirs[d] == nil {
			dirs[d] = map[string]int{}
		}
		dirs[d][kindOf(path.Base(f))]++
	}

	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString("# 目录概览\n\n")
	sb.WriteString("由 `keel sync --codemap` 生成，勿手改。\n")
	sb.WriteString("只列 git 登记的文件所在的目录，不含符号与调用关系。\n\n")
	if len(names) == 0 {
		sb.WriteString("（仓库里还没有登记任何文件。）\n")
		return sb.String()
	}
	sb.WriteString("| 目录 | 文件数 | 类型 |\n|---|---|---|\n")
	for _, d := range names {
		total := 0
		for _, n := range dirs[d] {
			total += n
		}
		fmt.Fprintf(&sb, "| %s | %d | %s |\n", cell(d), total, cell(topKinds(dirs[d])))
	}
	return sb.String()
}

// kindOf 把文件名归成一类：有扩展名就用扩展名，没有就用文件名本身
// （Makefile、Dockerfile 这些本来就是靠名字识别的）。
func kindOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[i:]
	}
	return name
}

// topKinds 按条数降序、同数按名字升序，取前四类。顺序确定，跨机器一致。
func topKinds(m map[string]int) string {
	type kv struct {
		k string
		n int
	}
	rows := make([]kv, 0, len(m))
	for k, n := range m {
		rows = append(rows, kv{k, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].k < rows[j].k
	})
	var parts []string
	for i, r := range rows {
		if i == 4 {
			parts = append(parts, fmt.Sprintf("…另 %d 类", len(rows)-4))
			break
		}
		parts = append(parts, fmt.Sprintf("%s×%d", r.k, r.n))
	}
	return strings.Join(parts, "、")
}
