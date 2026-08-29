// Package claudecode implements the AgentRuntime adapter for Claude Code CLI.
package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

const (
	// RuntimeName is the registered name for this adapter in runtime.Registry.
	RuntimeName = "claude-code"
	defaultBin  = "claude"
)

// Adapter implements runtime.AgentRuntime for the Claude Code CLI.
type Adapter struct {
	// Binary is the path or executable name for claude (defaults to "claude").
	Binary string
}

// New returns a new Adapter configured with default settings.
func New() *Adapter {
	return &Adapter{
		Binary: defaultBin,
	}
}

// Name returns the runtime identifier "claude-code".
func (a *Adapter) Name() string {
	return RuntimeName
}

// BuildArgs constructs the command-line arguments for invoking Claude Code.
func (a *Adapter) BuildArgs(inv runtime.Invocation) []string {
	var args []string
	if inv.AgentName != "" {
		args = append(args, "--agent", inv.AgentName)
	}
	args = append(args, "-p", "--output-format", "stream-json", "--verbose")
	// The daemon runs the agent unattended, so there is no human to approve tool
	// use. Without an explicit permission mode the CLI defaults to "default",
	// which auto-denies non-allowlisted Bash calls — the agent then cannot run
	// its verification toolchain (make/lint/gofmt), commit/push fixes, or post its
	// review comment. bypassPermissions grants the autonomous agent the access it
	// needs; it operates inside an isolated git worktree and risky changes are
	// escalated to a human before ever reaching the agent.
	args = append(args, "--permission-mode", "bypassPermissions")
	if inv.Model != "" {
		args = append(args, "--model", inv.Model)
	}
	if inv.Limits.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(inv.Limits.MaxTurns))
	}
	args = append(args, "--", inv.Prompt)
	return args
}

// Capabilities declares what this adapter enforces and how it reports cost.
// Claude Code reports an authoritative terminal cost and enforces its own
// max-turns via the --max-turns flag; it takes a plain model name.
func (a *Adapter) Capabilities() runtime.Capabilities {
	return runtime.Capabilities{
		CostBasis:       runtime.CostBasisExact,
		EnforcesTimeout: true,
		EnforcesTurns:   true,
		EnforcesBudget:  false,
		ModelForm:       runtime.ModelFormPlain,
		AuthModel:       "claude CLI login / ANTHROPIC_API_KEY in the daemon's environment",
	}
}

// Run executes a single agent invocation using Claude Code CLI, writing
// raw output to logFile. The subprocess lifecycle (timeout, SIGTERM cancel,
// PID callback, exit-code unwrap) is handled by runtime.ExecRun.
func (a *Adapter) Run(ctx context.Context, inv runtime.Invocation, logFile io.Writer) (int, error) {
	bin := a.Binary
	if bin == "" {
		bin = defaultBin
	}
	return runtime.ExecRun(ctx, inv, logFile, runtime.ExecSpec{
		Binary: bin,
		Args:   a.BuildArgs(inv),
	})
}

type streamEvent struct {
	Type         string          `json:"type"`
	Subtype      string          `json:"subtype"`
	TotalCostUSD *float64        `json:"total_cost_usd"`
	NumTurns     *int            `json:"num_turns"`
	StopReason   json.RawMessage `json:"stop_reason"`
	IsError      *bool           `json:"is_error"`
	Result       string          `json:"result"`
}

// ParseResult reads Claude Code's stream-json output and extracts the terminal
// result event into a normalized runtime.Result with exact cost basis.
func (a *Adapter) ParseResult(log io.Reader) (*runtime.Result, error) {
	if log == nil {
		return nil, errors.New("claudecode: log reader is nil")
	}

	reader := bufio.NewReader(log)
	var terminalResult *runtime.Result

	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			var event streamEvent
			if jsonErr := json.Unmarshal([]byte(line), &event); jsonErr == nil {
				if event.Type == "result" {
					var cost float64
					if event.TotalCostUSD != nil {
						cost = *event.TotalCostUSD
					}
					var turns int
					if event.NumTurns != nil {
						turns = *event.NumTurns
					}
					var stopReason string
					if len(event.StopReason) > 0 {
						if unmarshalErr := json.Unmarshal(event.StopReason, &stopReason); unmarshalErr == nil {
							if strings.HasPrefix(stopReason, `"`) && strings.HasSuffix(stopReason, `"`) && len(stopReason) >= 2 {
								if unquoted, uerr := strconv.Unquote(stopReason); uerr == nil {
									stopReason = unquoted
								} else {
									stopReason = strings.Trim(stopReason, `"`)
								}
							}
						}
					}
					isError := false
					if event.IsError != nil && *event.IsError {
						isError = true
					}
					if event.Subtype == "error" {
						isError = true
					}

					terminalResult = &runtime.Result{
						Cost:       cost,
						CostBasis:  runtime.CostBasisExact,
						Turns:      turns,
						StopReason: stopReason,
						IsError:    isError,
						Summary:    event.Result,
					}
				}
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("claudecode: reading log: %w", err)
		}
	}

	if terminalResult == nil {
		return nil, errors.New("claudecode: no terminal result event found in log")
	}

	return terminalResult, nil
}

// ClassifyOutcome maps a parsed Result and process exit code to a runtime.Outcome.
func (a *Adapter) ClassifyOutcome(res *runtime.Result, exitCode int) runtime.Outcome {
	if res == nil {
		return runtime.OutcomeError
	}
	if res.StopReason == "timeout" || res.StopReason == "timed_out" {
		return runtime.OutcomeTimeout
	}
	if res.IsError || exitCode != 0 {
		return runtime.OutcomeFailed
	}
	return runtime.OutcomeSuccess
}

func init() {
	runtime.Register(New())
}
