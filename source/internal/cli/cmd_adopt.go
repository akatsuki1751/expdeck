package cli

import (
	"fmt"
	"io"

	"expdeck/internal/core"
)

func (a *app) cmdAdopt(args []string) (result, error) {
	why := ""
	pos, err := splitArgs(args, map[string]*string{"why": &why}, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp adopt <branch> [--why \"...\"]")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	if a.dry {
		return result{data: map[string]any{"dry_run": true, "branch": pos[0]}}, nil
	}
	exp, err := kernel.Adopt(pos[0], why)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"experiment": exp},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "adopted  %s  (%s)\n", exp.PathString(), exp.Branch)
		},
	}, nil
}
