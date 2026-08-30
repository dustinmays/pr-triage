---
title: Runtime capability table
tags: [adapters, runtime, config]
related: [[cost-basis-honesty]], [[result-shape-normalization]], [[config-resolve-once]]
source: plan.md
---

Declare a static, per-runtime capability table up front rather than
discovering differences live by probing:

- **Claude Code**: exact cost; enforces its own max-turns/budget via CLI
  flags; tool allowlist supported; resume requires passing the working
  directory (sessions are stored per-project-dir).
- **Codex**: no terminal cost field — known priced models are estimated from
  captured usage; unknown models render Cost=0 with cost basis unavailable;
  **timeout-only enforcement in v1** (does not enforce max-turns/budget itself,
  and the adapter does not self-enforce — runs are bounded by timeout, nothing
  more); no tool allowlist (sandbox-only).
- **OpenCode**: exact cost; does not enforce turns/budget at all; needs
  `provider/model` form (silently drops a model string with no slash —
  validate at config time); is a shared server, so provider credentials
  are deployment-level, not per-job (env freezes at server start).

Rule of thumb: never advertise a limit (timeout, tool allowlist, budget
cap) that a given adapter doesn't actually enforce — enforce it or don't
claim it.

As of [[0009-runtime-adapter-kit]] these facts are also declared **in code**
via `runtime.Capabilities` (the optional `CapabilityReporter` interface):
cost basis, which limits the adapter enforces, model form, and auth model.
`pr-triage runtime list` prints them, and a conformance test asserts the
declared cost basis matches what `ParseResult` actually produces — so this
table and the code cannot silently drift. This prose remains the narrative
reference; the struct is the enforced source of truth.
