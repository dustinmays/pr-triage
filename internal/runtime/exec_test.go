package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestExecRun_SuccessStreamsLogAndFiresPID(t *testing.T) {
	bin := writeScript(t, "echo hello-out\necho hello-err 1>&2\nexit 0\n")

	var gotPID int
	var buf strings.Builder
	inv := Invocation{
		Limits:      Limits{Timeout: 5 * time.Second},
		PIDCallback: func(pid int) { gotPID = pid },
	}

	exit, err := ExecRun(context.Background(), inv, &buf, ExecSpec{Binary: bin})
	if err != nil {
		t.Fatalf("ExecRun error: %v", err)
	}
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if gotPID <= 0 {
		t.Errorf("PIDCallback got %d, want > 0", gotPID)
	}
	out := buf.String()
	if !strings.Contains(out, "hello-out") || !strings.Contains(out, "hello-err") {
		t.Errorf("log missing stdout/stderr; got %q", out)
	}
}

func TestExecRun_NonZeroExitReturnsCodeNotError(t *testing.T) {
	bin := writeScript(t, "exit 3\n")
	exit, err := ExecRun(context.Background(), Invocation{}, nil, ExecSpec{Binary: bin})
	if err != nil {
		t.Fatalf("a process that ran and exited non-zero must return nil error; got %v", err)
	}
	if exit != 3 {
		t.Fatalf("exit = %d, want 3", exit)
	}
}

func TestExecRun_LaunchFailureReturnsMinusOneAndError(t *testing.T) {
	exit, err := ExecRun(context.Background(), Invocation{}, nil, ExecSpec{Binary: "/no/such/binary"})
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if exit != -1 {
		t.Fatalf("exit = %d, want -1 on launch failure", exit)
	}
}

func TestExecRun_PreCheckAbortsBeforeLaunch(t *testing.T) {
	sentinel := errors.New("precheck said no")
	// Binary intentionally invalid: if PreCheck did not short-circuit, we would
	// get a launch error instead of the sentinel.
	exit, err := ExecRun(context.Background(), Invocation{}, nil, ExecSpec{
		Binary:   "/no/such/binary",
		PreCheck: func(Invocation) error { return sentinel },
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel from PreCheck", err)
	}
	if exit != -1 {
		t.Fatalf("exit = %d, want -1 on PreCheck failure", exit)
	}
}

func TestExecRun_TimeoutTerminatesProcess(t *testing.T) {
	bin := writeScript(t, "sleep 10\n")
	inv := Invocation{Limits: Limits{Timeout: 100 * time.Millisecond}}

	start := time.Now()
	exit, err := ExecRun(context.Background(), inv, nil, ExecSpec{Binary: bin})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("timeout did not terminate in time: %v", elapsed)
	}
	// SIGTERM-killed process surfaces as (-1, nil) via ExitError, matching the
	// prior adapter behavior the orchestrator classifies as a timeout.
	if err == nil && exit == 0 {
		t.Error("expected non-success from a timed-out run")
	}
}

func TestExecRun_WorkdirIsApplied(t *testing.T) {
	bin := writeScript(t, "pwd\n")
	dir := t.TempDir()
	// Resolve symlinks so macOS /var vs /private/var does not spuriously fail.
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}

	var buf strings.Builder
	if _, err := ExecRun(context.Background(), Invocation{Workdir: dir}, &buf, ExecSpec{Binary: bin}); err != nil {
		t.Fatalf("ExecRun error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != realDir && got != dir {
		t.Errorf("pwd = %q, want %q", got, realDir)
	}
}
