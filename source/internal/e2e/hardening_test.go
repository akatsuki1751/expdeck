package e2e

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"expdeck/internal/core"
	"expdeck/internal/web"
)

func TestWebRequiresToken(t *testing.T) {
	repo := scaffold(t)
	kernel, err := core.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	server := web.New(kernel)
	server.Token = "s3cret"
	h := server.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token want 401, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?token=s3cret", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("token should exchange for a cookie via 303, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); strings.Contains(location, "token=") {
		t.Fatalf("token must not survive in the URL: %q", location)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie issued")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with a cookie want 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?token=wrong", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token want 401, got %d", rec.Code)
	}
}

func TestWebRejectsCrossOriginPost(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)

	req := httptest.NewRequest(http.MethodPost, "/e/sensitive/lr/run", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST should be forbidden, got %d", rec.Code)
	}

	same := httptest.NewRequest(http.MethodPost, "/e/sensitive/lr/run", nil)
	same.Header.Set("Origin", "http://"+same.Host)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, same)
	if rec.Code == http.StatusForbidden {
		t.Fatal("same-origin POST was wrongly rejected")
	}
}

func TestRunTimeoutKillsTraining(t *testing.T) {
	repo := runnable(t, "sleep 30\n")
	defer os.RemoveAll(attachRoot(repo))

	toml := "[run]\ncmd = \"bash run.sh\"\ntimeout = \"1s\"\n"
	if err := os.WriteFile(filepath.Join(repo, "exp.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	runID := runField(t, payload)["id"].(string)

	start := time.Now()
	code, payload := runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "25")
	if code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("timeout did not stop the run in time: %s", elapsed)
	}
	if got := runField(t, payload)["status"]; got != "timedout" {
		t.Fatalf("want timedout, got %v", got)
	}
}
