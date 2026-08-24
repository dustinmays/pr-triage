// Package daemon provides daemon process lifecycle helpers: PID files,
// single-instance mutual exclusion, heartbeat tracking, and graceful shutdown.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dustinmays/pr-triage/internal/db"
)

// ErrAlreadyRunning is returned when another daemon instance is actively running.
type ErrAlreadyRunning struct {
	PID int
}

func (e ErrAlreadyRunning) Error() string {
	return fmt.Sprintf("pr-triage daemon is already running (PID %d)", e.PID)
}

// DefaultStateDir returns the default daemon state directory (~/.pr-triage).
func DefaultStateDir() string {
	return db.DefaultDBDir()
}

// PIDPath returns the PID file path for the daemon.
func PIDPath(dir string) string {
	if dir == "" {
		dir = DefaultStateDir()
	}
	return filepath.Join(dir, "pr-triage.pid")
}

// HeartbeatPath returns the heartbeat file path for the daemon.
func HeartbeatPath(dir string) string {
	if dir == "" {
		dir = DefaultStateDir()
	}
	return filepath.Join(dir, "pr-triage.heartbeat")
}

// WritePID writes the current process PID to the PID file in dir.
func WritePID(dir string) error {
	if dir == "" {
		dir = DefaultStateDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create daemon state dir: %w", err)
	}
	return os.WriteFile(PIDPath(dir), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

// RemovePID removes the PID file.
func RemovePID(dir string) {
	_ = os.Remove(PIDPath(dir))
}

// IsRunning checks if a daemon process is actively alive for dir.
// If a PID file exists but the process is dead, the stale PID file is cleaned up.
func IsRunning(dir string) (bool, int) {
	path := PIDPath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return false, 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		_ = os.Remove(path)
		return false, 0
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(path)
		return false, 0
	}

	// Signal 0 tests process existence without delivering a real signal
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, syscall.EPERM) {
			// Process exists, but caller lacks permission to signal it (e.g. root daemon / PID 1)
			return true, pid
		}
		// Process is dead: clean up stale PID file
		_ = os.Remove(path)
		return false, 0
	}

	return true, pid
}

// WriteHeartbeat writes the current UTC timestamp to the heartbeat file.
func WriteHeartbeat(dir string) error {
	if dir == "" {
		dir = DefaultStateDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(HeartbeatPath(dir), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0644)
}

// ReadHeartbeat reads the last heartbeat timestamp from dir.
func ReadHeartbeat(dir string) (time.Time, error) {
	data, err := os.ReadFile(HeartbeatPath(dir))
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
}

// RemoveHeartbeat removes the heartbeat file.
func RemoveHeartbeat(dir string) {
	_ = os.Remove(HeartbeatPath(dir))
}

// Lock represents an acquired daemon single-instance lock.
type Lock struct {
	dir        string
	cancelBeat context.CancelFunc
}

// Release releases the daemon lock, removing PID and heartbeat files.
func (l *Lock) Release() {
	if l == nil {
		return
	}
	if l.cancelBeat != nil {
		l.cancelBeat()
	}
	RemovePID(l.dir)
	RemoveHeartbeat(l.dir)
}

// AcquireLock checks if a daemon is running in dir, writes the PID file,
// and starts periodic heartbeat writes. Returns ErrAlreadyRunning if alive.
func AcquireLock(dir string) (*Lock, error) {
	if dir == "" {
		dir = DefaultStateDir()
	}

	if running, pid := IsRunning(dir); running {
		if pid != os.Getpid() {
			return nil, ErrAlreadyRunning{PID: pid}
		}
	}

	if err := WritePID(dir); err != nil {
		return nil, fmt.Errorf("write pid: %w", err)
	}
	_ = WriteHeartbeat(dir)

	ctx, cancel := context.WithCancel(context.Background())
	lock := &Lock{
		dir:        dir,
		cancelBeat: cancel,
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = WriteHeartbeat(dir)
			}
		}
	}()

	return lock, nil
}
