---
id: escalated-state-overwritten-by-ci-failed
title: "Escalated PR state gets overwritten to ci_failed on the next poll"
kind: bug
severity: medium
area: poller, escalate
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood — PRs #94/#95 (2026-08-25)
status: open
related:
  - ../../../internal/poller/poller.go     # ProcessPR Case 3 terminal-state set; evaluateCheckRuns treats red gate as CI failure
  - ../../../internal/escalate/escalate.go  # sets state=escalated
  - ../../../internal/orchestrator/orchestrator.go
---

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
