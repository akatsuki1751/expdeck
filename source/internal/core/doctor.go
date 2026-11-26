package core

import (
	"fmt"
	"sort"
	"strings"
)

// 漂移种类。
const (
	DriftOrphanExperiment   = "orphan_experiment"
	DriftUnregisteredBranch = "unregistered_branch"
	DriftStaleSnapshot      = "stale_snapshot"
)

// Drift 是一条漂移：只描述，不携带修复动作。
type Drift struct {
	Kind    string `json:"kind"`    // orphan_experiment | unregistered_branch | stale_snapshot
	Subject string `json:"subject"` // 实验路径或分支名或 run 标识
	Detail  string `json:"detail"`  // 中文说明
}

// Doctor 体检记录与仓库是否漂移：只报告，不修改任何东西，按 Kind 排序。
func (k *Kernel) Doctor() ([]Drift, error) {
	exps, err := k.List()
	if err != nil {
		return nil, fmt.Errorf("%w: 读取实验记录失败: %v", ErrConflict, err)
	}
	branches, err := k.Repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("%w: 读取分支列表失败: %v", ErrConflict, err)
	}

	var drifts []Drift
	registered := make(map[string]bool, len(exps))

	// 1. orphan_experiment：记录在，分支没了。
	for _, exp := range exps {
		registered[exp.Branch] = true
		if !k.Repo.BranchExists(exp.Branch) {
			drifts = append(drifts, Drift{
				Kind:    DriftOrphanExperiment,
				Subject: exp.PathString(),
				Detail:  fmt.Sprintf("记录指向的分支 %s 不存在", exp.Branch),
			})
		}
	}

	// 2. unregistered_branch：分支在，记录没了。
	for _, branch := range branches {
		if !strings.HasPrefix(branch, "exp/") || registered[branch] {
			continue
		}
		drifts = append(drifts, Drift{
			Kind:    DriftUnregisteredBranch,
			Subject: branch,
			Detail:  "分支没有对应的实验记录",
		})
	}

	// Runs can point to normal branch commits or to legacy snapshot chains.
	for _, exp := range exps {
		runs, err := k.ListRuns(exp)
		if err != nil {
			return nil, fmt.Errorf("%w: 读取实验 %s 的 run 记录失败: %v", ErrConflict, exp.PathString(), err)
		}
		for _, run := range runs {
			if run.Snapshot == "" {
				continue
			}
			if _, err := k.Repo.Git("cat-file", "-e", run.Snapshot+"^{commit}"); err != nil {
				drifts = append(drifts, Drift{Kind: DriftStaleSnapshot, Subject: run.ID, Detail: fmt.Sprintf("run 记录的提交 %s 不存在", run.Snapshot)})
				continue
			}
			_, branchErr := k.Repo.Git("merge-base", "--is-ancestor", run.Snapshot, exp.Branch)
			_, legacyErr := k.Repo.Git("merge-base", "--is-ancestor", run.Snapshot, k.SnapRef(exp))
			if branchErr != nil && legacyErr != nil {
				drifts = append(drifts, Drift{Kind: DriftStaleSnapshot, Subject: run.ID, Detail: "run 提交不在实验分支或旧快照引用中，可能无法长期保留"})
			}
		}
	}

	sort.SliceStable(drifts, func(i, j int) bool {
		if drifts[i].Kind != drifts[j].Kind {
			return drifts[i].Kind < drifts[j].Kind
		}
		return drifts[i].Subject < drifts[j].Subject
	})
	return drifts, nil
}
