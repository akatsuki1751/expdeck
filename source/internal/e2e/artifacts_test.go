package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func withArtifactGlob(t *testing.T, repo string) {
	t.Helper()
	toml := "[run]\ncmd = \"bash run.sh\"\n\n[artifacts]\nglob = [\"out/*.pth\"]\nkeep = \"best\"\n"
	if err := os.WriteFile(filepath.Join(repo, "exp.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCollectsNewArtifacts(t *testing.T) {
	repo := runnable(t, "mkdir -p out && echo weights > out/model.pth\n")
	defer os.RemoveAll(attachRoot(repo))
	withArtifactGlob(t, repo)

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	runID := runField(t, payload)["id"].(string)
	if code, payload := runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}

	code, payload := runBinJSON(t, repo, "show", runID)
	if code != 0 {
		t.Fatalf("show: %d %v", code, payload)
	}
	artifacts, _ := runField(t, payload)["artifacts"].([]any)
	if len(artifacts) != 1 {
		t.Fatalf("want exactly one artifact, got %v", artifacts)
	}
	artifact := artifacts[0].(map[string]any)
	if artifact["name"] != "out/model.pth" {
		t.Fatalf("unexpected artifact name: %v", artifact["name"])
	}
	store, _ := artifact["store"].(string)
	if _, err := os.Stat(filepath.Join(repo, ".git", "exp", filepath.FromSlash(store))); err != nil {
		t.Fatalf("object not stored: %v", err)
	}

	// 一个没人引用的对象应该被清理，被引用的必须留下。
	orphanDir := filepath.Join(repo, ".git", "exp", "objects", "zz")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(orphanDir, "deadbeef")
	if err := os.WriteFile(orphan, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, payload := runBinJSON(t, repo, "gc"); code != 0 {
		t.Fatalf("gc: %d %v", code, payload)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("gc did not remove the unreferenced object")
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "exp", filepath.FromSlash(store))); err != nil {
		t.Fatalf("gc removed a referenced object: %v", err)
	}
}

func TestSecondFailedRunHasNoArtifacts(t *testing.T) {
	repo := runnable(t, "mkdir -p out && echo weights > out/model.pth\n")
	defer os.RemoveAll(attachRoot(repo))
	withArtifactGlob(t, repo)

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	first := runField(t, payload)["id"].(string)
	runBinJSON(t, repo, "wait", "--run", first, "--timeout", "20")

	// 上一次的产物还在工作树里，但这次训练立刻失败，不该被登记。
	if err := os.WriteFile(filepath.Join(worktreeOf(repo), "run.sh"), []byte("exit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, payload = runBinJSON(t, repo, "run", "sensitive/lr")
	second := runField(t, payload)["id"].(string)
	if code, payload := runBinJSON(t, repo, "wait", "--run", second, "--timeout", "20"); code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}

	_, payload = runBinJSON(t, repo, "show", second)
	if artifacts, ok := runField(t, payload)["artifacts"].([]any); ok && len(artifacts) != 0 {
		t.Fatalf("stale artifacts were attributed to the failed run: %v", artifacts)
	}
}
