package e2e

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"expdeck/internal/core"
	"expdeck/internal/web"
)

func newHandler(t *testing.T, repo string) http.Handler {
	t.Helper()
	kernel, err := core.Open(repo)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return web.New(kernel).Handler()
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestWebIndexShowsTree(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	code, body := get(t, h, "/")
	if code != http.StatusOK {
		t.Fatalf("index status %d: %s", code, body)
	}
	for _, want := range []string{"sensitive", "lr", "lr 降到 1e-4"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestWebDetailShowsNoteAndDiff(t *testing.T) {
	repo := scaffold(t)
	runJSON(t, repo, "note", "sensitive/lr", "--text", "笔记正文在这里")
	h := newHandler(t, repo)

	code, body := get(t, h, "/e/sensitive/lr")
	if code != http.StatusOK {
		t.Fatalf("detail status %d: %s", code, body)
	}
	for _, want := range []string{"lr 降到 1e-4", "笔记正文在这里", "exp/sensitive/lr"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q", want)
		}
	}
}

func TestWebEditFormRendersVersion(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	code, body := get(t, h, "/e/sensitive/lr/edit")
	if code != http.StatusOK {
		t.Fatalf("edit status %d: %s", code, body)
	}
	for _, want := range []string{`name="version"`, `name="content"`, "<textarea"} {
		if !strings.Contains(body, want) {
			t.Errorf("edit form missing %q", want)
		}
	}
}

func TestWebSaveUpdatesNote(t *testing.T) {
	repo := scaffold(t)
	runJSON(t, repo, "note", "sensitive/lr", "--text", "旧内容")
	_, printed := runJSON(t, repo, "note", "sensitive/lr", "--print")
	version := dataOf(t, printed)["version"].(string)

	h := newHandler(t, repo)
	form := url.Values{"version": {version}, "content": {"新内容"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/e/sensitive/lr/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status %d: %s", rec.Code, rec.Body.String())
	}
	_, payload := runJSON(t, repo, "note", "sensitive/lr", "--print")
	if got := dataOf(t, payload)["content"].(string); got != "新内容" {
		t.Fatalf("note not updated: %q", got)
	}
}

func TestWebSaveWithStaleVersionConflicts(t *testing.T) {
	repo := scaffold(t)
	runJSON(t, repo, "note", "sensitive/lr", "--text", "旧内容")
	_, printed := runJSON(t, repo, "note", "sensitive/lr", "--print")
	stale := dataOf(t, printed)["version"].(string)
	runJSON(t, repo, "note", "sensitive/lr", "--text", "别人改过了", "--expect", stale)

	h := newHandler(t, repo)
	form := url.Values{"version": {stale}, "content": {"我的覆盖"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/e/sensitive/lr/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "重新加载") {
		t.Fatalf("conflict page should explain what happened:\n%s", rec.Body.String())
	}
	_, payload := runJSON(t, repo, "note", "sensitive/lr", "--print")
	if got := dataOf(t, payload)["content"].(string); got != "别人改过了" {
		t.Fatalf("conflicting save overwrote the note: %q", got)
	}
}

func TestWebUnknownExperimentIs404(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	if code, _ := get(t, h, "/e/nope"); code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", code)
	}
}
