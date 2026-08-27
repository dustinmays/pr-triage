// Package opencode implements the AgentRuntime adapter for the OpenCode CLI.
package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

const (
	// RuntimeName is the registered name for this adapter in runtime.Registry.
	RuntimeName = runtime.NameOpenCode // "opencode"
	defaultBin  = "opencode"
)

// Adapter implements runtime.AgentRuntime for the OpenCode CLI.
type Adapter struct {
	// Binary is the path or executable name for opencode (defaults to "opencode").
	Binary string
}

func New() *Adapter { return &Adapter{Binary: defaultBin} }

func (a *Adapter) Name() string { return RuntimeName }

// BuildArgs constructs the command-line arguments for invoking OpenCode in
// non-interactive JSON-streaming mode. The prompt is passed as the trailing
// positional message argument (OpenCode's `run [message..]`); do NOT use a
// "--" separator (opencode's yargs would divert everything after it away from
// the message positional).
func (a *Adapter) BuildArgs(inv runtime.Invocation) []string {
	args := []string{"run", "--format", "json"}
	if inv.Model != "" {
		args = append(args, "-m", inv.Model)
	}
	if inv.Workdir != "" {
		args = append(args, "--dir", inv.Workdir)
	}
	if inv.AgentName != "" {
		args = append(args, "--agent", inv.AgentName)
	}
	args = append(args, inv.Prompt)
	return args
}

// Run executes a single agent invocation using OpenCode CLI, writing raw
// output to logFile.
//
// Note: OpenCode has no --max-turns flag, so Limits.MaxTurns is not enforced
// here; the caller is responsible for that limit. Timeout IS enforced via
// context + SIGTERM.
func (a *Adapter) Run(ctx context.Context, inv runtime.Invocation, logFile io.Writer) (int, error) {
	if inv.Model != "" && !strings.Contains(inv.Model, "/") {
		return -1, fmt.Errorf("opencode: model %q must be in provider/model form", inv.Model)
	}

	if inv.Limits.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, inv.Limits.Timeout)
		defer cancel()
	}

	bin := a.Binary
	if bin == "" {
		bin = defaultBin
	}

	args := a.BuildArgs(inv)
	cmd := exec.CommandContext(ctx, bin, args...)
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

	err := cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}

	return 0, nil
}

type streamEvent struct {
	Type string `json:"type"`
	Part struct {
		Type   string   `json:"type"`
		Cost   *float64 `json:"cost"`
		Reason string   `json:"reason"`
		Text   string   `json:"text"`
	} `json:"part"`
}

// ParseResult reads OpenCode's newline-delimited JSON stream and accumulates a
// normalized runtime.Result with exact cost basis. Each step_finish event is
// one model step with its own cost; costs are summed across steps and each
// step counts as one turn. Text events are concatenated in order into the
// Summary.
func (a *Adapter) ParseResult(log io.Reader) (*runtime.Result, error) {
	if log == nil {
		return nil, errors.New("opencode: log reader is nil")
	}

	reader := bufio.NewReader(log)
	var totalCost float64
	var turns int
	var lastReason string
	var summary strings.Builder
	sawStep := false

	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			var event streamEvent
			if jsonErr := json.Unmarshal([]byte(line), &event); jsonErr == nil {
				switch event.Type {
				case "step_finish":
					sawStep = true
					turns++
					if event.Part.Cost != nil {
						totalCost += *event.Part.Cost
					}
					lastReason = event.Part.Reason
				case "text":
					summary.WriteString(event.Part.Text)
				}
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("opencode: reading log: %w", err)
		}
	}

	if !sawStep {
		return nil, errors.New("opencode: no step_finish event found in log")
	}

	return &runtime.Result{
		Cost:       totalCost,
		CostBasis:  runtime.CostBasisExact,
		Turns:      turns,
		StopReason: lastReason,
		IsError:    false,
		Summary:    summary.String(),
	}, nil
}

// ClassifyOutcome maps a parsed Result and process exit code to a runtime.Outcome.
func (a *Adapter) ClassifyOutcome(res *runtime.Result, exitCode int) runtime.Outcome {
	if res == nil {
		return runtime.OutcomeError
	}
	if res.IsError || exitCode != 0 {
		return runtime.OutcomeFailed
	}
	return runtime.OutcomeSuccess
}

func init() { runtime.Register(New()) }
