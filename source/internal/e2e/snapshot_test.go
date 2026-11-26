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

func TestRunRequiresCommittedChangesAndUsesBranchCommit(t *testing.T) {
	repo := runnable(t, "echo snap\n")
	defer os.RemoveAll(attachRoot(repo))
	wt := worktreeOf(repo)
	if err := os.WriteFile(filepath.Join(wt, "scratch_gpu.py"), []byte("patch = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD"))
	code, out := runBin(t, repo, "run", "sensitive/lr")
	if code == 0 || !strings.Contains(out, "请先手动提交") {
		t.Fatalf("dirty run accepted: %d %s", code, out)
	}
	if got := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD")); got != before {
		t.Fatal("run changed HEAD")
	}
	if got := git(t, wt, "status", "--porcelain"); !strings.Contains(got, "scratch_gpu.py") {
		t.Fatal("untracked file was modified")
	}
	git(t, wt, "add", "scratch_gpu.py")
	git(t, wt, "-c", "user.email=test@example.invalid", "-c", "user.name=Test", "commit", "-qm", "manual experiment change")
	head := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD"))
	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 0 {
		t.Fatalf("run: %v", payload)
	}
	id := runField(t, payload)["id"].(string)
	if code, payload = runBinJSON(t, repo, "wait", "--run", id, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %v", payload)
	}
	_, payload = runBinJSON(t, repo, "show", id, "--diff")
	if got := runField(t, payload)["snapshot"]; got != head {
		t.Fatalf("recorded commit %v, want %s", got, head)
	}
	if !strings.Contains(payload["data"].(map[string]any)["diff"].(string), "scratch_gpu.py") {
		t.Fatal("committed change missing from diff")
	}
	if refs := git(t, repo, "for-each-ref", "--format=%(refname)", "refs/exp/snap"); strings.TrimSpace(refs) != "" {
		t.Fatalf("new run created legacy ref: %s", refs)
	}
	git(t, repo, "gc", "--prune=now")
	if out := git(t, repo, "cat-file", "-p", head); strings.TrimSpace(out) == "" {
		t.Fatal("branch commit was collected")
	}
	_, doctor := runJSON(t, repo, "doctor")
	if strings.Contains(stringOf(doctor), "stale_snapshot") {
		t.Fatalf("normal commit flagged stale: %v", doctor)
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

	git(t, worktreeOf(repo), "add", "second_change.txt")
	git(t, worktreeOf(repo), "-c", "user.email=test@example.invalid", "-c", "user.name=Test", "commit", "-qm", "second change")
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
		git(t, tree, "add", file)
		git(t, tree, "-c", "user.email=test@example.invalid", "-c", "user.name=Test", "commit", "-qm", file)
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

func TestLegacySnapshotRemainsReadableAndNextRunUsesBranch(t *testing.T) {
	repo := runnable(t, "echo ok\n")
	defer os.RemoveAll(attachRoot(repo))
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(git(t, repo, "rev-parse", exp.Branch+"^{tree}"))
	old := strings.TrimSpace(git(t, repo, "-c", "user.email=test@example.invalid", "-c", "user.name=Test", "commit-tree", tree, "-m", "legacy snapshot"))
	git(t, repo, "update-ref", k.SnapRef(exp), old)
	legacy := &core.Run{ID: "20200101-000000-0000", Exp: exp.ID, Snapshot: old, Base: exp.StartCommit, Status: core.RunFinished, StartedAt: "2020-01-01T00:00:00Z"}
	if err = k.SaveRun(exp, legacy); err != nil {
		t.Fatal(err)
	}
	wt := worktreeOf(repo)
	if err = os.WriteFile(filepath.Join(wt, "branch-change.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, wt, "add", "branch-change.txt")
	git(t, wt, "-c", "user.email=test@example.invalid", "-c", "user.name=Test", "commit", "-qm", "regular commit")
	head := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD"))
	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 0 {
		t.Fatalf("run: %v", payload)
	}
	id := runField(t, payload)["id"].(string)
	if code, payload = runBinJSON(t, repo, "wait", "--run", id, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %v", payload)
	}
	_, payload = runBinJSON(t, repo, "show", id, "--diff")
	r := runField(t, payload)
	if r["snapshot"] != head || r["base"] != old {
		t.Fatalf("unexpected commit/base: %v", r)
	}
	if !strings.Contains(payload["data"].(map[string]any)["diff"].(string), "branch-change.txt") {
		t.Fatal("missing change")
	}
	if got := strings.TrimSpace(git(t, repo, "rev-parse", k.SnapRef(exp))); got != old {
		t.Fatal("legacy ref modified")
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	git(t, repo, "init", "--bare", "-q", remote)
	git(t, repo, "push", "-q", remote, exp.Branch)
	if got := strings.TrimSpace(git(t, remote, "rev-parse", "refs/heads/"+exp.Branch)); got != head {
		t.Fatalf("remote branch missing run commit: %s", got)
	}
}

func TestDirtyWorktreeRejectedByCLIAndWeb(t *testing.T) {
	repo := runnable(t, "echo ok\n")
	defer os.RemoveAll(attachRoot(repo))
	wt := worktreeOf(repo)
	file := filepath.Join(wt, "run.sh")
	if err := os.WriteFile(file, []byte("echo changed\n"), 0755); err != nil {
		t.Fatal(err)
	}
	before := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD"))
	h := newHandler(t, repo)
	rec := post(t, h, "/e/sensitive/lr/run")
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Fatalf("web accepted dirty worktree: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	git(t, wt, "add", "run.sh")
	code, out := runBin(t, repo, "run", "sensitive/lr")
	if code == 0 || !strings.Contains(out, "请先手动提交") {
		t.Fatalf("CLI accepted staged changes: %d %s", code, out)
	}
	if got := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD")); got != before {
		t.Fatal("run created a commit")
	}
	if got := git(t, wt, "diff", "--cached", "--name-only"); !strings.Contains(got, "run.sh") {
		t.Fatal("staged changes were altered")
	}
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	runs, err := k.ListRuns(exp)
	if err != nil || len(runs) != 0 {
		t.Fatalf("rejected run created records: %v %v", runs, err)
	}
}
func TestRepeatedRunsUseSameCommitWithoutCreatingCommits(t *testing.T) {
	repo := runnable(t, "echo ok\n")
	defer os.RemoveAll(attachRoot(repo))
	wt := worktreeOf(repo)
	head := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD"))
	var ids []string
	for i := 0; i < 2; i++ {
		code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
		if code != 0 {
			t.Fatalf("run: %v", payload)
		}
		id := runField(t, payload)["id"].(string)
		ids = append(ids, id)
		if code, payload = runBinJSON(t, repo, "wait", "--run", id, "--timeout", "20"); code != 0 {
			t.Fatalf("wait: %v", payload)
		}
	}
	if got := strings.TrimSpace(git(t, wt, "rev-parse", "HEAD")); got != head {
		t.Fatal("run moved branch")
	}
	_, payload := runBinJSON(t, repo, "show", ids[1], "--diff")
	if runField(t, payload)["snapshot"] != head || runField(t, payload)["base"] != head {
		t.Fatalf("wrong run commits: %v", payload)
	}
	if diff := payload["data"].(map[string]any)["diff"].(string); diff != "" {
		t.Fatalf("same commit produced diff: %s", diff)
	}
}
