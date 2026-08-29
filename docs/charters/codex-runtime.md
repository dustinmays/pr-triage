---
title: "Charter — Implement Codex runtime for the triage agent"
status: draft
kind: charter
date: 2026-08-28
tags: [charter, behavioral-contract, runtime, codex, adapters]
related:
  - ../chunk-kickoff-behavioral-testing-spike.md   # the front-of-loop pattern this artifact dogfoods
  - ../runtime-capability-table.md
  - ../cost-basis-honesty.md
  - ../hard-fail-philosophy.md
  - ../result-shape-normalization.md
  - ../opencode-runtime.md
  - ../adr/0003-exec-subprocess-adapters.md
---

# Charter — Implement Codex runtime for the triage agent

**Status: draft charter.** Produced as an on-paper dogfood of the front-of-loop
charter+behavioral-contract pattern ([[chunk-kickoff-behavioral-testing-spike]]).
Saved for reuse: a future planning agent can refine it, and a TDD behavioral-testing
agent can pick up the behavioral contract below and implement the tests **red-first**.

## Grounding (prior decisions this charter points to)

The runtime seam already exists — this is an *adapter* chunk, not a "add runtime
support" chunk:

- `internal/runtime/runtime.go` — the `AgentRuntime` interface
  (`Run` / `ParseResult` / `ClassifyOutcome`) each adapter implements and
  self-registers via `init()`.
- `internal/runtime/registry.go` — `NameCodex = "codex"` **already defined**.
- `internal/cli/init.go` — `--runtime` / `--model` flags already accepted and pinned
  into `routing.routine` (top-level `runtime:`/`model:` are display-only; the daemon
  reads `routing.<tier>.{runtime,model}` — see [[opencode-runtime]]).
- `cmd/pr-triage/main.go` — adapters are activated by blank-import; claude-code +
  opencode today, codex to be added.
- Constraints inherited: [[runtime-capability-table]], [[cost-basis-honesty]],
  [[hard-fail-philosophy]], [[result-shape-normalization]], [[0003-exec-subprocess-adapters]].

## Outcome

A third `AgentRuntime` adapter so a chunk owner can route triage to **Codex**
end-to-end, producing reports at parity with claude-code / opencode. The pre-scan →
tier → agent → report flow is unchanged; Codex is just another routed runtime.

## Scope boundaries

**In scope**
- `internal/runtime/codex/codex.go` implementing `Run` / `ParseResult` /
  `ClassifyOutcome`, self-registered via `init()`; blank-imported in
  `cmd/pr-triage/main.go`.
- `pr-triage init --runtime codex --model <m>` writes valid config (flags already
  wired — **verify**, don't rebuild).
- The daemon `run` loop picks up completed pre-scans exactly as today; the Codex
  adapter consumes the pre-scan JSON and emits a normalized `runtime.Result` that the
  existing report renders.
- Codex progress (accumulating cost, turns, stop reason) flows through the **same
  event channel** (`internal/events`) as the other runtimes — no interface-level
  special-casing (this is the "future TUI/GUI headroom" clause).

**Explicitly out of scope**
- **Self-enforced turn/budget limits** — decided against for v1 (see Known
  limitations). Timeout is enforced; turns/budget are not.
- **Named-agent-def (`--agent`) parity** — Codex has no equivalent named-def system;
  parity comes from the orchestrator rendering the same review-agent prompt and
  passing it inline. Named-def parity is a non-goal.
- CLI/config *generalization* for the future front-of-loop bundling (tracked
  separately).
- Any new interface / TUI work.

## Constraints

- **Cost-basis honesty** ([[cost-basis-honesty]]): Codex exposes no terminal cost
  field, so cost is **estimated** (`CostBasisEstimated`), priced from a model table.
  A model absent from the table renders estimated/unknown — never fabricated-exact.
- **Model form differs from OpenCode**: Codex uses a plain model name, NOT
  `provider/model`. Do **not** copy OpenCode's slash validation — it would reject
  every valid Codex model.
- **Hard-fail philosophy** ([[hard-fail-philosophy]]): an unmapped tier / unusable
  runtime fails loudly to escalate rather than degrading silently.

## Behavioral contract (Given/When/Then)

```gherkin
Scenario: Config selection writes a valid Codex route
  Given a repo initialized with `pr-triage init --runtime codex --model <m>`
  When I run `pr-triage config show`
  Then routing.routine.runtime is "codex"
  And  routing.routine.model is "<m>"

Scenario: Triage processes a pre-scan on Codex
  Given the daemon is running with routine → codex
  And a PR has a completed pre-scan JSON
  When triage processes that PR
  Then the Codex adapter runs, reads the pre-scan JSON, and produces a report
  And the report reflects the same review-agent instructions as the other runtimes

Scenario: Honest cost basis for an unknown model (negative case)
  Given a Codex run for a model NOT in the price table
  When the report is produced
  Then cost is marked estimated/unknown
  And it is never presented as exact

Scenario: Codex model is not required to be provider/model form
  Given routing.routine.model is a plain Codex model name (no slash)
  When the Codex adapter runs
  Then it does NOT reject the model for lacking a "/" (unlike OpenCode)

Scenario: Progress flows through the shared event channel
  Given a Codex run is in progress
  When it emits step/cost/stop events
  Then those events reach the daemon event stream in the same normalized shape
       as claude-code and opencode (a future UI needs no Codex-specific code)
```

## Known limitations (v1, accepted)

- **No turn/budget cap.** Codex does not enforce max-turns/budget itself, and we are
  **not** building stream-watching self-enforcement in v1. Codex runs are bounded by
  **timeout only**. Documented limitation, revisit later if it bites.
- **Sandbox reachability — potential gotcha (unverified).** Codex is sandbox-only
  (no tool allowlist). It is *assumed* the pre-scan JSON is readable inside the
  sandbox and the report is writable out; this is where "worked in my shell" may
  diverge from "worked in the daemon." Flagged, not solved.
- **Auth precondition — must test.** Codex authentication (`codex login` /
  `OPENAI_API_KEY`, env frozen at daemon start) needs explicit testing, analogous to
  OpenCode's "if it works in your shell, the daemon can use it" rule.

## Notes for the implementer

- **Model definitions may already be handled.** A recent task synced model
  definitions across providers — so the Codex model form *and* the price-table
  entries this charter depends on (cost-basis, plain-name form) may already be
  covered. Confirm against that work before building a bespoke Codex model/price map;
  reuse the shared definitions if present. This is the same "agent-definition /
  model plumbing reflects down to the actual prompt/runtime" concern.
- Follow the OpenCode adapter as the closest structural template
  (`internal/runtime/opencode/opencode.go`), diverging on: model-form validation
  (plain, not slash), cost basis (estimated, not exact), and the stream/JSON schema
  Codex actually emits.
