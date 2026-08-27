package opencode

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

func TestRegistryRegistration(t *testing.T) {
	rt, err := runtime.Get(RuntimeName)
	if err != nil {
		t.Fatalf("expected runtime %q to be registered, got error: %v", RuntimeName, err)
	}
	if rt.Name() != RuntimeName {
		t.Fatalf("expected Name() = %q, got %q", RuntimeName, rt.Name())
	}
}

func TestBuildArgs(t *testing.T) {
	a := New()

	tests := []struct {
		name     string
		inv      runtime.Invocation
		expected []string
	}{
		{
			name: "full options",
			inv: runtime.Invocation{
				Model:     "openrouter/z-ai/glm-5.3-flash",
				Workdir:   "/tmp/wt",
				AgentName: "review-agent",
				Prompt:    "do the thing",
			},
			expected: []string{
				"run", "--format", "json",
				"-m", "openrouter/z-ai/glm-5.3-flash",
				"--dir", "/tmp/wt",
				"--agent", "review-agent",
				"do the thing",
			},
		},
		{
			name: "optional flags omitted",
			inv: runtime.Invocation{
				Prompt: "fix bug",
			},
			expected: []string{
				"run", "--format", "json",
				"fix bug",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := a.BuildArgs(tc.inv)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("unexpected args:\ngot:  %v\nwant: %v", got, tc.expected)
			}
		})
	}
}

// multiStepFixture is the empirically captured NDJSON stream from opencode
// 1.18.21 for a run that made one tool call before finishing.
var multiStepFixture = `{"type":"step_start","part":{"type":"step-start"}}` + "\n" +
	`{"type":"step_finish","part":{"type":"step-finish","cost":0.00066775,"reason":"tool-calls"}}` + "\n" +
	`{"type":"text","part":{"type":"text","text":"Contents: ` + "`" + `hello world` + "`" + `\n\ndone"}}` + "\n" +
	`{"type":"step_finish","part":{"type":"step-finish","cost":0.0002235,"reason":"stop"}}` + "\n"

func TestParseResult_MultiStepSumsCostAndCountsTurns(t *testing.T) {
	a := New()
	res, err := a.ParseResult(strings.NewReader(multiStepFixture))
	if err != nil {
		t.Fatalf("ParseResult returned error: %v", err)
	}

	wantCost := 0.00066775 + 0.0002235
	if math.Abs(res.Cost-wantCost) > 1e-9 {
		t.Errorf("expected Cost %v (sum of per-step costs), got %v", wantCost, res.Cost)
	}
	if res.CostBasis != runtime.CostBasisExact {
		t.Errorf("expected CostBasis %q, got %q", runtime.CostBasisExact, res.CostBasis)
	}
	if res.Turns != 2 {
		t.Errorf("expected Turns 2 (one per step_finish), got %d", res.Turns)
	}
	if res.StopReason != "stop" {
		t.Errorf("expected StopReason %q, got %q", "stop", res.StopReason)
	}
	wantSummary := "Contents: " + "`" + "hello world" + "`" + "\n\ndone"
	if res.Summary != wantSummary {
		t.Errorf("expected Summary %q, got %q", wantSummary, res.Summary)
	}
	if err := res.Validate(); err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestParseResult_NoStepFinishIsError(t *testing.T) {
	fixture := `{"type":"step_start","part":{"type":"step-start"}}` + "\n" +
		`{"type":"text","part":{"type":"text","text":"partial output"}}` + "\n"

	a := New()
	res, err := a.ParseResult(strings.NewReader(fixture))
	if err == nil {
		t.Fatalf("expected error when no step_finish event is present")
	}
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}

	if _, err := a.ParseResult(nil); err == nil {
		t.Errorf("expected error on nil reader")
	}
}

func TestParseResult_SkipsNonJSONLines(t *testing.T) {
	fixture := "opencode v1.18.21 starting up\n" +
		multiStepFixture[:len(multiStepFixture)-1] // strip trailing newline so we can interleave
	fixture = strings.Replace(fixture,
		`{"type":"text"`,
		"[WARN] stderr noise line\n{\"type\":\"text\"",
		1,
	) + "\nanother non-JSON log line\n"

	a := New()
	res, err := a.ParseResult(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseResult failed on interleaved log noise: %v", err)
	}
	if res.Turns != 2 {
		t.Errorf("expected Turns 2 despite non-JSON lines, got %d", res.Turns)
	}
}

func TestClassifyOutcome(t *testing.T) {
	a := New()

	tests := []struct {
		name     string
		res      *runtime.Result
		exitCode int
		expected runtime.Outcome
	}{
		{
			name:     "nil result gives OutcomeError",
			res:      nil,
			exitCode: 0,
			expected: runtime.OutcomeError,
		},
		{
			name: "is_error true gives OutcomeFailed",
			res: &runtime.Result{
				CostBasis: runtime.CostBasisExact,
				IsError:   true,
			},
			exitCode: 0,
			expected: runtime.OutcomeFailed,
		},
		{
			name: "non-zero exit code gives OutcomeFailed",
			res: &runtime.Result{
				CostBasis: runtime.CostBasisExact,
			},
			exitCode: 1,
			expected: runtime.OutcomeFailed,
		},
		{
			name: "clean success gives OutcomeSuccess",
			res: &runtime.Result{
				Cost:       0.001,
				CostBasis:  runtime.CostBasisExact,
				Turns:      1,
				StopReason: "stop",
			},
			exitCode: 0,
			expected: runtime.OutcomeSuccess,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := a.ClassifyOutcome(tc.res, tc.exitCode)
			if got != tc.expected {
				t.Fatalf("expected outcome %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestRun_ModelWithoutSlashIsRejected(t *testing.T) {
	a := New()
	inv := runtime.Invocation{Model: "claude-3-7-sonnet", Prompt: "test"}
	exitCode, err := a.Run(context.Background(), inv, nil)
	if err == nil {
		t.Fatalf("expected error for slash-less model, got nil")
	}
	if exitCode != -1 {
		t.Errorf("expected exit code -1 on launch failure, got %d", exitCode)
	}
}

func TestRun_Execution(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-opencode")
	script := `#!/bin/sh
echo '{"type":"step_start","part":{"type":"step-start"}}'
echo '{"type":"step_finish","part":{"type":"step-finish","cost":0.0005,"reason":"tool-calls"}}'
echo '{"type":"text","part":{"type":"text","text":"all done"}}'
echo '{"type":"step_finish","part":{"type":"step-finish","cost":0.00025,"reason":"stop"}}'
exit 0
`
	if err := os.WriteFile(mockBin, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	adapter := &Adapter{Binary: mockBin}

	var buf strings.Builder
	inv := runtime.Invocation{
		Model:       "openrouter/z-ai/glm-5.3-flash",
		Prompt:      "hello world",
		Workdir:     tmpDir,
		Limits:      runtime.Limits{Timeout: 5 * time.Second},
		PIDCallback: func(pid int) {},
	}

	exitCode, err := adapter.Run(context.Background(), inv, &buf)
	if err != nil {
		t.Fatalf("Run failed unexpectedly: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	res, err := adapter.ParseResult(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseResult failed on run output: %v", err)
	}
	wantCost := 0.0005 + 0.00025
	if math.Abs(res.Cost-wantCost) > 1e-9 {
		t.Errorf("expected summed cost %v, got %v", wantCost, res.Cost)
	}
	if res.Summary != "all done" {
		t.Errorf("expected Summary %q, got %q", "all done", res.Summary)
	}

	if outcome := adapter.ClassifyOutcome(res, exitCode); outcome != runtime.OutcomeSuccess {
		t.Errorf("expected outcome %q, got %q", runtime.OutcomeSuccess, outcome)
	}
}

func TestRun_CommandNotFound(t *testing.T) {
	adapter := &Adapter{Binary: "/nonexistent/binary/path"}
	inv := runtime.Invocation{Prompt: "test"}
	_, err := adapter.Run(context.Background(), inv, nil)
	if err == nil {
		t.Fatalf("expected error for nonexistent binary, got nil")
	}
}
