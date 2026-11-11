package cli

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"expdeck/internal/core"
)

func (a *app) cmdRun(args []string) (result, error) {
	expRef := ""
	gpuText := ""
	pos, err := splitArgs(args, map[string]*string{"exp": &expRef, "gpu": &gpuText}, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if expRef == "" && len(pos) == 1 {
		expRef = pos[0]
	} else if expRef != "" && len(pos) != 0 {
		return result{}, fail(ExitUsage, "usage: exp run <exp> [--gpu N]")
	}
	if expRef == "" {
		return result{}, fail(ExitUsage, "usage: exp run <exp> [--gpu N]")
	}
	gpu := 0
	if gpuText != "" {
		value, convErr := strconv.Atoi(gpuText)
		if convErr != nil || value < 0 {
			return result{}, fail(ExitUsage, "--gpu 需要一个非负整数")
		}
		gpu = value
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exp, err := kernel.Find(expRef)
	if err != nil {
		return result{}, err
	}
	if a.dry {
		return result{data: map[string]any{"dry_run": true, "experiment": exp.PathString()}}, nil
	}
	run, err := kernel.StartRun(exp, gpu)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"run": run},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "started  %s  (%s)\n", run.ID, exp.PathString())
			fmt.Fprintf(w, "log      %s\n", run.Log)
			fmt.Fprintf(w, "wait     exp wait --run %s\n", run.ID)
		},
	}, nil
}

func (a *app) cmdStatus(args []string) (result, error) {
	if _, err := splitArgs(args, nil, nil); err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	exps, err := kernel.List()
	if err != nil {
		return result{}, err
	}
	type row struct {
		Exp    string `json:"experiment"`
		Run    string `json:"run"`
		Status string `json:"status"`
		PID    int    `json:"pid"`
	}
	var rows []row
	for _, exp := range exps {
		running, err := kernel.RunningRun(exp)
		if err != nil {
			return result{}, err
		}
		if running == nil {
			continue
		}
		rows = append(rows, row{Exp: exp.PathString(), Run: running.ID, Status: running.Status, PID: running.PID})
	}
	return result{
		data: map[string]any{"running": rows},
		human: func(w io.Writer) {
			if len(rows) == 0 {
				fmt.Fprintln(w, "(nothing running)")
				return
			}
			for _, r := range rows {
				fmt.Fprintf(w, "%s  %s  pid=%d  %s\n", r.Run, r.Exp, r.PID, r.Status)
			}
		},
	}, nil
}

func (a *app) cmdWait(args []string) (result, error) {
	runID := ""
	timeoutText := ""
	if _, err := splitArgs(args, map[string]*string{"run": &runID, "timeout": &timeoutText}, nil); err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if runID == "" {
		return result{}, fail(ExitUsage, "usage: exp wait --run <id> [--timeout 秒]")
	}
	timeout := time.Hour
	if timeoutText != "" {
		seconds, convErr := strconv.Atoi(timeoutText)
		if convErr != nil || seconds <= 0 {
			return result{}, fail(ExitUsage, "--timeout 需要正整数秒")
		}
		timeout = time.Duration(seconds) * time.Second
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	run, err := kernel.WaitRun(runID, timeout)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"run": run},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "%s  %s", run.ID, run.Status)
			if run.ExitCode != nil {
				fmt.Fprintf(w, "  exit=%d", *run.ExitCode)
			}
			fmt.Fprintln(w)
		},
	}, nil
}

func (a *app) cmdInterrupt(args []string) (result, error) {
	runID := ""
	pos, err := splitArgs(args, map[string]*string{"run": &runID}, nil)
	if err != nil {
		return result{}, fail(ExitUsage, "%v", err)
	}
	if runID == "" && len(pos) == 1 {
		runID = pos[0]
	}
	if runID == "" {
		return result{}, fail(ExitUsage, "usage: exp interrupt --run <id>")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, fail(ExitInvalid, "%v", err)
	}
	run, err := kernel.InterruptRun(runID)
	if err != nil {
		return result{}, err
	}
	return result{
		data: map[string]any{"run": run},
		human: func(w io.Writer) {
			fmt.Fprintf(w, "interrupting %s\n", run.ID)
		},
	}, nil
}

// cmdWatch 是监督进程的入口，由 exp run 以子进程方式拉起。
func (a *app) cmdWatch(args []string) (result, error) {
	pos, err := splitArgs(args, nil, nil)
	if err != nil || len(pos) != 1 {
		return result{}, fail(ExitUsage, "internal: __watch <runDir>")
	}
	kernel, err := core.Open(a.env.Dir)
	if err != nil {
		return result{}, err
	}
	if err := kernel.Watch(pos[0]); err != nil {
		return result{}, err
	}
	return result{}, nil
}
