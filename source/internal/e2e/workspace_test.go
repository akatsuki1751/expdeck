package e2e

import (
	"encoding/json"
	"expdeck/internal/core"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebRenamePreservesAttachedWorktreeAndRecords(t *testing.T) {
	repo := scaffold(t)
	defer os.RemoveAll(attachRoot(repo))
	h := newHandler(t, repo)
	post(t, h, "/e/sensitive/lr/attach")
	_, before := runJSON(t, repo, "show", "sensitive/lr")
	old := dataOf(t, before)["experiment"].(map[string]any)
	rec := postForm(t, h, "/e/sensitive/lr/rename", url.Values{"path": {"ablation/new-name"}})
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/e/ablation/new-name") {
		t.Fatalf("rename: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	code, after := runJSON(t, repo, "show", "ablation/new-name")
	if code != 0 {
		t.Fatalf("missing renamed experiment: %v", after)
	}
	current := dataOf(t, after)["experiment"].(map[string]any)
	if current["id"] != old["id"] {
		t.Fatal("renaming changed experiment ID")
	}
	wt := filepath.Join(attachRoot(repo), "sensitive", "lr")
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree lost: %v", err)
	}
	if got := git(t, wt, "branch", "--show-current"); !strings.Contains(got, "exp/ablation/new-name") {
		t.Fatalf("branch not renamed: %s", got)
	}
}
func TestWebRenameConflictLeavesOriginal(t *testing.T) {
	repo := scaffold(t)
	runJSON(t, repo, "new", "other/test", "--why", "other")
	h := newHandler(t, repo)
	rec := postForm(t, h, "/e/sensitive/lr/rename", url.Values{"path": {"other/test"}})
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Fatalf("expected conflict: %s", rec.Header().Get("Location"))
	}
	if code, _ := runJSON(t, repo, "show", "sensitive/lr"); code != 0 {
		t.Fatal("original experiment disappeared")
	}
}
func TestWebAuditLogsResultsWithoutFormValues(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)
	postForm(t, h, "/new", url.Values{"path": {"log/test"}, "why": {"private hypothesis"}})
	postForm(t, h, "/new", url.Values{"path": {"invalid/path"}})
	data, err := os.ReadFile(filepath.Join(repo, ".git", "exp", "logs", "web.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"result":"ok"`) || !strings.Contains(text, `"result":"error"`) {
		t.Fatalf("missing results: %s", text)
	}
	if strings.Contains(text, "private hypothesis") {
		t.Fatal("form value leaked into log")
	}
}

func TestRenameLeavesLegacyBookUntouched(t *testing.T) {
	repo := scaffold(t)
	k, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := k.Find("sensitive/lr")
	if err != nil {
		t.Fatal(err)
	}
	original := exp.Hypothesis
	if _, _, err = k.Backup(); err != nil {
		t.Fatal(err)
	}
	if err = k.SetHypothesis(exp, "not yet backed up"); err != nil {
		t.Fatal(err)
	}
	h := newHandler(t, repo)
	rec := postForm(t, h, "/e/sensitive/lr/rename", url.Values{"path": {"baseline/new-name"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/e/baseline/new-name" {
		t.Fatalf("rename: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	current, err := k.Find(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Hypothesis != "not yet backed up" || current.Branch != "exp/baseline/new-name" {
		t.Fatalf("current record: %+v", current)
	}
	if k.Repo.BranchExists("exp/sensitive/lr") || !k.Repo.BranchExists("exp/baseline/new-name") {
		t.Fatal("branch not renamed")
	}
	var saved core.Experiment
	if err = json.Unmarshal([]byte(git(t, repo, "show", fmt.Sprintf("book:exps/%s/meta.json", exp.ID))), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.PathString() != "sensitive/lr" || saved.Branch != "exp/sensitive/lr" || saved.Hypothesis != original {
		t.Fatalf("backup changed unexpectedly: %+v", saved)
	}
}
func TestHypothesisCLIAndNavigation(t *testing.T) {
	repo := scaffold(t)
	if code, out := run(t, repo, "hypothesis", "sensitive/lr", "--text", "updated hypothesis"); code != 0 {
		t.Fatalf("hypothesis: %s", out)
	}
	h := newHandler(t, repo)
	code, body := get(t, h, "/e/sensitive/lr")
	for _, want := range []string{"updated hypothesis", `data-inline-rename`, `/e/sensitive/lr/rename`, `<span>sensitive</span><span class="crumb-sep">/</span><span>lr</span>`} {
		if code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("missing %q: %d", want, code)
		}
	}
}
