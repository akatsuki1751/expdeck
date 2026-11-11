package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

// binary 构建一次真实的可执行文件：run 需要真的拉起子进程，不能用进程内调用。
func binary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "expdeck-bin-")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "exp")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/exp")
		cmd.Dir = "../.."
		out, err := cmd.CombinedOutput()
		if err != nil {
			binErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
}

// runBin 用真实二进制跑一条命令。
func runBin(t *testing.T, repo string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(binary(t), args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), string(out)
		}
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
	return 0, string(out)
}

func runBinJSON(t *testing.T, repo string, args ...string) (int, map[string]any) {
	t.Helper()
	code, out := runBin(t, repo, args...)
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("not JSON (exit %d): %s", code, out)
	}
	return code, payload
}

func runField(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil
	}
	run, _ := data["run"].(map[string]any)
	return run
}

// runnable 准备一个「已挂载 + 有 run 脚本」的仓库。
func runnable(t *testing.T, script string) string {
	t.Helper()
	repo := newRepo(t)
	if code, out := runBin(t, repo, "init"); code != 0 {
		t.Fatalf("init: %d %s", code, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "[run]\ncmd = \"bash run.sh\"\n"
	if err := os.WriteFile(filepath.Join(repo, "exp.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.email=a@b.c", "-c", "user.name=test", "commit", "-q", "-m", "runnable")

	if code, out := runBin(t, repo, "new", "sensitive/lr", "--why", "跑一次"); code != 0 {
		t.Fatalf("new: %d %s", code, out)
	}
	if code, out := runBin(t, repo, "attach", "sensitive/lr"); code != 0 {
		t.Fatalf("attach: %d %s", code, out)
	}
	return repo
}

// stopRun 中断并等到确实结束，避免临时目录清理时还有进程在写文件。
func stopRun(t *testing.T, repo, runID string) {
	t.Helper()
	runBin(t, repo, "interrupt", "--run", runID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		_, payload := runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "1")
		if run := runField(t, payload); run != nil && run["status"] != "running" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s did not stop", runID)
}

func TestRunFinishesAndWritesLog(t *testing.T) {
	repo := runnable(t, "echo hello-from-exp\n")
	defer os.RemoveAll(attachRoot(repo))

	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 0 {
		t.Fatalf("run: %d %v", code, payload)
	}
	runID := runField(t, payload)["id"].(string)

	code, payload = runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "30")
	if code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}
	done := runField(t, payload)
	if done["status"] != "finished" {
		t.Fatalf("want finished, got %v", done["status"])
	}
	body, err := os.ReadFile(done["log"].(string))
	if err != nil {
		t.Fatalf("log missing: %v", err)
	}
	if !strings.Contains(string(body), "hello-from-exp") {
		t.Fatalf("log does not contain output: %s", body)
	}
}

func TestRunRefusesSecondRunOnSameExperiment(t *testing.T) {
	repo := runnable(t, "sleep 5\n")
	defer os.RemoveAll(attachRoot(repo))

	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 0 {
		t.Fatalf("first run: %d %v", code, payload)
	}
	runID := runField(t, payload)["id"].(string)

	code, payload = runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 5 {
		t.Fatalf("second run should conflict (5), got %d %v", code, payload)
	}
	stopRun(t, repo, runID)
}

func TestRunRequiresAttachedWorktree(t *testing.T) {
	repo := newRepo(t)
	runBin(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "run.sh"), []byte("true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "exp.toml"), []byte("[run]\ncmd = \"bash run.sh\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runBin(t, repo, "new", "sensitive/lr", "--why", "x")

	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 4 {
		t.Fatalf("want exit 4 without worktree, got %d %v", code, payload)
	}
}

func TestStatusListsRunning(t *testing.T) {
	repo := runnable(t, "sleep 5\n")
	defer os.RemoveAll(attachRoot(repo))

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	runID := runField(t, payload)["id"].(string)

	code, payload := runBinJSON(t, repo, "status")
	if code != 0 {
		t.Fatalf("status: %d %v", code, payload)
	}
	if !strings.Contains(fmt.Sprint(payload["data"]), runID) {
		t.Fatalf("status does not mention %s: %v", runID, payload["data"])
	}
	stopRun(t, repo, runID)
}

func TestInterruptMarksRunInterrupted(t *testing.T) {
	repo := runnable(t, "sleep 30\n")
	defer os.RemoveAll(attachRoot(repo))

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	runID := runField(t, payload)["id"].(string)

	if code, payload := runBinJSON(t, repo, "interrupt", "--run", runID); code != 0 {
		t.Fatalf("interrupt: %d %v", code, payload)
	}

	code, payload := runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "30")
	if code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}
	done := runField(t, payload)
	if done["status"] != "interrupted" {
		t.Fatalf("want interrupted, got %v", done["status"])
	}
}
