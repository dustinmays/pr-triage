---
id: escalation-comment-lacks-trigger-reason
title: "Escalation comment/reason doesn't say WHICH signal tripped it"
kind: enhancement
severity: medium
area: orchestrator, escalate, observability
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood — PRs #94/#95 escalated (2026-08-25)
status: open
related:
  - ../../../internal/orchestrator/orchestrator.go   # builds Reason as fmt.Sprintf("risk tier %q triggered escalation", tier)
  - ../../../internal/escalate/escalate.go            # posts the comment from req.Reason
  - ../../../internal/report/report.go                # Report.Signals[] carry id + evidence{file,line,detail}
  - ./per-chunk-triage-config.md
---

## What

When the daemon escalates a PR it posts:

```
⚠️ pr-triage escalation
Cc @dustinmays
Reason: risk tier "escalate" triggered escalation
```

The reason is generic — it names neither the **triggering signal** (e.g.
`workflow_changed`) nor its **evidence** (which file/line). The same generic
string is stored in `runs.stop_reason`. To find out *why* a PR escalated, a human
has to open the pre-scan check-run and scroll the full JSON payload for the
`present:true` signal. That defeats the point of the escalation notification.

## Why it matters

The whole value of escalation is "a human's attention is needed here" — and the
first thing that human asks is "needed for *what*?" The daemon already has the
answer: `Classify()` matched a specific present signal to the escalate tier, and
each signal carries `evidence[{file,line,detail}]`. It's just thrown away.

## Fix sketch

- In `HandleReportReady`, when classifying to escalate, collect the present
  signal id(s) that matched an escalate rule (and their evidence), and build a
  reason like:
  > Escalated: `workflow_changed` present — .github/workflows/ci.yml (changed a
  > CI workflow). Signal maps to the `escalate` tier.
- Pass that through `escalate.Request.Reason` so it lands in both the PR comment
  and `runs.stop_reason` (which also fixes the terse stop_reason).
- Keep it compact — the specific signal(s) + one evidence line each, not the
  whole payload.

Cheap, high-value observability win. Pairs with
[per-chunk-triage-config](./per-chunk-triage-config.md) (seeing *why* is the
first step to deciding a signal should be routine for this chunk).
