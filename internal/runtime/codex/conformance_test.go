package codex

import (
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
	"github.com/dustinmays/pr-triage/internal/runtime/runtimetest"
)

// Given the Codex adapter and a golden execution log, when run through the shared runtimetest conformance suite, all contract invariants pass.
// This enforces behavioral parity and contract consistency across all supported runtime adapters.
func TestConformance(t *testing.T) {
	adapter := mustGetCodex(t)
	golden := adapterLog(t, knownPricedModel, fixtureSuccess)

	runtimetest.Run(t, runtimetest.Case{
		Adapter:       adapter,
		Name:          runtime.NameCodex,
		GoldenLog:     golden,
		WantCostBasis: runtime.CostBasisEstimated,
		WantOutcome:   runtime.OutcomeSuccess,
	})
}
