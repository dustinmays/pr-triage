---
id: agent-can-escape-worktree
title: "Review agent can cd out of its isolated worktree into the main checkout"
kind: enhancement
severity: medium
area: runtime, orchestrator, security
found_by: dustinmays
found_in: chunk/scanner-hardening — #116 review-prompt root-cause (2026-08-27)
related:
  - ../../../internal/orchestrator/orchestrator.go   # buildReviewPrompt instructs "stay in this worktree"
  - ../../../internal/runtime/claudecode/            # adapter sets Workdir but doesn't confine the process
status: open
---

## What

The review agent runs in an isolated git worktree (`inv.Workdir = worktreePath`),
but nothing confines it there. In the #116 dogfood (PR #115) the agent ran
`cd /Users/dustin/repos/pr-triage` and reviewed the **main checkout** (stale at
an old commit) instead of the PR head in its worktree — producing a confused,
wrong-context review.

#116 mitigates this at the PROMPT level: `buildReviewPrompt` now tells the agent
"Stay in this worktree; do not cd elsewhere." That is a soft guard — a
probabilistic instruction the agent can still ignore.

## Why it matters

The whole isolation model (agent fixes code in a throwaway worktree, we
commit/push from it) assumes the agent operates ON that worktree. An agent that
`cd`s to the real repo could review/modify the wrong tree, read unrelated state,
or (with bypassPermissions) touch files outside the sandbox.

## Options (not decided)

- Run the adapter process with the worktree as a hardened boundary (e.g. a
  restricted cwd/tool allowlist that rejects absolute paths outside Workdir).
  Pairs with `agent-permission-mode-hardening`.
- Pass the worktree path explicitly and have the agent def forbid absolute-path
  navigation; assert in the agent's own summary that it stayed in-tree.
- Detect post-hoc: if the agent's changes/log reference paths outside the
  worktree, flag the run.

Follow-up to #116; the prompt-level guard is sufficient for now.
