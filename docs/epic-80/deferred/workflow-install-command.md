---
id: workflow-install-command
title: "Add a `pr-triage workflow` command that ensures the pre-scan CI job exists"
kind: enhancement
severity: low
area: cli, workflows
found_by: dustinmays
found_in: chunk/scanner-hardening dogfood run (2026-08-24)
status: open
related:
  - ./report-check-name-coupling-fragile.md   # the fragility this mitigates
  - ../../../.github/workflows/pr-prescan.yml   # the workflow it would install
  - ../../../internal/cli/init.go               # sibling setup command
---

## What

A command — `pr-triage workflow` (or folded into `pr-triage init`) — that
inspects the current repository and **installs or ensures** the pre-scan CI job
(`.github/workflows/pr-prescan.yml` + `scripts/pr-prescan.sh`) that publishes the
`pr-prescan-report` check run the daemon depends on.

## Why it matters

Today the daemon silently does nothing on a repo that lacks the pre-scan job
(see [report-check-name-coupling-fragile](./report-check-name-coupling-fragile.md)).
An install/ensure command turns a silent misconfiguration into a one-command
setup, and keeps the workflow's check-run name in sync with the daemon's expected
`report.ReportCheckName`.

## Sketch

- `pr-triage workflow --check` : report whether the pre-scan workflow + script
  are present and publish the expected check name.
- `pr-triage workflow --install` : write/update the workflow + script (idempotent),
  tailored to the detected stack (Go / Swift / …).
- Could run as part of `init` with a prompt, matching the interactive-by-default,
  every-prompt-is-also-a-flag pattern.

Follow-up work — not blocking Epic 80; captured so it isn't lost.
