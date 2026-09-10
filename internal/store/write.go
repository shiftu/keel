package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateObject 以独占方式新建一个对象文件。文件已存在则报错（ID 碰撞在这里暴露）。
// 写入用同目录临时文件 + 原子替换，避免留下半份内容。
func (s *Store) CreateObject(id ID, slug string, fm any, body string) (string, error) {
	dir := s.SubDir(id.Kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName(id, slug))
	data, err := encodeFrontmatter(fm, body)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("%s 已存在（ID 碰撞）", s.Rel(path))
		}
		return "", err
	}
	_ = f.Close()
	if err := writeAtomic(path, data); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return s.Rel(path), nil
}

// ReplaceObject 覆盖已有对象文件（原子替换）。
func (s *Store) ReplaceObject(path string, fm any, body string) error {
	data, err := encodeFrontmatter(fm, body)
	if err != nil {
		return err
	}
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(s.Root, filepath.FromSlash(path))
	}
	return writeAtomic(full, data)
}

// writeAtomic 在同目录写临时文件再 rename，保证读者只看到完整内容。
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}

// WriteFileAtomic 供渲染产物使用：建目录 + 原子写。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeAtomic(path, data); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}
