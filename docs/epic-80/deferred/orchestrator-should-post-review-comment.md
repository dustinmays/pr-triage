---
id: orchestrator-should-post-review-comment
title: "Post the agent's review deterministically from the orchestrator, not the agent"
kind: enhancement
severity: medium
area: orchestrator, agents, observability
found_by: dustinmays
found_in: chunk/scanner-hardening dogfood — run #3 on PR #93 (2026-08-24)
status: fixed
related:
  - ../../../internal/orchestrator/orchestrator.go   # runs the agent, has the Result
  - ../../../internal/github/client.go               # CreateComment already exists (used by escalator)
  - ../../../agents/review-agent.md                   # step 4 asks the agent to post a comment
  - ./agent-permission-mode-hardening.md              # sibling: the permission fix that unblocked verification
---

## What

After the permission fix, run #3's agent ran its full verification toolchain
(make/vet/lint/test/test-race) with zero denials and wrote a thorough review
summary — but it **never posted it to the PR**. The log shows it said "Let me
create a comprehensive summary comment to post on the PR", wrote the text to
`/tmp/review_summary.md`, then ended (end_turn) without ever running
`gh pr comment`. The review lives only in the run log; a human looking at PR #93
sees nothing from pr-triage.

Relying on the agent (`agents/review-agent.md` step 4) to remember and execute a
`gh pr comment` is unreliable — especially on smaller models like haiku.

## Why it matters

The daemon's output is invisible on the PR. For a triage tool, the visible review
comment is much of the point: it's how a human sees "pr-triage reviewed this,
here's what it found / did." No comment ≈ silent operation.

## Recommendation (leaning strongly toward doing this)

Make posting **deterministic in the orchestrator**, not the agent:

- The agent's `Result` already captures its final summary text (the `result`
  field of the stream-json terminal event).
- On a successful routine run, have the orchestrator post that summary as a PR
  comment via the existing `github.Client.CreateComment` (the escalator already
  uses it).
- Reuse the check-run summary's 65KB-style truncation discipline for长 bodies.
- Mark the comment with a machine tag (e.g. `<!-- pr-triage:review -->`) and make
  it idempotent (update-or-create) so re-runs on a new SHA don't spam.
- Keep the agent free to *also* comment on specific lines, but the top-level
  "here's what I did" summary should not depend on the model remembering to.

This decouples "did the agent produce a review" from "did the review reach the
PR", the same way we decoupled report ingestion from an arbitrary check run.

## Resolution (2026-08-24)

Implemented the core recommendation:
- Added `runtime.Result.Summary`; the claude-code adapter now populates it from
  the terminal `result` event (it was parsed but discarded before).
- `orchestrator.postReviewComment` posts the summary to the PR on a successful
  routine run via the existing `CreateComment`, tagged `<!-- pr-triage:review -->`,
  best-effort (a comment failure never fails the run), truncated on a UTF-8
  boundary under GitHub's ~65536-char limit. Test asserts the comment is posted
  with the marker + summary.

**Remaining (small follow-up, not yet done):** true update-or-create idempotency
(list comments, find the marker, update instead of appending) so same-SHA
re-runs don't post duplicates. Needs `ListComments`/`UpdateComment` on the client.
Create-only is acceptable meanwhile because each new push is a new SHA / fresh
review; only same-SHA recovery re-runs can duplicate.
