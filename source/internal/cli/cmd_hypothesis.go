package cli

import (
	"expdeck/internal/core"
	"fmt"
	"io"
)

func (a *app) cmdHypothesis(args []string) (result, error) {
	text := noteUnset
	pos, err := splitArgs(args, map[string]*string{"text": &text}, nil)
	if err != nil || len(pos) != 1 || text == noteUnset {
		return result{}, fail(ExitUsage, "usage: exp hypothesis <experiment> --text <text>")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, err
	}
	exp, err := kernel.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	if !a.dry {
		if err = kernel.SetHypothesis(exp, text); err != nil {
			return result{}, err
		}
	}
	return result{data: map[string]any{"experiment": exp.PathString(), "hypothesis": text, "dry_run": a.dry}, human: func(w io.Writer) { fmt.Fprintln(w, text) }}, nil
}
