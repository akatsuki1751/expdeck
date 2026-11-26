package gitx

import "fmt"

// DiffRun compares a run's base with its recorded commit, including legacy snapshots.
func (r *Repo) DiffRun(base, snapshot string) (string, error) {
	if snapshot == "" {
		return "", fmt.Errorf("该 run 没有代码提交")
	}
	if base == "" {
		return r.Git("diff-tree", "--root", "-p", snapshot)
	}
	return r.Git("diff", base, snapshot)
}
