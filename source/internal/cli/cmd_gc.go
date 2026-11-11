package cli

import (
	"fmt"
	"io"

	"expdeck/internal/core"
)

func (a *app) cmdGC(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) > 0 {
		return result{}, fail(ExitUsage, "gc takes no arguments")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	dry := a.dry
	removed, err := kernel.GC(dry)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"removed": removed, "dry_run": dry},
		human: func(w io.Writer) {
			if dry {
				fmt.Fprintf(w, "would remove %d unused objects\n", removed)
				return
			}
			fmt.Fprintf(w, "removed %d unused objects\n", removed)
		},
	}, nil
}
