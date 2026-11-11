package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"expdeck/internal/cli"
)

func attachRoot(repo string) string {
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-exp")
}

func TestAttachCreatesWorktree(t *testing.T) {
	repo := scaffold(t)
	defer os.RemoveAll(attachRoot(repo))

	code, payload := runJSON(t, repo, "attach", "sensitive/lr")
	if code != 0 {
		t.Fatalf("attach: %d %v", code, payload)
	}
	wt, _ := dataOf(t, payload)["worktree"].(string)
	if wt == "" {
		t.Fatalf("no worktree in payload: %v", payload)
	}
	if fi, err := os.Stat(wt); err != nil || !fi.IsDir() {
		t.Fatalf("worktree directory missing: %v", err)
	}
	if out := git(t, repo, "worktree", "list", "--porcelain"); !strings.Contains(out, "exp/sensitive/lr") {
		t.Fatalf("branch not checked out anywhere:\n%s", out)
	}
}

func TestAttachIsIdempotent(t *testing.T) {
	repo := scaffold(t)
	defer os.RemoveAll(attachRoot(repo))

	_, first := runJSON(t, repo, "attach", "sensitive/lr")
	code, second := runJSON(t, repo, "attach", "sensitive/lr")
	if code != 0 {
		t.Fatalf("second attach failed: %d %v", code, second)
	}
	if a, b := dataOf(t, first)["worktree"], dataOf(t, second)["worktree"]; a != b {
		t.Fatalf("attach moved the worktree: %v vs %v", a, b)
	}
}

func TestDetachKeepsBranchAndCapturesUntracked(t *testing.T) {
	repo := scaffold(t)
	defer os.RemoveAll(attachRoot(repo))

	_, payload := runJSON(t, repo, "attach", "sensitive/lr")
	wt := dataOf(t, payload)["worktree"].(string)
	untracked := filepath.Join(wt, "hack_datasets.py")
	if err := os.WriteFile(untracked, []byte("print('quick fix')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, payload := runJSON(t, repo, "detach", "sensitive/lr")
	if code != 0 {
		t.Fatalf("detach: %d %v", code, payload)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree still on disk: %v", err)
	}
	if out := git(t, repo, "branch", "--list", "exp/sensitive/lr"); !strings.Contains(out, "exp/sensitive/lr") {
		t.Fatalf("detach deleted the branch:\n%s", out)
	}

	patches, err := filepath.Glob(filepath.Join(repo, ".git", "exp", "records", "exps", "*", "dirty.patch"))
	if err != nil || len(patches) != 1 {
		t.Fatalf("expected one dirty patch, got %v (%v)", patches, err)
	}
	body, err := os.ReadFile(patches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "hack_datasets.py") {
		t.Fatalf("patch lost the untracked file:\n%s", body)
	}
}

func TestDetachWithoutWorktreeFails(t *testing.T) {
	repo := scaffold(t)
	code, _ := run(t, repo, "detach", "sensitive/lr")
	if code != cli.ExitNotFound {
		t.Fatalf("want exit %d, got %d", cli.ExitNotFound, code)
	}
}

func TestDetachKeepsRecords(t *testing.T) {
	repo := scaffold(t)
	defer os.RemoveAll(attachRoot(repo))

	runJSON(t, repo, "note", "sensitive/lr", "--text", "观察记录")
	runJSON(t, repo, "attach", "sensitive/lr")
	if code, payload := runJSON(t, repo, "detach", "sensitive/lr"); code != 0 {
		t.Fatalf("detach: %d %v", code, payload)
	}

	code, payload := runJSON(t, repo, "note", "sensitive/lr", "--print")
	if code != 0 {
		t.Fatalf("note after detach: %d %v", code, payload)
	}
	if got := dataOf(t, payload)["content"].(string); got != "观察记录" {
		t.Fatalf("record lost after detach: %q", got)
	}
}
