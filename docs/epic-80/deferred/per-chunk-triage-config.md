---
id: per-chunk-triage-config
title: "Per-chunk (context-scoped) triage config: what's routine vs escalate for THIS chunk"
kind: enhancement
severity: high
area: config, orchestrator, product
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood — nearly every infra PR escalated (2026-08-25)
status: open
related:
  - ../../../internal/config/config.go        # SignalTiers + Routing (currently per-repo only)
  - ../../../internal/orchestrator/orchestrator.go  # Classify/Route
  - ./escalation-comment-lacks-trigger-reason.md
  - ./skill-log-build-state.md                # chunk-owner setup responsibility
  - ./skill-defer-finding.md
---

## What

The signal→tier config (`signal_tiers` + `routing`) is currently **per-repo,
global**. But whether a signal should escalate is often **context-dependent on
the chunk of work**. In this very chunk (scanner-hardening), the work *is* CI
workflows, Makefile/scanner config, and stack detection — so almost every PR
trips an escalate signal (`workflow_changed`, `safeguard_config_changed`,
`stack_choice_changed`) and routes to a human, even though those changes are the
expected, on-purpose content of the chunk. The "routine autonomous review" lane
barely applies.

## Why it matters (and why it belongs in pr-triage)

The user's framing: setting up a chunk should include an **early chunk-owner
responsibility** to declare what's expected/routine vs genuinely risky *for that
chunk*. And this determination — "triage this PR given the context" — is literally
what the tool is named for. It should live in pr-triage, and it generalizes to
every project that adopts the tool (each project/chunk has different "expected"
changes). Without it, infra-heavy chunks are all-escalate and the daemon can't
meaningfully auto-review them.

## Design sketch (not decided)

- A **per-chunk config overlay** the chunk owner sets up at chunk start: e.g. a
  `.pr-triage/chunks/<chunk-branch>.yaml` (or a section keyed by base ref) that
  overrides `signal_tiers` for PRs targeting that chunk branch — "in this chunk,
  `workflow_changed` and `safeguard_config_changed` are routine."
- Resolution order: explicit per-chunk overlay → repo config → defaults (mirrors
  the resolve-once discipline in docs/config-resolve-once.md).
- Keep escalation honest: even when downgraded to routine, the review agent
  should still call out the sensitive change in its summary.
- A **chunk-setup skill/agent** (pairs with [skill-log-build-state] and
  [skill-defer-finding]) that scaffolds this overlay + STATE.md + deferred/ at
  chunk kickoff — the "early chunk-owner responsibility" made concrete.

## Immediate implication (this chunk)

Because everything here escalates, driving the chunk to completion requires either
per-PR owner review, tuning these signals to routine for the chunk, or authorizing
the driver to merge escalated infra PRs. This finding is the durable fix.
