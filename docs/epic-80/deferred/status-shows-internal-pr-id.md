---
id: status-shows-internal-pr-id
title: "`status` shows the internal DB row id, not the GitHub PR number"
kind: bug
severity: low
area: cli, observability
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood (2026-08-25)
status: open
related:
  - ../../../internal/cli/status.go   # renders "PR #<...>" from run rows
  - ../../../internal/db/schema.go      # runs.pr_id is a FK to prs.id, not the GitHub number
---

## What

`pr-triage status` prints `PR #<n>` using `runs.pr_id` (the internal
autoincrement FK into `prs`), not `prs.number` (the GitHub PR number). Example
observed:

| status shows | actually is |
|---|---|
| PR #27 | GitHub #94 (prs.id 27) |
| PR #25 | GitHub #95 (prs.id 25) |
| PR #1  | GitHub #93 (prs.id 1)  |

`#93`'s row happened to be id 1, so it displayed as "PR #1" and looked plausible —
which masked the bug until later PRs landed at ids 25/27.

## Why it matters

The number a user sees must match the GitHub PR they can open. Showing the
internal id is actively misleading ("PR #27" points at a PR that isn't #27).

## Fix

`status` should resolve/join `runs.pr_id → prs.number` and display the GitHub
number (keep the internal id out of the UI, or label it distinctly). Check other
display paths (logs, checkout TUI) for the same confusion.
