// Package gitx 是所有 git 调用的唯一出口。
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo 是一个已定位的仓库。
type Repo struct {
	Root      string // 主检出的工作区根
	CommonDir string // 所有 worktree 共享的 .git 目录
}

// Open 在 dir 里定位仓库。不是仓库时返回错误。
func Open(dir string) (*Repo, error) {
	if dir == "" {
		dir = "."
	}
	top, err := execGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository")
	}
	common, err := execGit(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("not a git repository")
	}
	top = strings.TrimSpace(top)
	common = strings.TrimSpace(common)
	if !filepath.IsAbs(common) {
		common = filepath.Join(top, common)
	}
	return &Repo{Root: filepath.Clean(top), CommonDir: filepath.Clean(common)}, nil
}

// Git 在仓库根执行一条 git 命令。
func (r *Repo) Git(args ...string) (string, error) { return execGit(r.Root, args...) }

// BranchExists 报告分支是否存在。
func (r *Repo) BranchExists(name string) bool {
	_, err := r.Git("rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// CreateBranch 从 HEAD 建一条分支。
func (r *Repo) CreateBranch(name string) error {
	_, err := r.Git("branch", name)
	return err
}

// Branches 返回全部分支名。
func (r *Repo) Branches() ([]string, error) {
	out, err := r.Git("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func execGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
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
