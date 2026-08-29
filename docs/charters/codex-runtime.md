---
title: "Charter — Implement Codex runtime for the triage agent"
status: ratified
kind: charter
date: 2026-08-28
ratified: 2026-08-29 15:23 MDT
behavioral_contract_ratified: 2026-08-29 16:25 MDT
tags: [charter, behavioral-contract, runtime, codex, adapters]
related:
  - ../chunk-kickoff-behavioral-testing-spike.md   # the front-of-loop pattern this artifact dogfoods
  - ../runtime-capability-table.md
  - ../cost-basis-honesty.md
  - ../hard-fail-philosophy.md
  - ../result-shape-normalization.md
  - ../opencode-runtime.md
  - ../adr/0003-exec-subprocess-adapters.md
  - ../adr/0009-runtime-adapter-kit.md
  - ../experiments/issue-129-front-of-loop-report.md
---

# Charter — Implement Codex runtime for the triage agent

**Status: charter ratified (2026-08-29 15:23 MDT); behavioral contract ratified
(2026-08-29 16:25 MDT).** Produced as an on-paper dogfood of the
front-of-loop charter+behavioral-contract pattern ([[chunk-kickoff-behavioral-testing-spike]]),
refined by the issue-#129 grounding pass (see
[[issue-129-front-of-loop-report]]), then ratified by the human owner with
the binding clarifications recorded below. A TDD behavioral-testing agent can pick up
the behavioral contract below and implement the tests **red-first** (with the
config-selection exemption noted there).

At Gate 2 the owner spot-checked a subset of the executable bindings and approved
proceeding without reviewing all 32 adapter tests. The durable human decision surface is
the nine scenarios below; the 32 adapter tests are the machine-verifiable Go binding.

## Grounding (prior decisions this charter points to)

The runtime seam already exists — this is an *adapter* chunk, not a "add runtime
support" chunk:

- `internal/runtime/runtime.go` — the `AgentRuntime` interface
  (`Run` / `ParseResult` / `ClassifyOutcome`) each adapter implements and
  self-registers via `init()`.
- `adr/0009-runtime-adapter-kit.md` — adapters are **kit-native**: built on the
  shared executor (`runtime.ExecRun`), declare static `runtime.Capabilities`, and
  conform via the `internal/runtime/runtimetest` harness.
- `internal/runtime/registry.go` — `NameCodex = "codex"` **already defined**.
- `internal/cli/init.go` — `--runtime` / `--model` flags already accepted and pinned
  into `routing.routine` (top-level `runtime:`/`model:` are display-only; the daemon
  reads `routing.<tier>.{runtime,model}` — see [[opencode-runtime]]).
- `cmd/pr-triage/main.go` — adapters are activated by blank-import; claude-code +
  opencode today, codex to be added.
- Constraints inherited: [[runtime-capability-table]], [[cost-basis-honesty]],
  [[hard-fail-philosophy]], [[result-shape-normalization]], [[0003-exec-subprocess-adapters]].

## Outcome

A third, **kit-native** `AgentRuntime` adapter so a chunk owner can route triage to
**Codex** end-to-end, producing reports at parity with claude-code / opencode. The
pre-scan → tier → agent → report flow is unchanged; Codex is just another routed
runtime built from the adapter kit.

## Scope boundaries

**In scope**
- `internal/runtime/codex/codex.go` — a **kit-native** adapter (shared executor
  `runtime.ExecRun`, declared `runtime.Capabilities`, `runtimetest` conformance),
  implementing `Run` / `ParseResult` / `ClassifyOutcome`, self-registered via
  `init()`; blank-imported in `cmd/pr-triage/main.go`.
- The ratified **invocation contract**: `codex exec --json --ephemeral --sandbox
  workspace-write`, with the process cwd as the worktree, an inline prompt (the
  orchestrator-rendered review-agent prompt), an exact `-m <model>` passthrough when
  a model is configured, and **no** `--skip-git-repo-check` in production.
- A **namespaced structured invocation envelope** written by the adapter to its log:
  Codex usage events carry no model identity and `ParseResult` receives only a log
  reader, so the adapter records the invocation context (model, etc.) beside the
  captured JSONL for honest parsing.
- `pr-triage init --runtime codex --model <m>` writes valid config (flags already
  wired — **verify**, don't rebuild).
- The daemon `run` loop picks up completed pre-scans exactly as today; the Codex
  adapter consumes the pre-scan JSON and emits a normalized `runtime.Result` that the
  existing report renders.

**Explicitly out of scope**
- **Self-enforced turn/budget limits** — decided against for v1 (see Known
  limitations). Timeout is enforced; turns/budget are not.
- **Named-agent-def (`--agent`) parity** — Codex has no equivalent named-def system;
  parity comes from the orchestrator rendering the same review-agent prompt and
  passing it inline. Named-def parity is a non-goal.
- **UI / generic event wiring** — no shared emitter exists in production today, so
  no Codex-specific (or generic) event plumbing is built in this chunk; see the
  narrowed shared-event scenario.
- CLI/config *generalization* for the future front-of-loop bundling (tracked
  separately).

## Constraints

- **Cost-basis honesty** ([[cost-basis-honesty]]): Codex exposes no terminal cost
  field, and its usage events carry no model identity. `ParseResult` works on the
  **captured JSONL**: a **known priced model** renders cost **estimated from
  captured usage** (`CostBasisEstimated`); an **unknown model** renders `Cost=0`
  with `CostBasisUnavailable`. Never fabricated-exact.
- **Model form differs from OpenCode**: Codex uses a plain model name, NOT
  `provider/model`. There is **no slash validation**; a configured model is passed
  through exactly as `-m <model>`. Do **not** copy OpenCode's slash validation — it
  would reject every valid Codex model.
- **Auth**: saved `codex login` or an invocation-scoped `CODEX_API_KEY` (the
  charter's earlier `OPENAI_API_KEY` claim was stale and is dropped).
- **Hard-fail philosophy** ([[hard-fail-philosophy]]): an unmapped tier / unusable
  runtime fails loudly to escalate rather than degrading silently.

## Behavioral contract (Given/When/Then)

```gherkin
Scenario: Config selection writes a valid Codex route
  Given a repo initialized with `pr-triage init --runtime codex --model <m>`
  When I run `pr-triage config show`
  Then routing.routine.runtime is "codex"
  And  routing.routine.model is "<m>"
  # Pre-existing green verification: exempt from all-new-tests-red (see Notes).

Scenario: Adapter invokes Codex with the ratified contract
  Given a routed Codex triage run with model <m> configured
  When the adapter launches Codex via the shared executor
  Then it runs `codex exec --json --ephemeral --sandbox workspace-write` with the
       process cwd as the worktree
  And  it passes `-m <m>` exactly as configured, with no slash validation
  And  it passes the review-agent prompt inline
  And  it does NOT pass `--skip-git-repo-check` in production
  And  it writes a namespaced structured invocation envelope to its log
       (model identity must survive into parsing)

Scenario: ParseResult derives the normalized Result from captured JSONL
  Given a captured Codex JSONL log for a completed run
  When ParseResult parses the log
  Then Summary comes from the terminal `agent_message` item
  And turns are derived from terminal turn events (runtime-local turns)
  And cost / turn / stop-reason fields of the normalized Result are fully populated

Scenario: Honest cost basis for a known priced model
  Given a captured Codex run whose model is in the price table
  When ParseResult parses the log
  Then cost is estimated from captured usage (CostBasisEstimated)

Scenario: Honest cost basis for an unknown model (negative case)
  Given a captured Codex run whose model is NOT in the price table
  When ParseResult parses the log
  Then Cost is 0 with CostBasisUnavailable
  And it is never presented as estimated or exact

Scenario: Codex model is not required to be provider/model form
  Given routing.routine.model is a plain Codex model name (no slash)
  When the Codex adapter runs
  Then it does NOT reject the model for lacking a "/" (unlike OpenCode)

Scenario: Normalized Result stands alone (shared events narrowed)
  Given a Codex run completes
  When the adapter produces its runtime.Result
  Then the Result is fully populated in the normalized shape
  And no Codex-specific event plumbing is required (production has no shared
      emitter wiring yet; a future emitter would consume the same Result)

Scenario: Triage processes a pre-scan on Codex
  Given the daemon is running with routine → codex
  And a PR has a completed pre-scan JSON
  When triage processes that PR
  Then the Codex adapter runs, reads the pre-scan JSON, and produces a report
  And the report reflects the same review-agent instructions as the other runtimes

Scenario: Final smoke proves environment preconditions separately
  Given a real Codex invocation in the daemon-like environment
  Then the smoke separately proves auth works (saved `codex login` or
       invocation-scoped `CODEX_API_KEY`), the pre-scan JSON is readable inside
       the workspace-write sandbox, and the report is writable there
```

## Known limitations (v1, accepted)

- **Timeout-only v1 (ratified).** Codex does not enforce max-turns/budget itself, and
  we are **not** building stream-watching self-enforcement. Codex runs are bounded by
  **timeout only** — no turns/budget cap. Documented limitation, revisit later if it
  bites.
- **Sandbox reachability — proven by smoke, not assumed.** Codex is sandbox-only
  (no tool allowlist). The final smoke **separately proves** auth, pre-scan-JSON
  readability inside the `workspace-write` sandbox, and report writability — the
  places where "worked in my shell" may diverge from "worked in the daemon."
- **Git-repo precondition.** `codex exec` requires a Git repo and production passes
  **no** `--skip-git-repo-check`; the process cwd (the real worktree) satisfies it.
  The generic **runtime doctor initializes its temp workdir as a Git repo** so
  doctor/conformance runs meet the same precondition.
- **Auth precondition — proven by smoke.** Codex authentication is saved
  `codex login` or an invocation-scoped `CODEX_API_KEY`; the smoke proves it works
  in the daemon-like environment, analogous to OpenCode's "if it works in your
  shell, the daemon can use it" rule.

## Notes for the implementer

- **No shared model-price table exists** (`internal/` has none — grounding disproved
  the earlier "recent sync may have covered pricing" assumption). The Codex adapter
  owns its known-model price map: known priced models are estimated from captured
  usage; unknown models render `Cost=0` / `CostBasisUnavailable`. Model identity
  reaches `ParseResult` via the adapter-written namespaced invocation envelope in
  the log.
- **Test standard (ratified):** standard-library Go `testing` only — sentence-style,
  one-behavior-per-test names, concrete assertions, captured JSONL **golden
  fixtures**, and fake executables; no testify-style dependencies. The
  **config-selection scenario is pre-existing green verification** (the flags are
  already wired) and is **exempt from all-new-tests-red**; all **new adapter
  behaviors remain RED-first**.
- Follow the OpenCode adapter as the closest structural template
  (`internal/runtime/opencode/opencode.go`), diverging on: invocation flags (the
  ratified `codex exec` contract above), model handling (plain, exact `-m`
  passthrough, no slash validation), cost basis (usage-estimated or unavailable,
  never exact), and the JSONL schema Codex actually emits.
