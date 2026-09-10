package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// readBody 解析 --body 契约：`-` 读 stdin 至 EOF，其他值当作文件路径。
func readBody(e *env, spec string) (string, error) {
	if spec == "-" {
		data, err := io.ReadAll(e.stdin)
		if err != nil {
			return "", fmt.Errorf("读取标准输入失败: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(spec)
	if err != nil {
		return "", fmt.Errorf("读取 --body 文件失败: %w", err)
	}
	return string(data), nil
}

// editBody 打开 $EDITOR 让人写正文。给人用，agent 走 --body -。
func editBody(initial string) (string, error) {
	editor := os.Getenv("KEEL_EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return "", fmt.Errorf("--edit 需要设置 $EDITOR（或用 --body - 从标准输入读正文）")
	}
	dir, err := os.MkdirTemp("", "keel-edit")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "KEEL_EDITMSG.md")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return "", err
	}
	parts := strings.Fields(editor)
	parts = append(parts, path)
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("编辑器退出异常: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// countLines 统计非空尾行之前的行数，用于正文长度上限。
func countLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
