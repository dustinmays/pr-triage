# 0009 — Runtime adapter kit: shared exec, declared capabilities, conformance harness, doctor

**Status:** Expected (implemented in this PR; pending sign-off by Dustin. Additive/opt-in — the two shipped adapters were migrated onto the kit with their existing tests unchanged and green.)

## Decision

Keep the `AgentRuntime` interface ([[0003-exec-subprocess-adapters]]) as the seam, but wrap it in a small **adapter kit** of four additive pieces so adding *and vetting* a runtime is an afternoon, not a spelunk:

1. **A shared subprocess runner** (`runtime.ExecRun`) that owns the identical `Run()` machinery every adapter copies today — timeout context, `SIGTERM` cancel, `PIDCallback`, exit-code unwrap, log fan-out. A new adapter supplies only its binary, argv, and an optional pre-launch check; it does not re-implement process lifecycle.

2. **A declared capability descriptor** (`runtime.Capabilities`, exposed via an optional `CapabilityReporter` interface). The per-runtime facts currently living as prose in [[runtime-capability-table]] — cost basis, whether the adapter enforces turns/budget, model-form requirement, auth model — become a struct the adapter returns. `config show`, `init`, and the doctor command read it; a test asserts it matches reality.

3. **A conformance harness** (`internal/runtime/runtimetest`) — one table-driven suite any adapter opts into with a few lines. It exercises the contract the orchestrator depends on: registry round-trip, `ParseResult` on a golden log, non-empty `CostBasis` ([[cost-basis-honesty]]), `ClassifyOutcome` mapping ([[result-shape-normalization]]), and model-form validation. The adapter's own tests then only cover its stream-schema quirks.

4. **A doctor command** (`pr-triage runtime check <name>`) that runs a canned trivial prompt through the *real* adapter in a temp workdir and reports: binary found, auth OK, produced a parseable Result, cost basis. This closes the "worked in my shell ≠ worked in the daemon" gap the Codex charter keeps flagging (auth precondition, sandbox reachability).

The interface signatures the orchestrator calls (`Run`/`ParseResult`/`ClassifyOutcome`) do **not** change. The kit is a set of helpers an adapter *may* lean on, plus one *optional* interface — so the in-flight Codex adapter and the two shipped adapters compile and pass unchanged, and migrate on their own schedule.

## Why

- **The real per-runtime work is small; the boilerplate is large.** `claudecode.Run` and `opencode.Run` are ~50 lines and differ only in binary name and one pre-launch validation. Everything else — timeout, cancel signal, PID callback, `exec.ExitError` unwrap, log wiring — is byte-for-byte duplicated and will be copied a third time for Codex. Extracting `ExecRun` deletes that copy and removes a whole class of "the new adapter forgot to send SIGTERM / forgot the PID callback" bugs.
- **A capability table nobody's code reads will drift.** [[runtime-capability-table]] is correct today because it's young. Once it's a struct the adapter returns and a conformance test checks, "never advertise a limit you don't enforce" stops being a rule of thumb and becomes a failing test. It also lets `config show` tell a human *why* a route behaves as it does (estimated cost, no turn cap) instead of them reading a doc.
- **Vetting is where runtime integration actually fails, and it has no tooling.** Every hard part in the Codex charter is a *vetting* gap, not an *interface* gap: does auth carry into the daemon's frozen env, is the pre-scan JSON readable inside the sandbox, does the model form validate. A `runtime check` doctor turns each of those from "find out in production when a PR silently escalates" into a five-second local command. This is the single highest-leverage piece for "make it easy to vet quickly."
- **A shared conformance suite makes a new adapter provably correct before it's wired in.** Today an adapter is "done" when its author's hand-written tests pass — and those tests can quietly omit the CostBasis invariant or the timeout classification. One opt-in harness means every runtime clears the same bar, and the bar improves for all of them at once when the orchestrator's needs grow.
- **Additive keeps the concurrent Codex work safe.** The user is integrating Codex in parallel. Because the kit changes no called signature and the capability method is an optional interface, Codex can land against today's pattern and adopt the kit afterward with a mechanical diff — no merge collision, no coordination stall.

## Alternatives considered

- **A code-generator / scaffold command (`pr-triage runtime new <name>`)** that stamps out an adapter skeleton — rejected for v1. It optimizes the ~30 minutes of typing, not the days of vetting, and generated adapters still copy the boilerplate the generator emits. `ExecRun` + the harness attack the actual cost. Revisit if adapter count grows past ~5.
- **A plugin / `exec`-a-manifest architecture** (runtimes as external binaries described by a JSON manifest, à la a generic "harness protocol") — rejected. It's the right shape for a marketplace of third-party runtimes, but this tool has a handful of first-party adapters in one Go binary; a manifest layer adds an IPC boundary and a schema to version for no current payoff. The in-tree interface stays. Reconsider only if outside contributors need to ship runtimes without touching this repo.
- **Making `Capabilities()` a required interface method** — rejected in favor of an optional `CapabilityReporter`. Required would force-touch all three adapters (including in-flight Codex) in lockstep, which is exactly the coupling this ADR avoids. Callers treat "doesn't implement it" as "capabilities unknown."
- **Enforcing turn/budget in `ExecRun` by stream-watching** — deliberately out of scope. Self-enforced limits are a per-runtime concern (OpenCode/Codex can't do it via a flag; Claude Code can) and belong in the adapter or a future shared stream-watcher, not the process runner. `ExecRun` enforces timeout only, matching today's behavior.

## Current baseline

- **Interface (unchanged):** `internal/runtime/runtime.go` — `AgentRuntime{ Name, Run, ParseResult, ClassifyOutcome }`.
- **Registry (unchanged):** `internal/runtime/registry.go` — `init()` self-registration; `NameClaudeCode/NameCodex/NameOpenCode`.
- **Shared runner:** `internal/runtime/exec.go` — `ExecRun` + `ExecSpec{Binary, Args, PreCheck}`. Covered by `internal/runtime/exec_test.go`.
- **Capability descriptor:** `internal/runtime/capabilities.go` — `Capabilities`, the optional `CapabilityReporter` interface, `ModelForm`, and `CapabilitiesOf`.
- **Conformance harness:** `internal/runtime/runtimetest/runtimetest.go` — `runtimetest.Run(t, Case{...})`. Wired into `claudecode/conformance_test.go` and `opencode/conformance_test.go`.
- **Doctor + list:** `internal/cli/runtime_check.go` — `pr-triage runtime check <name>` and `pr-triage runtime list`. Covered by `runtime_check_test.go`.
- **Migrated adapters:** `internal/runtime/claudecode/claudecode.go` and `internal/runtime/opencode/opencode.go` now call `ExecRun` and implement `Capabilities()`; their pre-existing tests are unchanged and green.
- **Single call site, undisturbed:** `internal/orchestrator/orchestrator.go:658-789` (`runtime.Get` → `Run` → `ParseResult` → `ClassifyOutcome`).
- **Prose the capability descriptor absorbs:** [[runtime-capability-table]], [[cost-basis-honesty]], [[result-shape-normalization]], [[hard-fail-philosophy]].
- **Reference code + step-by-step checklist:** `docs/adding-a-runtime.md` (companion to this ADR).

## Open

- **Where `Capabilities` is surfaced to humans.** `runtime list` prints them today; whether `config show` should also explain a route, and whether `init` should *warn* when a chosen route's capabilities are weak (e.g. estimated cost), is undecided.
- **Doctor depth.** v1 `runtime check` runs a trivial prompt and asserts a parseable, successful Result. Whether it should also assert the agent could write a file (sandbox reachability) or post a comment is deferred until the Codex sandbox question is answered empirically.
- **Does `runtimetest` belong in CI per-adapter or as one aggregate suite?** Currently per-adapter (fails the PR that adds the adapter); the aggregate-over-registry option is open.
- **Codex adoption.** Codex has not been started yet; it should be authored directly on the kit (`ExecRun` + `Capabilities()` + a `conformance_test.go`) rather than the pre-kit pattern.
- **Runtime-native config is out of scope — the invocation stays thin.** pr-triage's config and startup flags carry only what routes a job: runtime name, model (when the runtime takes one per call), and agent def. A runtime's *own* configuration surface — OpenCode providers/plugins, DeepSeek Harness (`dsh`) Cordis plugins/profiles, `~/.dsh/settings.yaml`, credentials — is the operator's to configure in that runtime's own config, and is expected to be **picked up ambiently** when the subagent is invoked. We deliberately do **not** mirror, absorb, or pass that surface through `routing.<tier>` or the daemon's flags; doing so would recreate every runtime's config schema inside ours and guarantee drift. The adapter execs the runtime and lets the runtime read its own environment.
- **Model selection that lives in the runtime's config, not in argv (`ModelSelection`).** Surfaced by DeepSeek Harness (headless `dsh` takes no per-call `-m`; the model is set in `~/.dsh/settings.yaml`). The current `ModelForm` capability assumes a per-invocation model string, which does not fit. Proposed: add a `ModelSelection` capability with values `per-invocation` (claude-code, opencode, codex — `routing.<tier>.model` is injected and authoritative) vs `preconfigured` (dsh — the model comes from the runtime's ambient config, so `routing.<tier>.model` is advisory/ignored *by design*, not silently dropped). This makes the "model set elsewhere" case honest and declared, consistent with the thin-invocation principle above, rather than reprising the `config-model-silently-ignored` failure. No `Run`/`ParseResult` interface change is implied — it is a capability field plus how `config show`/the doctor report it. Only worth building when a `preconfigured`-model runtime (e.g. `dsh`) is actually adopted; nothing blocks claude-code/opencode/codex today.
