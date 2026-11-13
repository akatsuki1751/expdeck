package core

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type ExportSelection struct {
	Experiment string
	Runs       []string
}
type ExportOptions struct {
	Logs          bool
	Results       bool
	FilterResults bool // hide result metrics from run metadata when not selected
	Weights       bool
	Artifacts     bool // legacy CLI option: export all artifact files
}

// Export writes a zip containing selected run metadata and optionally its logs and artifacts.
func (k *Kernel) Export(w io.Writer, selection []ExportSelection, opts ExportOptions) error {
	if len(selection) == 0 {
		return fmt.Errorf("%w: select an experiment", ErrInvalid)
	}
	zw := zip.NewWriter(w)
	add := func(name string, data []byte) error {
		f, e := zw.Create(name)
		if e != nil {
			return e
		}
		_, e = f.Write(data)
		return e
	}
	addFile := func(name, source string) error {
		f, e := os.Open(source)
		if e != nil {
			return fmt.Errorf("missing export file %s: %w", source, e)
		}
		defer f.Close()
		z, e := zw.Create(name)
		if e != nil {
			return e
		}
		_, e = io.Copy(z, f)
		return e
	}
	seen := map[string]bool{}
	for _, choice := range selection {
		exp, err := k.Find(choice.Experiment)
		if err != nil {
			return err
		}
		if seen[exp.ID] {
			return fmt.Errorf("%w: duplicate experiment", ErrInvalid)
		}
		seen[exp.ID] = true
		segments, err := ParsePath(exp.PathString())
		if err != nil {
			return err
		}
		base := path.Join("experiments", strings.Join(segments, "/"))
		meta, err := json.MarshalIndent(exp, "", "  ")
		if err != nil {
			return err
		}
		if err = add(path.Join(base, "experiment.json"), meta); err != nil {
			return err
		}
		note, _, err := k.ReadNote(exp)
		if err != nil {
			return err
		}
		if err = add(path.Join(base, "note.md"), []byte(note)); err != nil {
			return err
		}
		ids := choice.Runs
		if len(ids) == 0 {
			run, _, err := k.Representative(exp)
			if err != nil {
				return err
			}
			if run == nil {
				return fmt.Errorf("%w: %s has no representative run", ErrInvalid, exp.PathString())
			}
			ids = []string{run.ID}
		}
		runSeen := map[string]bool{}
		for _, id := range ids {
			if runSeen[id] {
				continue
			}
			runSeen[id] = true
			run, err := k.LoadRun(exp, id)
			if err != nil {
				return err
			}
			if run.Status == RunRunning {
				return fmt.Errorf("%w: run %s is running", ErrConflict, id)
			}
			runbase := path.Join(base, "runs", run.ID)
			exportRun := *run
			if opts.FilterResults && !opts.Results {
				exportRun.Metrics = nil
			}
			exportRun.Artifacts = nil
			for _, artifact := range run.Artifacts {
				if (isWeightArtifact(artifact.Name) && (opts.Weights || opts.Artifacts)) ||
					(!isWeightArtifact(artifact.Name) && (opts.Results || opts.Artifacts)) {
					exportRun.Artifacts = append(exportRun.Artifacts, artifact)
				}
			}
			raw, err := json.MarshalIndent(exportRun, "", "  ")
			if err != nil {
				return err
			}
			if err = add(path.Join(runbase, "run.json"), raw); err != nil {
				return err
			}
			if opts.Results && len(run.Metrics) > 0 {
				metrics, err := json.MarshalIndent(run.Metrics, "", "  ")
				if err != nil {
					return err
				}
				if err = add(path.Join(runbase, "metrics.json"), metrics); err != nil {
					return err
				}
			}
			if opts.Logs && run.Log != "" {
				if err = addFile(path.Join(runbase, "stdout.log"), run.Log); err != nil {
					return err
				}
			}
			if len(exportRun.Artifacts) > 0 {
				for i, a := range exportRun.Artifacts {
					// Stored hashes, not names or paths from records, determine the source.
					if len(a.SHA256) != 64 || strings.Trim(a.SHA256, "0123456789abcdef") != "" {
						return fmt.Errorf("%w: invalid artifact digest", ErrInvalid)
					}
					source := filepath.Join(k.ObjectsDir(), a.SHA256[:2], a.SHA256)
					name := filepath.Base(a.Name)
					if name == "." || name == string(filepath.Separator) {
						name = "artifact"
					}
					if err = addFile(path.Join(runbase, "artifacts", fmt.Sprintf("%03d-%s", i, name)), source); err != nil {
						return err
					}
				}
			}
		}
	}
	return zw.Close()
}

// isWeightArtifact distinguishes common model checkpoint formats from other outputs.
func isWeightArtifact(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pt", ".pth", ".ckpt", ".safetensors", ".onnx", ".h5", ".hdf5", ".keras", ".weights", ".bin":
		return true
	default:
		return false
	}
}
