package codex

import (
	"math"
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

// Given a successful Codex JSONL log, when ParseResult executes, it extracts the terminal agent_message text as the Result.Summary.
// This surfaces the agent's final decision string (e.g., SCHEMA_OK) to the triage reporter.
func TestParseResultUsesTerminalAgentMessageAsSummary(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureSuccess)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}
	if res == nil {
		t.Fatal("ParseResult returned nil Result with nil error")
	}

	const wantSummary = "SCHEMA_OK"
	if res.Summary != wantSummary {
		t.Fatalf("Result.Summary = %q, want terminal agent_message text %q", res.Summary, wantSummary)
	}
}

// Given a log ending in a turn.completed event, when parsed, Result.Turns is recorded as 1.
// This maps Codex's terminal turn event into a normalized turn metric for triage reports.
func TestParseResultCountsCompletedTerminalTurnAsRuntimeLocalTurn(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureSuccess)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if res.Turns != 1 {
		t.Fatalf("Result.Turns = %d, want 1 runtime-local turn from turn.completed event", res.Turns)
	}
}

// Given a log with a turn.completed event, when parsed, Result.StopReason is set to "turn.completed".
// This normalizes the successful termination reason across different runtime engines.
func TestParseResultCompletedEventStopReason(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureSuccess)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	const wantStopReason = "turn.completed"
	if res.StopReason != wantStopReason {
		t.Fatalf("Result.StopReason = %q, want normalized terminal reason %q", res.StopReason, wantStopReason)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("Result failed Validate(): %v", err)
	}
}

// Given a log with a turn.completed event, when parsed, Result.IsError is false.
// This ensures successful runs validate cleanly and are not flagged as runtime failures.
func TestParseResultCompletedEventNonErrorState(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureSuccess)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if res.IsError {
		t.Fatalf("Result.IsError = true, want false for successful run")
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("Result failed Validate(): %v", err)
	}
}

// Given captured token usage for a known priced model, when ParseResult runs, it calculates estimated cost and sets CostBasisEstimated.
// This provides honest, transparent pricing based on local rate cards when the runtime provides token counts.
func TestParseResultEstimatesKnownModelCostFromCapturedUsage(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureSuccess)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if res.CostBasis != runtime.CostBasisEstimated {
		t.Fatalf("Result.CostBasis = %q, want %q for known priced model %q",
			res.CostBasis, runtime.CostBasisEstimated, knownPricedModel)
	}
	if math.Abs(res.Cost-knownPricedExpectedCost) > costAssertionTolerance {
		t.Fatalf("Result.Cost = %v, want exactly %v calculated from (16426-6912)*4/1e6 + 6912*0.4/1e6 + 7*20/1e6",
			res.Cost, knownPricedExpectedCost)
	}
}

// Given a log for an unpriced or unknown model, when parsed, Result.Cost is 0.0 with CostBasisUnavailable.
// This adheres to cost-basis honesty by never fabricating price estimates for unknown models.
func TestParseResultReportsUnavailableCostForUnknownModel(t *testing.T) {
	rt := mustGetCodex(t)
	const unknownModel = "totally-unknown-model"
	log := adapterLog(t, unknownModel, fixtureSuccess)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if res.CostBasis != runtime.CostBasisUnavailable {
		t.Fatalf("Result.CostBasis = %q, want %q for unpriced unknown model %q",
			res.CostBasis, runtime.CostBasisUnavailable, unknownModel)
	}
	if res.Cost != 0.0 {
		t.Fatalf("Result.Cost = %v, want exactly 0.0 when cost basis is unavailable", res.Cost)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("Result failed Validate(): %v", err)
	}
}

// Given a log ending in a turn.failed event, when parsed, Result.Turns is recorded as 1.
// This ensures failed attempts still accurately count towards execution metrics.
func TestParseResultCountsFailedTerminalTurnAsRuntimeLocalTurn(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureFailed)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if res.Turns != 1 {
		t.Fatalf("Result.Turns = %d, want 1 runtime-local turn from turn.failed event", res.Turns)
	}
}

// Given a log with a failed turn event, when parsed, Result.StopReason is set to "turn.failed".
// This captures the explicit failure reason in the normalized Result structure.
func TestParseResultFailedEventStopReason(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureFailed)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	const wantStopReason = "turn.failed"
	if res.StopReason != wantStopReason {
		t.Fatalf("Result.StopReason = %q, want normalized failed reason %q", res.StopReason, wantStopReason)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("Result failed Validate(): %v", err)
	}
}

// Given a log with a turn.failed event, when parsed, Result.IsError is true.
// This flags the execution as an error so triage logic can route it to failure handling.
func TestParseResultFailedEventErrorState(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureFailed)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if !res.IsError {
		t.Fatalf("Result.IsError = false, want true for failed turn run")
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("Result failed Validate(): %v", err)
	}
}

// Given a failed run log that lacks usage events, when parsed, Result.CostBasis is CostBasisUnavailable.
// This avoids guessing token consumption when the subprocess terminates before emitting usage metrics.
func TestParseResultReportsUnavailableCostForFailedRunWithoutUsage(t *testing.T) {
	rt := mustGetCodex(t)
	log := adapterLog(t, knownPricedModel, fixtureFailed)

	res, err := rt.ParseResult(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseResult failed unexpectedly: %v", err)
	}

	if res.CostBasis != runtime.CostBasisUnavailable {
		t.Fatalf("Result.CostBasis = %q, want %q when no usage events were emitted",
			res.CostBasis, runtime.CostBasisUnavailable)
	}
	if res.Cost != 0.0 {
		t.Fatalf("Result.Cost = %v, want 0.0 when no usage was captured", res.Cost)
	}
}

// Given a truncated log lacking any terminal turn event, when ParseResult runs, it returns a loud parse error.
// This implements hard-fail philosophy by rejecting incomplete or corrupted execution streams.
func TestParseResultMissingTerminalTurnIsALoudParseError(t *testing.T) {
	rt := mustGetCodex(t)
	// Truncate fixture so it has start and message items, but neither turn.completed nor turn.failed.
	lines := strings.Split(strings.TrimSpace(loadFixture(t, fixtureSuccess)), "\n")
	if len(lines) < 3 {
		t.Fatalf("unexpected short success fixture: %d lines", len(lines))
	}
	truncatedChildLog := strings.Join(lines[:len(lines)-1], "\n")
	incompleteLog := invocationEnvelopeJSON(knownPricedModel) + "\n" + truncatedChildLog + "\n"

	res, err := rt.ParseResult(strings.NewReader(incompleteLog))
	if err == nil {
		t.Fatalf("ParseResult succeeded on log missing terminal turn, expected loud error (got res=%+v)", res)
	}
}

// Given a nil log reader, when ParseResult is invoked, it returns a non-nil error immediately.
// This defends against nil-pointer panics when parsing uninitialized log streams.
func TestParseResultNilReaderIsALoudError(t *testing.T) {
	rt := mustGetCodex(t)
	if _, err := rt.ParseResult(nil); err == nil {
		t.Fatal("ParseResult(nil) returned nil error, want loud error on nil reader")
	}
}

// Given a log containing extraneous non-JSON header or footer lines, when parsed, ParseResult ignores the noise and extracts valid JSONL events.
// This makes parsing resilient against unexpected CLI stdout or stderr banner output.
func TestParseResultIgnoresNonJSONLogNoise(t *testing.T) {
	rt := mustGetCodex(t)
	childFixture := loadFixture(t, fixtureSuccess)

	noisyLog := invocationEnvelopeJSON(knownPricedModel) + "\n" +
		"[INFO] Codex CLI 0.151.0 starting session\n" +
		childFixture +
		"Warning: unexpected non-json log footer line\n"

	res, err := rt.ParseResult(strings.NewReader(noisyLog))
	if err != nil {
		t.Fatalf("ParseResult failed with non-JSON log noise: %v", err)
	}
	if res.Summary != "SCHEMA_OK" {
		t.Fatalf("Result.Summary = %q, want %q despite non-JSON noise lines", res.Summary, "SCHEMA_OK")
	}
	if res.Turns != 1 {
		t.Fatalf("Result.Turns = %d, want 1 despite non-JSON noise lines", res.Turns)
	}
}
