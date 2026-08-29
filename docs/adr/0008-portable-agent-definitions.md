# 0008 — Portable agent definitions: one neutral source, generated per tool

**Status:** Locked

## Decision

Each agent is defined once in a tool-agnostic source file, `agents/<name>.agent.yaml`, which is the single source of truth. Every tool's native format — Claude Code (`.claude/agents/<name>.md`), OpenCode (`.opencode/agents/<name>.md`), and a reserved Codex slot — is **generated** from that source by `cmd/agent-sync`. A validate-only CI job (`make agents-check`) regenerates in memory and **fails any PR whose committed files have drifted** from the source; humans reconcile with `make agents-sync`.

## Why

- **Hand-duplicated agent files drift.** Before this, `agents/*.md` and `.claude/agents/*.md` were byte-identical copies maintained by hand — two places to edit, guaranteed to diverge. One source removes the class of bug entirely.
- **Drift is a hard failure, not a review-time catch.** Generation alone doesn't prevent someone editing a generated file directly; the `agents-check` CI gate makes any divergence red on the PR, so the source stays authoritative without relying on discipline.
- **Tool constraints are encoded once, in the generator.** Two OpenCode facts verified empirically against opencode 1.18.21 are baked in: project agents are read from `.opencode/agents/` (plural), and `opencode run --agent <name>` only applies an agent whose `mode: all` (or `primary`) — `subagent` mode is silently ignored there. Encoding these once means every generated OpenCode agent is correct by construction.
- **New tools slot in without re-authoring agents.** Adding Codex (or any future runtime) is a new renderer, not a rewrite of every agent.

## Alternatives considered

- **Treat Claude Code `.md` as canonical, generate the others from it** — rejected. It privileges one tool's format as the source and couples the "universal" concept to Claude Code's frontmatter. A tool-agnostic YAML source keeps every target on equal footing.
- **Auto-sync workflow (a bot regenerates and commits back to the PR branch)** — rejected in favor of validate-only. No workflow writes to contributor branches; drift is the author's to fix deterministically via `make agents-sync`, with no surprise bot commits.
- **Leave the hand-maintained duplicates** — rejected; that is precisely the drift this decision eliminates.

## Current baseline

- **Source:** `agents/<name>.agent.yaml` — fields `name`, `description`, `tools` (canonical Claude tool names), `model` (optional, may be empty), `mode`, `prompt`. Four agents today: `default`, `review-agent`, `security-expert`, `senior-review`.
- **Generator:** `internal/agentsync` (`LoadAll`, `RenderClaude`, `RenderOpenCode`, `Sync`, `Check`; deterministic/sorted output) + `cmd/agent-sync` (`-check` validate mode). Codex is a commented `renderCodex` stub that generates nothing.
- **Targets:** `.claude/agents/<name>.md` and `.opencode/agents/<name>.md` (always `mode: all`, tools mapped to OpenCode's lowercase names).
- **Tooling:** `make agents-sync` / `make agents-check`; `.github/workflows/agent-sync.yml` runs the check on every PR. Shipped in #125 / #127. Prose reference: `docs/portable-agents.md`.

## Open

- **Codex target is unimplemented** (reserved stub) — pending a real Codex adapter and its verified agent format.
- **Per-agent model vs. routing override:** an agent's `model` field can carry a per-agent model, but the daemon's `routing.<tier>.model` overrides it when set (the OpenCode adapter omits `-m` when the routing model is empty, deferring to the agent's own model). Documented in `docs/opencode-runtime.md`.
