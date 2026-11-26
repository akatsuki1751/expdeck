// diff.go 计算实验分支相对主干的改动，供网页展示。

package core

import (
	"fmt"
	"strings"
)

// BranchDiff 返回实验分支相对主干的 diff 文本。
//
// 主干名依次尝试 refs/heads/main、refs/heads/master，都没有则退回 HEAD；
// 然后取 `git merge-base <主干> <exp.Branch>`，再 diff merge-base..分支。
// 出错时返回空串与错误，由调用方决定怎么降级展示。
func (k *Kernel) BranchDiff(exp *Experiment) (string, error) {
	base := k.trunkRef()
	mergeBase, err := k.Repo.Git("merge-base", base, exp.Branch)
	if err != nil {
		return "", fmt.Errorf("%w: 取 %q 与 %q 的合并基点失败: %v", ErrConflict, base, exp.Branch, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)
	if mergeBase == "" {
		return "", fmt.Errorf("%w: 分支 %q 与主干 %q 没有合并基点", ErrConflict, exp.Branch, base)
	}
	out, err := k.Repo.Git("diff", mergeBase+".."+exp.Branch)
	if err != nil {
		return "", fmt.Errorf("%w: 读取分支 %q 的改动失败: %v", ErrConflict, exp.Branch, err)
	}
	return out, nil
}

// trunkRef 返回主干引用名：main、master，都没有时用 HEAD。
func (k *Kernel) trunkRef() string {
	for _, name := range []string{"main", "master"} {
		if k.Repo.BranchExists(name) {
			return name
		}
	}
	return "HEAD"
}

// ExperimentBase returns the recorded branch creation commit. Legacy records
// lack it, so their merge base with the current trunk is an explicit estimate.
func (k *Kernel) ExperimentBase(exp *Experiment) (string, bool, error) {
	if exp.StartCommit != "" {
		if _, err := k.Repo.Git("cat-file", "-e", exp.StartCommit+"^{commit}"); err != nil {
			return "", false, fmt.Errorf("%w: 实验起点不可用: %v", ErrConflict, err)
		}
		return exp.StartCommit, false, nil
	}
	runs, err := k.ListRuns(exp)
	if err != nil {
		return "", true, err
	}
	if len(runs) > 0 && runs[0].Base != "" {
		return runs[0].Base, true, nil
	}
	base, err := k.Repo.Git("merge-base", k.trunkRef(), exp.Branch)
	if err != nil || strings.TrimSpace(base) == "" {
		return "", true, fmt.Errorf("%w: 旧实验未记录起点，无法推测分支起点", ErrConflict)
	}
	return strings.TrimSpace(base), true, nil
}

// ExperimentRunDiff compares the experiment origin against its representative run.
func (k *Kernel) ExperimentRunDiff(exp *Experiment, run *Run) (string, bool, error) {
	base, inferred, err := k.ExperimentBase(exp)
	if err != nil {
		return "", inferred, err
	}
	diff, err := k.Repo.DiffRun(base, run.Snapshot)
	return diff, inferred, err
}
