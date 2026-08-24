package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

// fakeRuntime is a minimal AgentRuntime used only for testing the registry
// and the Result/Invocation round trip. It writes and parses a trivial
// "cost,turns,stop_reason,is_error" log line.
type fakeRuntime struct{}

func (fakeRuntime) Name() string { return "fake" }

func (fakeRuntime) Run(_ context.Context, inv Invocation, logFile io.Writer) (int, error) {
	_, err := fmt.Fprintf(logFile, "1.5,3,end_turn,false\n")
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func (fakeRuntime) ParseResult(log io.Reader) (*Result, error) {
	scanner := bufio.NewScanner(log)
	if !scanner.Scan() {
		return nil, fmt.Errorf("fakeRuntime: empty log")
	}
	parts := strings.Split(scanner.Text(), ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("fakeRuntime: malformed log line %q", scanner.Text())
	}
	cost, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, err
	}
	turns, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}
	isError, err := strconv.ParseBool(parts[3])
	if err != nil {
		return nil, err
	}
	return &Result{
		Cost:       cost,
		CostBasis:  CostBasisExact,
		Turns:      turns,
		StopReason: parts[2],
		IsError:    isError,
	}, nil
}

func (fakeRuntime) ClassifyOutcome(res *Result, exitCode int) Outcome {
	if res.IsError || exitCode != 0 {
		return OutcomeFailed
	}
	return OutcomeSuccess
}

func TestRegistryRegisterAndGet(t *testing.T) {
	Register(fakeRuntime{})

	rt, err := Get("fake")
	if err != nil {
		t.Fatalf("Get(fake) returned unexpected error: %v", err)
	}
	if rt.Name() != "fake" {
		t.Fatalf("expected runtime name %q, got %q", "fake", rt.Name())
	}

	var buf strings.Builder
	inv := Invocation{AgentName: "test-agent", Model: "test-model", Prompt: "hello", Workdir: "."}
	exitCode, err := rt.Run(context.Background(), inv, &buf)
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	res, err := rt.ParseResult(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseResult returned unexpected error: %v", err)
	}
	if res.CostBasis != CostBasisExact {
		t.Fatalf("expected CostBasis %q, got %q", CostBasisExact, res.CostBasis)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}

	outcome := rt.ClassifyOutcome(res, exitCode)
	if outcome != OutcomeSuccess {
		t.Fatalf("expected outcome %q, got %q", OutcomeSuccess, outcome)
	}
}

func TestGetUnknownRuntime(t *testing.T) {
	if _, err := Get("nope"); err == nil {
		t.Fatal("expected non-nil error for unknown runtime name, got nil")
	}
}

func TestResultValidateRequiresCostBasis(t *testing.T) {
	res := &Result{Cost: 0}
	if err := res.Validate(); err == nil {
		t.Fatal("expected Validate to reject empty CostBasis, got nil error")
	}
}

func TestValidateKnownAndUnknown(t *testing.T) {
	resetRegistry(t)

	if err := Validate(DefaultName); err != nil {
		t.Errorf("Validate(%q) returned unexpected error: %v", DefaultName, err)
	}

	if err := Validate(""); err == nil {
		t.Error("Validate(\"\") expected error, got nil")
	}

	if err := Validate("non-existent-runtime"); err == nil {
		t.Error("Validate(\"non-existent-runtime\") expected error, got nil")
	}

	names := KnownNames()
	if len(names) == 0 {
		t.Error("KnownNames() returned empty slice")
	}
}
