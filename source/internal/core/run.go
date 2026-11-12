package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"expdeck/internal/records"
)

// run.go 定义一次运行及其状态判定。
const (
	RunRunning     = "running"
	RunFinished    = "finished"
	RunInterrupted = "interrupted"
	RunCrashed     = "crashed"
	RunTimedOut    = "timedout"
)

// Run 是一次执行。
type Run struct {
	ID         string   `json:"id"`
	Exp        string   `json:"exp"`
	Status     string   `json:"status"`
	Command    string   `json:"command"`
	Cwd        string   `json:"cwd"`
	PID        int      `json:"pid"`
	PGID       int      `json:"pgid"`
	BootID     string   `json:"boot_id"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at,omitempty"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	Log        string   `json:"log"`
	Snapshot   string   `json:"snapshot,omitempty"`
	Base       string   `json:"base,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Artifacts  []Artifact         `json:"artifacts,omitempty"`
}

// RunsDir 是一个实验的 run 目录。
func (k *Kernel) RunsDir(exp *Experiment) string {
	return filepath.Join(k.Store.ExpDir(exp.ID), "runs")
}

// RunDir 是某个 run 的目录。
func (k *Kernel) RunDir(exp *Experiment, runID string) string {
	return filepath.Join(k.RunsDir(exp), runID)
}

// RunPath 是 run.json 的路径。
func (k *Kernel) RunPath(exp *Experiment, runID string) string {
	return filepath.Join(k.RunDir(exp, runID), "run.json")
}

// LogPath 是训练日志的路径（放在 records 之外，避免被备份带走）。
func (k *Kernel) LogPath(runID string) string {
	return filepath.Join(k.Store.Logs(), runID+".log")
}

// RegistryPath 是存活登记的路径。
func (k *Kernel) RegistryPath(runID string) string {
	return filepath.Join(k.Store.Registry(), runID+".json")
}

// NewRunID 生成 `20261111-211109-a1b2` 形式的标识：字典序即时间序。
func NewRunID() string {
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return time.Now().Format("20060102-150405") + "-0000"
	}
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(suffix)
}

// SaveRun 原子写 run.json。
func (k *Kernel) SaveRun(exp *Experiment, run *Run) error {
	return records.WriteJSON(k.RunPath(exp, run.ID), run)
}

// LoadRun 读一个 run。
func (k *Kernel) LoadRun(exp *Experiment, runID string) (*Run, error) {
	var run Run
	if err := records.ReadJSON(k.RunPath(exp, runID), &run); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: run %s", ErrNotFound, runID)
		}
		return nil, err
	}
	return &run, nil
}

// ListRuns 读出一个实验的全部 run，按标识排序。
func (k *Kernel) ListRuns(exp *Experiment) ([]*Run, error) {
	entries, err := os.ReadDir(k.RunsDir(exp))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var runs []*Run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		run, err := k.LoadRun(exp, entry.Name())
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}
	sortRuns(runs)
	return runs, nil
}

func sortRuns(runs []*Run) {
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].ID < runs[j-1].ID; j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
}

// FindRun 在所有实验里找一个 run。
func (k *Kernel) FindRun(runID string) (*Experiment, *Run, error) {
	exps, err := k.List()
	if err != nil {
		return nil, nil, err
	}
	for _, exp := range exps {
		run, err := k.LoadRun(exp, runID)
		if err == nil {
			return exp, run, nil
		}
	}
	return nil, nil, fmt.Errorf("%w: run %s", ErrNotFound, runID)
}

// BootID 返回本次开机标识。
func BootID() string {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// procStartTime 读 /proc/<pid>/stat 的第 22 个字段（进程启动时刻）。
func procStartTime(pid int) (uint64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	text := string(data)
	close := strings.LastIndex(text, ")")
	if close < 0 {
		return 0, false
	}
	fields := strings.Fields(text[close+1:])
	// close 之后第一个字段是第 3 个字段（state），启动时刻是第 22 个。
	if len(fields) < 20 {
		return 0, false
	}
	value, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// pidAlive 判断 pid 是否仍然存在。
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, ok := procStartTime(pid)
	return ok
}

// Reconcile 把「写在文件里的状态」对齐到现实。
// 返回 true 表示状态被改写，调用方需要落盘。
func (k *Kernel) Reconcile(run *Run) bool {
	if run.Status != RunRunning {
		return false
	}
	if run.BootID != "" && run.BootID != BootID() {
		run.Status = RunCrashed
		run.FinishedAt = time.Now().Format(time.RFC3339)
		return true
	}
	if run.PID <= 0 {
		started, err := time.Parse(time.RFC3339, run.StartedAt)
		if err == nil && time.Since(started) > 2*time.Minute {
			run.Status = RunCrashed
			run.FinishedAt = time.Now().Format(time.RFC3339)
			return true
		}
		return false
	}
	if !pidAlive(run.PID) {
		run.Status = RunCrashed
		run.FinishedAt = time.Now().Format(time.RFC3339)
		return true
	}
	return false
}

// RunningRun 返回该实验当前正在跑的 run（已对齐现实），没有则 nil。
func (k *Kernel) RunningRun(exp *Experiment) (*Run, error) {
	runs, err := k.ListRuns(exp)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if k.Reconcile(run) {
			_ = k.SaveRun(exp, run)
		}
		if run.Status == RunRunning {
			return run, nil
		}
	}
	return nil, nil
}
