---
id: override-command-state-first
title: "Build `pr-triage override` — local, state-first per-PR escalation override"
kind: build-item
severity: high
area: cli, orchestrator, poller, db
found_by: dustinmays
found_in: chunk/scanner-hardening — escalation override research (2026-08-26)
status: implemented (#101 / D.4, 2026-08-26)
related:
  - ../design/escalation-override.md          # full design + decision (Option C primary, A fast-follow)
  - ../../adr/0006-local-state-is-source-of-truth.md
  - ./per-chunk-triage-config.md
  - ./escalated-state-overwritten-by-ci-failed.md   # must be fixed as part of this
  - ./escalation-comment-lacks-trigger-reason.md
---

## What

DECIDED and specced. Build a local `pr-triage override <pr> [--signal <id> ...]`
command that records a SHA-keyed override in the store; the daemon consults it in
`HandleReportReady` before escalating, waives the specified signals for that head
SHA, and runs the probabilistic review agent instead of parking the PR. GitHub
label/gate are reconciled as a *projection* of that state per
[[0006-local-state-is-source-of-truth]]. A `triage-override` label is the
fast-follow front door reusing the same plumbing.

Full design, options comparison, and cross-cutting decisions:
[escalation-override](../design/escalation-override.md).

## Scope (as decided)

- Primary: Option C (local CLI + `overrides` store table). Fast-follow: Option A
  (label). Defer: comment slash-command / approving review.
- Override = "proceed to run the review agent," not "merge by fiat." Waives
  *specific* signals, pinned to a *specific* head SHA; cleared on any new push.
- Depends on fixing [escalated-state-overwritten-by-ci-failed](./escalated-state-overwritten-by-ci-failed.md)
  (make `escalated` terminal for its SHA, with a single override exit edge).

## Home

Orthogonal to Chunk A/B/C (scanner hardening + Swift). Candidate for its own chunk
or a follow-up epic ("stateful control plane / human-in-the-loop"), alongside
[per-chunk-triage-config](./per-chunk-triage-config.md) and
[chunk-setup-agent](./chunk-setup-agent.md).

## Implementation notes (D.4 / #101)

Built under Chunk D (#97) as part of the state-first MVP. Shape as designed:
`overrides(repo_id, pr_number, head_sha, waived_signals, reason, created_at,
consumed_at)`; `RecordOverride`/`GetActiveOverride`/`MarkOverrideConsumed`;
`pr-triage override <pr> [--signal ...] [--reason ...] [--repo owner/name]`;
`Orchestrator.applyOverride` consulted in `HandleReportReady` before escalating;
SHA-pinned; one-shot on full waiver (consumed); partial waiver still escalates
for the remaining signals.

**Key mechanism that wasn't in the original design:** because D.1 made
`escalated` a *terminal* poller state, an override alone would never be consulted
— the poller stops re-emitting `report_ready` for an escalated PR. So the
`override` command, when the PR is currently `escalated`, **re-arms** it by
resetting the PR state to `ci_passed` (same head SHA, whose report check still
passed). The poller then re-emits `report_ready` on its next poll, which re-runs
`HandleReportReady`, which consults the override. This is the state-first trigger:
a local state write drives re-evaluation, no daemon restart, no GitHub round-trip.

`target_kind`-driven escalations (e.g. `chunk_completion`) are intentionally
**not** waivable by the override (only signal-driven escalations are).

Deferred fast-follows still open: the `triage-override` GitHub-side label as an
*input* channel; recording the override application directly on the `runs` row
(currently the audit trail is the `overrides.consumed_at` timestamp linkable by
`(repo_id, pr_number, head_sha)`).
