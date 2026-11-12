package core

import (
	"expdeck/internal/records"
	"fmt"
	"path/filepath"
	"strings"
)

// Rename changes the experiment path and branch without moving its worktree.
func (k *Kernel) Rename(exp *Experiment, newPath string) error {
	segs, err := ParsePath(newPath)
	if err != nil {
		return err
	}
	newBranch := "exp/" + strings.Join(segs, "/")
	if newBranch == exp.Branch {
		return nil
	}
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return fmt.Errorf("%w: 获取记录锁失败: %v", ErrConflict, err)
	}
	defer lock.Unlock()
	current, err := k.Find(exp.ID)
	if err != nil {
		return err
	}
	if current.Branch != exp.Branch {
		return fmt.Errorf("%w: 实验已被修改，请重试", ErrConflict)
	}
	running, err := k.RunningRun(current)
	if err != nil {
		return err
	}
	if running != nil {
		return fmt.Errorf("%w: 运行期间不能重命名实验", ErrConflict)
	}
	exps, err := k.List()
	if err != nil {
		return err
	}
	for _, other := range exps {
		if other.ID != exp.ID && (isPrefix(other.Path, segs) || isPrefix(segs, other.Path)) {
			return fmt.Errorf("%w: 路径与实验 %q 冲突", ErrConflict, other.PathString())
		}
	}
	if _, err := k.Repo.Git("check-ref-format", "--branch", newBranch); err != nil {
		return fmt.Errorf("%w: 无效分支名: %v", ErrInvalid, err)
	}
	if k.Repo.BranchExists(newBranch) {
		return fmt.Errorf("%w: 分支 %q 已存在", ErrConflict, newBranch)
	}
	oldBranch := current.Branch
	if _, err := k.Repo.Git("branch", "-m", oldBranch, newBranch); err != nil {
		return fmt.Errorf("%w: 重命名分支失败: %v", ErrConflict, err)
	}
	current.Path, current.Branch = segs, newBranch
	if err := records.WriteJSON(filepath.Join(k.Store.ExpDir(current.ID), "meta.json"), current); err != nil {
		if _, rollback := k.Repo.Git("branch", "-m", newBranch, oldBranch); rollback != nil {
			return fmt.Errorf("写入记录失败: %v；恢复分支失败，请手动恢复 %s -> %s: %v", err, newBranch, oldBranch, rollback)
		}
		return err
	}
	exp.Path, exp.Branch = current.Path, current.Branch
	return nil
}
