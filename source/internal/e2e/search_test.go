package e2e

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestWebSearchNamesAndSavedNotes(t *testing.T) {
	repo := scaffold(t)
	runJSON(t, repo, "note", "sensitive/lr", "--text", "独特的笔记关键词 NeedleCase")
	h := newHandler(t, repo)
	for _, query := range []string{"SENSITIVE", "needlecase", "笔记关键词"} {
		code, body := get(t, h, "/search?q="+url.QueryEscape(query))
		if code != http.StatusOK || !strings.Contains(body, "<tbody>") || !strings.Contains(strings.Split(body, "<tbody>")[1], "sensitive/lr") {
			t.Fatalf("query %q: status %d, body %s", query, code, body)
		}
	}
	code, body := get(t, h, "/search?q=no-such-keyword")
	if code != http.StatusOK || !strings.Contains(body, "没有找到匹配的实验") {
		t.Fatalf("empty results: %d", code)
	}
}
