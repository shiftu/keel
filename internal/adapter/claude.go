package adapter

import (
	"path/filepath"

	"github.com/shiftu/keel/internal/render"
)

type claude struct{}

func (c *claude) Name() string          { return "claude" }
func (c *claude) SchemaVersion() string { return "2026-09-10" }

func (c *claude) Capabilities(root string) []CapReport {
	bin, present := binaryPresent("claude")
	reps := []CapReport{
		{CapInstruction, Supported, "CLAUDE.md 标记块"},
		{CapSkill, Supported, ".claude/skills/"},
		{CapMCPStdio, Supported, ".mcp.json（官方支持 ${VAR} 展开）"},
	}
	hookSupport, detail := Unknown, "已写入 .claude/settings.json；是否真的触发要由一次实际会话确认"
	if !fileExists(filepath.Join(root, ".claude", "settings.json")) {
		hookSupport, detail = Unsupported, "还没有 .claude/settings.json，先跑 keel sync"
	}
	reps = append(reps,
		CapReport{CapSessionStart, hookSupport, detail},
		CapReport{CapStopContinue, hookSupport, detail})
	if !present {
		for i := range reps {
			if reps[i].Support == Supported {
				continue
			}
			reps[i].Detail += "；PATH 里没有 claude"
		}
	} else {
		reps = append(reps, CapReport{Capability: "binary", Support: Supported, Detail: bin})
	}
	return reps
}

func (c *claude) Artifacts(in Input) ([]render.Artifact, error) {
	arts := []render.Artifact{
		{
			Path:    "CLAUDE.md",
			Mode:    render.ModeMarkdownBlock,
			Content: in.Protocol,
			Adapter: c.Name(),
		},
		{
			Path:    ".claude/settings.json",
			Mode:    render.ModeJSONHookEntries,
			Adapter: c.Name(),
			Hooks: map[string][]any{
				"SessionStart": {hookEntry("keel hook claude session-start")},
				"Stop":         {hookEntry("keel hook claude stop")},
			},
		},
	}
	if in.SoftRules != "" {
		arts = append(arts, render.Artifact{
			Path:    ".claude/rules/keel.md",
			Mode:    render.ModeWholeFile,
			Content: in.SoftRules,
			Adapter: c.Name(),
		})
	}
	if entries := mcpEntries(in.Config); entries != nil {
		arts = append(arts, render.Artifact{
			Path:    ".mcp.json",
			Mode:    render.ModeJSONObjectKeys,
			JSONKey: "mcpServers",
			Entries: entries,
			Adapter: c.Name(),
		})
	}
	skills, err := skillArtifacts(in, c.Name(), ".claude/skills")
	if err != nil {
		return nil, err
	}
	return append(arts, skills...), nil
}
