package gitx

import (
	"fmt"
	"path/filepath"
	"strings"
)

// WorktreeAdd 把一个已存在的分支检出到 dir，形成新的 worktree。
func (r *Repo) WorktreeAdd(dir, branch string) error {
	if _, err := r.Git("worktree", "add", dir, branch); err != nil {
		return fmt.Errorf("添加 worktree %s（分支 %s）失败: %w", dir, branch, err)
	}
	return nil
}

// WorktreeRemove 删除一个 worktree；force 时即使还有未提交改动也强删。
func (r *Repo) WorktreeRemove(dir string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, dir)
	if _, err := r.Git(args...); err != nil {
		return fmt.Errorf("移除 worktree %s 失败: %w", dir, err)
	}
	return nil
}

// Worktrees 解析 `git worktree list --porcelain`，返回 目录 → 分支 的映射。
// 分离 HEAD 的 worktree 对应空分支名。
func (r *Repo) Worktrees() (map[string]string, error) {
	out, err := r.Git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("读取 worktree 列表失败: %w", err)
	}
	worktrees := make(map[string]string)
	var dir, branch string
	flush := func() {
		if dir != "" {
			worktrees[filepath.Clean(filepath.FromSlash(dir))] = branch
		}
		dir, branch = "", ""
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			dir = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	flush()
	return worktrees, nil
}

// IsDirty 报告 dir 的工作区是否有未提交改动（含未跟踪文件）。
func (r *Repo) IsDirty(dir string) (bool, error) {
	out, err := r.Git("-C", dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("检查 %s 的改动失败: %w", dir, err)
	}
	return strings.TrimSpace(out) != "", nil
}

// DirtyPatch 返回 dir 相对 HEAD 的未提交改动，包含未跟踪文件的内容。
// 先把未跟踪文件标记为"意图添加"，这样 git diff 才会带上它们的内容；
// dir 马上就要被整个删除，所以这里改动它的索引没有副作用。
func (r *Repo) DirtyPatch(dir string) (string, error) {
	if _, err := r.Git("-C", dir, "add", "-A", "-N"); err != nil {
		return "", fmt.Errorf("标记 %s 的未跟踪文件失败: %w", dir, err)
	}
	out, err := r.Git("-C", dir, "diff", "HEAD")
	if err != nil {
		return "", fmt.Errorf("导出 %s 的改动失败: %w", dir, err)
	}
	return out, nil
}
