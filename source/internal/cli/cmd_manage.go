package cli

import (
	"expdeck/internal/core"
	"fmt"
	"io"
	"os"
	"strings"
)

func (a *app) cmdRunOutcome(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil || len(pos) != 2 {
		return result{}, fail(ExitUsage, "usage: exp run-outcome <run-id> valid|invalid|auto")
	}
	k, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, err
	}
	exp, run, err := k.FindRun(pos[0])
	if err != nil {
		return result{}, err
	}
	if !a.dry {
		if err = k.SetRunOutcome(exp, run.ID, pos[1]); err != nil {
			return result{}, err
		}
	}
	return result{data: map[string]any{"run": run.ID, "outcome": pos[1], "dry_run": a.dry}, human: func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", run.ID, pos[1]) }}, nil
}
func (a *app) cmdRepresentative(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil || (len(pos) != 1 && len(pos) != 2) {
		return result{}, fail(ExitUsage, "usage: exp representative <experiment> [run-id|auto]")
	}
	k, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, err
	}
	exp, err := k.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	if len(pos) == 1 {
		run, manual, err := k.Representative(exp)
		if err != nil {
			return result{}, err
		}
		return result{data: map[string]any{"experiment": exp.PathString(), "run": run, "manual": manual}, human: func(w io.Writer) {
			if run == nil {
				fmt.Fprintln(w, "暂无有效 run")
			} else {
				fmt.Fprintf(w, "%s (manual=%t)\n", run.ID, manual)
			}
		}}, nil
	}
	id := pos[1]
	if id == "auto" {
		id = ""
	}
	if !a.dry {
		if err = k.SetRepresentative(exp, id); err != nil {
			return result{}, err
		}
	}
	return result{data: map[string]any{"experiment": exp.PathString(), "run": pos[1], "dry_run": a.dry}, human: func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", exp.PathString(), pos[1]) }}, nil
}
func (a *app) cmdDelete(args []string) (result, error) {
	confirm := false
	pos, err := splitArgs(args, nil, map[string]*bool{"yes": &confirm})
	if err != nil || len(pos) != 1 {
		return result{}, fail(ExitUsage, "usage: exp delete <experiment> --yes")
	}
	k, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, err
	}
	exp, err := k.Find(pos[0])
	if err != nil {
		return result{}, err
	}
	if !a.dry && !confirm {
		return result{}, fail(ExitUsage, "deletion requires --yes")
	}
	if !a.dry {
		if err = k.DeleteExperiment(exp); err != nil {
			return result{}, err
		}
	}
	return result{data: map[string]any{"experiment": exp.PathString(), "deleted": !a.dry}, human: func(w io.Writer) { fmt.Fprintf(w, "deleted=%t: %s\n", !a.dry, exp.PathString()) }}, nil
}
func (a *app) cmdExport(args []string) (result, error) {
	output := ""
	runs := ""
	logs := false
	artifacts := false
	pos, err := splitArgs(args, map[string]*string{"output": &output, "runs": &runs}, map[string]*bool{"logs": &logs, "artifacts": &artifacts})
	if err != nil || len(pos) == 0 || output == "" {
		return result{}, fail(ExitUsage, "usage: exp export <experiment>... --output file.zip [--runs id,id] [--logs] [--artifacts]")
	}
	if runs != "" && len(pos) != 1 {
		return result{}, fail(ExitUsage, "--runs requires exactly one experiment")
	}
	k, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, err
	}
	sel := make([]core.ExportSelection, 0, len(pos))
	for _, ref := range pos {
		choice := core.ExportSelection{Experiment: ref}
		if runs != "" {
			choice.Runs = strings.Split(runs, ",")
		}
		sel = append(sel, choice)
	}
	if a.dry {
		return result{data: map[string]any{"dry_run": true, "output": output}}, nil
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result{}, err
	}
	err = k.Export(f, sel, core.ExportOptions{Logs: logs, Artifacts: artifacts})
	closeErr := f.Close()
	if err != nil {
		os.Remove(output)
		return result{}, err
	}
	if closeErr != nil {
		os.Remove(output)
		return result{}, closeErr
	}
	return result{data: map[string]any{"output": output, "experiments": len(sel)}, human: func(w io.Writer) { fmt.Fprintf(w, "exported: %s\n", output) }}, nil
}
