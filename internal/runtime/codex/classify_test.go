package codex

import (
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

// Given a non-error Result and exit code 0, when ClassifyOutcome is evaluated, it returns OutcomeSuccess.
// This marks cleanly finished triage runs as successful.
func TestClassifyOutcomeSuccessRunIsSuccess(t *testing.T) {
	rt := mustGetCodex(t)
	res := &runtime.Result{
		Cost:       knownPricedExpectedCost,
		CostBasis:  runtime.CostBasisEstimated,
		Turns:      1,
		StopReason: "turn.completed",
		IsError:    false,
		Summary:    "SCHEMA_OK",
	}

	outcome := rt.ClassifyOutcome(res, 0)
	if outcome != runtime.OutcomeSuccess {
		t.Fatalf("ClassifyOutcome(clean result, exit 0) = %q, want %q", outcome, runtime.OutcomeSuccess)
	}
}

// Given a clean Result but a non-zero child exit code, when ClassifyOutcome is evaluated, it returns OutcomeFailed.
// This ensures process-level exit failures override parsed in-stream result status.
func TestClassifyOutcomeNonZeroExitIsFailed(t *testing.T) {
	rt := mustGetCodex(t)
	res := &runtime.Result{
		Cost:       knownPricedExpectedCost,
		CostBasis:  runtime.CostBasisEstimated,
		Turns:      1,
		StopReason: "turn.completed",
		IsError:    false,
	}

	outcome := rt.ClassifyOutcome(res, 1)
	if outcome != runtime.OutcomeFailed {
		t.Fatalf("ClassifyOutcome(result, exit 1) = %q, want %q", outcome, runtime.OutcomeFailed)
	}
}

// Given a Result with IsError set to true, when ClassifyOutcome is evaluated, it returns OutcomeFailed.
// This guarantees that in-stream agent failures prevent false-positive triage approvals.
func TestClassifyOutcomeRuntimeErrorResultIsFailed(t *testing.T) {
	rt := mustGetCodex(t)
	res := &runtime.Result{
		Cost:       0.0,
		CostBasis:  runtime.CostBasisUnavailable,
		Turns:      1,
		StopReason: "turn.failed",
		IsError:    true,
	}

	outcome := rt.ClassifyOutcome(res, 0)
	if outcome != runtime.OutcomeFailed {
		t.Fatalf("ClassifyOutcome(IsError result, exit 0) = %q, want %q", outcome, runtime.OutcomeFailed)
	}
}

// Given a nil Result pointer, when ClassifyOutcome is evaluated, it returns OutcomeError.
// This classifies parse and execution crashes as infrastructure errors.
func TestClassifyOutcomeNilResultIsError(t *testing.T) {
	rt := mustGetCodex(t)
	outcome := rt.ClassifyOutcome(nil, 0)
	if outcome != runtime.OutcomeError {
		t.Fatalf("ClassifyOutcome(nil, exit 0) = %q, want %q", outcome, runtime.OutcomeError)
	}
}
