package core

import (
	"fmt"
	"os"
	"path/filepath"

	"expdeck/internal/records"
)

// WorktreePath 返回实验 worktree 的默认位置：
// 仓库同级的 <repoName>-exp/<路径分段...>，路径按 Path 分段拼，不用分支名。
func (k *Kernel) WorktreePath(exp *Experiment) string {
	repoName := filepath.Base(k.Repo.Root)
	base := filepath.Join(filepath.Dir(k.Repo.Root), repoName+"-exp")
	segs := append([]string{base}, exp.Path...)
	return filepath.Join(segs...)
}

// Attach 把实验分支检出到默认 worktree 目录；已挂载时返回既有目录。
func (k *Kernel) Attach(exp *Experiment) (string, error) {
	return k.AttachTo(exp, k.WorktreePath(exp))
}

// AttachTo 把实验分支检出到指定目录；dir 为空时用默认位置。
// 同一分支已有 worktree 时直接返回它，不报错。
func (k *Kernel) AttachTo(exp *Experiment, dir string) (string, error) {
	worktrees, err := k.Attached()
	if err != nil {
		return "", err
	}
	if existing, ok := findWorktree(worktrees, exp.Branch); ok {
		return existing, nil
	}
	if dir == "" {
		dir = k.WorktreePath(exp)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(k.Repo.Root, dir)
	}
	dir = filepath.Clean(dir)
	if _, err := os.Stat(dir); err == nil {
		return "", fmt.Errorf("%w: 目录 %q 已存在，但不是分支 %q 的 worktree", ErrConflict, dir, exp.Branch)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("%w: 无法检查目录 %q: %v", ErrConflict, dir, err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("%w: 无法创建目录 %q: %v", ErrConflict, filepath.Dir(dir), err)
	}
	if err := k.Repo.WorktreeAdd(dir, exp.Branch); err != nil {
		return "", fmt.Errorf("%w: 分支 %q 挂载失败: %v", ErrConflict, exp.Branch, err)
	}
	return dir, nil
}

// Detach 卸载实验的 worktree：先固化未提交改动，再强制删除目录；分支保留。
// 返回写出的 dirty.patch 路径；没有脏改动时返回空串。
func (k *Kernel) Detach(exp *Experiment) (string, error) {
	worktrees, err := k.Attached()
	if err != nil {
		return "", err
	}
	dir, ok := findWorktree(worktrees, exp.Branch)
	if !ok {
		return "", fmt.Errorf("%w: 实验 %q 没有挂载的 worktree", ErrNotFound, exp.PathString())
	}
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return "", fmt.Errorf("%w: 获取记录锁失败: %v", ErrConflict, err)
	}
	defer lock.Unlock()
	patchPath := ""
	dirty, err := k.Repo.IsDirty(dir)
	if err != nil {
		return "", fmt.Errorf("%w: 无法检查 worktree %q 的改动: %v", ErrConflict, dir, err)
	}
	if dirty {
		patch, err := k.Repo.DirtyPatch(dir)
		if err != nil {
			return "", fmt.Errorf("%w: 无法导出 worktree %q 的改动: %v", ErrConflict, dir, err)
		}
		patchPath = filepath.Join(k.Store.ExpDir(exp.ID), "dirty.patch")
		if err := records.WriteFile(patchPath, []byte(patch)); err != nil {
			return "", fmt.Errorf("%w: 无法写入 %q: %v", ErrConflict, patchPath, err)
		}
	}
	if err := k.Repo.WorktreeRemove(dir, true); err != nil {
		return "", fmt.Errorf("%w: 卸载 worktree 失败: %v", ErrConflict, err)
	}
	return patchPath, nil
}

// Attached 返回全部 worktree 的 目录 → 分支 映射。
func (k *Kernel) Attached() (map[string]string, error) {
	worktrees, err := k.Repo.Worktrees()
	if err != nil {
		return nil, fmt.Errorf("%w: 无法读取 worktree 列表: %v", ErrConflict, err)
	}
	return worktrees, nil
}

// findWorktree 在 目录 → 分支 映射里找某条分支的 worktree 目录。
func findWorktree(worktrees map[string]string, branch string) (string, bool) {
	for dir, b := range worktrees {
		if b == branch {
			return dir, true
		}
	}
	return "", false
}
