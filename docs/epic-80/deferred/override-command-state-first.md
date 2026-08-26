---
id: override-command-state-first
title: "Build `pr-triage override` — local, state-first per-PR escalation override"
kind: build-item
severity: high
area: cli, orchestrator, poller, db
found_by: dustinmays
found_in: chunk/scanner-hardening — escalation override research (2026-08-26)
status: planned
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
