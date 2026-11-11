package cli

import (
	"fmt"
	"io"

	"expdeck/internal/core"
)

// ---------- backup ----------

func (a *app) cmdBackup(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 0 {
		return result{}, fail(ExitUsage, "backup takes no arguments")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	if a.dry {
		return result{data: map[string]any{"dry_run": true, "records": kernel.Store.Records()}}, nil
	}
	commit, files, err := kernel.Backup()
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"commit": commit, "files": files},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "backup  %s  (%d files)\n", commit, files)
		},
	}, nil
}

// ---------- restore ----------

func (a *app) cmdRestore(args []string) (result, error) {
	force := false
	pos, err := splitArgs(args, nil, map[string]*bool{"force": &force})
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if len(pos) != 0 {
		return result{}, fail(ExitUsage, "restore takes no arguments")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	if a.dry {
		return result{data: map[string]any{"dry_run": true, "force": force}}, nil
	}
	files, err := kernel.Restore(force)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"files": files},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "restored  %d files from book\n", files)
		},
	}, nil
}
