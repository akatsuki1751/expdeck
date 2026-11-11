package e2e

import (
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
