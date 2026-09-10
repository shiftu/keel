package adapter

import (
	"path/filepath"
	"strings"

	"github.com/shiftu/keel/internal/render"
	"github.com/shiftu/keel/internal/store"
)

type codex struct{}

func (c *codex) Name() string          { return "codex" }
func (c *codex) SchemaVersion() string { return "2026-09-10" }

func (c *codex) Capabilities(root string) []CapReport {
	bin, present := binaryPresent("codex")
	reps := []CapReport{
		{CapInstruction, Supported, "AGENTS.md 标记块（Codex 有层级覆盖与体积上限，同样的文本不保证等价的有效上下文）"},
		{CapSkill, Supported, ".agents/skills/（发现规则与当前工作目录相关）"},
		{CapMCPStdio, Supported, ".codex/config.toml 的 [mcp_servers.*]，用 env_vars 声明转发变量"},
	}
	detail := "项目层 .codex/ 需要在 Codex 里走一次原生信任流程；新建或修改 hook 会影响信任状态。" +
		"文件写出去 ≠ hook 已启用，keel 不会绕过原生信任。"
	support := Unknown
	if !fileExists(filepath.Join(root, ".codex", "hooks.json")) {
		support = Unsupported
		detail = "还没有 .codex/hooks.json，先跑 keel sync"
	}
	reps = append(reps,
		CapReport{CapSessionStart, support, detail},
		CapReport{CapStopContinue, support, detail})
	if present {
		reps = append(reps, CapReport{Capability: "binary", Support: Supported, Detail: bin})
	}
	return reps
}

func (c *codex) Artifacts(in Input) ([]render.Artifact, error) {
	protocol := in.Protocol
	if in.SoftRules != "" {
		// Codex 没有对应的 rules 机制，软规则并进同一个块。
		protocol = protocol + "\n\n" + in.SoftRules
	}
	arts := []render.Artifact{
		{
			Path:    "AGENTS.md",
			Mode:    render.ModeMarkdownBlock,
			Content: protocol,
			Adapter: c.Name(),
		},
		{
			Path:    ".codex/hooks.json",
			Mode:    render.ModeJSONHookEntries,
			Adapter: c.Name(),
			Hooks: map[string][]any{
				"SessionStart": {hookEntry("keel hook codex session-start")},
				"Stop":         {hookEntry("keel hook codex stop")},
			},
		},
	}
	if len(in.Config.MCP.Servers) > 0 {
		arts = append(arts, render.Artifact{
			Path:    ".codex/config.toml",
			Mode:    render.ModeTOMLBlock,
			Content: codexMCPTOML(in.Config),
			Guard:   codexMCPGuards(in.Config),
			Adapter: c.Name(),
		})
	}
	skills, err := skillArtifacts(in, c.Name(), ".agents/skills")
	if err != nil {
		return nil, err
	}
	return append(arts, skills...), nil
}

// codexMCPTOML 渲染 [mcp_servers.*] 表。
// 用 env_vars 声明要转发的变量名，不写 env = { X = "${X}" } —— 那是 Claude 的语义，
// 不能假设 Codex 会做同样的展开。
func codexMCPTOML(cfg store.Config) string {
	var b strings.Builder
	for _, name := range sortedNames(cfg.MCP.Servers) {
		srv := cfg.MCP.Servers[name]
		b.WriteString("[mcp_servers." + name + "]\n")
		b.WriteString("command = " + tomlString(srv.Command) + "\n")
		if len(srv.Args) > 0 {
			parts := make([]string, len(srv.Args))
			for i, a := range srv.Args {
				parts[i] = tomlString(a)
			}
			b.WriteString("args = [" + strings.Join(parts, ", ") + "]\n")
		}
		if len(srv.EnvVars) > 0 {
			parts := make([]string, len(srv.EnvVars))
			for i, v := range srv.EnvVars {
				parts[i] = tomlString(v)
			}
			b.WriteString("env_vars = [" + strings.Join(parts, ", ") + "]\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// codexMCPGuards 让 sync 在同名服务器已经写在块外时报冲突。
func codexMCPGuards(cfg store.Config) []string {
	var g []string
	for _, name := range sortedNames(cfg.MCP.Servers) {
		g = append(g, "[mcp_servers."+name+"]")
	}
	return g
}
