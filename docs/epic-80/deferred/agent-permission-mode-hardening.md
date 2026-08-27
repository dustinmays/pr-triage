---
id: agent-permission-mode-hardening
title: "Autonomous agent uses bypassPermissions; tighten to a scoped allowlist"
kind: enhancement
severity: medium
area: runtime, security
found_by: dustinmays
found_in: chunk/scanner-hardening dogfood — run #2 on PR #93 (2026-08-24)
status: open
related:
  - ../../../internal/runtime/claudecode/claudecode.go   # BuildArgs adds --permission-mode
  - ../../../agents/review-agent.md                        # agent def step 4: post a summary comment
  - ../STATE.md
---

## Context

Run #2's log showed the daemon-spawned agent hit `permission_denials` for
`make all`, `golangci-lint run`, and `gofmt -l .`, and never posted its review
comment — because the adapter invoked `claude` in the default permission mode
with no interactive approver, so non-allowlisted Bash was auto-denied. The agent
was effectively read-only yet still concluded "all 136+ tests pass, approved for
merge" — a **miscalibrated claim it could not actually verify.**

Immediate fix (landed): `BuildArgs` now passes `--permission-mode bypassPermissions`
so the unattended agent can run its toolchain, commit/push, and comment.

## Why this is still a finding

`bypassPermissions` is the blunt instrument. The agent can run *any* command in
the worktree. Mitigating context: it runs in an isolated git worktree and risky
changes escalate to a human before the agent is ever invoked. But we can do
better:

## Options (not decided)

- **Scoped allowlist** instead of full bypass: `--allowedTools` limited to
  `Bash(make:*) Bash(go:*) Bash(git:*) Bash(gh:*) Bash(golangci-lint:*)
  Bash(gofmt:*) Read Edit Write`. Safer, but brittle as the toolchain grows.
- **Make the permission mode configurable** per repo/routing tier (routine may
  warrant less than a full-fix tier).
- **Agent-prompt fix**: the agent must not claim verification it was blocked from
  running — report "could not run X" rather than asserting success. (Partly moot
  once it can actually run them, but good defensiveness.)
- Consider a network-restricted sandbox for the worktree.

Leaning toward: scoped allowlist + configurable mode once the toolchain stabilizes.
