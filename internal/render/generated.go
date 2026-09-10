// Package render 把 .keel/ 渲染成各工具的原生文件。
// 所有写入都先规划、再落盘；托管内容被人改过就报冲突，不覆盖。
package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// GeneratedSchema 是 generated.yaml 的 schema 版本。
const GeneratedSchema = 1

// Generated 是可提交的产物清单：谁拥有哪个文件的哪一部分，上次写成什么样。
type Generated struct {
	Schema        int               `yaml:"schema"`
	AdapterSchema map[string]string `yaml:"adapter_schema"`
	Files         []GenFile         `yaml:"files"`
}

// GenFile 记录一个产物文件里 keel 拥有的部分。
type GenFile struct {
	Path string `yaml:"path"`
	// Owned 说明拥有范围：整文件 "*"、标记块 "keel:begin..keel:end"、
	// 或 JSON 里的具体路径 "hooks.SessionStart"。
	Owned string `yaml:"owned"`
	// Digest 是上次写入时该范围内容的 sha256。人改过就对不上。
	Digest string `yaml:"digest"`
	// OwnedJSON 保存 keel 在 JSON 文件里写过的具体条目（canonical JSON）。
	// 有了它才能在不猜 command 前缀的前提下认出哪些条目是自己的。
	OwnedJSON string `yaml:"owned_json,omitempty"`
	// Source 是产物的来源对象，用于源被删时清理。
	Source  string `yaml:"source,omitempty"`
	Adapter string `yaml:"adapter,omitempty"`
}

// LoadGenerated 读取 generated.yaml；不存在时返回空清单。
func LoadGenerated(path string) (Generated, error) {
	g := Generated{Schema: GeneratedSchema, AdapterSchema: map[string]string{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return g, nil
	}
	if err != nil {
		return g, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&g); err != nil {
		return g, fmt.Errorf("%s 解析失败: %w", path, err)
	}
	if g.AdapterSchema == nil {
		g.AdapterSchema = map[string]string{}
	}
	return g, nil
}

// Marshal 序列化产物清单，文件按路径排序保证不同机器结果一致。
func (g Generated) Marshal() ([]byte, error) {
	g.Schema = GeneratedSchema
	sort.Slice(g.Files, func(i, j int) bool {
		if g.Files[i].Path != g.Files[j].Path {
			return g.Files[i].Path < g.Files[j].Path
		}
		return g.Files[i].Owned < g.Files[j].Owned
	})
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(g); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Record 找出某个文件某个范围的上次记录。
func (g Generated) Record(path, owned string) (GenFile, bool) {
	for _, f := range g.Files {
		if f.Path == path && f.Owned == owned {
			return f, true
		}
	}
	return GenFile{}, false
}

// Digest 计算托管内容的摘要。
func Digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}
