---
id: report-check-name-coupling-fragile
title: "Daemon hard-couples to a check run named exactly 'pr-prescan-report'"
kind: enhancement
severity: medium
area: poller, report, workflows
found_by: dustinmays
found_in: chunk/scanner-hardening dogfood run — PR #93 stuck in report_ready (2026-08-24)
status: fixed
fixed_in: "#113 — poller flags ReportMissing at ceiling when gating passed; orchestrator escalates on ReportMissing + report.ErrMissing (escalate-on-missing option). Configurable check-name + install-command options remain (see workflow-install-command)."
related:
  - ../../../internal/poller/poller.go     # reportCheckRunID(), pollCI report gate
  - ../../../internal/report/report.go      # ReportCheckName constant
  - ../../../.github/workflows/pr-prescan.yml  # publishes the check run
  - ./workflow-install-command.md           # proposed mitigation
---

## What

The daemon now ingests the pre-scan report from a check run identified **by
name** (`report.ReportCheckName == "pr-prescan-report"`). This fixed the real
bug where it fetched an arbitrary check run's (empty) output and got stuck in
`report_ready` forever (PR #93). But it introduces a hard coupling: if a repo's
`pr-prescan.yml` doesn't publish a check run with *exactly* that name, the daemon
never sees a report — it waits until the CI-timeout ceiling, then marks the PR
`ci_failed` and silently drops it.

## Why it matters

- Silent failure mode: green CI + no report check → PR is dropped, no escalation,
  no human notified. Easy to misconfigure a new repo and not know why nothing
  triages.
- The name is an implicit contract spread across two places (workflow YAML and a
  Go const) with nothing enforcing they agree.

## Options (not decided)

- **Escalate instead of silently ci_failed** when gating is green but no report
  check appears within the ceiling — at least a human gets pinged. (Cheapest
  robustness win.)
- Make the check name **configurable** per repo (config field) rather than a
  hardcoded const.
- Add a `pr-triage workflow`/`init` command that installs/ensures the pre-scan
  job exists — see [workflow-install-command](./workflow-install-command.md).
- Fall back to scanning all completed check runs for a JSON payload matching the
  report schema if the named one is absent (loosens the contract, more magic).

Leaning toward: escalate-on-missing-report (safety) + the install command
(prevention).
