---
title: "Human override for escalated PRs — design & decision"
status: decided
date: 2026-08-26
tags: [design, escalation, override, state, cli]
related:
  - ../../adr/0006-local-state-is-source-of-truth.md
  - ../deferred/per-chunk-triage-config.md
  - ../deferred/escalated-state-overwritten-by-ci-failed.md
  - ../deferred/escalation-comment-lacks-trigger-reason.md
---

# Human override for escalated PRs

## Decision

**Build a local, state-first `pr-triage override` CLI command (Option C below) as
the primary mechanism, with a GitHub `triage-override` label (Option A) as a
fast-follow that reuses the same plumbing.** Defer the comment slash-command and
approving-review flavors. This follows [[0006-local-state-is-source-of-truth]]:
the human tells the local app, local state changes, and GitHub is reconciled as a
projection.

"Override" means **"proceed to run the probabilistic review agent"** (which may
still fix or flag things), NOT "skip straight to mergeable." It waives **specific
signals**, pinned to a **specific head SHA**.

## Problem

When a pre-scan signal maps to the `escalate` tier, the daemon labels the PR
`needs-owner-review`, the `owner-review-gate` check goes red, and no agent runs —
it's handed to a human. There is no clean way for the owner to say "I looked, the
flag is acceptable, proceed with the review." Removing the label does nothing:
the daemon won't re-evaluate an unchanged head SHA, and re-evaluation would
re-escalate deterministically (the label is an *output* of escalation, not an
*input* to routing). And we don't want to restart the daemon to change behavior.

## Core mechanism (shared by all front doors)

Record an **override record in the store**, keyed to
`(repo_id, pr_number, head_sha, waived_signals[])`. The orchestrator consults it
in `HandleReportReady` *before* the escalate branch: if a matching override
exists for the observed head SHA, treat the waived signals as absent, so
`Classify`/`Route` yield the normal `routine` lane and the review agent runs. The
front-door options differ only in how that record gets created.

## Options surveyed

Grounded in established practice — Prow (`/override`, `/ok-to-test`, `/lgtm`
label), bors (`r+`), Atlantis (`apply`), Mergify commands, Renovate (a *polling*
bot that drives off labels/checkboxes because it has no webhook), and GitHub-native
required-reviews / branch-protection.

| # | Human trigger | Daemon detects it (no restart) | Extra GitHub calls | SHA precision | Cost |
|---|---|---|---|---|---|
| **A. Label** (`triage-override`) | add a label in GitHub | reads `pr.Labels` — already fetched every poll | **none** | good (poll-interval race) | low |
| **B. Comment command** (`/triage proceed`) | comment on the PR | new `ListComments`/PR/poll + parser | +1/PR/poll | excellent | med-high |
| **C. Local CLI** (`pr-triage override <pr>`) | run a local command | reads the store row it wrote | **none** | **exact** | low-med |
| **D. Approving review** | click "Approve" | new `ListReviews`/PR/poll | +1/PR/poll | good | med |

**Why C is primary:** it's the codebase's native control-plane/worker idiom (the
CLI already opens the store the daemon reads, cf. `internal/cli/status.go`), adds
**zero** GitHub API traffic, captures the **exact** current head SHA at command
time (no label race), and is trivially secure for a solo user (local FS access =
authority). Small footprint: one Cobra subcommand + one store method + one lookup
in `HandleReportReady`.

**Why A is the fast-follow:** once C's record + classifier-gate + reconcile
plumbing exist, the label reuses all of it and only reads `pr.Labels` (already
in-hand every poll) — the GitHub-native, phone-from-anywhere trigger with no added
API calls. Direct precedent: Prow's `ok-to-test`/`lgtm` labels.

**Why B/D are deferred:** B (comments) adds per-PR API traffic, a mini-command
parser, self-comment-loop guards, and the "who is authorized" problem that only
matters once you're not the sole committer. D (approving review) conflates
"approve the code" with "authorize the robot" — and you may want the robot
*because* you haven't fully vetted the code.

## Cross-cutting design answers

- **SHA-pinning (required).** The human judged *specific code*. Key the override to
  the head SHA and consult it only when the observed SHA matches. Extend the
  existing new-push reset (`ci_running`) to also delete/expire the override and
  strip the `triage-override` label, so approval never bleeds onto new code. Waive
  *specific* signals, not a blanket bypass, so a *different* escalate signal on new
  code still escalates.
- **Semantics.** Override → run the review agent (lane = routine), not
  merge-by-fiat. Strictly safer than Prow `/override` (which just forces a check
  green). The agent's summary should still surface the waived sensitive change
  (honest escalation — cf. [[escalation-comment-lacks-trigger-reason]]).
- **Break the re-escalation loop.** Gate `Classify` on the override (waived signals
  treated as absent). Requires fixing [[escalated-state-overwritten-by-ci-failed]]:
  make `escalated` a terminal state for its SHA, with a single exit edge —
  `escalated + matching override → re-enter the report path once → agent_running`,
  then `done`/`failed`; mark the override consumed so it fires exactly once.
- **Reconcile the red `owner-review-gate`** inside the override handler (remove the
  `needs-owner-review` label / flip the check green), never as something the human
  must remember. Given the "proceed" semantics, flip the gate green on accepting
  the override and let the agent run — the human has taken responsibility for the
  waived signal; the agent is added assurance, not a second gate.
- **vs. per-chunk config.** Complementary. [[per-chunk-triage-config]] is *standing
  policy* ("in this chunk, `workflow_changed` is always routine"); the override is
  an *ad-hoc, one-PR, one-SHA* exception. Rule of thumb: if you'd waive it every
  time in this chunk, put it in chunk config; if it's a one-off judgment call,
  override.

## Implementation sketch (Option C)

- **Store:** an `overrides` table `(repo_id, pr_number, head_sha, waived_signals,
  created_at, consumed_at)`; methods `RecordOverride`, `GetActiveOverride(repo,pr,sha)`.
- **CLI:** `pr-triage override <pr> [--signal <id> ...]` (default: waive all
  present escalate signals). Opens the store like `status`, resolves the PR's
  current head SHA via `GetPRHeadSHA`, inserts the row.
- **Orchestrator:** in `HandleReportReady`, load the override for `(pr, headSHA)`
  before the escalate branch; subtract waived signals; reconcile the gate/label on
  accept; mark consumed after the run.
- **Poller:** add `escalated` to the Case-3 terminal set; on new head SHA, clear
  overrides + strip the label.

## Key files

`internal/orchestrator/orchestrator.go` (`HandleReportReady` — classifier gate),
`internal/escalate/escalate.go` (idempotency + label/comment),
`internal/poller/poller.go` (`ProcessPR` Case 3 + new-push reset),
`internal/config/config.go` (`Classify`/`Route`),
`internal/cli/status.go` (store-backed subcommand pattern),
`internal/db/schema.go` (add `overrides` table),
`internal/github/client.go` (`GetPRHeadSHA`, `AddLabels` present; `ListComments`/
`ListReviews` only needed for B/D).

_Source: design research memo, 2026-08-26._
