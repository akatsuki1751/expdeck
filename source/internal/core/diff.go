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
