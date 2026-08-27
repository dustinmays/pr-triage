---
title: Local app state is the source of truth; GitHub is a projection
status: accepted
date: 2026-08-26
tags: [architecture, state, github, escalation, portability]
---

## Context

pr-triage began as a script idea and has grown into a stateful workflow tool: it
tracks repos, per-PR state, and agent runs in a local SQLite store, and it also
reads and writes GitHub artifacts (the pre-scan check run, the `needs-owner-review`
label, review/escalation comments, the `owner-review-gate` check). As the tool
matured it became unclear which of these is authoritative. A concrete bug made the
ambiguity expensive: an escalated PR's state was silently overwritten to
`ci_failed` because the daemon let a GitHub-derived rendering — the red
`owner-review-gate` that the daemon itself had caused — feed back and clobber its
own state (see [[escalated-state-overwritten-by-ci-failed]]). Leaning on GitHub
labels as de-facto app state is the weakest part of the design.

## Decision

**The local SQLite store is the single source of truth for everything pr-triage
manages and every decision it makes. GitHub artifacts are a projection of that
truth, never the truth itself.** GitHub plays exactly two roles, and neither is
"app state":

- **Inbound signals** — things humans or CI produce that the app *ingests* (the
  pre-scan report check; a human "proceed"/override). The app records its own
  *interpretation* in local state; it does not treat the GitHub artifact as state.
- **Outbound projections** — labels, comments, and checks the app *writes to
  communicate to people who aren't running the app* (`needs-owner-review`, the
  review comment, the gate). These are notifications reconciled *from* state,
  eventually-consistent.

Reconciliation is **one-directional: state → GitHub**. On any divergence, local
state wins and the app reconciles GitHub to match it, idempotently. The app must
never let a GitHub-derived value it rendered flow back and mutate its own state.

Labels remain important — they signal status to collaborators and other tooling —
but they are the shadow, not the object.

## Consequences

- **Robustness:** whole classes of feedback-loop bugs (like the escalated→ci_failed
  drift) become impossible by construction — the app reasons from its own state,
  not from artifacts it just emitted.
- **Local-first control plane:** human interactions are best modeled as writes to
  local state (e.g. a `pr-triage override` CLI writing a store row the daemon reads
  on its next poll — see [[escalation-override]] / [[per-chunk-triage-config]]),
  with GitHub projection as a side effect. This matches the control-plane/worker
  split already used elsewhere ("control plane writes a row, worker reads it").
- **Provider decoupling / portability:** because the app reasons from its own
  state and treats GitHub as one I/O adapter, other providers (GitLab, a plain git
  remote, a local diff) become additional adapters later. This is a deliberate
  differentiator from hosted, webhook-bound bots (Prow, Mergify, Renovate) that
  *are* GitHub Apps and cannot be provider-agnostic.
- **Design rule for every future feature:** decide first what local state changes;
  treat any GitHub read as an inbound signal to interpret into state, and any
  GitHub write as an outbound projection of state. Same hard-fail discipline as
  [[hard-fail-philosophy]] and the resolve-once discipline in
  [[config-resolve-once]].
