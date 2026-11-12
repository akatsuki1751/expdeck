package e2e

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWebCreatesExperiment(t *testing.T) {
	repo := newRepo(t)
	runJSON(t, repo, "init")
	h := newHandler(t, repo)

	rec := postForm(t, h, "/new", url.Values{"path": {"web/created"}, "why": {"通过网页创建"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d: %s", rec.Code, rec.Body.String())
	}
	if code, payload := runJSON(t, repo, "show", "web/created"); code != 0 {
		t.Fatalf("experiment not registered: %d %v", code, payload)
	}
}

func TestWebCreateWithoutHypothesisReportsError(t *testing.T) {
	repo := newRepo(t)
	runJSON(t, repo, "init")
	h := newHandler(t, repo)

	rec := postForm(t, h, "/new", url.Values{"path": {"no/why"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "error=") {
		t.Fatalf("missing error redirect: %q", location)
	}
	if code, _ := run(t, repo, "show", "no/why"); code == 0 {
		t.Fatal("experiment was created without a hypothesis")
	}
}

func TestWebAttachAndDetach(t *testing.T) {
	repo := scaffold(t)
	defer os.RemoveAll(attachRoot(repo))
	h := newHandler(t, repo)

	if rec := post(t, h, "/e/sensitive/lr/attach"); rec.Code != http.StatusSeeOther {
		t.Fatalf("attach: want 303, got %d", rec.Code)
	}
	if _, body := get(t, h, "/e/sensitive/lr"); !strings.Contains(body, "/e/sensitive/lr/detach") {
		t.Fatalf("detail page should offer detach after attach")
	}
	if rec := post(t, h, "/e/sensitive/lr/detach"); rec.Code != http.StatusSeeOther {
		t.Fatalf("detach: want 303, got %d", rec.Code)
	}
	if _, body := get(t, h, "/e/sensitive/lr"); !strings.Contains(body, "/e/sensitive/lr/attach") {
		t.Fatalf("detail page should offer attach after detach")
	}
}

func TestWebConcludeAndReopen(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	rec := postForm(t, h, "/e/sensitive/lr/conclude", url.Values{"decision": {"abandon"}, "why": {"网页结案"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("conclude: want 303, got %d", rec.Code)
	}
	_, payload := runJSON(t, repo, "show", "sensitive/lr")
	exp := dataOf(t, payload)["experiment"].(map[string]any)
	if exp["status"] != "abandoned" {
		t.Fatalf("want abandoned, got %v", exp["status"])
	}

	if rec := post(t, h, "/e/sensitive/lr/reopen"); rec.Code != http.StatusSeeOther {
		t.Fatalf("reopen: want 303, got %d", rec.Code)
	}
	_, payload = runJSON(t, repo, "show", "sensitive/lr")
	exp = dataOf(t, payload)["experiment"].(map[string]any)
	if exp["status"] != "active" {
		t.Fatalf("want active, got %v", exp["status"])
	}
}

func TestWebAdoptButton(t *testing.T) {
	repo := scaffold(t)
	git(t, repo, "branch", "exp/ablation/sigreg")
	h := newHandler(t, repo)

	if _, body := get(t, h, "/"); !strings.Contains(body, "exp/ablation/sigreg") {
		t.Fatalf("index should list the unregistered branch")
	}
	rec := postForm(t, h, "/adopt", url.Values{"branch": {"exp/ablation/sigreg"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("adopt: want 303, got %d", rec.Code)
	}
	if code, payload := runJSON(t, repo, "show", "ablation/sigreg"); code != 0 {
		t.Fatalf("adopted experiment missing: %d %v", code, payload)
	}
}

func TestWebBackupButton(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	if rec := post(t, h, "/backup"); rec.Code != http.StatusSeeOther {
		t.Fatalf("backup: want 303, got %d", rec.Code)
	}
	if out := git(t, repo, "branch", "--list", "book"); !strings.Contains(out, "book") {
		t.Fatalf("book branch missing after web backup: %s", out)
	}
}
