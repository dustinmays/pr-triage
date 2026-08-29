package runtime

// ModelForm describes the shape a runtime requires for its model identifier.
// It exists so a caller (or the doctor command) can validate a configured
// model against the runtime that will receive it, and so the "provider/model
// vs plain name" divergence between OpenCode and Codex is declared rather than
// rediscovered.
type ModelForm string

const (
	// ModelFormPlain means the runtime takes a bare model name (e.g. Codex,
	// Claude Code: "claude-3-7-sonnet"). No slash is required.
	ModelFormPlain ModelForm = "plain"
	// ModelFormProviderSlashModel means the runtime requires "provider/model"
	// form (e.g. OpenCode: "openrouter/z-ai/glm-5.3-flash"). A slash-less model
	// is rejected before launch.
	ModelFormProviderSlashModel ModelForm = "provider/model"
	// ModelFormUnknown means the model form is unspecified.
	ModelFormUnknown ModelForm = ""
)

// Capabilities is a static, self-reported description of what a runtime adapter
// actually does. It turns the prose in docs/runtime-capability-table.md into a
// value the code can read: config show can explain a route, the doctor command
// can validate a model, and a conformance test can assert the declared
// CostBasis matches what ParseResult really produces.
//
// The governing rule (see docs/runtime-capability-table.md): never advertise a
// limit a runtime does not enforce. If EnforcesTurns is false, the adapter must
// not pretend Limits.MaxTurns is honored.
type Capabilities struct {
	// CostBasis is the basis this runtime reports cost on: exact (terminal cost
	// field), estimated (priced from a table), or unavailable. It must match
	// the CostBasis the adapter's ParseResult populates on a successful run.
	CostBasis CostBasis
	// EnforcesTimeout reports whether the adapter enforces Limits.Timeout
	// itself (all exec-based adapters do, via ExecRun's context deadline).
	EnforcesTimeout bool
	// EnforcesTurns reports whether the adapter enforces Limits.MaxTurns. Claude
	// Code does (via --max-turns); OpenCode and Codex do not.
	EnforcesTurns bool
	// EnforcesBudget reports whether the adapter enforces a spend cap itself.
	EnforcesBudget bool
	// ModelForm is the form this runtime requires for its model identifier.
	ModelForm ModelForm
	// AuthModel is a short human-readable description of how the runtime
	// authenticates (e.g. "codex login / OPENAI_API_KEY, frozen at daemon
	// start"). Surfaced by the doctor command to explain auth failures.
	AuthModel string
}

// CapabilityReporter is the optional interface an AgentRuntime may implement to
// declare its Capabilities. It is deliberately separate from AgentRuntime so an
// adapter that has not adopted it (including one authored before the kit) still
// compiles and runs — callers treat a non-implementer as "capabilities
// unknown" via CapabilitiesOf.
type CapabilityReporter interface {
	Capabilities() Capabilities
}

// CapabilitiesOf returns the Capabilities declared by r, or ok=false if r does
// not implement CapabilityReporter. Callers must handle ok=false as "unknown",
// never as "no capabilities".
func CapabilitiesOf(r AgentRuntime) (caps Capabilities, ok bool) {
	if cr, isReporter := r.(CapabilityReporter); isReporter {
		return cr.Capabilities(), true
	}
	return Capabilities{}, false
}
