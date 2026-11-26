package core

import (
	"fmt"
	"strings"
)

// SnapRef 是某个实验的快照引用名。
func (k *Kernel) SnapRef(exp *Experiment) string {
	return "refs/exp/snap/" + exp.ID
}

// Snapshot records the committed experiment branch HEAD without making a commit.
// The previous run's commit remains the comparison base, including legacy snapshots.
func (k *Kernel) Snapshot(exp *Experiment, worktree string) (commit, base string, err error) {
	dirty, err := k.Repo.IsDirty(worktree)
	if err != nil {
		return "", "", err
	}
	if dirty {
		return "", "", fmt.Errorf("%w: 工作树有未提交的修改或未跟踪文件，请先手动提交后再运行", ErrConflict)
	}
	branch, err := k.Repo.Git("-C", worktree, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != exp.Branch {
		return "", "", fmt.Errorf("%w: 工作树未检出实验分支 %q", ErrConflict, exp.Branch)
	}
	head, err := k.Repo.Git("-C", worktree, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("%w: 无法读取实验提交: %v", ErrConflict, err)
	}
	commit = strings.TrimSpace(head)
	runs, err := k.ListRuns(exp)
	if err != nil {
		return "", "", err
	}
	if len(runs) > 0 {
		base = runs[len(runs)-1].Snapshot
		if base == "" {
			return "", "", fmt.Errorf("%w: 上一次 run 缺少提交记录，无法比较", ErrConflict)
		}
	} else {
		base, _, err = k.ExperimentBase(exp)
		if err != nil {
			return "", "", err
		}
	}
	return commit, base, nil
}

// RunDiff 返回某个 run 自己的改动（上一个快照到本快照）。
func (k *Kernel) RunDiff(run *Run) (string, error) {
	return k.Repo.DiffRun(run.Base, run.Snapshot)
}

// RunDiffForExperiment handles legacy first runs whose stored base is empty.
func (k *Kernel) RunDiffForExperiment(exp *Experiment, run *Run) (string, error) {
	base := run.Base
	if base == "" {
		var err error
		base, _, err = k.ExperimentBase(exp)
		if err != nil {
			return "", err
		}
	}
	return k.Repo.DiffRun(base, run.Snapshot)
}
