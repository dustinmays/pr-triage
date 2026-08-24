---
title: Worktrees are core
tags: [worktrees, git, architecture]
related: [[github-dedup]]
source: plan.md
---

The agent's job is not read-only: it should make easy fixes directly
(commit/push to the PR branch), attempt thorough changes when warranted,
and escalate to a human when a change is unavoidable but out of its
judgment/authority. That means git worktrees are load-bearing, not
optional — each agent invocation runs in its own isolated worktree.

Stale worktrees are pruned on an age basis, default 72 hours,
configurable. Prune via `git worktree remove` (falling back to `--force`
plus `git worktree prune` if the directory was already hand-deleted),
never a raw `rm -rf`, to keep git's internal worktree metadata
consistent.
