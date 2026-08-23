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
- **Codex**: estimated cost only (no terminal cost field — priced from a
  hardcoded per-model table); does not enforce max-turns/budget itself,
  so the adapter must self-enforce by watching the stream; no tool
  allowlist (sandbox-only).
- **OpenCode**: exact cost; does not enforce turns/budget at all; needs
  `provider/model` form (silently drops a model string with no slash —
  validate at config time); is a shared server, so provider credentials
  are deployment-level, not per-job (env freezes at server start).

Rule of thumb: never advertise a limit (timeout, tool allowlist, budget
cap) that a given adapter doesn't actually enforce — enforce it or don't
claim it.
