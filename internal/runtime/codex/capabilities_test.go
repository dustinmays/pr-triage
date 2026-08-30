package codex

import (
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

func mustGetCodexCapabilities(t *testing.T) runtime.Capabilities {
	t.Helper()
	rt := mustGetCodex(t)
	caps, ok := runtime.CapabilitiesOf(rt)
	if !ok {
		t.Fatal("Codex adapter does not implement CapabilityReporter")
	}
	return caps
}

// Given the Codex capability profile, inspecting CostBasis returns CostBasisEstimated.
// This guarantees callers know dollar costs are locally estimated from usage events rather than exact API invoices.
func TestCapabilitiesDeclaresEstimatedCostBasis(t *testing.T) {
	caps := mustGetCodexCapabilities(t)
	if caps.CostBasis != runtime.CostBasisEstimated {
		t.Fatalf("Capabilities.CostBasis = %q, want %q", caps.CostBasis, runtime.CostBasisEstimated)
	}
}

// Given the Codex capability profile, EnforcesTimeout returns true.
// This signals to the orchestrator that triage execution is bounded by a wall-clock timeout.
func TestCapabilitiesEnforcesTimeout(t *testing.T) {
	caps := mustGetCodexCapabilities(t)
	if !caps.EnforcesTimeout {
		t.Fatalf("Capabilities.EnforcesTimeout = false, want true")
	}
}

// Given the Codex capability profile, EnforcesTurns returns false.
// This documents the v1 contract that Codex does not enforce a maximum turn limit flag.
func TestCapabilitiesDoesNotEnforceTurns(t *testing.T) {
	caps := mustGetCodexCapabilities(t)
	if caps.EnforcesTurns {
		t.Fatalf("Capabilities.EnforcesTurns = true, want false (Codex does not enforce --max-turns)")
	}
}

// Given the Codex capability profile, EnforcesBudget returns false.
// This documents that Codex does not support runtime spend caps, preventing callers from expecting budget enforcement.
func TestCapabilitiesDoesNotEnforceBudget(t *testing.T) {
	caps := mustGetCodexCapabilities(t)
	if caps.EnforcesBudget {
		t.Fatalf("Capabilities.EnforcesBudget = true, want false (Codex does not enforce budget spend caps)")
	}
}

// Given the Codex capability profile, ModelForm returns ModelFormPlain.
// This ensures CLI validation accepts standard model names without requiring a provider slash prefix.
func TestCapabilitiesDeclaresPlainModelForm(t *testing.T) {
	caps := mustGetCodexCapabilities(t)
	if caps.ModelForm != runtime.ModelFormPlain {
		t.Fatalf("Capabilities.ModelForm = %q, want %q", caps.ModelForm, runtime.ModelFormPlain)
	}
}

// Given the Codex capability profile, AuthModel describes saved login and CODEX_API_KEY.
// This gives operators clear guidance on the supported authentication mechanisms when checking runtime readiness.
func TestCapabilitiesAuthModelNamesSavedLoginAndCodexAPIKey(t *testing.T) {
	caps := mustGetCodexCapabilities(t)
	authLower := strings.ToLower(caps.AuthModel)
	if !strings.Contains(authLower, "codex login") || !strings.Contains(authLower, "codex_api_key") {
		t.Fatalf("Capabilities.AuthModel %q must describe saved codex login or CODEX_API_KEY auth", caps.AuthModel)
	}
}
