package e2e

import (
	"strings"
	"testing"
)

func TestWorkspaceTreeHasFolderDisclosureAndProjectMenu(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)
	_, body := get(t, h, "/")
	for _, want := range []string{`class="tree-branch"`, `data-tree-path="sensitive"`, `aria-label="项目操作"`, `data-open-dialog="create-dialog"`, `id="create-dialog"`, `data-toggle-right`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}
func TestExperimentInspectorDoesNotContainOperationForms(t *testing.T) {
	repo := scaffold(t)
	h := newHandler(t, repo)
	_, body := get(t, h, "/e/sensitive/lr")
	i := strings.Index(body, `<aside class="tools inspector"`)
	if i < 0 {
		t.Fatal("missing inspector")
	}
	j := strings.Index(body[i:], "</aside>")
	if j < 0 {
		t.Fatal("missing inspector end")
	}
	panel := body[i : i+j]
	if strings.Contains(panel, "<form") || strings.Contains(panel, "data-toggle-right") {
		t.Fatal("inspector contains operations or toggle")
	}
	if strings.Contains(body, `data-conclude-url=`) {
		t.Fatal("legacy conclude action should not be in menu")
	}
	if !strings.Contains(body, "删除实验") {
		t.Fatal("delete action missing")
	}
}
