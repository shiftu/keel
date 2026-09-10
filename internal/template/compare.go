package template

import (
	"bytes"

	"github.com/shiftu/keel/internal/store"
	"gopkg.in/yaml.v3"
)

// Normalize 返回用于比较的内容。
//
// keel.yaml 比的是**配置的含义**，不是字节：两边都解码成 store.Config、清掉 tools、
// 再用同一个编码器写回。两个原因缺一不可：
//
//   - tools 是本机探测结果，跟模板不一样是正常的。不清掉的话配置文件会永远停在
//     「本地已改」，再也收不到模板的策略更新。
//   - init 写本地 keel.yaml 时是把结构体序列化出去的，字段顺序和空值写法跟模板
//     手写的那份对不上。只清 tools 而不做规范化，第一次 update 就会把「什么都没改」
//     报成冲突。
//
// 解不开就退回原字节：宁可多报一次冲突，也不能把两份不同的配置当成一样。
func Normalize(p string, data []byte) []byte {
	if p != ConfigPath {
		return data
	}
	cfg := store.DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return data
	}
	cfg.Tools = nil
	out, err := cfg.Marshal()
	if err != nil {
		return data
	}
	return out
}

// MergeConfig 把模板的 keel.yaml 写进来，但保留本地的 tools。
func MergeConfig(theirs, ours []byte) []byte {
	localTools, ok := mapKey(ours, "tools")
	if !ok {
		return theirs
	}
	var node yaml.Node
	if err := yaml.Unmarshal(theirs, &node); err != nil {
		return theirs
	}
	if !setMapKey(&node, "tools", localTools) {
		return theirs
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return theirs
	}
	if err := enc.Close(); err != nil {
		return theirs
	}
	return buf.Bytes()
}

func docMapping(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

func mapKey(data []byte, key string) (*yaml.Node, bool) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, false
	}
	m := docMapping(&node)
	if m == nil {
		return nil, false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1], true
		}
	}
	return nil, false
}

func setMapKey(n *yaml.Node, key string, val *yaml.Node) bool {
	m := docMapping(n)
	if m == nil {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return true
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
	return true
}
