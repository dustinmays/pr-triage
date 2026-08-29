---
title: Portable agent definitions
tags: [agents, tooling, ci]
related: [[runtime-capability-table]]
source: plan.md
---

Each agent has exactly one source of truth: `agents/<name>.agent.yaml`, a
tool-agnostic definition (`name`, `description`, `tools`, `model`, `mode`,
`prompt`). Every tool-specific agent file — `.claude/agents/<name>.md`,
`.opencode/agents/<name>.md` — is generated from it by
`internal/agentsync` (invoked via `cmd/agent-sync`). Hand-editing a
generated file is pointless: the next sync overwrites it.

Run `make agents-sync` to regenerate all generated files from source after
editing any `agents/*.agent.yaml`.

The sync is validate-only in CI: `.github/workflows/agent-sync.yml` runs
`make agents-check`, which regenerates in memory and fails the PR if any
generated file is missing or differs from source (drift). It never writes
anything back — a human runs `make agents-sync` locally and commits the
result.

Two OpenCode facts drove the generator's shape (see
[[runtime-capability-table]] for the full per-runtime table):

- OpenCode agent files live at `.opencode/agents/<name>.md` — plural
  "agents", unlike Claude Code's `.claude/agents/<name>.md`.
- `opencode run --agent <name>` only applies an agent at the top level when
  its frontmatter has `mode: all` (or `primary`); `mode: subagent` is
  silently ignored. So every generated OpenCode agent emits `mode: all`
  regardless of the source's `mode` field, which is reserved for future
  subagent support.

Codex is a reserved target: `agentsync.Targets` and the `renderCodex` stub
exist in code, but no Codex agent file is generated yet. Adding Codex
support later means implementing `renderCodex` and wiring its `Target`
into `Targets` — the neutral `agents/*.agent.yaml` sources should not need
to change.
