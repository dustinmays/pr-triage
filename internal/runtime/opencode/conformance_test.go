package opencode

import (
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
	"github.com/dustinmays/pr-triage/internal/runtime/runtimetest"
)

func TestConformance(t *testing.T) {
	runtimetest.Run(t, runtimetest.Case{
		Adapter:       New(),
		Name:          RuntimeName,
		GoldenLog:     multiStepFixture, // reuse the captured 1.18.21 NDJSON stream
		WantCostBasis: runtime.CostBasisExact,
		WantOutcome:   runtime.OutcomeSuccess,
	})
}
