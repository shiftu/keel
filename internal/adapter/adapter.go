// Package adapter 只做三件事：能力探测、产物渲染、hook 事件编解码。
// 业务策略在 policy / evidence / check 里，不散落到各家模板中。
package adapter

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/shiftu/keel/internal/render"
	"github.com/shiftu/keel/internal/store"
)

// Support 是一项能力的确认程度。没法确认就说 unknown，
// 不能因为「文件写出去了」就宣称已启用。
type Support string

const (
	Supported   Support = "supported"
	Unsupported Support = "unsupported"
	Unknown     Support = "unknown"
)

// Capability 是适配器声明的可验证能力。
type Capability string

const (
	CapInstruction  Capability = "instruction"
	CapSkill        Capability = "skill"
	CapMCPStdio     Capability = "mcp-stdio"
	CapSessionStart Capability = "session-start"
	CapStopContinue Capability = "stop-continue"
)

// CapReport 是一项能力的探测结果。
type CapReport struct {
	Capability Capability `json:"capability"`
	Support    Support    `json:"support"`
	Detail     string     `json:"detail,omitempty"`
}

// Input 是渲染产物需要的一切。
type Input struct {
	Protocol  string // CLAUDE.md / AGENTS.md 里的稳定协议正文
	SoftRules string // 非执行的软规则摘要
	Config    store.Config
	Skills    fs.FS // 根下是 <skill-name>/...
}

// Adapter 是一家工具的适配器。
type Adapter interface {
	Name() string
	// SchemaVersion 标记这套产物的形状版本，记进 generated.yaml。
	SchemaVersion() string
	// Capabilities 探测本机状态。root 是仓库根。
	Capabilities(root string) []CapReport
	// Artifacts 生成待写入的产物。
	Artifacts(in Input) ([]render.Artifact, error)
}

// All 返回受支持的适配器。
func All() []Adapter { return []Adapter{&claude{}, &codex{}} }

// ByName 按名字取适配器。
func ByName(name string) (Adapter, bool) {
	for _, a := range All() {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}

// Detect 返回 PATH 里能找到的工具名。
func Detect() []string {
	var out []string
	for _, a := range All() {
		if _, err := exec.LookPath(binaryFor(a.Name())); err == nil {
			out = append(out, a.Name())
		}
	}
	sort.Strings(out)
	return out
}

func binaryFor(name string) string { return name }

func binaryPresent(name string) (string, bool) {
	p, err := exec.LookPath(binaryFor(name))
	return p, err == nil
}

// hookEntry 是 Claude 与 Codex 共用的 hook 条目形状。
func hookEntry(command string) any {
	return map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	}
}

// skillArtifacts 把 .keel/skills/ 复制到工具的技能目录。整文件归 keel。
func skillArtifacts(in Input, adapterName, dir string) ([]render.Artifact, error) {
	if in.Skills == nil {
		return nil, nil
	}
	var arts []render.Artifact
	err := fs.WalkDir(in.Skills, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "." {
			return err
		}
		data, rerr := fs.ReadFile(in.Skills, p)
		if rerr != nil {
			return rerr
		}
		arts = append(arts, render.Artifact{
			Path:    path.Join(dir, p),
			Mode:    render.ModeWholeFile,
			Content: string(data),
			Source:  path.Join(store.DirName, "skills", p),
			Adapter: adapterName,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取技能目录: %w", err)
	}
	return arts, nil
}

// mcpEntries 把 keel.yaml 的 MCP 配置转成 JSON 形状。只声明要转发的环境变量名，
// 不写明文密钥。
func mcpEntries(cfg store.Config) map[string]any {
	if len(cfg.MCP.Servers) == 0 {
		return nil
	}
	out := map[string]any{}
	for name, srv := range cfg.MCP.Servers {
		entry := map[string]any{"command": srv.Command}
		if len(srv.Args) > 0 {
			args := make([]any, len(srv.Args))
			for i, a := range srv.Args {
				args[i] = a
			}
			entry["args"] = args
		}
		if len(srv.EnvVars) > 0 {
			env := map[string]any{}
			for _, v := range srv.EnvVars {
				// Claude 官方支持 ${VAR} 展开。
				env[v] = "${" + v + "}"
			}
			entry["env"] = env
		}
		out[name] = entry
	}
	return out
}

func sortedNames(m map[string]store.MCPServer) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
