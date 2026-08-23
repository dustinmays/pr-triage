---
title: Adapters exec subprocesses
status: accepted
date: 2026-08-23
tags: [architecture, adapters, runtime]
---

## Context

Three runtime targets exist: Claude Code, Codex CLI, and OpenCode CLI.
agent-minder (a more elaborate sibling project) runs OpenCode via a
persistent `opencode serve` process driven over SDK + SSE to get exact
cost data, since one-shot `opencode run --format json` is documented by
OpenCode itself as undocumented and churny across 1.x.

## Decision

All three runtimes are treated as plain exec'd subprocesses behind a
common adapter interface (`Invoke(prompt, timeout) -> (result, error)`).
The OpenCode-server route is explicitly skipped for this smaller-scoped
tool.

## Consequences

We accept OpenCode's rougher JSON output and estimated/undocumented cost
behavior as a known limitation rather than building a server-lifecycle
subsystem (port management, shutdown hooks, shared credentials at
deployment time) to work around it. See [[runtime-capability-table]] for
the resulting per-runtime capability differences.
