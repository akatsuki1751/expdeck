package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflow(t *testing.T) {
	oldRoot := root
	defer func() { root = oldRoot }()
	dir := t.TempDir()
	root = dir
	command := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, e := c.CombinedOutput(); e != nil {
			t.Fatalf("git %v: %s (%v)", args, out, e)
		}
	}
	command("init")
	command("config", "user.name", "Test")
	command("config", "user.email", "test@example.com")
	if e := os.WriteFile(filepath.Join(dir, "source"), []byte("initial"), 0644); e != nil {
		t.Fatal(e)
	}
	command("add", "source")
	command("commit", "-m", "initial")
	if e := initProject(false); e != nil {
		t.Fatal(e)
	}
	command("add", "xpd.toml")
	command("commit", "-m", "config")
	if e := create("exp/group/test", "hypothesis"); e != nil {
		t.Fatal(e)
	}
	r := readRecord("exp/group/test")
	r.Notes = "results\nnext steps"
	if e := save(r); e != nil {
		t.Fatal(e)
	}
	if got := readRecord(r.Branch); got.Notes != r.Notes || got.Hypothesis != r.Hypothesis {
		t.Fatalf("bad record: %+v", got)
	}
	wt := filepath.Join(t.TempDir(), "worktree")
	if e := attach(r.Branch, wt); e != nil {
		t.Fatal(e)
	}
	xs, e := list()
	if e != nil || len(xs) != 1 || xs[0].Worktree != wt {
		t.Fatalf("list: %+v, %v", xs, e)
	}
	if e = os.WriteFile(filepath.Join(wt, "source"), []byte("changed"), 0644); e != nil {
		t.Fatal(e)
	}
	if e = detach(r.Branch); e == nil {
		t.Fatal("dirty worktree was removed")
	}
	if e = os.WriteFile(filepath.Join(wt, "source"), []byte("initial"), 0644); e != nil {
		t.Fatal(e)
	}
	if e = detach(r.Branch); e != nil {
		t.Fatal(e)
	}
	if !exists(r.Branch) {
		t.Fatal("branch lost")
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	_ = request
	var output strings.Builder
	if e = page.Execute(&output, struct {
		Token, Message string
		Experiments    []experiment
	}{"token", "", xs}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(output.String(), "hypothesis") || !strings.Contains(output.String(), "results") {
		t.Fatal("web missing experiment content")
	}
}

func TestCreateRequiresInitialCommit(t *testing.T) {
	oldRoot := root
	defer func() { root = oldRoot }()
	root = t.TempDir()
	c := exec.Command("git", "init")
	c.Dir = root
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	err := create("exp/ablation/sigreg", "")
	if err == nil || !strings.Contains(err.Error(), "首次提交") {
		t.Fatalf("expected initial commit guidance, got %v", err)
	}
	if exists("exp/ablation/sigreg") {
		t.Fatal("branch created without a commit")
	}
}
