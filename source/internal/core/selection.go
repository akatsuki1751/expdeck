package core

import (
	"expdeck/internal/records"
	"fmt"
	"strings"
)

// RunValid preserves legacy finished runs as valid unless explicitly marked.
func RunValid(run *Run) bool {
	if run.Status == RunRunning {
		return false
	}
	if run.Outcome == "valid" {
		return true
	}
	if run.Outcome == "invalid" {
		return false
	}
	return run.Status == RunFinished && (run.ExitCode == nil || *run.ExitCode == 0)
}

func (k *Kernel) Representative(exp *Experiment) (*Run, bool, error) {
	runs, err := k.ListRuns(exp)
	if err != nil {
		return nil, false, err
	}
	run, manual := SelectRepresentative(exp, runs)
	return run, manual, nil
}

// SelectRepresentative selects from an already loaded, ordered run list.
func SelectRepresentative(exp *Experiment, runs []*Run) (*Run, bool) {
	for _, run := range runs {
		if exp.RepresentativeRun != "" && run.ID == exp.RepresentativeRun && RunValid(run) {
			return run, true
		}
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if RunValid(runs[i]) {
			return runs[i], false
		}
	}
	return nil, false
}

func (k *Kernel) SetRunOutcome(exp *Experiment, id, outcome string) error {
	if outcome != "valid" && outcome != "invalid" && outcome != "auto" {
		return fmt.Errorf("%w: outcome must be valid, invalid or auto", ErrInvalid)
	}
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Unlock()
	current, err := k.Find(exp.ID)
	if err != nil {
		return err
	}
	run, err := k.LoadRun(current, id)
	if err != nil {
		return err
	}
	if run.Status == RunRunning {
		return fmt.Errorf("%w: run is still running", ErrConflict)
	}
	if outcome == "auto" {
		outcome = ""
	}
	run.Outcome = outcome
	if err := k.SaveRun(current, run); err != nil {
		return err
	}
	if current.RepresentativeRun == id && !RunValid(run) {
		current.RepresentativeRun = ""
		return records.WriteJSON(k.metaPath(current), current)
	}
	return nil
}

func (k *Kernel) SetRepresentative(exp *Experiment, id string) error {
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Unlock()
	current, err := k.Find(exp.ID)
	if err != nil {
		return err
	}
	if id != "" {
		run, err := k.LoadRun(current, id)
		if err != nil {
			return err
		}
		if !RunValid(run) {
			return fmt.Errorf("%w: representative run must be valid", ErrInvalid)
		}
	}
	current.RepresentativeRun = id
	return records.WriteJSON(k.metaPath(current), current)
}

// SetHypothesis updates only the current experiment's hypothesis.
func (k *Kernel) SetHypothesis(exp *Experiment, hypothesis string) error {
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Unlock()
	current, err := k.Find(exp.ID)
	if err != nil {
		return err
	}
	current.Hypothesis = strings.TrimSpace(hypothesis)
	return records.WriteJSON(k.metaPath(current), current)
}
