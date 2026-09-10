// Package templates 内嵌 keel init 写出的静态内容。
package templates

import (
	"embed"
	"io/fs"
)

//go:embed skills intent.md
var files embed.FS

// Skills 返回内置技能的文件系统（根是 skills/）。
func Skills() fs.FS {
	sub, err := fs.Sub(files, "skills")
	if err != nil {
		panic(err)
	}
	return sub
}

// SkillNames 是内置技能的名字。
func SkillNames() []string {
	return []string{"keel-decide", "keel-learn", "keel-review"}
}

// Intent 是 intent.md 的骨架。keel 不猜项目目标，只留待补充清单。
func Intent() []byte {
	b, err := files.ReadFile("intent.md")
	if err != nil {
		panic(err)
	}
	return b
}
