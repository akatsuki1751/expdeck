package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"expdeck/internal/cli"
)

// run 在一次调用里跑一条命令，返回退出码与合并输出。
func run(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := cli.Run(cli.Env{Dir: dir, Stdout: &buf, Stderr: &buf, IsTTY: false}, args)
	return code, buf.String()
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// newRepo 建一个带一次提交的临时仓库。
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "-c", "user.email=a@b.c", "-c", "user.name=test",
		"commit", "-q", "--allow-empty", "-m", "init")
	return dir
}

func dataDir(t *testing.T, repo string) string {
	t.Helper()
	return filepath.Join(repo, ".git", "exp")
}

func TestInitCreatesLayout(t *testing.T) {
	repo := newRepo(t)

	code, out := run(t, repo, "init")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(out, `"ok": true`) {
		t.Fatalf("expected ok envelope, got %s", out)
	}
	for _, want := range []string{
		filepath.Join(".git", "exp", "records", "exps"),
		filepath.Join(".git", "exp", "registry"),
	} {
		if _, err := os.Stat(filepath.Join(repo, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "exp.toml")); err != nil {
		t.Errorf("exp.toml not written: %v", err)
	}
	agents, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil || !strings.Contains(string(agents), "exp:agent-instructions") {
		t.Errorf("AGENTS.md block missing: %v", err)
	}
}

func TestInitOutsideRepoFails(t *testing.T) {
	dir := t.TempDir()

	code, out := run(t, dir, "init")
	if code != cli.ExitInvalid {
		t.Fatalf("want exit %d, got %d (%s)", cli.ExitInvalid, code, out)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("init created files outside a repo: %v", entries)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	repo := newRepo(t)
	if code, out := run(t, repo, "init"); code != 0 {
		t.Fatalf("first init failed: %d %s", code, out)
	}
	if code, out := run(t, repo, "init"); code != 0 {
		t.Fatalf("second init failed: %d %s", code, out)
	}
	agents, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if strings.Count(string(agents), "exp:agent-instructions") != 1 {
		t.Fatalf("agents block duplicated:\n%s", agents)
	}
}

func TestNewRequiresHypothesis(t *testing.T) {
	repo := newRepo(t)
	if code, out := run(t, repo, "init"); code != 0 {
		t.Fatalf("init: %d %s", code, out)
	}

	code, out := run(t, repo, "new", "sensitive/lr")
	if code != cli.ExitInvalid {
		t.Fatalf("want exit %d, got %d (%s)", cli.ExitInvalid, code, out)
	}
	if branches := git(t, repo, "branch", "--list", "exp/*"); strings.TrimSpace(branches) != "" {
		t.Fatalf("branch created despite missing hypothesis: %s", branches)
	}
	expDir := filepath.Join(dataDir(t, repo), "records", "exps")
	entries, _ := os.ReadDir(expDir)
	if len(entries) != 0 {
		t.Fatalf("record created despite missing hypothesis: %v", entries)
	}
}

func TestNewCreatesBranchAndRecord(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "init")

	code, out := run(t, repo, "new", "sensitive/lr", "--why", "lr 降到 1e-4")
	if code != 0 {
		t.Fatalf("new failed: %d %s", code, out)
	}
	if branches := git(t, repo, "branch", "--list", "exp/sensitive/lr"); strings.TrimSpace(branches) == "" {
		t.Fatal("branch exp/sensitive/lr was not created")
	}

	code, out = run(t, repo, "ls")
	if code != 0 {
		t.Fatalf("ls failed: %d %s", code, out)
	}
	for _, want := range []string{"sensitive", "lr", "lr 降到 1e-4", "active"} {
		if !strings.Contains(out, want) {
			t.Errorf("ls output missing %q:\n%s", want, out)
		}
	}
}

func TestShowUnknownExperiment(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "init")

	code, out := run(t, repo, "show", "nope")
	if code != cli.ExitNotFound {
		t.Fatalf("want exit %d, got %d (%s)", cli.ExitNotFound, code, out)
	}
}

func TestOverlappingPathsConflict(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "init")
	if code, out := run(t, repo, "new", "a/b", "--why", "child"); code != 0 {
		t.Fatalf("new a/b: %d %s", code, out)
	}

	code, out := run(t, repo, "new", "a", "--why", "parent")
	if code != cli.ExitConflict {
		t.Fatalf("want exit %d, got %d (%s)", cli.ExitConflict, code, out)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "init")

	code, _ := run(t, repo, "new", "dry/only", "--why", "x", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run failed: %d", code)
	}
	if branches := git(t, repo, "branch", "--list", "exp/*"); strings.TrimSpace(branches) != "" {
		t.Fatalf("dry run created a branch: %s", branches)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	repo := newRepo(t)
	if code, _ := run(t, repo, "bogus"); code != cli.ExitUsage {
		t.Fatalf("want exit %d, got %d", cli.ExitUsage, code)
	}
}
