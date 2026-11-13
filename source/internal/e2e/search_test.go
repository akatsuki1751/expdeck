package e2e

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestWebSearchNamesAndHypothesesNotNotes(t *testing.T) {
	repo := scaffold(t)
	runJSON(t, repo, "note", "sensitive/lr", "--text", "UniqueNoteNeedle")
	h := newHandler(t, repo)
	for _, query := range []string{"SENSITIVE", "1e-4", "降到"} {
		code, body := get(t, h, "/search?q="+url.QueryEscape(query))
		if code != http.StatusOK || !strings.Contains(body, "sensitive/lr") {
			t.Fatalf("query %q: status %d, body %s", query, code, body)
		}
		if strings.Contains(body, "<html") {
			t.Fatalf("search should return a sidebar fragment: %s", body)
		}
	}
	for _, query := range []string{"UniqueNoteNeedle", "no-such-keyword"} {
		code, body := get(t, h, "/search?q="+url.QueryEscape(query))
		if code != http.StatusOK || !strings.Contains(body, "没有匹配的实验") {
			t.Fatalf("query %q: status %d, body %s", query, code, body)
		}
	}
}
