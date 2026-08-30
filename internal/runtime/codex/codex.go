// Package codex implements the AgentRuntime adapter for the Codex CLI.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

const (
	// RuntimeName is the registered name for this adapter in runtime.Registry.
	RuntimeName   = runtime.NameCodex // "codex"
	defaultBin    = "codex"
	schemaVersion = 1
)

type adapterEnvelopePayload struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Model   string `json:"model"`
}

type adapterEnvelope struct {
	PrTriageCodex adapterEnvelopePayload `json:"pr_triage_codex"`
}

type rawEnvelope struct {
	PrTriageCodex *adapterEnvelopePayload `json:"pr_triage_codex,omitempty"`
}

type streamEvent struct {
	Type  string       `json:"type"`
	Item  *streamItem  `json:"item,omitempty"`
	Usage *streamUsage `json:"usage,omitempty"`
}

type streamItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Message string `json:"message,omitempty"`
}

type streamUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

type modelPrice struct {
	uncachedInputPerMillion float64
	cachedInputPerMillion   float64
	outputPerMillion        float64
}

var modelPrices = map[string]modelPrice{
	"gpt-5.6-sol": {
		uncachedInputPerMillion: 4.00,
		cachedInputPerMillion:   0.40,
		outputPerMillion:        20.00,
	},
}

// Adapter implements runtime.AgentRuntime for the Codex CLI.
type Adapter struct {
	// Binary is the path or executable name for codex (defaults to "codex").
	Binary string
}

// New returns a new Adapter configured with default settings.
func New() *Adapter {
	return &Adapter{Binary: defaultBin}
}

// Name returns the runtime identifier "codex".
func (a *Adapter) Name() string {
	return RuntimeName
}

// BuildArgs constructs the command-line arguments for invoking Codex.
// Codex runs in non-interactive JSON mode with ephemeral sessions inside
// a workspace-write sandbox. The prompt is passed inline as the trailing
// positional argument. No provider slash validation is applied.
func (a *Adapter) BuildArgs(inv runtime.Invocation) []string {
	args := []string{"exec", "--json", "--ephemeral", "--sandbox", "workspace-write"}
	if inv.Model != "" {
		args = append(args, "-m", inv.Model)
	}
	args = append(args, inv.Prompt)
	return args
}

// Capabilities declares what this adapter enforces and how it reports cost.
// Codex reports estimated cost from local rate cards and enforces timeout
// via ExecRun's context deadline; turns and budget limits are not enforced.
func (a *Adapter) Capabilities() runtime.Capabilities {
	return runtime.Capabilities{
		CostBasis:       runtime.CostBasisEstimated,
		EnforcesTimeout: true,
		EnforcesTurns:   false,
		EnforcesBudget:  false,
		ModelForm:       runtime.ModelFormPlain,
		AuthModel:       "saved codex login / invocation-scoped CODEX_API_KEY",
	}
}

// Run executes a single agent invocation using the Codex CLI, writing raw
// output to logFile. The subprocess lifecycle (timeout, SIGTERM cancel, PID
// callback, exit-code unwrap) is handled by runtime.ExecRun.
//
// Before child execution output is captured, a namespaced version-1 invocation
// envelope is written to logFile so ParseResult can recover the model for
// cost estimation.
func (a *Adapter) Run(ctx context.Context, inv runtime.Invocation, logFile io.Writer) (int, error) {
	bin := a.Binary
	if bin == "" {
		bin = defaultBin
	}

	if logFile != nil {
		env := adapterEnvelope{
			PrTriageCodex: adapterEnvelopePayload{
				Version: schemaVersion,
				Kind:    "invocation",
				Model:   inv.Model,
			},
		}
		b, err := json.Marshal(env)
		if err != nil {
			return -1, fmt.Errorf("codex: marshal invocation envelope: %w", err)
		}
		b = append(b, '\n')
		n, err := logFile.Write(b)
		if err != nil {
			return -1, fmt.Errorf("codex: write invocation envelope: %w", err)
		}
		if n != len(b) {
			return -1, fmt.Errorf("codex: write invocation envelope: %w", io.ErrShortWrite)
		}
	}

	return runtime.ExecRun(ctx, inv, logFile, runtime.ExecSpec{
		Binary: bin,
		Args:   a.BuildArgs(inv),
	})
}

// ParseResult reads the captured Codex JSONL log stream and extracts a
// normalized runtime.Result. It derives the summary from the terminal
// agent_message item, counts runtime-local turns from terminal turn events,
// and computes estimated cost for known priced models from captured usage.
func (a *Adapter) ParseResult(log io.Reader) (*runtime.Result, error) {
	if log == nil {
		return nil, errors.New("codex: log reader is nil")
	}

	reader := bufio.NewReader(log)
	var (
		model             string
		summary           string
		turns             int
		stopReason        string
		isError           bool
		sawTerminalTurn   bool
		sawUsage          bool
		totalInputTokens  int
		totalCachedTokens int
		totalOutputTokens int
	)

	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			var env rawEnvelope
			if jsonErr := json.Unmarshal([]byte(line), &env); jsonErr == nil && env.PrTriageCodex != nil {
				if env.PrTriageCodex.Kind == "invocation" && env.PrTriageCodex.Version == schemaVersion {
					model = env.PrTriageCodex.Model
				}
			} else {
				var event streamEvent
				if jsonErr := json.Unmarshal([]byte(line), &event); jsonErr == nil && event.Type != "" {
					switch event.Type {
					case "item.completed":
						if event.Item != nil && event.Item.Type == "agent_message" {
							summary = event.Item.Text
						}
					case "turn.completed":
						sawTerminalTurn = true
						turns++
						stopReason = "turn.completed"
						isError = false
						if event.Usage != nil {
							sawUsage = true
							totalInputTokens += event.Usage.InputTokens
							totalCachedTokens += event.Usage.CachedInputTokens
							totalOutputTokens += event.Usage.OutputTokens
						}
					case "turn.failed":
						sawTerminalTurn = true
						turns++
						stopReason = "turn.failed"
						isError = true
						if event.Usage != nil {
							sawUsage = true
							totalInputTokens += event.Usage.InputTokens
							totalCachedTokens += event.Usage.CachedInputTokens
							totalOutputTokens += event.Usage.OutputTokens
						}
					}
				}
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("codex: reading log: %w", err)
		}
	}

	if !sawTerminalTurn {
		return nil, errors.New("codex: no terminal turn event found in log")
	}

	cost := 0.0
	costBasis := runtime.CostBasisUnavailable
	if sawUsage && model != "" {
		if price, ok := modelPrices[model]; ok {
			uncachedInput := totalInputTokens - totalCachedTokens
			if uncachedInput < 0 {
				uncachedInput = 0
			}
			uncachedCost := float64(uncachedInput) * price.uncachedInputPerMillion / 1e6
			cachedCost := float64(totalCachedTokens) * price.cachedInputPerMillion / 1e6
			outputCost := float64(totalOutputTokens) * price.outputPerMillion / 1e6
			cost = uncachedCost + cachedCost + outputCost
			costBasis = runtime.CostBasisEstimated
		}
	}

	return &runtime.Result{
		Cost:       cost,
		CostBasis:  costBasis,
		Turns:      turns,
		StopReason: stopReason,
		IsError:    isError,
		Summary:    summary,
	}, nil
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
