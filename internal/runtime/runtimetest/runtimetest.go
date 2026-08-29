// Package runtimetest provides a shared conformance harness that any
// AgentRuntime adapter can opt into with a few lines, so every runtime clears
// the same contract the orchestrator depends on: registry round-trip, a
// parseable Result with a non-empty CostBasis, a correct outcome
// classification, and (when declared) capabilities that match real behavior.
//
// The adapter's own test file then only needs to cover its stream-schema
// quirks. See docs/adding-a-runtime.md.
package runtimetest

import (
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

// Case describes one adapter's conformance expectations.
type Case struct {
	// Adapter is the adapter instance under test. It is used for ParseResult /
	// ClassifyOutcome; registry lookup uses Name.
	Adapter runtime.AgentRuntime
	// Name is the registry name the adapter self-registers under. The harness
	// asserts runtime.Get(Name) resolves to an adapter reporting this name.
	Name string
	// GoldenLog is a captured real (or realistic) run log in the runtime's own
	// stream format. ParseResult must parse it into a valid Result.
	GoldenLog string
	// WantCostBasis is the CostBasis ParseResult must report for GoldenLog. If
	// the adapter implements CapabilityReporter, its declared CostBasis must
	// match this too.
	WantCostBasis runtime.CostBasis
	// WantOutcome is the Outcome ClassifyOutcome must return for the parsed
	// GoldenLog result at exit code 0.
	WantOutcome runtime.Outcome
}

// Run executes the shared conformance assertions for c. Call it from a test in
// the adapter's package:
//
//	func TestConformance(t *testing.T) {
//	    runtimetest.Run(t, runtimetest.Case{ ... })
//	}
func Run(t *testing.T, c Case) {
	t.Helper()

	if c.Adapter == nil {
		t.Fatal("runtimetest: Case.Adapter is nil")
	}

	t.Run("registry round-trip", func(t *testing.T) {
		got, err := runtime.Get(c.Name)
		if err != nil {
			t.Fatalf("runtime.Get(%q) failed: %v (adapter not blank-imported or wrong Name?)", c.Name, err)
		}
		if got.Name() != c.Name {
			t.Fatalf("registered adapter reports Name() = %q, want %q", got.Name(), c.Name)
		}
		if c.Adapter.Name() != c.Name {
			t.Fatalf("Case.Adapter.Name() = %q, want %q", c.Adapter.Name(), c.Name)
		}
	})

	t.Run("ParseResult yields a valid Result", func(t *testing.T) {
		res, err := c.Adapter.ParseResult(strings.NewReader(c.GoldenLog))
		if err != nil {
			t.Fatalf("ParseResult(GoldenLog) failed: %v", err)
		}
		if res == nil {
			t.Fatal("ParseResult returned a nil Result with no error")
		}
		if err := res.Validate(); err != nil {
			t.Fatalf("Result failed Validate (cost-basis-honesty): %v", err)
		}
		if res.CostBasis != c.WantCostBasis {
			t.Fatalf("CostBasis = %q, want %q", res.CostBasis, c.WantCostBasis)
		}
	})

	t.Run("ParseResult rejects a nil reader", func(t *testing.T) {
		if _, err := c.Adapter.ParseResult(nil); err == nil {
			t.Fatal("ParseResult(nil) returned no error; a nil reader must be a loud error")
		}
	})

	t.Run("ClassifyOutcome maps the golden result", func(t *testing.T) {
		res, err := c.Adapter.ParseResult(strings.NewReader(c.GoldenLog))
		if err != nil {
			t.Fatalf("ParseResult(GoldenLog) failed: %v", err)
		}
		if got := c.Adapter.ClassifyOutcome(res, 0); got != c.WantOutcome {
			t.Fatalf("ClassifyOutcome(golden, exit 0) = %q, want %q", got, c.WantOutcome)
		}
		if got := c.Adapter.ClassifyOutcome(nil, 0); got != runtime.OutcomeError {
			t.Fatalf("ClassifyOutcome(nil, 0) = %q, want %q", got, runtime.OutcomeError)
		}
	})

	t.Run("declared capabilities match behavior", func(t *testing.T) {
		caps, ok := runtime.CapabilitiesOf(c.Adapter)
		if !ok {
			t.Skip("adapter does not implement CapabilityReporter (optional)")
		}
		if caps.CostBasis != c.WantCostBasis {
			t.Fatalf("declared Capabilities.CostBasis = %q, but GoldenLog parses to %q; the capability table must not drift from behavior",
				caps.CostBasis, c.WantCostBasis)
		}
	})
}
