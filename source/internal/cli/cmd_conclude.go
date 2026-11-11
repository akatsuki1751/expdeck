// cmd_conclude.go 提供 conclude / reopen 两个命令：结案与重开。

package cli

import (
	"fmt"
	"io"

	"expdeck/internal/core"
)

// cmdConclude 结案：--decision keep|abandon，--why 必填。
func (a *app) cmdConclude(args []string) (result, error) {
	const usage = "usage: exp conclude <path|id> --decision keep|abandon --why \"...\""
	var decision, why string
	pos, err := splitArgs(args, map[string]*string{"decision": &decision, "why": &why}, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "%s", usage)
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	if a.dry {
		return result{data: map[string]any{
			"dry_run":    true,
			"experiment": exp.PathString(),
			"decision":   decision,
			"why":        why,
			"status":     exp.Status,
		}}, nil
	}
	if err := kernel.Conclude(exp, decision, why); err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{
			"experiment": exp.PathString(),
			"decision":   decision,
			"status":     exp.Status,
		},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "closed  %s  [%s]  %s\n", exp.PathString(), exp.Status, decision)
		},
	}, nil
}

// cmdReopen 把已结案的实验改回 active。
func (a *app) cmdReopen(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp reopen <path|id>")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	if a.dry {
		return result{data: map[string]any{
			"dry_run":    true,
			"experiment": exp.PathString(),
			"status":     exp.Status,
		}}, nil
	}
	if err := kernel.Reopen(exp); err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{
			"experiment": exp.PathString(),
			"status":     exp.Status,
		},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "reopened  %s  [%s]\n", exp.PathString(), exp.Status)
		},
	}, nil
}
