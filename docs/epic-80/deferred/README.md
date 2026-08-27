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
| [report-check-name-coupling-fragile](./report-check-name-coupling-fragile.md) | enhancement | medium | poller, report, workflows | daemon hard-couples to a check named `pr-prescan-report`; missing → silent `ci_failed` drop |
| [workflow-install-command](./workflow-install-command.md) | enhancement | low | cli, workflows | `pr-triage workflow` to install/ensure the pre-scan CI job exists |
| [init-writes-opaque-partial-config](./init-writes-opaque-partial-config.md) | enhancement | low | config, cli | `init` hides the active signal/routing tables (now correctness-safe after the Load merge fix) |
| [skill-defer-finding](./skill-defer-finding.md) | tooling | n/a | workflow, agents | build a skill encoding how agents file deferred findings |
| [skill-log-build-state](./skill-log-build-state.md) | tooling | n/a | workflow, agents | build a skill encoding how the chunk owner maintains STATE.md |
| [per-chunk-triage-config](./per-chunk-triage-config.md) | enhancement | high | config, orchestrator, product | chunk-owner-scoped signal→tier overlay; infra chunks are all-escalate without it |
| [chunk-setup-agent](./chunk-setup-agent.md) | enhancement | high | cli, config, agents, product | interactive setup command/agent that tailors BOTH the pre-scan tiers and the review agent def from repo + chunk charter |
| [override-command-state-first](./override-command-state-first.md) | build-item | high | cli, orchestrator, poller, db | DECIDED: `pr-triage override` local state-first per-PR escalation override (see [design](../design/escalation-override.md)) |
| [escalation-comment-lacks-trigger-reason](./escalation-comment-lacks-trigger-reason.md) | enhancement | medium | orchestrator, escalate, observability | escalation comment/stop_reason should name the triggering signal + evidence, not just "escalate tripped" |
| [escalated-state-overwritten-by-ci-failed](./escalated-state-overwritten-by-ci-failed.md) | bug | medium | poller, escalate | escalated PR flips to `ci_failed` on re-poll (escalated not terminal; red owner-review-gate misread as CI fail) |
| [status-shows-internal-pr-id](./status-shows-internal-pr-id.md) | bug | low | cli, observability | `status` prints internal `runs.pr_id` instead of the GitHub PR number |
| [scanner-scans-its-own-test-fixtures](./scanner-scans-its-own-test-fixtures.md) | bug | medium | scanner, poller | scanner trips signals on fixture files (paths matched anywhere via `(^\|/)`; also scans `apply.sh`/`golden.json`) — confirmed escalating #106 |
| [schema-sql-matches-migration-regex](./schema-sql-matches-migration-regex.md) | question | low | scanner | editing `internal/db/schema.sql` also trips `migration_history_rewritten` (broad `MIGRATION_RE`) |

## Resolved

| Finding | Kind | Area | Resolution |
|---------|------|------|------------|
| [orchestrator-should-post-review-comment](./orchestrator-should-post-review-comment.md) | enhancement | orchestrator, agents | orchestrator now posts the agent's `Result.Summary` deterministically (marker + truncation); update-or-create idempotency is the small remaining follow-up |
