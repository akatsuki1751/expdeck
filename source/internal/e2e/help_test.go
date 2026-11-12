package e2e

import (
	"strings"
	"testing"

	"expdeck/internal/cli"
)

func TestHelpExitsZero(t *testing.T) {
	repo := newRepo(t)

	code, out := run(t, repo, "--help")
	if code != cli.ExitOK {
		t.Fatalf("want exit 0, got %d", code)
	}
	for _, want := range []string{"ExpDeck", "用法", "new <path>", "退出码"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestHelpCommandMatchesFlag(t *testing.T) {
	repo := newRepo(t)

	flagCode, flagText := run(t, repo, "--help")
	helpCode, helpText := run(t, repo, "help")
	if flagCode != cli.ExitOK || helpCode != cli.ExitOK {
		t.Fatalf("help should exit 0, got %d and %d", flagCode, helpCode)
	}
	if flagText != helpText {
		t.Fatal("`exp help` and `exp --help` should print the same text")
	}
}

func TestNoArgumentsIsUsageError(t *testing.T) {
	repo := newRepo(t)

	code, out := run(t, repo)
	if code != cli.ExitUsage {
		t.Fatalf("want exit %d, got %d", cli.ExitUsage, code)
	}
	if !strings.Contains(out, "用法") {
		t.Fatalf("no-arg run should show usage, got %q", out)
	}
}

func TestVersionReportsInjectedValue(t *testing.T) {
	repo := newRepo(t)

	code, payload := runJSON(t, repo, "version")
	if code != 0 {
		t.Fatalf("version: %d %v", code, payload)
	}
	if got := dataOf(t, payload)["version"]; got == nil || got == "" {
		t.Fatalf("version is empty: %v", payload)
	}
}

func TestUnknownCommandStillUsageError(t *testing.T) {
	repo := newRepo(t)

	code, _ := run(t, repo, "nonsense")
	if code != cli.ExitUsage {
		t.Fatalf("want exit %d, got %d", cli.ExitUsage, code)
	}
}
