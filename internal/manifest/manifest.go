// Package manifest 解析依赖清单，得到结构化的依赖集合。
// 目的是把「格式调整」「版本变更」「新增依赖」区分开——
// 目录 diff 做不到这件事，而新增依赖是架构腐化最常见的入口。
package manifest

import (
	"encoding/json"
	"path"
	"strings"
)

// Dep 是一条依赖。
type Dep struct {
	Name     string
	Version  string
	Indirect bool
	// Group 是它在清单里的位置，如 require / dependencies / devDependencies。
	Group string
}

// Set 是一份清单解析出的依赖集合。
type Set struct {
	Kind string
	Deps map[string]Dep
}

// Supported 报告 keel 是否会解析这个清单。不支持的只报「待判断信号」，
// 不谎称能证明新增依赖。
func Supported(p string) bool {
	switch path.Base(p) {
	case "go.mod", "package.json":
		return true
	}
	return false
}

// Parse 解析清单内容。data 为 nil 表示该版本没有这个文件。
func Parse(p string, data []byte) (Set, bool) {
	if data == nil {
		return Set{Kind: path.Base(p), Deps: map[string]Dep{}}, Supported(p)
	}
	switch path.Base(p) {
	case "go.mod":
		return parseGoMod(data), true
	case "package.json":
		return parsePackageJSON(data)
	}
	return Set{}, false
}

func parseGoMod(data []byte) Set {
	set := Set{Kind: "go.mod", Deps: map[string]Dep{}}
	inBlock := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := raw
		indirect := strings.Contains(line, "// indirect")
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case inBlock && line == ")":
			inBlock = false
		case strings.HasPrefix(line, "require ("):
			inBlock = true
		case strings.HasPrefix(line, "require "):
			addGoDep(set, strings.TrimSpace(strings.TrimPrefix(line, "require ")), indirect)
		case inBlock:
			addGoDep(set, line, indirect)
		}
	}
	return set
}

func addGoDep(set Set, line string, indirect bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	set.Deps[fields[0]] = Dep{Name: fields[0], Version: fields[1], Indirect: indirect, Group: "require"}
}

func parsePackageJSON(data []byte) (Set, bool) {
	set := Set{Kind: "package.json", Deps: map[string]Dep{}}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		// 语法坏了：不假装解析成功。
		return set, false
	}
	for _, group := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
		raw, ok := doc[group]
		if !ok {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return set, false
		}
		for name, ver := range m {
			set.Deps[name] = Dep{Name: name, Version: ver, Group: group,
				Indirect: group == "optionalDependencies"}
		}
	}
	return set, true
}

// Diff 是两份清单之间的变化。
type Diff struct {
	Added   []Dep
	Removed []Dep
	// Changed 是版本变了但依赖还在。
	Changed []Dep
}

// Empty 报告是否没有依赖集合层面的变化（纯格式调整会落在这里）。
func (d Diff) Empty() bool { return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0 }

// DirectAdded 只返回直接依赖的新增。间接依赖是传递结果，不要求单独解释。
func (d Diff) DirectAdded() []Dep {
	var out []Dep
	for _, dep := range d.Added {
		if !dep.Indirect {
			out = append(out, dep)
		}
	}
	return out
}

// Compare 比较两份清单。
func Compare(before, after Set) Diff {
	var d Diff
	for name, dep := range after.Deps {
		old, ok := before.Deps[name]
		switch {
		case !ok:
			d.Added = append(d.Added, dep)
		case old.Version != dep.Version:
			d.Changed = append(d.Changed, dep)
		}
	}
	for name, dep := range before.Deps {
		if _, ok := after.Deps[name]; !ok {
			d.Removed = append(d.Removed, dep)
		}
	}
	sortDeps(d.Added)
	sortDeps(d.Removed)
	sortDeps(d.Changed)
	return d
}

func sortDeps(list []Dep) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Name < list[j-1].Name; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}
