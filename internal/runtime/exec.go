package runtime

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"syscall"
)

// ExecSpec describes how to launch a runtime's CLI as a subprocess. Adapters
// build one of these and hand it to ExecRun instead of re-implementing the
// process lifecycle (timeout, cancel signal, PID callback, exit-code unwrap,
// log fan-out) that is identical across every exec-based adapter.
type ExecSpec struct {
	// Binary is the executable name or path to launch (e.g. "claude").
	Binary string
	// Args is the full argument vector, typically from the adapter's BuildArgs.
	Args []string
	// PreCheck, if non-nil, runs before the process is launched. Returning a
	// non-nil error aborts the launch and is surfaced by ExecRun as
	// (-1, err) — the same "could not be executed at all" signal the
	// orchestrator escalates on. Use it for validation that must happen before
	// spending a subprocess, e.g. model-form checks.
	PreCheck func(Invocation) error
}

// ExecRun launches spec.Binary with spec.Args and manages the full run
// lifecycle on behalf of an adapter's Run method:
//
//   - applies inv.Limits.Timeout as a context deadline (when > 0);
//   - cancels a timed-out or cancelled run with SIGTERM (not SIGKILL), so the
//     child can flush its output;
//   - runs spec.PreCheck (if set) before launch, mapping its error to (-1, err);
//   - sets the working directory to inv.Workdir (when non-empty);
//   - streams both stdout and stderr to logFile (when non-nil);
//   - fires inv.PIDCallback with the child PID immediately after launch;
//   - unwraps *exec.ExitError into (exitCode, nil) so a process that ran and
//     exited non-zero is reported distinctly from a process that could not be
//     launched at all, which returns (-1, err).
//
// This is the shared implementation behind claudecode.Run, opencode.Run, and
// any future adapter; see docs/adding-a-runtime.md.
func ExecRun(ctx context.Context, inv Invocation, logFile io.Writer, spec ExecSpec) (int, error) {
	if spec.PreCheck != nil {
		if err := spec.PreCheck(inv); err != nil {
			return -1, err
		}
	}

	if inv.Limits.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, inv.Limits.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, spec.Binary, spec.Args...)
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Signal(syscall.SIGTERM)
		}
		return nil
	}

	if inv.Workdir != "" {
		cmd.Dir = inv.Workdir
	}

	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	if inv.PIDCallback != nil && cmd.Process != nil {
		inv.PIDCallback(cmd.Process.Pid)
	}

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}

	return 0, nil
}
