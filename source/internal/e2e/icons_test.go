package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebEmbedsIconsAndRevalidatesStaticAssets(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)
	_, body := get(t, h, "/")
	for _, want := range []string{`/static/icons/lucide.svg#folder`, `/static/brand/favicon.svg`, `aria-label="设置"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, path := range []string{"/static/icons/lucide.svg", "/static/icons/icons.css", "/static/brand/favicon.svg"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: status=%d cache=%q", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}
