package store

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion 是当前对象 frontmatter 的 schema 版本。
const SchemaVersion = 1

// Date 是 civil date（YYYY-MM-DD），不带时区。
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

func NewDate(t time.Time) Date { return Date{t.Truncate(24 * time.Hour)} }

func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Format(dateLayout)
}

func (d *Date) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("日期应为 YYYY-MM-DD 字符串: %w", err)
	}
	if strings.TrimSpace(s) == "" {
		*d = Date{}
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("日期 %q 不是 YYYY-MM-DD", s)
	}
	*d = Date{t}
	return nil
}

func (d Date) MarshalYAML() (any, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

const fmDelim = "---"

// splitFrontmatter 把文件内容切成 frontmatter YAML 和正文。
func splitFrontmatter(data []byte) (fm []byte, body string, err error) {
	text := string(data)
	text = strings.TrimPrefix(text, "\ufeff")
	if !strings.HasPrefix(text, fmDelim+"\n") && text != fmDelim {
		return nil, "", fmt.Errorf("文件必须以 --- 开头的 YAML frontmatter 起始")
	}
	rest := strings.TrimPrefix(text, fmDelim+"\n")
	idx := strings.Index(rest, "\n"+fmDelim)
	if idx < 0 {
		return nil, "", fmt.Errorf("frontmatter 缺少结束的 ---")
	}
	fmText := rest[:idx]
	after := rest[idx+len("\n"+fmDelim):]
	after = strings.TrimPrefix(after, "\n")
	return []byte(fmText), after, nil
}

// decodeFrontmatter 严格解码核心字段：未知的核心字段名报错，不静默保留。
func decodeFrontmatter(fm []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(fm))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("frontmatter 解析失败: %w", err)
	}
	return nil
}

// encodeFrontmatter 把 frontmatter 与正文渲染成文件内容。
func encodeFrontmatter(v any, body string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(fmDelim + "\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("frontmatter 序列化失败: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	buf.WriteString(fmDelim + "\n")
	if body != "" {
		if !strings.HasPrefix(body, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteString("\n")
		}
	}
	return buf.Bytes(), nil
}
