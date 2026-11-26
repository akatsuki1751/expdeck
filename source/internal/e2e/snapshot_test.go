package e2e

import (
	"expdeck/internal/core"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func worktreeOf(repo string) string {
	return filepath.Join(attachRoot(repo), "sensitive", "lr")
}

func TestRunSnapshotIncludesUntrackedAndSurvivesGC(t *testing.T) {
	repo := runnable(t, "echo snap\n")
	defer os.RemoveAll(attachRoot(repo))

	if err := os.WriteFile(filepath.Join(worktreeOf(repo), "scratch_gpu.py"), []byte("patch = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	runID := runField(t, payload)["id"].(string)
	if code, payload := runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}

	code, payload := runBinJSON(t, repo, "show", runID)
	if code != 0 {
		t.Fatalf("show: %d %v", code, payload)
	}
	snapshot, _ := runField(t, payload)["snapshot"].(string)
	if snapshot == "" {
		t.Fatal("run has no snapshot")
	}

	code, payload = runBinJSON(t, repo, "show", runID, "--diff")
	if code != 0 {
		t.Fatalf("show --diff: %d %v", code, payload)
	}
	diff, _ := payload["data"].(map[string]any)["diff"].(string)
	if !strings.Contains(diff, "scratch_gpu.py") {
		t.Fatalf("diff does not include the untracked file:\n%s", diff)
	}

	if refs := git(t, repo, "for-each-ref", "--format=%(refname)", "refs/exp/snap"); !strings.Contains(refs, "refs/exp/snap") {
		t.Fatalf("no snapshot ref:\n%s", refs)
	}
	git(t, repo, "gc", "--prune=now")
	if out := git(t, repo, "cat-file", "-p", snapshot); strings.TrimSpace(out) == "" {
		t.Fatal("snapshot object was collected by gc")
	}
}

func TestSecondRunDiffShowsOnlyItsOwnChanges(t *testing.T) {
	repo := runnable(t, "echo one\n")
	defer os.RemoveAll(attachRoot(repo))

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	first := runField(t, payload)["id"].(string)
	runBinJSON(t, repo, "wait", "--run", first, "--timeout", "20")

	if err := os.WriteFile(filepath.Join(worktreeOf(repo), "second_change.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, payload = runBinJSON(t, repo, "run", "sensitive/lr")
	second := runField(t, payload)["id"].(string)
	if code, payload := runBinJSON(t, repo, "wait", "--run", second, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}

	code, payload := runBinJSON(t, repo, "show", second, "--diff")
	if code != 0 {
		t.Fatalf("show --diff: %d %v", code, payload)
	}
	diff, _ := payload["data"].(map[string]any)["diff"].(string)
	if !strings.Contains(diff, "second_change.txt") {
		t.Fatalf("diff missing the new change:\n%s", diff)
	}
	if strings.Contains(diff, "run.sh") {
		t.Fatalf("diff should not repeat the baseline files:\n%s", diff)
	}
}

func TestWebRunAndRepresentativeDiffs(t *testing.T) {
	repo := runnable(t, "echo ok\n")
	defer os.RemoveAll(attachRoot(repo))
	tree := worktreeOf(repo)
	runOne := func(file string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tree, file), []byte("content "+file+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
		if code != 0 {
			t.Fatalf("run: %v", payload)
		}
		id := runField(t, payload)["id"].(string)
		if code, payload = runBinJSON(t, repo, "wait", "--run", id, "--timeout", "20"); code != 0 {
			t.Fatalf("wait: %v", payload)
		}
		return id
	}
	first := runOne("first-only.txt")
	second := runOne("second-only.txt")
	third := runOne("third-only.txt")
	h := newHandler(t, repo)
	check := func(path string, present, absent []string) {
		t.Helper()
		code, body := get(t, h, path)
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, body)
		}
		for _, x := range present {
			if !strings.Contains(body, x) {
				t.Errorf("%s missing %q", path, x)
			}
		}
		for _, x := range absent {
			if strings.Contains(body, x) {
				t.Errorf("%s unexpectedly contains %q", path, x)
			}
		}
	}
	check("/e/sensitive/lr?run="+url.QueryEscape(first), []string{"相对实验起点的改动", "first-only.txt"}, []string{"second-only.txt", "third-only.txt"})
	check("/e/sensitive/lr?run="+url.QueryEscape(second), []string{"相对上一次运行的改动", "second-only.txt"}, []string{"first-only.txt", "third-only.txt"})
	check("/e/sensitive/lr", []string{"实验整体改动", "first-only.txt", "second-only.txt", "third-only.txt"}, []string{"run.sh"})
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	if err = k.SetRunOutcome(exp, third, "invalid"); err != nil {
		t.Fatal(err)
	}
	check("/e/sensitive/lr", []string{"first-only.txt", "second-only.txt"}, []string{"third-only.txt", "run.sh"})
	if err = k.SetRepresentative(exp, first); err != nil {
		t.Fatal(err)
	}
	check("/e/sensitive/lr", []string{"first-only.txt"}, []string{"second-only.txt", "third-only.txt", "run.sh"})
}

func TestFirstRunComparesWithCreationCommit(t *testing.T) {
	repo := runnable(t, "echo ok\n")
	defer os.RemoveAll(attachRoot(repo))
	wt := worktreeOf(repo)
	if err := os.WriteFile(filepath.Join(wt, "committed-after-branch.txt"), []byte("new code\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, wt, "add", "committed-after-branch.txt")
	git(t, wt, "-c", "user.email=test@example.invalid", "-c", "user.name=Test", "commit", "-qm", "experiment work")
	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 0 {
		t.Fatalf("run: %v", payload)
	}
	id := runField(t, payload)["id"].(string)
	if code, payload = runBinJSON(t, repo, "wait", "--run", id, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %v", payload)
	}
	code, payload = runBinJSON(t, repo, "show", id, "--diff")
	if code != 0 {
		t.Fatalf("show: %v", payload)
	}
	diff := payload["data"].(map[string]any)["diff"].(string)
	if !strings.Contains(diff, "committed-after-branch.txt") || strings.Contains(diff, "run.sh") {
		t.Fatalf("wrong first baseline: %s", diff)
	}
}

func TestLatestRunUsesPreciseStartTime(t *testing.T) {
	repo := scaffold(t)
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	first := &core.Run{ID: "20261126-120000-ffff", Exp: exp.ID, Status: core.RunFinished, StartedAt: "2026-11-26T12:00:00.100Z"}
	second := &core.Run{ID: "20261126-120000-0000", Exp: exp.ID, Status: core.RunFinished, StartedAt: "2026-11-26T12:00:00.200Z"}
	if err = k.SaveRun(exp, first); err != nil {
		t.Fatal(err)
	}
	if err = k.SaveRun(exp, second); err != nil {
		t.Fatal(err)
	}
	run, _, err := k.Representative(exp)
	if err != nil || run == nil || run.ID != second.ID {
		t.Fatalf("wrong latest run: %+v %v", run, err)
	}
}
