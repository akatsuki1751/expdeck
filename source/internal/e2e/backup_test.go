package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"expdeck/internal/cli"
)

func recordsDir(repo string) string {
	return filepath.Join(repo, ".git", "exp", "records")
}

func TestBackupCreatesBookBranch(t *testing.T) {
	repo := scaffold(t)

	code, payload := runJSON(t, repo, "backup")
	if code != 0 {
		t.Fatalf("backup: %d %v", code, payload)
	}
	if out := git(t, repo, "branch", "--list", "book"); !strings.Contains(out, "book") {
		t.Fatalf("book branch missing:\n%s", out)
	}
	if out := git(t, repo, "ls-tree", "-r", "--name-only", "book"); !strings.Contains(out, "meta.json") {
		t.Fatalf("book tree is empty:\n%s", out)
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	repo := scaffold(t)

	if code, payload := runJSON(t, repo, "note", "sensitive/lr", "--text", "这段必须在恢复后还在"); code != 0 {
		t.Fatalf("note: %d %v", code, payload)
	}
	if code, payload := runJSON(t, repo, "backup"); code != 0 {
		t.Fatalf("backup: %d %v", code, payload)
	}

	if err := os.RemoveAll(recordsDir(repo)); err != nil {
		t.Fatal(err)
	}

	code, payload := runJSON(t, repo, "restore")
	if code != 0 {
		t.Fatalf("restore: %d %v", code, payload)
	}

	code, payload = runJSON(t, repo, "note", "sensitive/lr", "--print")
	if code != 0 {
		t.Fatalf("note after restore: %d %v", code, payload)
	}
	if got := dataOf(t, payload)["content"].(string); got != "这段必须在恢复后还在" {
		t.Fatalf("note lost after restore: %q", got)
	}

	code, payload = runJSON(t, repo, "ls")
	if code != 0 {
		t.Fatalf("ls after restore: %d %v", code, payload)
	}
	if !strings.Contains(strings.Join(keysOf(payload), " "), "sensitive") {
		t.Logf("ls payload: %v", payload)
	}
}

func keysOf(payload map[string]any) []string {
	var out []string
	for k := range payload {
		out = append(out, k)
	}
	return out
}

func TestRestoreRefusesToClobberRecords(t *testing.T) {
	repo := scaffold(t)
	if code, payload := runJSON(t, repo, "backup"); code != 0 {
		t.Fatalf("backup: %d %v", code, payload)
	}

	code, _ := run(t, repo, "restore")
	if code != cli.ExitConflict {
		t.Fatalf("restore over existing records should conflict, got %d", code)
	}

	code, payload := runJSON(t, repo, "restore", "--force")
	if code != 0 {
		t.Fatalf("restore --force: %d %v", code, payload)
	}
}

func TestRestoreWithoutBookFails(t *testing.T) {
	repo := scaffold(t)
	if code, _ := run(t, repo, "restore"); code != cli.ExitNotFound {
		t.Fatalf("want exit %d without a book branch, got %d", cli.ExitNotFound, code)
	}
}

func TestBackupDryRunWritesNothing(t *testing.T) {
	repo := scaffold(t)
	if code, payload := runJSON(t, repo, "backup", "--dry-run"); code != 0 {
		t.Fatalf("dry run: %d %v", code, payload)
	}
	if out := git(t, repo, "branch", "--list", "book"); strings.TrimSpace(out) != "" {
		t.Fatalf("dry run created the book branch:\n%s", out)
	}
}
