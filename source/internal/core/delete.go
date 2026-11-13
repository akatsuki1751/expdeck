package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeleteExperiment refuses all dirty or ignored worktree files and leaves shared artifacts intact.
func (k *Kernel) DeleteExperiment(exp *Experiment) error {
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Unlock()
	current, err := k.Find(exp.ID)
	if err != nil {
		return err
	}
	runs, err := k.ListRuns(current)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status == RunRunning {
			return fmt.Errorf("%w: experiment has a running or unreconciled process", ErrConflict)
		}
	}
	trees, err := k.Attached()
	if err != nil {
		return err
	}
	for dir, branch := range trees {
		if branch != current.Branch {
			continue
		}
		if filepath.Clean(dir) == filepath.Clean(k.Repo.Root) {
			return fmt.Errorf("%w: cannot remove the current worktree", ErrConflict)
		}
		status, err := k.Repo.Git("-C", dir, "status", "--porcelain", "--ignored")
		if err != nil {
			return err
		}
		if strings.TrimSpace(status) != "" {
			return fmt.Errorf("%w: worktree contains modified, untracked, or ignored files: %s", ErrConflict, dir)
		}
		if err := k.Repo.WorktreeRemove(dir, false); err != nil {
			return err
		}
	}
	if k.Repo.BranchExists(current.Branch) {
		if _, err := k.Repo.Git("branch", "-D", "--", current.Branch); err != nil {
			return err
		}
	}
	// Snapshot refs and artifacts are retained so shared or historical data is not silently destroyed.
	return os.RemoveAll(k.Store.ExpDir(current.ID))
}
