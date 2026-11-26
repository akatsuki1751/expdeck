package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenCommonDirFromSubdirectoriesAndWorktree(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git(repo, "init")
	git(repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	nested := filepath.Join(repo, "one", "two")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "worktree")
	git(repo, "worktree", "add", "-b", "other", wt)
	wtNested := filepath.Join(wt, "nested")
	if err := os.Mkdir(wtNested, 0755); err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{repo, nested, wt, wtNested} {
		t.Run(dir, func(t *testing.T) {
			got, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := filepath.EvalSymlinks(got.CommonDir)
			if err != nil {
				t.Fatal(err)
			}
			if actual != expected {
				t.Fatalf("common dir = %q, want %q", actual, expected)
			}
		})
	}
}
