---
title: "Epic 80 — Deferred findings backlog"
kind: deferred-index
epic: 80
updated: 2026-08-23
# This index is DERIVED from the sibling *.md files. Do not hand-edit rows to add
# a finding — create a new `<kebab-slug>.md` file instead (that keeps writes
# collision-free across parallel agents). Regenerate this table from the
# directory when findings are added or their status changes.
related:
  - ../STATE.md
---

# Deferred findings — Epic 80

The "found it broken / worth improving, but not now" backlog for the scanner-
hardening epic. One file per finding; this table just indexes them.

**How to add one:** create `docs/epic-80/deferred/<kebab-slug>.md` with the
frontmatter schema below. Never edit another finding's file or hand-append rows
here — different filenames can't git-conflict; a shared list can. Fixing a
finding flips `status:` in its own file.

**Frontmatter schema:** `id, title, kind (bug|enhancement|tooling|question),
severity (low|medium|high|n/a), area, found_by, found_in, status
(open|fixed|wontfix), related[]`.

## Open

| Finding | Kind | Sev | Area | Notes |
|---------|------|-----|------|-------|
| [config-model-silently-ignored](./config-model-silently-ignored.md) | bug | medium | config, orchestrator | `--model` / top-level `config.model` is ignored; agent runs on `routing.Model` |
| [agent-permission-mode-hardening](./agent-permission-mode-hardening.md) | enhancement | medium | runtime, security | agent now uses `bypassPermissions`; tighten to a scoped allowlist / make configurable |
| [orchestrator-should-post-review-comment](./orchestrator-should-post-review-comment.md) | enhancement | medium | orchestrator, agents, observability | agent forgot to post its review; orchestrator should post the Result deterministically |
| [report-check-name-coupling-fragile](./report-check-name-coupling-fragile.md) | enhancement | medium | poller, report, workflows | daemon hard-couples to a check named `pr-prescan-report`; missing → silent `ci_failed` drop |
| [workflow-install-command](./workflow-install-command.md) | enhancement | low | cli, workflows | `pr-triage workflow` to install/ensure the pre-scan CI job exists |
| [init-writes-opaque-partial-config](./init-writes-opaque-partial-config.md) | enhancement | low | config, cli | `init` hides the active signal/routing tables (now correctness-safe after the Load merge fix) |
| [skill-defer-finding](./skill-defer-finding.md) | tooling | n/a | workflow, agents | build a skill encoding how agents file deferred findings |
| [skill-log-build-state](./skill-log-build-state.md) | tooling | n/a | workflow, agents | build a skill encoding how the chunk owner maintains STATE.md |

## Resolved

_(none yet)_
