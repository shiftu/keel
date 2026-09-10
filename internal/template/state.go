// Package template 实现跨仓库复用：把模板仓库 .keel/ 的可复用部分导入本仓库，
// 把来源钉死在一个 commit 上，之后用三方比较做更新——模板给的是建议，不是既成事实。
package template

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// StateSchema 是 template.yaml 的 schema 版本。
const StateSchema = 1

// FileName 是状态文件在 .keel/ 下的名字。
const FileName = "template.yaml"

// State 是 .keel/template.yaml：来源、钉住的版本、以及每个文件导入当时的基线摘要。
//
// 基线摘要才是三方比较的真相；commit 只是出处。冲突的文件基线不推进，
// 所以「两边都变了」会一直报到人处理为止，不会被下一次 update 悄悄抹掉。
type State struct {
	Schema int    `yaml:"schema"`
	Source string `yaml:"source"`
	Ref    string `yaml:"ref"`
	Commit string `yaml:"commit"`
	Files  []File `yaml:"files"`
}

// File 是一个导入文件的基线。Path 相对 .keel/。
type File struct {
	Path   string `yaml:"path"`
	Digest string `yaml:"digest"`
}

// LoadState 读取 template.yaml；不存在时返回 ok=false。
func LoadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var s State
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return State{}, false, fmt.Errorf("%s 解析失败: %w", path, err)
	}
	if s.Schema != StateSchema {
		return State{}, false, fmt.Errorf("%s 的 schema 应为 %d，实际 %d", path, StateSchema, s.Schema)
	}
	return s, true, nil
}

// Marshal 序列化状态，文件按路径排序保证不同机器结果一致。
func (s State) Marshal() ([]byte, error) {
	s.Schema = StateSchema
	sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Base 返回某个文件的导入基线摘要。
func (s State) Base(path string) (string, bool) {
	for _, f := range s.Files {
		if f.Path == path {
			return f.Digest, true
		}
	}
	return "", false
}

// SetBase 推进（或新增）一个文件的基线。
func (s *State) SetBase(path, digest string) {
	for i := range s.Files {
		if s.Files[i].Path == path {
			s.Files[i].Digest = digest
			return
		}
	}
	s.Files = append(s.Files, File{Path: path, Digest: digest})
}

// Digest 是基线摘要的算法。和 render.Digest 同构，但这里比的是模板文件，
// 不依赖产物包，故不跨包引用。
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
