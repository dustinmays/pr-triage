package claudecode

import (
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
	"github.com/dustinmays/pr-triage/internal/runtime/runtimetest"
)

// goldenLog is a minimal Claude Code stream-json run ending in a successful
// terminal result event.
const goldenLog = `{"type":"system","subtype":"init"}
{"type":"result","subtype":"success","total_cost_usd":0.025,"num_turns":2,"stop_reason":"end_turn","is_error":false,"result":"looks good"}
`

func TestConformance(t *testing.T) {
	runtimetest.Run(t, runtimetest.Case{
		Adapter:       New(),
		Name:          RuntimeName,
		GoldenLog:     goldenLog,
		WantCostBasis: runtime.CostBasisExact,
		WantOutcome:   runtime.OutcomeSuccess,
	})
}
