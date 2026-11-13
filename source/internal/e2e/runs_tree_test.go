package e2e

import (
	"expdeck/internal/core"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestTreeShowsRunsAndSelectedRunDetails(t *testing.T) {
	repo := scaffold(t)
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, repo, "new", "empty/test", "--why", "no run"); code != 0 {
		t.Fatalf("new: %s", out)
	}
	for _, id := range []string{"20261113-120001-a1b2", "20261113-120002-c3d4"} {
		r := &core.Run{ID: id, Exp: exp.ID, Status: core.RunFinished, Command: "python train.py", Log: k.LogPath(id), StartedAt: "2026-11-13T12:00:00Z"}
		if err = k.SaveRun(exp, r); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(k.LogPath(id), []byte("training complete\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := newHandler(t, repo)
	code, body := get(t, h, "/")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{`data-run-toggle="` + exp.ID + `"`, `?run=20261113-120001-a1b2`, `?run=20261113-120002-c3d4`, `class="rail-new"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	code, body = get(t, h, "/e/sensitive/lr?run=20261113-120001-a1b2")
	if code != http.StatusOK || !strings.Contains(body, "python train.py") || !strings.Contains(body, "training complete") {
		t.Fatalf("run details unavailable: %d %s", code, body)
	}
	code, _ = get(t, h, "/e/empty/test?run=20261113-120001-a1b2")
	if code != http.StatusNotFound {
		t.Fatalf("run from another experiment accepted: %d", code)
	}
	code, _ = get(t, h, "/e/sensitive/lr?run=..%2F..%2Fanything")
	if code != http.StatusNotFound {
		t.Fatalf("invalid run accepted: %d", code)
	}
}
