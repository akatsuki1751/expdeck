package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"expdeck/internal/cli"
)

// runJSON 跑一条命令并解析信封。
func runJSON(t *testing.T, dir string, args ...string) (int, map[string]any) {
	t.Helper()
	code, out := run(t, dir, args...)
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not JSON (exit %d): %s", code, out)
	}
	return code, payload
}

func dataOf(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok envelope, got %v", payload)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", payload["data"])
	}
	return data
}

func scaffold(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	if code, out := run(t, repo, "init"); code != 0 {
		t.Fatalf("init: %d %s", code, out)
	}
	if code, out := run(t, repo, "new", "sensitive/lr", "--why", "lr 降到 1e-4"); code != 0 {
		t.Fatalf("new: %d %s", code, out)
	}
	return repo
}

func TestNoteReplaceAndAppend(t *testing.T) {
	repo := scaffold(t)

	code, payload := runJSON(t, repo, "note", "sensitive/lr", "--text", "第一段")
	if code != 0 {
		t.Fatalf("note --text: %d %v", code, payload)
	}
	first := dataOf(t, payload)["version"].(string)
	if first == "" {
		t.Fatal("expected a version hash")
	}

	code, payload = runJSON(t, repo, "note", "sensitive/lr", "--append", "又试了一次")
	if code != 0 {
		t.Fatalf("note --append: %d %v", code, payload)
	}
	if got := dataOf(t, payload)["version"].(string); got == first {
		t.Fatal("append did not change the version")
	}

	code, payload = runJSON(t, repo, "note", "sensitive/lr", "--print")
	if code != 0 {
		t.Fatalf("note --print: %d %v", code, payload)
	}
	content := dataOf(t, payload)["content"].(string)
	if !strings.Contains(content, "第一段") || !strings.Contains(content, "又试了一次") {
		t.Fatalf("append lost content:\n%s", content)
	}
	if !strings.Contains(content, "## ") {
		t.Fatalf("append should add a timestamped section:\n%s", content)
	}
}

func TestNoteStaleVersionConflicts(t *testing.T) {
	repo := scaffold(t)

	code, payload := runJSON(t, repo, "note", "sensitive/lr", "--text", "v1")
	if code != 0 {
		t.Fatalf("first write: %d %v", code, payload)
	}
	v1 := dataOf(t, payload)["version"].(string)

	code, payload = runJSON(t, repo, "note", "sensitive/lr", "--text", "v2", "--expect", v1)
	if code != 0 {
		t.Fatalf("in-sequence write should succeed: %d %v", code, payload)
	}

	code, payload = runJSON(t, repo, "note", "sensitive/lr", "--text", "v3", "--expect", v1)
	if code != cli.ExitConflict {
		t.Fatalf("want exit %d on stale version, got %d %v", cli.ExitConflict, code, payload)
	}

	code, payload = runJSON(t, repo, "note", "sensitive/lr", "--print")
	if code != 0 {
		t.Fatalf("print: %d", code)
	}
	if got := dataOf(t, payload)["content"].(string); got != "v2" {
		t.Fatalf("conflicting write changed the file: %q", got)
	}
}

func TestNoteDryRunWritesNothing(t *testing.T) {
	repo := scaffold(t)

	if code, payload := runJSON(t, repo, "note", "sensitive/lr", "--text", "x", "--dry-run"); code != 0 {
		t.Fatalf("dry run: %d %v", code, payload)
	}
	code, payload := runJSON(t, repo, "note", "sensitive/lr", "--print")
	if code != 0 {
		t.Fatalf("print: %d", code)
	}
	if got := dataOf(t, payload)["content"].(string); got != "" {
		t.Fatalf("dry run wrote the note: %q", got)
	}
}

func TestNoteRequiresExactlyOneMode(t *testing.T) {
	repo := scaffold(t)

	if code, _ := run(t, repo, "note", "sensitive/lr"); code != cli.ExitUsage {
		t.Fatalf("no mode should be a usage error, got %d", code)
	}
	if code, _ := run(t, repo, "note", "sensitive/lr", "--text", "a", "--append", "b"); code != cli.ExitUsage {
		t.Fatalf("two modes should be a usage error, got %d", code)
	}
}

func TestConcludeAndReopen(t *testing.T) {
	repo := scaffold(t)

	code, payload := runJSON(t, repo, "conclude", "sensitive/lr", "--decision", "abandon", "--why", "验证集过拟合")
	if code != 0 {
		t.Fatalf("conclude: %d %v", code, payload)
	}
	if got := dataOf(t, payload)["status"]; got != "abandoned" {
		t.Fatalf("want abandoned, got %v", got)
	}

	if code, payload := runJSON(t, repo, "conclude", "sensitive/lr", "--decision", "keep", "--why", "x"); code != cli.ExitConflict {
		t.Fatalf("second conclude should conflict, got %d %v", code, payload)
	}

	code, payload = runJSON(t, repo, "reopen", "sensitive/lr")
	if code != 0 {
		t.Fatalf("reopen: %d %v", code, payload)
	}
	if got := dataOf(t, payload)["status"]; got != "active" {
		t.Fatalf("want active after reopen, got %v", got)
	}
}

func TestConcludeRequiresReasonAndKnownDecision(t *testing.T) {
	repo := scaffold(t)

	if code, _ := run(t, repo, "conclude", "sensitive/lr", "--decision", "abandon"); code != cli.ExitInvalid {
		t.Fatalf("missing --why should be invalid, got %d", code)
	}
	if code, _ := run(t, repo, "conclude", "sensitive/lr", "--decision", "maybe", "--why", "x"); code != cli.ExitInvalid {
		t.Fatalf("unknown decision should be invalid, got %d", code)
	}
}
