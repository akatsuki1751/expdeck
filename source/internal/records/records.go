// Package records 负责数据根下的一切文件读写：路径、原子写、锁。
package records

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Store 指向 <common-dir>/exp。
type Store struct {
	Root string
}

// New 由 .git 目录构造。
func New(commonDir string) Store {
	return Store{Root: filepath.Join(commonDir, "exp")}
}

func (s Store) Records() string  { return filepath.Join(s.Root, "records") }
func (s Store) ExpsDir() string  { return filepath.Join(s.Records(), "exps") }
func (s Store) Registry() string { return filepath.Join(s.Root, "registry") }
func (s Store) Gpus() string     { return filepath.Join(s.Root, "gpus") }
func (s Store) Logs() string     { return filepath.Join(s.Root, "logs") }
func (s Store) Cache() string    { return filepath.Join(s.Root, "cache") }

// ExpDir 是一个实验的目录。
func (s Store) ExpDir(id string) string { return filepath.Join(s.ExpsDir(), id) }

// InitLayout 建立数据根的骨架目录。
func (s Store) InitLayout() error {
	for _, dir := range []string{s.ExpsDir(), s.Registry(), s.Gpus(), s.Logs(), s.Cache()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Exists 报告数据根是否已建立。
func (s Store) Exists() bool {
	fi, err := os.Stat(s.ExpsDir())
	return err == nil && fi.IsDir()
}

// ReadJSON 读一个 JSON 文件。
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// WriteJSON 原子地写一个 JSON 文件（带缩进与尾换行）。
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(path, append(b, '\n'))
}

// WriteFile 原子写：同目录临时文件 → fsync → rename。
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Lock 是一把基于锁文件的全库互斥锁。
type Lock struct{ path string }

// AcquireLock 取得锁；超过 stale 的锁文件会被当成残留清除。
func (s Store) AcquireLock() (*Lock, error) {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(s.Root, "lock")
	deadline := time.Now().Add(15 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return &Lock{path: path}, nil
		}
		if fi, statErr := os.Stat(path); statErr == nil && time.Since(fi.ModTime()) > 30*time.Second {
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for lock %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Unlock 释放锁。
func (l *Lock) Unlock() error {
	if l == nil {
		return nil
	}
	return os.Remove(l.path)
}
