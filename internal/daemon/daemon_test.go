package daemon_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/dustinmays/pr-triage/internal/daemon"
)

func TestDaemon_PIDAndIsRunning(t *testing.T) {
	tmpDir := t.TempDir()

	running, _ := daemon.IsRunning(tmpDir)
	if running {
		t.Fatal("expected IsRunning to be false initially")
	}

	if err := daemon.WritePID(tmpDir); err != nil {
		t.Fatalf("WritePID failed: %v", err)
	}

	running, pid := daemon.IsRunning(tmpDir)
	if !running {
		t.Fatal("expected IsRunning to be true after WritePID")
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}

	daemon.RemovePID(tmpDir)
	running, _ = daemon.IsRunning(tmpDir)
	if running {
		t.Fatal("expected IsRunning to be false after RemovePID")
	}
}

func TestDaemon_StalePIDCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := daemon.PIDPath(tmpDir)

	// Write non-existent PID (e.g. 99999999)
	_ = os.WriteFile(pidFile, []byte("99999999\n"), 0644)

	running, _ := daemon.IsRunning(tmpDir)
	if running {
		t.Fatal("expected IsRunning to be false for dead PID")
	}

	// PID file should have been cleaned up automatically
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("expected stale PID file to be removed, but it still exists")
	}
}

func TestDaemon_AcquireLock_RefusesSecondInstance(t *testing.T) {
	tmpDir := t.TempDir()

	lock1, err := daemon.AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("AcquireLock 1 failed: %v", err)
	}
	defer lock1.Release()

	// Second acquire in another directory/context should see running PID
	// If it's same PID it's idempotent, but with different simulated PID it refuses:
	// Let's write a different live PID (e.g. PID 1 / launchd/init) to test refusal
	pidFile := daemon.PIDPath(tmpDir)
	_ = os.WriteFile(pidFile, []byte("1\n"), 0644) // PID 1 is always alive on POSIX

	_, err = daemon.AcquireLock(tmpDir)
	if err == nil {
		t.Fatal("expected AcquireLock to fail when PID 1 is running")
	}

	var alreadyRunning daemon.ErrAlreadyRunning
	if !errors.As(err, &alreadyRunning) {
		t.Errorf("expected ErrAlreadyRunning, got %v", err)
	}
	if alreadyRunning.PID != 1 {
		t.Errorf("alreadyRunning.PID = %d, want 1", alreadyRunning.PID)
	}
}

func TestDaemon_Heartbeat(t *testing.T) {
	tmpDir := t.TempDir()

	if err := daemon.WriteHeartbeat(tmpDir); err != nil {
		t.Fatalf("WriteHeartbeat failed: %v", err)
	}

	ts, err := daemon.ReadHeartbeat(tmpDir)
	if err != nil {
		t.Fatalf("ReadHeartbeat failed: %v", err)
	}

	if time.Since(ts) > 5*time.Second {
		t.Errorf("heartbeat timestamp %v is older than expected", ts)
	}

	daemon.RemoveHeartbeat(tmpDir)
	if _, err := daemon.ReadHeartbeat(tmpDir); err == nil {
		t.Fatal("expected error reading removed heartbeat")
	}
}
