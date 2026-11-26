package core

import (
	"os"
	"testing"

	"expdeck/internal/records"
)

func TestReconcileWaitsForLiveWatcherAfterTrainingExits(t *testing.T) {
	k := &Kernel{}
	run := &Run{Status: RunRunning, PID: 99999999, WatcherPID: os.Getpid(), BootID: BootID()}
	if k.Reconcile(run) || run.Status != RunRunning {
		t.Fatalf("live watcher should finish the run, got %s", run.Status)
	}
	run.WatcherPID = 99999999
	if !k.Reconcile(run) || run.Status != RunCrashed {
		t.Fatalf("dead watcher and training should be crashed, got %s", run.Status)
	}
}

func TestReconcilePreservesCompletedRecord(t *testing.T) {
	k := &Kernel{Store: records.New(t.TempDir())}
	exp := &Experiment{ID: "experiment"}
	final := &Run{ID: "run", Exp: exp.ID, Status: RunTimedOut}
	if err := k.SaveRun(exp, final); err != nil {
		t.Fatal(err)
	}
	stale := &Run{ID: final.ID, Exp: exp.ID, Status: RunRunning, PID: 99999999, WatcherPID: 99999999, BootID: BootID()}
	if k.Reconcile(stale) || stale.Status != RunTimedOut {
		t.Fatalf("completed record was not preserved: %+v", stale)
	}
}
