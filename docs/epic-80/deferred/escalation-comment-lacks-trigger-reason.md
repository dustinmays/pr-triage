---
id: escalation-comment-lacks-trigger-reason
title: "Escalation comment/reason doesn't say WHICH signal tripped it"
kind: enhancement
severity: medium
area: orchestrator, escalate, observability
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood — PRs #94/#95 escalated (2026-08-25)
status: resolved (#99 / D.2, 2026-08-26)
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

## Resolution (D.2 / #99)

`config.ClassifyWithReason` now returns a `Classification` carrying the tier plus
the present signal IDs that matched the escalate rule (all of them, in rule
order) or a `ByTargetKind`/`TargetKind` marker. The orchestrator's new
`escalationReason` helper renders that into the `escalate.Request.Reason` — which
lands in both the PR comment and `runs.stop_reason` — naming each triggering
signal and listing its `file:line — detail` evidence. Per ADR 0007 it emits only
the deterministic pre-scan facts (no AI phrasing), so the human's read isn't
skewed. `chunk_completion` PRs get a distinct "target_kind requires human review"
reason. Covered by `TestClassifyWithReason` and the augmented high-risk /
chunk-completion orchestrator escalation tests.
