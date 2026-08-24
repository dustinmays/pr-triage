package claudecode

import (
	"context"
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
				AgentName: "code-reviewer",
				Model:     "claude-3-7-sonnet",
				Prompt:    "review this PR",
				Limits: runtime.Limits{
					MaxTurns: 10,
				},
			},
			expected: []string{
				"--agent", "code-reviewer",
				"-p", "--output-format", "stream-json", "--verbose",
				"--model", "claude-3-7-sonnet",
				"--max-turns", "10",
				"--", "review this PR",
			},
		},
		{
			name: "minimal options",
			inv: runtime.Invocation{
				Prompt: "fix bug",
			},
			expected: []string{
				"-p", "--output-format", "stream-json", "--verbose",
				"--", "fix bug",
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

func TestParseResult_Success(t *testing.T) {
	fixture := `{"type":"system","message":"agent started"}
{"type":"assistant","message":"analyzing diff"}
{"type":"tool_use","name":"bash","input":{"command":"git status"}}
{"type":"result","subtype":"success","total_cost_usd":0.01234,"num_turns":3,"stop_reason":"end_turn","is_error":false,"result":"LGTM"}
`
	a := New()
	res, err := a.ParseResult(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseResult returned error: %v", err)
	}

	if res.Cost != 0.01234 {
		t.Errorf("expected Cost 0.01234, got %v", res.Cost)
	}
	if res.CostBasis != runtime.CostBasisExact {
		t.Errorf("expected CostBasis %q, got %q", runtime.CostBasisExact, res.CostBasis)
	}
	if res.Turns != 3 {
		t.Errorf("expected Turns 3, got %d", res.Turns)
	}
	if res.StopReason != "end_turn" {
		t.Errorf("expected StopReason %q, got %q", "end_turn", res.StopReason)
	}
	if res.IsError {
		t.Errorf("expected IsError false, got true")
	}
	if err := res.Validate(); err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestParseResult_DoubleQuotedStopReason(t *testing.T) {
	fixture := `{"type":"result","subtype":"success","total_cost_usd":0.05,"num_turns":2,"stop_reason":"\"end_turn\"","is_error":false}`
	a := New()
	res, err := a.ParseResult(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseResult returned error: %v", err)
	}

	if res.StopReason != "end_turn" {
		t.Errorf("expected unquoted StopReason %q, got %q", "end_turn", res.StopReason)
	}
}

func TestParseResult_ErrorEvent(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
	}{
		{
			name:    "is_error true",
			fixture: `{"type":"result","total_cost_usd":0.002,"num_turns":1,"stop_reason":"error","is_error":true}`,
		},
		{
			name:    "subtype error",
			fixture: `{"type":"result","subtype":"error","total_cost_usd":0.002,"num_turns":1,"stop_reason":"error","is_error":false}`,
		},
	}

	a := New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := a.ParseResult(strings.NewReader(tc.fixture))
			if err != nil {
				t.Fatalf("ParseResult failed: %v", err)
			}
			if !res.IsError {
				t.Fatalf("expected IsError true, got false")
			}
			if res.CostBasis != runtime.CostBasisExact {
				t.Errorf("expected CostBasis %q, got %q", runtime.CostBasisExact, res.CostBasis)
			}
		})
	}
}

func TestParseResult_ZeroCost(t *testing.T) {
	fixture := `{"type":"result","subtype":"success","total_cost_usd":0,"num_turns":0,"stop_reason":"end_turn","is_error":false}`
	a := New()
	res, err := a.ParseResult(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseResult returned error: %v", err)
	}

	if res.Cost != 0.0 {
		t.Errorf("expected Cost 0.0, got %v", res.Cost)
	}
	if res.CostBasis != runtime.CostBasisExact {
		t.Errorf("expected CostBasis %q, got %q", runtime.CostBasisExact, res.CostBasis)
	}
	if err := res.Validate(); err != nil {
		t.Errorf("Validate failed on zero cost: %v", err)
	}
}

func TestParseResult_MissingOrEmpty(t *testing.T) {
	a := New()

	if _, err := a.ParseResult(nil); err == nil {
		t.Errorf("expected error on nil reader")
	}

	if _, err := a.ParseResult(strings.NewReader("")); err == nil {
		t.Errorf("expected error on empty reader")
	}

	nonResult := `{"type":"system","message":"hello"}
{"type":"assistant","message":"world"}`
	if _, err := a.ParseResult(strings.NewReader(nonResult)); err == nil {
		t.Errorf("expected error when no result event is present")
	}
}

func TestParseResult_LongLines(t *testing.T) {
	longPayload := strings.Repeat("x", 200*1024)
	fixture := `{"type":"assistant","message":"` + longPayload + `"}` + "\n" +
		`{"type":"result","subtype":"success","total_cost_usd":0.042,"num_turns":5,"stop_reason":"end_turn","is_error":false}` + "\n"

	a := New()
	res, err := a.ParseResult(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseResult failed on long lines: %v", err)
	}
	if res.Cost != 0.042 {
		t.Errorf("expected cost 0.042, got %v", res.Cost)
	}
	if res.Turns != 5 {
		t.Errorf("expected turns 5, got %d", res.Turns)
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
			name: "clean success gives OutcomeSuccess",
			res: &runtime.Result{
				Cost:       0.01,
				CostBasis:  runtime.CostBasisExact,
				Turns:      2,
				StopReason: "end_turn",
				IsError:    false,
			},
			exitCode: 0,
			expected: runtime.OutcomeSuccess,
		},
		{
			name: "is_error true gives OutcomeFailed",
			res: &runtime.Result{
				Cost:       0.01,
				CostBasis:  runtime.CostBasisExact,
				Turns:      2,
				StopReason: "error",
				IsError:    true,
			},
			exitCode: 0,
			expected: runtime.OutcomeFailed,
		},
		{
			name: "non-zero exit code gives OutcomeFailed",
			res: &runtime.Result{
				Cost:       0.01,
				CostBasis:  runtime.CostBasisExact,
				Turns:      2,
				StopReason: "end_turn",
				IsError:    false,
			},
			exitCode: 1,
			expected: runtime.OutcomeFailed,
		},
		{
			name: "timeout stop_reason gives OutcomeTimeout",
			res: &runtime.Result{
				Cost:       0.01,
				CostBasis:  runtime.CostBasisExact,
				Turns:      10,
				StopReason: "timeout",
				IsError:    false,
			},
			exitCode: 0,
			expected: runtime.OutcomeTimeout,
		},
		{
			name: "timed_out stop_reason gives OutcomeTimeout",
			res: &runtime.Result{
				Cost:       0.01,
				CostBasis:  runtime.CostBasisExact,
				Turns:      10,
				StopReason: "timed_out",
				IsError:    false,
			},
			exitCode: 143,
			expected: runtime.OutcomeTimeout,
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

func TestRun_Execution(t *testing.T) {
	// Create a mock executable script in a temporary directory.
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-claude")
	script := `#!/bin/sh
echo '{"type":"system","message":"start"}'
echo '{"type":"result","subtype":"success","total_cost_usd":0.025,"num_turns":2,"stop_reason":"end_turn","is_error":false}'
exit 0
`
	if err := os.WriteFile(mockBin, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	adapter := &Adapter{Binary: mockBin}

	var buf strings.Builder
	inv := runtime.Invocation{
		AgentName: "reviewer",
		Model:     "claude-3-7-sonnet",
		Prompt:    "hello world",
		Workdir:   tmpDir,
		Limits: runtime.Limits{
			Timeout:  5 * time.Second,
			MaxTurns: 5,
		},
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
	if res.Cost != 0.025 {
		t.Errorf("expected cost 0.025, got %v", res.Cost)
	}
	if res.CostBasis != runtime.CostBasisExact {
		t.Errorf("expected CostBasis %q, got %q", runtime.CostBasisExact, res.CostBasis)
	}
	if res.Turns != 2 {
		t.Errorf("expected turns 2, got %d", res.Turns)
	}

	outcome := adapter.ClassifyOutcome(res, exitCode)
	if outcome != runtime.OutcomeSuccess {
		t.Errorf("expected outcome %q, got %q", runtime.OutcomeSuccess, outcome)
	}
}

func TestRun_NonZeroExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-claude-fail")
	script := `#!/bin/sh
echo '{"type":"result","subtype":"error","total_cost_usd":0.005,"num_turns":1,"stop_reason":"error","is_error":true}'
exit 2
`
	if err := os.WriteFile(mockBin, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	adapter := &Adapter{Binary: mockBin}

	var buf strings.Builder
	inv := runtime.Invocation{
		Prompt: "test failure",
	}

	exitCode, err := adapter.Run(context.Background(), inv, &buf)
	if err != nil {
		t.Fatalf("Run returned unexpected error for non-zero exit: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}

	res, err := adapter.ParseResult(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseResult failed: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError true")
	}

	outcome := adapter.ClassifyOutcome(res, exitCode)
	if outcome != runtime.OutcomeFailed {
		t.Errorf("expected outcome %q, got %q", runtime.OutcomeFailed, outcome)
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

func TestRun_PIDCallbackAndTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-claude-sleep")
	script := `#!/bin/sh
sleep 10
`
	if err := os.WriteFile(mockBin, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	adapter := &Adapter{Binary: mockBin}

	var capturedPID int
	inv := runtime.Invocation{
		Prompt: "sleep test",
		Limits: runtime.Limits{
			Timeout: 100 * time.Millisecond,
		},
		PIDCallback: func(pid int) {
			capturedPID = pid
		},
	}

	start := time.Now()
	exitCode, err := adapter.Run(context.Background(), inv, nil)
	elapsed := time.Since(start)

	if capturedPID <= 0 {
		t.Errorf("expected PID callback to receive valid PID, got %d", capturedPID)
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout did not terminate process in time: %v", elapsed)
	}
	if err == nil && exitCode == 0 {
		t.Errorf("expected timeout failure, got success")
	}
}
