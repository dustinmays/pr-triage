// Package runtime defines the AgentRuntime abstraction shared by all
// coding-agent adapters (claude-code, codex, opencode, ...), along with the
// registry adapters self-register into and the result/invocation types used
// to describe a run.
package runtime

import (
	"errors"
	"time"
)

// CostBasis records how a Result's Cost (and, by convention, Turns) value
// was derived. It exists so a genuine cost of 0 can never be confused with
// "not measured": every Result MUST carry a non-empty CostBasis.
//
// Adapters must populate CostBasis from the runtime's own structured
// output (e.g. a terminal JSON event), never by inferring cost from log
// scraping.
type CostBasis string

const (
	// CostBasisExact means the runtime reported an authoritative cost value
	// (e.g. Claude Code's total_cost_usd terminal event).
	CostBasisExact CostBasis = "exact"
	// CostBasisEstimated means the cost was computed from a hardcoded
	// per-model price table because the runtime has no terminal cost field
	// (e.g. Codex).
	CostBasisEstimated CostBasis = "estimated"
	// CostBasisUnavailable means no cost could be determined at all.
	CostBasisUnavailable CostBasis = "unavailable"
	// CostBasisRuntimeDefined means the cost concept doesn't map cleanly to
	// USD for this runtime and the adapter defines its own semantics.
	CostBasisRuntimeDefined CostBasis = "runtime-defined"
)

// Outcome is a normalized classification of how a run ended, derived from a
// Result and the process exit code.
type Outcome string

const (
	// OutcomeSuccess means the run completed and produced a usable result.
	OutcomeSuccess Outcome = "success"
	// OutcomeFailed means the run completed but reported failure (e.g.
	// Result.IsError or a non-zero exit code).
	OutcomeFailed Outcome = "failed"
	// OutcomeTimeout means the run was terminated for exceeding its
	// configured Limits.Timeout.
	OutcomeTimeout Outcome = "timeout"
	// OutcomeError means the run could not be classified normally, e.g. the
	// log could not be parsed at all.
	OutcomeError Outcome = "error"
)

// ErrEmptyCostBasis is returned by Result.Validate when CostBasis is empty.
var ErrEmptyCostBasis = errors.New("runtime: result has empty CostBasis")

// Result is the normalized outcome of a single agent run, as parsed from a
// runtime's log output by that runtime's adapter.
type Result struct {
	// Cost is the run's cost in USD. Its meaning depends on CostBasis: a
	// value of 0 with CostBasis == CostBasisExact means the run genuinely
	// cost nothing; a value of 0 with CostBasis == CostBasisUnavailable
	// means cost was never measured.
	Cost float64
	// CostBasis records how Cost was derived. Required: must be non-empty.
	CostBasis CostBasis
	// Turns is the number of turns the run took, in that runtime's own
	// units. Turn counts are not comparable across runtimes; only compare
	// a run's Turns to that same run's own Limits.MaxTurns.
	Turns int
	// StopReason is the runtime-reported reason the run stopped, normalized
	// (e.g. unquoted) by the adapter.
	StopReason string
	// IsError indicates the runtime itself reported the run as an error.
	IsError bool
}

// Validate reports whether the Result is well-formed. It currently enforces
// that CostBasis is set, per the cost-basis-honesty design principle: a
// genuine cost of 0 must never be confused with "not measured".
func (r *Result) Validate() error {
	if r.CostBasis == "" {
		return ErrEmptyCostBasis
	}
	return nil
}

// Limits describes the constraints a caller wants applied to a run. Not
// every runtime enforces every limit itself; see the runtime capability
// table in docs/runtime-capability-table.md. An adapter must either enforce
// a limit itself (e.g. by watching the stream) or make clear that it does
// not, rather than silently ignoring it.
type Limits struct {
	// Timeout is the maximum wall-clock duration the run may take.
	Timeout time.Duration
	// MaxTurns is the maximum number of turns the run may take.
	MaxTurns int
}

// Invocation describes a single agent run request.
type Invocation struct {
	// AgentName identifies the agent/template being run (for logging and
	// bookkeeping, not the runtime binary itself).
	AgentName string
	// Model is the model identifier to pass to the runtime.
	Model string
	// Prompt is the prompt/instructions to give the agent.
	Prompt string
	// Workdir is the working directory the run should execute in (e.g. a
	// git worktree checkout).
	Workdir string
	// Limits are the constraints to apply to the run.
	Limits Limits
	// PIDCallback is called immediately upon process launch with the child PID.
	PIDCallback func(pid int)
}
