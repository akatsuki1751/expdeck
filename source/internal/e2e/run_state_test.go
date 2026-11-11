package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func runFile(t *testing.T, repo, runID string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repo, ".git", "exp", "records", "exps", "*", "runs", runID, "run.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("cannot locate run.json for %s: %v %v", runID, matches, err)
	}
	return matches[0]
}

type runRecord struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	PID    int    `json:"pid"`
	PGID   int    `json:"pgid"`
}

func readRun(t *testing.T, path string) runRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record runRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func waitForPID(t *testing.T, path string) runRecord {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		record := readRun(t, path)
		if record.PID > 0 {
			return record
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("training pid never appeared in run.json")
	return runRecord{}
}

func watcherPID(t *testing.T, runID string) int {
	t.Helper()
	out, err := exec.Command("pgrep", "-f", "__watch.*"+runID).Output()
	if err != nil {
		t.Fatalf("cannot find watcher for %s: %v", runID, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		t.Fatalf("no watcher process for %s", runID)
	}
	var pid int
	pid, err = strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || pid <= 0 {
		t.Fatalf("bad pid %q", fields[0])
	}
	return pid
}

// TestWatcherKilledStillBlocksSecondRun 是 SPEC 的第二条总验收：
// 监督进程被杀、训练仍在跑时，仍要拒绝重复开跑；训练也死了之后，状态要收敛成崩溃。
func TestWatcherKilledStillBlocksSecondRun(t *testing.T) {
	repo := runnable(t, "sleep 30\n")
	defer os.RemoveAll(attachRoot(repo))

	_, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	runID := runField(t, payload)["id"].(string)

	path := runFile(t, repo, runID)
	record := waitForPID(t, path)

	watcher := watcherPID(t, runID)
	if err := syscall.Kill(watcher, syscall.SIGKILL); err != nil {
		t.Fatalf("kill watcher: %v", err)
	}

	code, payload := runBinJSON(t, repo, "run", "sensitive/lr")
	if code != 5 {
		t.Fatalf("training still alive, second run must be refused (5), got %d %v", code, payload)
	}

	if err := syscall.Kill(record.PID, syscall.SIGKILL); err != nil {
		t.Fatalf("kill training: %v", err)
	}

	code, payload = runBinJSON(t, repo, "status")
	if code != 0 {
		t.Fatalf("status: %d %v", code, payload)
	}
	if strings.Contains(stringOf(payload["data"]), runID) {
		t.Fatalf("dead run still reported as running: %v", payload["data"])
	}

	code, payload = runBinJSON(t, repo, "wait", "--run", runID, "--timeout", "10")
	if code != 0 {
		t.Fatalf("wait: %d %v", code, payload)
	}
	if got := runField(t, payload)["status"]; got != "crashed" {
		t.Fatalf("want crashed after everything died, got %v", got)
	}
}

func stringOf(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
