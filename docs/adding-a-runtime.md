---
title: Adding a runtime
tags: [adapters, runtime, howto, onboarding]
related: [[runtime-capability-table]], [[cost-basis-honesty]], [[result-shape-normalization]], [[hard-fail-philosophy]]
source: adr/0009-runtime-adapter-kit.md
---

How to add — and *vet* — a new coding-agent runtime. Companion to
[[0009-runtime-adapter-kit]]. Read that ADR for *why* the seams are shaped this
way; this doc is the *how*.

The **adapter kit** ([[0009-runtime-adapter-kit]]) is shipped: a new runtime
leans on a shared subprocess runner, declares its capabilities, opts into a
conformance harness, and is vetted with a doctor command — so the per-runtime
code you actually write is small. The kit is additive: the older
copy-the-boilerplate pattern still compiles, but author new runtimes on the kit.

Two reading paths:

- **Author a runtime on the kit** (recommended, and what Codex should use) —
  [With the adapter kit](#with-the-adapter-kit).
- **The full touch-point list** regardless of pattern —
  [The checklist](#the-checklist).

---

## What a runtime actually has to do

The orchestrator drives every runtime through exactly four calls
(`internal/orchestrator/orchestrator.go:658-789`):

```
adapter, _ := runtime.Get(routing.Runtime)   // registry lookup by name
exitCode, err := adapter.Run(ctx, inv, &logBuf)
res, _ := adapter.ParseResult(bytes.NewReader(logBuf.Bytes()))
outcome := adapter.ClassifyOutcome(res, exitCode)
```

Everything a runtime must implement exists to satisfy those four lines. The
genuinely runtime-specific parts are small:

| Part | Runtime-specific? | Notes |
|------|-------------------|-------|
| Process lifecycle (timeout, SIGTERM, PID cb, exit unwrap) | **No** | Identical across adapters — the kit's `ExecRun` owns it. |
| `BuildArgs` (binary + argv from `Invocation`) | **Yes** | The main thing you write. |
| Pre-launch validation (e.g. model form) | Sometimes | OpenCode rejects slash-less models; Codex must NOT. |
| Stream schema + `ParseResult` | **Yes** | Each CLI emits its own JSON shape. |
| `ClassifyOutcome` quirks | Mostly no | Most map exit≠0 → failed; some read a stop reason. |
| Cost basis | **Yes** | Exact (terminal field) vs estimated (price table) vs unavailable. |

Keep the runtime-specific surface small; lean on shared machinery for the rest.

---

## The checklist

Follow the OpenCode adapter (`internal/runtime/opencode/opencode.go`) as the
closest structural template. Six touch points:

1. **Reserve the name** in `internal/runtime/registry.go`:
   ```go
   const NameMyRuntime = "my-runtime"
   ```
   (Codex is already reserved as `NameCodex`.)

2. **Write the adapter** `internal/runtime/myruntime/myruntime.go` implementing
   `Name / Run / ParseResult / ClassifyOutcome`, self-registered via
   `func init() { runtime.Register(New()) }`.

3. **Blank-import it** in `cmd/pr-triage/main.go` so `init()` fires:
   ```go
   _ "github.com/dustinmays/pr-triage/internal/runtime/myruntime"
   ```
   Without this the registry is empty for that name and every run hard-fails to
   escalate ([[hard-fail-philosophy]]).

4. **Add the agent renderer** in `internal/agentsync` if the runtime consumes a
   named agent def (like the `renderCodex` stub — see [[0008-portable-agent-definitions]]).
   Skip if the runtime takes the prompt inline only.

5. **Verify `init` flags** — `--runtime`/`--model` are already generic
   (`internal/cli/init.go`). You do **not** wire new flags; you confirm
   `pr-triage init --runtime my-runtime --model <m>` writes
   `routing.routine.{runtime,model}` and `config show` reflects it.

6. **Write tests.** Opt into the shared conformance harness
   (`internal/runtime/runtimetest`) for the contract invariants — see
   [One-line conformance test](#one-line-conformance-test) — then add your own
   tests mirroring `opencode_test.go` for `BuildArgs`, stream-schema quirks, and
   `Run` against a fake binary.

### The invariants you must not miss

These are the ones the orchestrator silently depends on — get them wrong and a PR
escalates or misreports rather than erroring loudly:

- **Every `Result` carries a non-empty `CostBasis`** ([[cost-basis-honesty]]).
  A genuine `Cost: 0` with `CostBasisExact` ≠ "not measured" with
  `CostBasisUnavailable`. `Result.Validate()` enforces non-empty; call it in a test.
- **`CostBasis` comes from structured output, never log-scraping.** If the CLI has
  no terminal cost field, use `CostBasisEstimated` from a price table — never
  regex a dollar amount out of prose.
- **Set `Summary`** to the agent's final free-text. The orchestrator posts it to
  the PR deterministically (`orchestrator.go:761`); an empty Summary means a blank
  review comment.
- **`Run` returns `(exitCode, nil)` when the process ran and exited non-zero**, and
  `(-1, err)` only when it could *not be executed at all* (binary missing, model
  rejected pre-launch). The orchestrator escalates on the latter but not the
  former — conflating them turns a normal agent failure into a config-fault page.
- **Enforce timeout via context + SIGTERM; do not claim limits you don't enforce**
  ([[runtime-capability-table]]). If the CLI has no `--max-turns`, say so in a
  comment; don't pretend `Limits.MaxTurns` is honored.
- **Validate model form at the right strictness.** OpenCode *requires*
  `provider/model`; Codex forbids that assumption. Copy the validation only if
  your CLI actually needs it.

---

## With the adapter kit

The kit shrinks steps 2 and 6 dramatically. Here's the reduced path; the
claude-code and opencode adapters are worked examples of every piece below.

### `Run` becomes argv + a hook

Instead of ~50 lines of `exec.CommandContext` plumbing, supply a `Spec` and call
the shared runner:

```go
// runtime.ExecRun (in internal/runtime) — owns timeout, SIGTERM cancel,
// PIDCallback, exit-code unwrap, and log fan-out for every adapter.
func (a *Adapter) Run(ctx context.Context, inv runtime.Invocation, logFile io.Writer) (int, error) {
    return runtime.ExecRun(ctx, inv, logFile, runtime.ExecSpec{
        Binary:  a.binaryOr("my-runtime"),
        Args:    a.BuildArgs(inv),
        PreCheck: func(inv runtime.Invocation) error {
            // return a non-nil error to fail BEFORE launch (→ exitCode -1, err).
            // e.g. model-form validation. Return nil if there's nothing to check.
            return nil
        },
    })
}
```

`ExecRun` reference shape (what the kit provides — you consume it, you don't write
it per adapter):

```go
type ExecSpec struct {
    Binary   string
    Args     []string
    PreCheck func(Invocation) error // optional; runs before launch
}

// ExecRun applies Limits.Timeout as a context deadline, cancels with SIGTERM,
// fires inv.PIDCallback on launch, streams stdout+stderr to logFile, and unwraps
// *exec.ExitError into (exitCode, nil). A launch failure or PreCheck error
// returns (-1, err) — the signal the orchestrator escalates on.
func ExecRun(ctx context.Context, inv Invocation, logFile io.Writer, spec ExecSpec) (int, error)
```

You still write `BuildArgs` and `ParseResult` — those are the parts that are
actually different.

### Declare capabilities instead of documenting them

Implement the optional `CapabilityReporter` so the facts in
[[runtime-capability-table]] live in code:

```go
func (a *Adapter) Capabilities() runtime.Capabilities {
    return runtime.Capabilities{
        CostBasis:       runtime.CostBasisEstimated, // this runtime has no terminal cost field
        EnforcesTimeout: true,
        EnforcesTurns:   false, // no --max-turns; do not advertise it
        EnforcesBudget:  false,
        ModelForm:       runtime.ModelFormPlain, // or ModelFormProviderSlashModel
        AuthModel:       "codex login / OPENAI_API_KEY, frozen at daemon start",
    }
}
```

Callers that need capabilities type-assert for `CapabilityReporter`; an adapter
that doesn't implement it reports "unknown", so this never breaks an older
adapter. A conformance test asserts the declared `CostBasis` matches what
`ParseResult` actually produces — so the table can't drift from behavior.

### One-line conformance test

Opt into the shared suite; it covers the invariants above so your own test file
only needs the stream-schema quirks unique to your CLI:

```go
package myruntime

import (
    "testing"
    "github.com/dustinmays/pr-triage/internal/runtime"
    "github.com/dustinmays/pr-triage/internal/runtime/runtimetest"
)

func TestConformance(t *testing.T) {
    runtimetest.Run(t, runtimetest.Case{
        Adapter:       New(),
        Name:          RuntimeName,
        GoldenLog:     myRuntimeStreamJSON, // a captured real run, in this CLI's stream format
        WantCostBasis: runtime.CostBasisEstimated,
        WantOutcome:   runtime.OutcomeSuccess,
    })
}
```

`runtimetest.Run` asserts, purely from `GoldenLog` (no subprocess): registry
round-trip (so a missing blank-import is caught), `ParseResult(GoldenLog)` yields
a `Result` that passes `Validate()`, `CostBasis` is non-empty and matches
`WantCostBasis`, `ParseResult(nil)` errors, `ClassifyOutcome` maps the golden
result to `WantOutcome` and maps a nil result to `OutcomeError`, and — if the
adapter implements `CapabilityReporter` — the declared cost basis matches the
parsed one, so the capability table can't drift from behavior. Your own test file
still covers process launch against a fake binary (see `opencode_test.go`'s
`TestRun_Execution`) and any stream-schema edge cases.

### Vet it in five seconds, not in production

Before wiring the runtime into a real route, run the doctor:

```
pr-triage runtime list                          # registered runtimes + declared capabilities
pr-triage runtime check my-runtime --model <m>  # end-to-end probe
```

`runtime check` runs a canned one-line prompt through the *real* adapter in a
temp workdir and reports a ✓/✗ checklist, in order:

1. **registered** — the name resolves in the registry (catches a missing
   blank-import in `cmd/pr-triage/main.go`);
2. **model form** — the `--model` matches the runtime's declared `ModelForm`
   (a slash-less model for a `provider/model` runtime fails here, *before*
   spending a subprocess);
3. **process launched** — the binary was found and ran (a launch failure prints
   the error and the log tail);
4. **parseable result** — `ParseResult` succeeded and `Validate()` passed, and it
   prints the cost basis so you learn *before* production that a route reports
   estimated cost;
5. **classified success** — anything else (failed/timeout) prints the last ~20
   log lines, which is where an auth error shows up.

`check` exits non-zero on any failure, so it's usable in a smoke script. Run it in
the **same environment the daemon runs in** — the daemon's env is frozen at start,
so `codex login` state / `OPENAI_API_KEY` / `opencode auth` credentials must be
present *there*, not just in your interactive shell. This is the "works in my
shell ≠ works in the daemon" gap, made a five-second check.

---

## Anti-patterns (learned from the shipped adapters)

- **Don't scrape cost from log text.** ([[cost-basis-honesty]]) If there's no
  structured cost field, it's `CostBasisEstimated` from a price table or
  `CostBasisUnavailable` — never a regex over prose.
- **Don't copy OpenCode's slash validation blindly.** It's correct for OpenCode and
  wrong for Codex. Validation strictness is per-runtime.
- **Don't silently ignore a `Limits` field.** Enforce it or document that you
  don't. Silent omission is how a "budget cap" becomes a surprise bill.
- **Don't let the agent's comment delivery depend on the agent.** The orchestrator
  posts `Result.Summary` itself; your job is to *capture* the final message into
  Summary, reliably, even on models that never call `gh pr comment`.
- **Don't forget the blank import.** A perfectly correct adapter that isn't
  imported in `cmd/pr-triage/main.go` is invisible to the registry and every run
  routed to it hard-fails to escalate.
