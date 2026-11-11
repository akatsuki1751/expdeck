package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

func TestWebShowsRunButtonWhenIdle(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	code, body := get(t, h, "/e/sensitive/lr")
	if code != http.StatusOK {
		t.Fatalf("detail status %d", code)
	}
	if !strings.Contains(body, "/e/sensitive/lr/run") {
		t.Fatalf("detail page has no run action:\n%s", body)
	}
}

func TestWebRunFailsGracefullyWithoutWorktree(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	rec := post(t, h, "/e/sensitive/lr/run")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "error=") {
		t.Fatalf("failure should redirect back with an error, got %q", location)
	}

	code, body := get(t, h, location)
	if code != http.StatusOK {
		t.Fatalf("error page status %d", code)
	}
	if !strings.Contains(body, "挂载") && !strings.Contains(body, "error") {
		t.Fatalf("error message not shown:\n%s", body)
	}
}

func TestWebRunUnknownExperimentIs404(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	if rec := post(t, h, "/e/nope/run"); rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestWebInterruptWithoutRunShowsError(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	rec := post(t, h, "/e/sensitive/lr/interrupt")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "error=") {
		t.Fatalf("interrupting nothing should report an error, got %q", location)
	}
}

func TestWebLogEndpointIsSafe(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	code, body := get(t, h, "/log/does-not-exist")
	if code != http.StatusOK {
		t.Fatalf("log endpoint should answer 200 for unknown runs, got %d", code)
	}
	if body != "" {
		t.Fatalf("unknown run should produce empty log, got %q", body)
	}
}
