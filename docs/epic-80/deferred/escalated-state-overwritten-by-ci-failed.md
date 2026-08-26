---
id: escalated-state-overwritten-by-ci-failed
title: "Escalated PR state gets overwritten to ci_failed on the next poll"
kind: bug
severity: medium
area: poller, escalate
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood — PRs #94/#95 (2026-08-25)
status: resolved (#98 / D.1, 2026-08-26)
related:
  - ../../../internal/poller/poller.go     # ProcessPR Case 3 terminal-state set
  - ../../../internal/escalate/escalate.go  # sets state=escalated (label + comment only)
  - ../../../internal/orchestrator/orchestrator.go
---

## Resolution (D.1 / #98)

Root cause was **narrower than first written up**: escalation projects only a
`needs-owner-review` **label** and a comment — there is *no* `owner-review-gate`
check run in the current code (that was a planned projection, not built). So gap
#2 below (a red gate misread as CI failure) does not apply as written. The actual
drift was purely gap #1: `escalated` was absent from `ProcessPR` Case 3's
terminal set, so an escalated PR at an unchanged head SHA fell through to
`default → pollCI`, which re-emitted `report_ready` (re-escalating and, depending
on check state, overwriting the state).

Fixed by making `escalated` a human-owned terminal state in `ProcessPR` Case 3
(ADR 0006): re-polling an escalated PR at the same head SHA is a no-op; only a new
push (Case 2 SHA change) or a human override (D.4) may leave it. No
`evaluateCheckRuns` gate-filtering was needed because no such gate check exists.
Covered by `TestPoller_Escalated_SameSHA_NoOp` and
`TestPoller_Escalated_NewPush_ResetsToCIRunning`.

## What

After the daemon escalates a PR (state=`escalated`, `needs-owner-review` label,
`owner-review-gate` turns red), a subsequent poll flips the PR's `prs.state` to
`ci_failed`. Observed: #94/#95 recorded escalated runs, but their `prs.state` is
now `ci_failed`.

## Why

Two interacting gaps in the poller state machine (`ProcessPR`):

1. `escalated` is **not** in the recognized terminal set (Case 3 only short-circuits
   on `report_ready`, `agent_running`, `done`; and `ci_failed`). So an escalated PR
   on an unchanged head SHA is re-processed instead of left alone.
2. Re-processing calls `pollCI`, and `evaluateCheckRuns` sees the **red
   `owner-review-gate`** — which escalation itself turned red — as a check
   `failure`, so it marks the PR `ci_failed`, clobbering `escalated`.

The escalation's own gate failure is misread as a CI failure.

## Why it matters

The persisted state no longer reflects reality (the PR is escalated/awaiting owner
review, not "CI failed"). It corrupts `status`, history, and any logic keyed on
state, and could mask a genuine re-escalation on a new push.

## Fix sketch

- Add `escalated` to the poller's terminal/no-op states (don't re-process an
  escalated PR on an unchanged head SHA; re-enter only on a new head SHA).
- And/or have `evaluateCheckRuns` ignore pr-triage's own `owner-review-gate` check
  when judging CI health (it's a gate the daemon controls, not a build signal).
