package store

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TaskSchema 是接续摘要的 schema 版本。
const TaskSchema = 1

// Task 是一次任务的接续摘要。
//
// 它住在 cache/ 里，**不进 git**：这是同一个 worktree 上 Claude 与 Codex 之间的交接，
// 不是长期记忆。删掉它不会撤销任何已生效的知识，也不会丢掉证据。
// 要跨 clone 保留的经验走 keel note / keel decide。
type Task struct {
	Schema     int      `yaml:"schema"`
	Goal       string   `yaml:"goal"`
	BaseCommit string   `yaml:"base_commit"`
	Done       []string `yaml:"done"`
	Next       []string `yaml:"next"`
	Failing    []string `yaml:"failing"`
	Refs       []string `yaml:"refs"`
	UpdatedAt  string   `yaml:"updated_at"`
	UpdatedBy  string   `yaml:"updated_by"`
}

// TaskPath 是接续摘要的位置。每个 worktree 有自己的 .keel/cache/，天然互不干扰。
func (s *Store) TaskPath() string {
	return filepath.Join(s.CacheDir(), "task", "current.yaml")
}

// LoadTask 读接续摘要。没有摘要返回 (nil, nil)——那不是错误。
func (s *Store) LoadTask() (*Task, error) {
	data, err := os.ReadFile(s.TaskPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t Task
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("%s 解析失败：%w", s.Rel(s.TaskPath()), err)
	}
	if t.Schema != TaskSchema {
		return nil, fmt.Errorf("%s 的 schema 是 %d，本版 keel 只认 %d",
			s.Rel(s.TaskPath()), t.Schema, TaskSchema)
	}
	return &t, nil
}

// SaveTask 原子写入接续摘要。
func (s *Store) SaveTask(t *Task) error {
	t.Schema = TaskSchema
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.TaskPath(), data, 0o644)
}

// ClearTask 删除接续摘要。本来就没有也算成功。
func (s *Store) ClearTask() error {
	err := os.Remove(s.TaskPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
