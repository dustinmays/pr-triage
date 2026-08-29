---
title: Knowledge base index
tags: [index]
related: []
source: plan.md
---

Institutional knowledge for this project, decomposed into small,
single-fact markdown files with YAML front matter (`title`, `tags`,
`related`, `source`) so agents and humans can find and cross-link them.

## Facts

- [Cost-basis honesty](cost-basis-honesty.md)
- [Result-shape normalization](result-shape-normalization.md)
- [Config resolve-once](config-resolve-once.md)
- [Runtime capability table](runtime-capability-table.md)
- [OpenCode runtime](opencode-runtime.md)
- [Persistence discipline](persistence-discipline.md)
- [GitHub dedup](github-dedup.md)
- [Worktrees are core](worktrees-are-core.md)
- [Hard-fail philosophy](hard-fail-philosophy.md)
- [Scope guardrails](scope-guardrails.md)
- [Portable agent definitions](portable-agents.md)

## Architecture Decision Records

- [0000 - ADR template](adr/0000-template.md)
- [0001 - Delivery shape: Go CLI + SQLite + launchd](adr/0001-go-cli-sqlite-launchd.md)
- [0002 - The report stays in CI/CD](adr/0002-report-stays-in-cicd.md)
- [0003 - Adapters exec subprocesses](adr/0003-exec-subprocess-adapters.md)
- [0004 - Shared SQLite schema with repo_id](adr/0004-shared-sqlite-schema-repo-id.md)
- [0005 - Risk-tier routing lives in config](adr/0005-risk-tier-routing-in-config.md)
- [0006 - Local app state is the source of truth; GitHub is a projection](adr/0006-local-state-is-source-of-truth.md)
- [0007 - Manage human attention: deterministic-first, AI assists, the human decides](adr/0007-manage-human-attention-ai-assists-human-decides.md)
- [0008 - Portable agent definitions: one neutral source, generated per tool](adr/0008-portable-agent-definitions.md)
