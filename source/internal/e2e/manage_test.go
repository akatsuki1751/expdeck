package e2e

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"expdeck/internal/core"
)

func setupReviewRuns(t *testing.T) (string, *core.Kernel, *core.Experiment) {
	t.Helper()
	repo := scaffold(t)
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []*core.Run{
		{ID: "20261113-100000-a", Exp: exp.ID, Status: core.RunFinished, Metrics: map[string]float64{"r2": 0.8}},
		{ID: "20261113-110000-b", Exp: exp.ID, Status: core.RunFinished, Metrics: map[string]float64{"r2": 0.9}},
		{ID: "20261113-120000-c", Exp: exp.ID, Status: core.RunInterrupted},
	} {
		if err = k.SaveRun(exp, r); err != nil {
			t.Fatal(err)
		}
	}
	return repo, k, exp
}
func TestRunSelectionAndWebControls(t *testing.T) {
	repo, k, exp := setupReviewRuns(t)
	rep, manual, err := k.Representative(exp)
	if err != nil || manual || rep.ID != "20261113-110000-b" {
		t.Fatalf("default: %v %v %v", rep, manual, err)
	}
	if err = k.SetRunOutcome(exp, "20261113-110000-b", "invalid"); err != nil {
		t.Fatal(err)
	}
	rep, _, _ = k.Representative(exp)
	if rep.ID != "20261113-100000-a" {
		t.Fatalf("invalid selected: %v", rep)
	}
	if err = k.SetRunOutcome(exp, "20261113-120000-c", "valid"); err != nil {
		t.Fatal(err)
	}
	if err = k.SetRepresentative(exp, "20261113-100000-a"); err != nil {
		t.Fatal(err)
	}
	exp, _ = k.Find(exp.ID)
	rep, manual, _ = k.Representative(exp)
	if !manual || rep.ID != "20261113-100000-a" {
		t.Fatal("manual selection missing")
	}
	if err = k.SetRunOutcome(exp, rep.ID, "invalid"); err != nil {
		t.Fatal(err)
	}
	exp, _ = k.Find(exp.ID)
	rep, manual, _ = k.Representative(exp)
	if manual || rep.ID != "20261113-120000-c" {
		t.Fatal("invalidating selection did not reset")
	}
	h := newHandler(t, repo)
	code, body := get(t, h, "/e/sensitive/lr?run=20261113-120000-c")
	if code != http.StatusOK || !strings.Contains(body, "标记无效") || !strings.Contains(body, "设为代表 run") {
		t.Fatalf("run controls: %d %s", code, body)
	}
}
func TestExportAndInvalidPaths(t *testing.T) {
	repo, k, exp := setupReviewRuns(t)
	_, err := k.LoadRun(exp, "../../secret")
	if err == nil {
		t.Fatal("unsafe run id accepted")
	}
	var buf bytes.Buffer
	if err = k.Export(&buf, []core.ExportSelection{{Experiment: exp.ID}}, core.ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range zr.File {
		if strings.Contains(f.Name, "20261113-110000-b/run.json") {
			found = true
		}
		if strings.Contains(f.Name, "20261113-100000-a/run.json") {
			t.Fatal("unexpected run exported")
		}
	}
	if !found {
		t.Fatal("representative run not exported")
	}
	h := newHandler(t, repo)
	form := url.Values{"exp": {exp.ID}}
	req := httptest.NewRequest(http.MethodPost, "/export", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")) {
		t.Fatalf("web export: %d %s", rec.Code, rec.Body.String())
	}
}
func TestDeleteRefusesDirtyWorktreeAndPreservesArtifacts(t *testing.T) {
	repo, k, exp := setupReviewRuns(t)
	defer os.RemoveAll(attachRoot(repo))
	dir, err := k.Attach(exp)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "unsaved.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = k.DeleteExperiment(exp); err == nil {
		t.Fatal("dirty worktree deleted")
	}
	if _, err = os.Stat(filepath.Join(dir, "unsaved.txt")); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(dir, "unsaved.txt")); err != nil {
		t.Fatal(err)
	}
	if err = k.DeleteExperiment(exp); err != nil {
		t.Fatal(err)
	}
	if k.Repo.BranchExists(exp.Branch) {
		t.Fatal("branch remains")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("worktree remains: %v", err)
	}
	if _, err = k.Find(exp.ID); err == nil {
		t.Fatal("experiment remains")
	}
}

func TestExportArtifactsLogsAndMissingFiles(t *testing.T) {
	_, k, exp := setupReviewRuns(t)
	run, err := k.LoadRun(exp, "20261113-110000-b")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("model-weight")
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	obj := filepath.Join(k.ObjectsDir(), sum[:2], sum)
	if err = os.MkdirAll(filepath.Dir(obj), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(obj, data, 0600); err != nil {
		t.Fatal(err)
	}
	run.Artifacts = []core.Artifact{{Name: "best.pth", SHA256: sum}}
	run.Log = k.LogPath(run.ID)
	if err = os.WriteFile(run.Log, []byte("run output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = k.SaveRun(exp, run); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err = k.Export(&buf, []core.ExportSelection{{Experiment: exp.ID, Runs: []string{run.ID}}}, core.ExportOptions{Logs: true, Artifacts: true}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	log, weight := false, false
	for _, f := range zr.File {
		log = log || strings.HasSuffix(f.Name, "stdout.log")
		weight = weight || strings.HasSuffix(f.Name, "best.pth")
	}
	if !log || !weight {
		t.Fatal("logs or artifacts missing")
	}
	os.Remove(obj)
	buf.Reset()
	if err = k.Export(&buf, []core.ExportSelection{{Experiment: exp.ID}}, core.ExportOptions{Artifacts: true}); err == nil {
		t.Fatal("missing artifact accepted")
	}
}
func TestDeleteRejectsRunningAndIgnored(t *testing.T) {
	repo, k, exp := setupReviewRuns(t)
	defer os.RemoveAll(attachRoot(repo))
	running := &core.Run{ID: "20261113-130000-d", Exp: exp.ID, Status: core.RunRunning}
	if err := k.SaveRun(exp, running); err != nil {
		t.Fatal(err)
	}
	if err := k.DeleteExperiment(exp); err == nil {
		t.Fatal("running experiment deleted")
	}
	running.Status = core.RunFinished
	if err := k.SaveRun(exp, running); err != nil {
		t.Fatal(err)
	}
	dir, err := k.Attach(exp)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.bin\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "ignored.bin"), []byte("valuable"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = k.Detach(exp); err == nil {
		t.Fatal("ignored data was not protected")
	}
	if err = k.DeleteExperiment(exp); err == nil {
		t.Fatal("dirty experiment deleted")
	}
	if _, err = os.Stat(filepath.Join(dir, "ignored.bin")); err != nil {
		t.Fatal(err)
	}
}

func TestCLIReviewAndExport(t *testing.T) {
	repo, _, _ := setupReviewRuns(t)
	if code, out := run(t, repo, "run-outcome", "20261113-110000-b", "invalid"); code != 0 {
		t.Fatalf("outcome: %s", out)
	}
	if code, out := run(t, repo, "representative", "sensitive/lr", "20261113-100000-a"); code != 0 {
		t.Fatalf("representative: %s", out)
	}
	output := filepath.Join(t.TempDir(), "runs.zip")
	if code, out := run(t, repo, "export", "sensitive/lr", "--output", output); code != 0 {
		t.Fatalf("export: %s", out)
	}
	if _, err := zip.OpenReader(output); err != nil {
		t.Fatal(err)
	}
	if code, _ := run(t, repo, "delete", "sensitive/lr"); code == 0 {
		t.Fatal("delete must require --yes")
	}
}

func TestExportSeparatesResultsWeightsAndLogs(t *testing.T) {
	_, k, exp := setupReviewRuns(t)
	run, err := k.LoadRun(exp, "20261113-110000-b")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"scores.json", "best.pth"} {
		data := []byte(name)
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		object := filepath.Join(k.ObjectsDir(), sum[:2], sum)
		if err = os.MkdirAll(filepath.Dir(object), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(object, data, 0600); err != nil {
			t.Fatal(err)
		}
		run.Artifacts = append(run.Artifacts, core.Artifact{Name: name, SHA256: sum})
	}
	run.Log = k.LogPath(run.ID)
	if err = os.WriteFile(run.Log, []byte("training"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = k.SaveRun(exp, run); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		opts     core.ExportOptions
		expected string
		absent   []string
	}{
		{"logs", core.ExportOptions{Logs: true}, "stdout.log", []string{"scores.json", "best.pth", "metrics.json"}},
		{"results", core.ExportOptions{Results: true}, "scores.json", []string{"stdout.log", "best.pth"}},
		{"weights", core.ExportOptions{Weights: true}, "best.pth", []string{"stdout.log", "scores.json", "metrics.json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := k.Export(&buf, []core.ExportSelection{{Experiment: exp.ID}}, tc.opts); err != nil {
				t.Fatal(err)
			}
			zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, f := range zr.File {
				names = append(names, f.Name)
			}
			joined := strings.Join(names, "\n")
			if !strings.Contains(joined, tc.expected) {
				t.Fatalf("missing %s: %s", tc.expected, joined)
			}
			for _, name := range tc.absent {
				if strings.Contains(joined, name) {
					t.Fatalf("unexpected %s: %s", name, joined)
				}
			}
			if tc.name == "results" && !strings.Contains(joined, "metrics.json") {
				t.Fatal("missing metrics")
			}
		})
	}
}

func TestWebExportRespectsResultOption(t *testing.T) {
	repo, k, exp := setupReviewRuns(t)
	run, err := k.LoadRun(exp, "20261113-110000-b")
	if err != nil {
		t.Fatal(err)
	}
	run.Metrics = map[string]float64{"accuracy": 0.95}
	if err = k.SaveRun(exp, run); err != nil {
		t.Fatal(err)
	}
	h := newHandler(t, repo)
	form := url.Values{"exp": {exp.ID}, "weights": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/export", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "metrics.json") {
			t.Fatal("metrics file exported without selection")
		}
		if strings.HasSuffix(f.Name, "run.json") {
			reader, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "accuracy") {
				t.Fatal("metrics leaked into metadata")
			}
		}
	}
}
