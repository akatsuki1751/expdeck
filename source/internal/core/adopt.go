package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"expdeck/internal/records"
)

// Adopt 把一条已经存在、但没登记过的 exp/ 分支收编成实验。
// 幂等：分支已经有记录时直接把记录返回。
func (k *Kernel) Adopt(branch, why string) (*Experiment, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, fmt.Errorf("%w: 需要分支名", ErrInvalid)
	}
	if !strings.HasPrefix(branch, "exp/") {
		branch = "exp/" + NormalizePath(branch)
	}
	if !k.Repo.BranchExists(branch) {
		return nil, fmt.Errorf("%w: 分支 %s 不存在", ErrNotFound, branch)
	}
	path, err := ParsePath(strings.TrimPrefix(branch, "exp/"))
	if err != nil {
		return nil, err
	}
	exps, err := k.List()
	if err != nil {
		return nil, err
	}
	for _, exp := range exps {
		if exp.Branch == branch {
			return exp, nil
		}
	}
	for _, exp := range exps {
		if isPrefix(exp.Path, path) || isPrefix(path, exp.Path) {
			return nil, fmt.Errorf("%w: %q 与已登记的 %q 重叠", ErrConflict, branch, exp.PathString())
		}
	}
	hypothesis := strings.TrimSpace(why)
	if hypothesis == "" {
		hypothesis = "（分支先建了，假设待补）"
	}
	exp := &Experiment{
		ID:         NewID(),
		Path:       path,
		Branch:     branch,
		Hypothesis: hypothesis,
		Status:     StatusActive,
	}
	if err := records.WriteJSON(filepath.Join(k.Store.ExpDir(exp.ID), "meta.json"), exp); err != nil {
		return nil, err
	}
	return exp, nil
}

// Unregistered 返回有 exp/ 前缀、但没有对应记录的分支。
func (k *Kernel) Unregistered() ([]string, error) {
	branches, err := k.Repo.Branches()
	if err != nil {
		return nil, err
	}
	exps, err := k.List()
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(exps))
	for _, exp := range exps {
		known[exp.Branch] = struct{}{}
	}
	var out []string
	for _, branch := range branches {
		if !strings.HasPrefix(branch, "exp/") {
			continue
		}
		if _, ok := known[branch]; ok {
			continue
		}
		out = append(out, branch)
	}
	return out, nil
}
