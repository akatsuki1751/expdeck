package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Snapshot 把 worktree 的当前内容（含未跟踪文件）拍成一个提交。
// parent 为空表示这是第一次快照。
func (r *Repo) Snapshot(worktree, parent string) (string, error) {
	index, err := os.CreateTemp("", "exp-index-*")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	index.Close()
	// git 要求索引文件不存在时才肯新建。
	os.Remove(indexPath)
	defer os.Remove(indexPath)

	base := os.Environ()
	base = append(base,
		"GIT_DIR="+r.CommonDir,
		"GIT_WORK_TREE="+worktree,
		"GIT_INDEX_FILE="+indexPath,
	)

	if _, err := runIn(worktree, base, "add", "-A", "."); err != nil {
		return "", fmt.Errorf("快照 add 失败: %w", err)
	}
	tree, err := runIn(worktree, base, "write-tree")
	if err != nil {
		return "", fmt.Errorf("快照 write-tree 失败: %w", err)
	}
	tree = strings.TrimSpace(tree)

	// commit-tree 需要作者信息；仓库没配身份时给一次性兜底，避免快照直接失败。
	authorEnv := append(base,
		"GIT_AUTHOR_NAME=exp",
		"GIT_AUTHOR_EMAIL=exp@local",
		"GIT_COMMITTER_NAME=exp",
		"GIT_COMMITTER_EMAIL=exp@local",
	)
	args := []string{"commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, "-m", "exp snapshot "+time.Now().Format(time.RFC3339))
	commit, err := runIn(worktree, authorEnv, args...)
	if err != nil {
		return "", fmt.Errorf("快照 commit-tree 失败: %w", err)
	}
	return strings.TrimSpace(commit), nil
}

// DiffRun 返回一次快照相对它的基线的改动。
func (r *Repo) DiffRun(base, snapshot string) (string, error) {
	if snapshot == "" {
		return "", fmt.Errorf("该 run 没有快照")
	}
	if base == "" {
		return r.Git("diff-tree", "--root", "-p", snapshot)
	}
	return r.Git("diff", base, snapshot)
}

func runIn(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}
