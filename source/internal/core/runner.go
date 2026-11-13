package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"expdeck/internal/records"
)

// StartRun 起一次运行并立刻返回；等待交给 __watch 监督进程。
func (k *Kernel) StartRun(exp *Experiment, gpu int) (*Run, error) {
	lock, err := k.Store.AcquireLock()
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()
	exp, err = k.Find(exp.ID)
	if err != nil {
		return nil, err
	}
	worktrees, err := k.Attached()
	if err != nil {
		return nil, err
	}
	worktree, ok := findWorktree(worktrees, exp.Branch)
	if !ok {
		return nil, fmt.Errorf("%w: 实验未挂载，先运行 exp attach %s", ErrInvalid, exp.PathString())
	}
	running, err := k.RunningRun(exp)
	if err != nil {
		return nil, err
	}
	if running != nil {
		return nil, fmt.Errorf("%w: 已有运行中的 run %s", ErrConflict, running.ID)
	}
	config, err := k.LoadConfig()
	if err != nil {
		return nil, err
	}
	if config.RunCmd == "" {
		return nil, fmt.Errorf("%w: exp.toml 里缺少 [run] cmd", ErrInvalid)
	}
	if gpu > 0 {
		if err := k.claimGPU(gpu); err != nil {
			return nil, err
		}
	}
	run := &Run{
		ID:        NewRunID(),
		Exp:       exp.ID,
		Status:    RunRunning,
		Command:   config.RunCmd,
		Cwd:       worktree,
		BootID:    BootID(),
		StartedAt: time.Now().Format(time.RFC3339),
	}
	snapshot, base, err := k.Snapshot(exp, worktree)
	if err != nil {
		return nil, fmt.Errorf("%w: 打快照失败: %v", ErrConflict, err)
	}
	run.Snapshot = snapshot
	run.Base = base
	run.Log = k.LogPath(run.ID)
	if err := k.SaveRun(exp, run); err != nil {
		return nil, err
	}
	if err := k.WriteManifest(exp, run.ID, worktree); err != nil {
		return nil, fmt.Errorf("%w: 记录产物清单失败: %v", ErrConflict, err)
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	watcher := exec.Command(exe, "__watch", k.RunDir(exp, run.ID))
	watcher.Env = os.Environ()
	watcher.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer devNull.Close()
	watcher.Stdin = devNull
	watcher.Stdout = devNull
	watcher.Stderr = devNull
	if err := watcher.Start(); err != nil {
		return nil, fmt.Errorf("启动监督进程失败: %w", err)
	}
	_ = watcher.Process.Release()
	return run, nil
}

// Watch 是监督进程的主体：起训练、等退出、把结局写回 run.json。
func (k *Kernel) Watch(runDir string) error {
	var run Run
	if err := records.ReadJSON(filepath.Join(runDir, "run.json"), &run); err != nil {
		return err
	}
	exp, err := k.Find(run.Exp)
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(run.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command("/bin/sh", "-c", run.Command)
	cmd.Dir = run.Cwd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		run.Status = RunCrashed
		run.FinishedAt = time.Now().Format(time.RFC3339)
		_ = k.SaveRun(exp, &run)
		return err
	}
	run.PID = cmd.Process.Pid
	run.PGID = cmd.Process.Pid
	_ = k.SaveRun(exp, &run)
	_ = k.writeRegistry(&run)

	config, _ := k.LoadConfig()
	timeout := time.Duration(0)
	if config != nil {
		timeout = config.Timeout
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	timedOut := false
	if timeout > 0 {
		select {
		case waitErr = <-done:
		case <-time.After(timeout):
			timedOut = true
			_ = records.WriteFile(filepath.Join(runDir, ".timeout"), []byte("1\n"))
			_ = syscall.Kill(-run.PGID, syscall.SIGTERM)
			select {
			case waitErr = <-done:
			case <-time.After(10 * time.Second):
				_ = syscall.Kill(-run.PGID, syscall.SIGKILL)
				waitErr = <-done
			}
		}
	} else {
		waitErr = <-done
	}
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	switch {
	case timedOut:
		run.Status = RunTimedOut
	case hasMarker(runDir, ".interrupt"):
		run.Status = RunInterrupted
	default:
		run.Status = RunFinished
	}
	run.ExitCode = &exitCode
	run.FinishedAt = time.Now().Format(time.RFC3339)
	_ = k.CollectArtifacts(exp, &run)
	_ = k.SaveRun(exp, &run)
	_ = os.Remove(k.RegistryPath(run.ID))
	return nil
}

// InterruptRun 给运行中的 run 发中断信号，并留一个标记让监督进程记成 interrupted。
func (k *Kernel) InterruptRun(runID string) (*Run, error) {
	exp, run, err := k.FindRun(runID)
	if err != nil {
		return nil, err
	}
	if k.Reconcile(run) {
		_ = k.SaveRun(exp, run)
	}
	if run.Status != RunRunning {
		return nil, fmt.Errorf("%w: run %s 已经结束", ErrConflict, runID)
	}
	if err := records.WriteFile(filepath.Join(k.RunDir(exp, run.ID), ".interrupt"), []byte("1\n")); err != nil {
		return nil, err
	}
	// 监督进程可能还没把训练进程的 pid 写进记录，等它一下，否则信号发不出去。
	if run.PGID <= 0 {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			latest, err := k.LoadRun(exp, run.ID)
			if err != nil {
				break
			}
			if latest.PGID > 0 {
				run = latest
				break
			}
		}
	}
	if run.PGID > 0 {
		_ = syscall.Kill(-run.PGID, syscall.SIGINT)
		time.AfterFunc(10*time.Second, func() { _ = syscall.Kill(-run.PGID, syscall.SIGKILL) })
	}
	return run, nil
}

// WaitRun 阻塞到 run 结束或超时。
func (k *Kernel) WaitRun(runID string, timeout time.Duration) (*Run, error) {
	deadline := time.Now().Add(timeout)
	for {
		exp, run, err := k.FindRun(runID)
		if err != nil {
			return nil, err
		}
		if k.Reconcile(run) {
			_ = k.SaveRun(exp, run)
		}
		if run.Status != RunRunning {
			return run, nil
		}
		if time.Now().After(deadline) {
			return run, fmt.Errorf("%w: 等待 %s 超时", ErrConflict, runID)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func hasMarker(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func (k *Kernel) writeRegistry(run *Run) error {
	return records.WriteJSON(k.RegistryPath(run.ID), map[string]any{
		"run":     run.ID,
		"pid":     run.PID,
		"boot_id": run.BootID,
	})
}

// claimGPU 用占位文件认领一张卡；被活着的 run 占着就拒绝。
func (k *Kernel) claimGPU(n int) error {
	if err := os.MkdirAll(k.Store.Gpus(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(k.Store.Gpus(), fmt.Sprintf("%d.json", n))
	var holder struct {
		PID int    `json:"pid"`
		Run string `json:"run"`
	}
	if err := records.ReadJSON(path, &holder); err == nil && holder.PID > 0 && pidAlive(holder.PID) {
		return fmt.Errorf("%w: GPU %d 已被 run %s 占用", ErrConflict, n, holder.Run)
	}
	return records.WriteJSON(path, map[string]any{"pid": os.Getpid(), "run": "", "gpu": n})
}
