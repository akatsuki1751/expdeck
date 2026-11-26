package core

import (
	"fmt"
	"strings"
)

// SnapRef 是某个实验的快照引用名。
func (k *Kernel) SnapRef(exp *Experiment) string {
	return "refs/exp/snap/" + exp.ID
}

// Snapshot 给工作树拍一张快照，并把引用移到最新的那张上。
// 引用不能省：没有引用指向的提交会被 git 的常规清理回收。
func (k *Kernel) Snapshot(exp *Experiment, worktree string) (commit, base string, err error) {
	if existing, verifyErr := k.Repo.Git("rev-parse", "--verify", "--quiet", k.SnapRef(exp)); verifyErr == nil {
		base = strings.TrimSpace(existing)
	}
	if base == "" {
		base, _, err = k.ExperimentBase(exp)
		if err != nil {
			return "", "", err
		}
	}
	commit, err = k.Repo.Snapshot(worktree, base)
	if err != nil {
		return "", "", err
	}
	if _, err := k.Repo.Git("update-ref", k.SnapRef(exp), commit); err != nil {
		return "", "", fmt.Errorf("更新快照引用失败: %w", err)
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
