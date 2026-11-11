package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"expdeck/internal/core"
)

// cmdAttach 实现 exp attach <path|id> [dir]。
func (a *app) cmdAttach(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 && len(pos) != 2 {
		return result{}, fail(ExitUsage, "usage: exp attach <path|id> [dir]")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	dir := kernel.WorktreePath(exp)
	if len(pos) == 2 {
		if dir, err = a.worktreeDir(pos[1]); err != nil {
			return result{}, fail(ExitInvalid, "%v", err)
		}
	}
	if a.dry {
		payload := map[string]any{
			"dry_run":    true,
			"experiment": exp.PathString(),
			"worktree":   dir,
			"patch":      "",
		}
		return result{data: payload, human: func(w io.Writer) {
			fmt.Fprintf(w, "would attach  %s\n", exp.PathString())
			fmt.Fprintf(w, "  worktree  %s\n", dir)
		}}, nil
	}
	var worktree string
	if len(pos) == 2 {
		worktree, err = kernel.AttachTo(exp, dir)
	} else {
		worktree, err = kernel.Attach(exp)
	}
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"experiment": exp.PathString(), "worktree": worktree, "patch": ""},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "attached  %s\n", exp.PathString())
			fmt.Fprintf(w, "  worktree  %s\n", worktree)
		},
	}, nil
}

// cmdDetach 实现 exp detach <path|id>。
func (a *app) cmdDetach(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp detach <path|id>")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	worktree, err := attachedWorktree(kernel, exp)
	if err != nil {
		return result{}, err
	}
	if a.dry {
		payload := map[string]any{
			"dry_run":    true,
			"experiment": exp.PathString(),
			"worktree":   worktree,
			"patch":      "",
		}
		return result{data: payload, human: func(w io.Writer) {
			fmt.Fprintf(w, "would detach  %s\n", exp.PathString())
			fmt.Fprintf(w, "  worktree  %s\n", worktree)
		}}, nil
	}
	patch, err := kernel.Detach(exp)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"experiment": exp.PathString(), "worktree": worktree, "patch": patch},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "detached  %s\n", exp.PathString())
			fmt.Fprintf(w, "  worktree  %s\n", worktree)
			if patch != "" {
				fmt.Fprintf(w, "  patch     %s\n", patch)
			}
		},
	}, nil
}

// worktreeDir 把用户给出的目录解析成绝对路径；相对路径按调用目录算。
func (a *app) worktreeDir(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("worktree 目录不能为空")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.env.Dir, dir)
	}
	return filepath.Abs(dir)
}

// attachedWorktree 找到某条分支当前的 worktree 目录。
func attachedWorktree(kernel *core.Kernel, exp *core.Experiment) (string, error) {
	worktrees, err := kernel.Attached()
	if err != nil {
		return "", err
	}
	for dir, branch := range worktrees {
		if branch == exp.Branch {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%w: 实验 %q 没有挂载的 worktree", core.ErrNotFound, exp.PathString())
}
