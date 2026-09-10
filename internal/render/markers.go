package render

import (
	"fmt"
	"strings"
)

// 标记块：keel 只拥有两个标记之间的内容，块外一律原样保留。
const (
	MarkdownBegin = "<!-- keel:begin -->"
	MarkdownEnd   = "<!-- keel:end -->"
	TOMLBegin     = "# keel:begin"
	TOMLEnd       = "# keel:end"
)

// block 是一次标记块定位结果。
type block struct {
	before, inner, after string
	found                bool
}

// findBlock 定位标记块。只有一个标记是文件被改坏了，必须报错而不是猜。
func findBlock(content, begin, end string) (block, error) {
	i := strings.Index(content, begin)
	j := strings.Index(content, end)
	switch {
	case i < 0 && j < 0:
		return block{before: content}, nil
	case i < 0 || j < 0:
		return block{}, fmt.Errorf("只找到半个 keel 标记块（begin=%v end=%v）", i >= 0, j >= 0)
	case j < i:
		return block{}, fmt.Errorf("keel 标记块顺序颠倒")
	}
	inner := content[i+len(begin) : j]
	return block{
		before: content[:i],
		inner:  strings.Trim(inner, "\n"),
		after:  content[j+len(end):],
		found:  true,
	}, nil
}

// renderBlock 把新内容放回文件，块外内容不动。
func renderBlock(content, begin, end, inner string) (string, error) {
	b, err := findBlock(content, begin, end)
	if err != nil {
		return "", err
	}
	blockText := begin + "\n" + strings.Trim(inner, "\n") + "\n" + end
	if !b.found {
		if strings.TrimSpace(content) == "" {
			return blockText + "\n", nil
		}
		sep := "\n\n"
		if strings.HasSuffix(content, "\n\n") {
			sep = ""
		} else if strings.HasSuffix(content, "\n") {
			sep = "\n"
		}
		return content + sep + blockText + "\n", nil
	}
	after := b.after
	if !strings.HasSuffix(after, "\n") {
		after += "\n"
	}
	return b.before + blockText + after, nil
}
