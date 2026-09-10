package store

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 是 .keel/keel.yaml。核心字段严格校验，拼错的策略名直接报错。
type Config struct {
	Version   int             `yaml:"version"`
	Tools     []string        `yaml:"tools"`
	Workflow  WorkflowConfig  `yaml:"workflow"`
	Brief     BriefConfig     `yaml:"brief"`
	Gate      GateConfig      `yaml:"gate"`
	Sync      SyncConfig      `yaml:"sync"`
	MCP       MCPConfig       `yaml:"mcp"`
	Knowledge KnowledgeConfig `yaml:"knowledge"`
}

type WorkflowConfig struct {
	DefaultLevel int            `yaml:"default_level"`
	AutoPromote  bool           `yaml:"auto_promote"`
	Actions      map[string]int `yaml:"actions"`
	Paths        map[string]int `yaml:"paths"`
}

type BriefConfig struct {
	MaxBytes     int `yaml:"max_bytes"`
	MaxDecisions int `yaml:"max_decisions"`
	MaxNotes     int `yaml:"max_notes"`
}

type GateConfig struct {
	RequireTrailer    bool     `yaml:"require_trailer"`
	Manifests         []string `yaml:"manifests"`
	WatchTopLevelDirs bool     `yaml:"watch_top_level_dirs"`
}

type SyncConfig struct {
	CommitOutputs bool   `yaml:"commit_outputs"`
	SkillsMode    string `yaml:"skills_mode"`
}

type MCPConfig struct {
	Servers map[string]MCPServer `yaml:"servers"`
}

type MCPServer struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	EnvVars []string `yaml:"env_vars"`
}

type KnowledgeConfig struct {
	Docs    []string `yaml:"docs"`
	Exclude []string `yaml:"exclude"`
}

// DefaultConfig 是 keel init 写出的配置。
func DefaultConfig() Config {
	return Config{
		Version: 1,
		Tools:   []string{},
		Workflow: WorkflowConfig{
			DefaultLevel: 1,
			AutoPromote:  false,
			Actions: map[string]int{
				"dependency.add":    1,
				"data.delete":       0,
				"public-api.change": 0,
				"security":          0,
			},
			Paths: map[string]int{},
		},
		Brief: BriefConfig{MaxBytes: 16384, MaxDecisions: 8, MaxNotes: 8},
		Gate: GateConfig{
			RequireTrailer:    false,
			Manifests:         []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml", "requirements*.txt"},
			WatchTopLevelDirs: true,
		},
		Sync:      SyncConfig{CommitOutputs: true, SkillsMode: "copy"},
		MCP:       MCPConfig{Servers: map[string]MCPServer{}},
		Knowledge: KnowledgeConfig{Docs: []string{"README.md", "docs/**/*.md"}},
	}
}

// LoadConfig 读取并校验 keel.yaml。
func (s *Store) LoadConfig() (Config, error) {
	data, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("缺少 %s", s.Rel(s.ConfigPath()))
		}
		return Config{}, err
	}
	cfg := DefaultConfig()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("%s 解析失败: %w", s.Rel(s.ConfigPath()), err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", s.Rel(s.ConfigPath()), err)
	}
	return cfg, nil
}

// Validate 检查配置自身的合法性。auto_promote 在 MVP 必须为 false。
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("version 应为 1，实际 %d", c.Version)
	}
	if err := validLevel("workflow.default_level", c.Workflow.DefaultLevel); err != nil {
		return err
	}
	for k, v := range c.Workflow.Actions {
		if err := validLevel("workflow.actions."+k, v); err != nil {
			return err
		}
	}
	for k, v := range c.Workflow.Paths {
		if err := validLevel("workflow.paths."+k, v); err != nil {
			return err
		}
	}
	if c.Brief.MaxBytes <= 0 {
		return fmt.Errorf("brief.max_bytes 必须为正")
	}
	switch c.Sync.SkillsMode {
	case "copy", "link":
	default:
		return fmt.Errorf("sync.skills_mode %q 非法（应为 copy 或 link）", c.Sync.SkillsMode)
	}
	for name, srv := range c.MCP.Servers {
		if srv.Command == "" {
			return fmt.Errorf("mcp.servers.%s.command 不能为空", name)
		}
	}
	return nil
}

func validLevel(field string, v int) error {
	if v < 0 || v > 2 {
		return fmt.Errorf("%s 应为 0、1 或 2，实际 %d", field, v)
	}
	return nil
}

// Marshal 序列化配置，供 init 写文件。
func (c Config) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
