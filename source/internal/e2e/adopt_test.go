package e2e

import (
	"strings"
	"testing"

	"expdeck/internal/cli"
)

func TestLsReportsUnregisteredBranch(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "exp/ablation/sigreg")

	code, payload := runJSON(t, repo, "ls")
	if code != 0 {
		t.Fatalf("ls: %d %v", code, payload)
	}
	data := dataOf(t, payload)
	unregistered, _ := data["unregistered"].([]any)
	if len(unregistered) != 1 || unregistered[0] != "exp/ablation/sigreg" {
		t.Fatalf("unregistered branches wrong: %v", data["unregistered"])
	}
	experiments, _ := data["experiments"].([]any)
	if len(experiments) != 1 {
		t.Fatalf("unregistered branch leaked into the tree: %v", experiments)
	}
}

func TestAdoptRegistersExistingBranch(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "exp/ablation/sigreg")

	code, payload := runJSON(t, repo, "adopt", "ablation/sigreg", "--why", "sigreg 消融")
	if code != 0 {
		t.Fatalf("adopt: %d %v", code, payload)
	}
	exp := dataOf(t, payload)["experiment"].(map[string]any)
	if exp["branch"] != "exp/ablation/sigreg" {
		t.Fatalf("adopted wrong branch: %v", exp["branch"])
	}

	code, payload = runJSON(t, repo, "ls")
	if code != 0 {
		t.Fatalf("ls: %d", code)
	}
	data := dataOf(t, payload)
	if unregistered, _ := data["unregistered"].([]any); len(unregistered) != 0 {
		t.Fatalf("still unregistered after adopt: %v", unregistered)
	}
	if body := stringOf(data["experiments"]); !strings.Contains(body, "sigreg") {
		t.Fatalf("adopted experiment missing from tree: %s", body)
	}

	if code, payload := runJSON(t, repo, "show", "ablation/sigreg"); code != 0 {
		t.Fatalf("show after adopt: %d %v", code, payload)
	}
}

func TestAdoptIsIdempotent(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "exp/ablation/sigreg")

	_, first := runJSON(t, repo, "adopt", "ablation/sigreg")
	code, second := runJSON(t, repo, "adopt", "exp/ablation/sigreg")
	if code != 0 {
		t.Fatalf("second adopt: %d %v", code, second)
	}
	firstID := dataOf(t, first)["experiment"].(map[string]any)["id"]
	secondID := dataOf(t, second)["experiment"].(map[string]any)["id"]
	if firstID != secondID {
		t.Fatalf("adopt created a second record: %v vs %v", firstID, secondID)
	}
}

func TestAdoptMissingBranch(t *testing.T) {
	repo := scaffold(t)

	code, _ := run(t, repo, "adopt", "exp/does-not-exist")
	if code != cli.ExitNotFound {
		t.Fatalf("want exit %d, got %d", cli.ExitNotFound, code)
	}
}

func TestDoctorSeesAdoptedBranchAsRegistered(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "exp/ablation/sigreg")
	runJSON(t, repo, "adopt", "ablation/sigreg")

	code, payload := runJSON(t, repo, "doctor")
	if code != 0 {
		t.Fatalf("doctor: %d %v", code, payload)
	}
	if drifts := stringOf(dataOf(t, payload)["drifts"]); strings.Contains(drifts, "unregistered_branch") {
		t.Fatalf("adopted branch still reported as drift: %s", drifts)
	}
}
