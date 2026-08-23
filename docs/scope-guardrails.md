---
title: Scope guardrails
tags: [scope, architecture]
related: [[hard-fail-philosophy]], [[worktrees-are-core]]
source: plan.md
---

Explicitly out of scope for this tool: dependency-graph resolution
between jobs, multi-stage pipelines, any "lesson-learning"
self-improvement system, a Unix-socket control API, and the OpenCode
persistent-server route (`opencode serve` + SDK/SSE) — all three runtime
adapters are plain exec'd subprocesses instead. This tool stays
PR-triage-scoped, not a general agentic workflow platform.
