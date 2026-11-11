package e2e

import (
	"strings"
	"testing"
)

func TestDoctorReportsNoDriftOnHealthyRepo(t *testing.T) {
	repo := scaffold(t)

	code, payload := runJSON(t, repo, "doctor")
	if code != 0 {
		t.Fatalf("doctor: %d %v", code, payload)
	}
	if drifts := dataOf(t, payload)["drifts"]; drifts == nil {
		t.Fatalf("drifts should be an empty list, got %v", payload)
	}
}

func TestDoctorReportsOrphanExperiment(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "-D", "exp/sensitive/lr")

	code, payload := runJSON(t, repo, "doctor")
	if code != 0 {
		t.Fatalf("doctor: %d %v", code, payload)
	}
	body := stringOf(payload)
	if !strings.Contains(body, "orphan_experiment") || !strings.Contains(body, "sensitive/lr") {
		t.Fatalf("orphan experiment not reported: %s", body)
	}
}

func TestDoctorReportsUnregisteredBranch(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "exp/manual-only")

	code, payload := runJSON(t, repo, "doctor")
	if code != 0 {
		t.Fatalf("doctor: %d %v", code, payload)
	}
	body := stringOf(payload)
	if !strings.Contains(body, "unregistered_branch") || !strings.Contains(body, "exp/manual-only") {
		t.Fatalf("unregistered branch not reported: %s", body)
	}
}

func TestDoctorOnlyReports(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "-D", "exp/sensitive/lr")

	runJSON(t, repo, "doctor")

	if branches := git(t, repo, "branch", "--list", "exp/sensitive/lr"); strings.TrimSpace(branches) != "" {
		t.Fatalf("doctor created a branch: %s", branches)
	}
}
