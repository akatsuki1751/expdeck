package web

import "testing"

func TestParseDiffNumbersAndClassifiesLines(t *testing.T) {
	got := parseDiff("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -2,2 +2,2 @@\n unchanged\n-old\n+new\n")
	if len(got) != 5 {
		t.Fatalf("lines: %+v", got)
	}
	if got[1+1].Old != "2" || got[1+1].New != "2" || got[3].Kind != "remove" || got[3].Old != "3" || got[4].Kind != "add" || got[4].New != "3" {
		t.Fatalf("bad diff: %+v", got)
	}
}
