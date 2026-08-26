---
title: "Epic 80 — Scanner hardening & Swift support: build state"
kind: build-state-log
epic: 80
epic_issue: "#80"
chunks:
  - { id: A, issue: "#81", title: "Test harness & golden fixtures" }
  - { id: B, issue: "#82", title: "Swift/SwiftBar detection & signals" }
  - { id: C, issue: "#83", title: "Robustness & edge-case hardening" }
chunk_branch: chunk/scanner-hardening
owner: dustinmays
status: in-progress
updated: 2026-08-23
# One curated file, single-writer (the chunk owner). Worker agents READ this for
# context; they do not write it. Ad-hoc "found it broken, not now" items go to
# ./deferred/ instead (one file per finding, collision-free).
related:
  - ./deferred/README.md            # the deferred-findings backlog for this epic
  - ../../internal/config/config.go # config Load/Classify/Route (hardened this session)
  - ../../internal/db/schema.go     # db.Open (hardened this session)
  - ../live-trial-runbook.md        # the dogfood runbook this epic is exercised through
---

# Epic 80 — Scanner hardening & Swift support

The single shared "where is the build" doc for this epic. It exists because when
several agents (Claude, Gemini, OpenCode) work a chunk in separate worktrees
there is no shared memory — a private per-agent memory can't be read by the
others. This file is the shared, in-repo version. **The chunk owner is the only
writer.** Everyone else reads it for context and files noticed-but-deferred items
into [`./deferred/`](./deferred/README.md).

## Goal

Harden the deterministic pre-scan scanner and add first-class Swift / SwiftBar
support, dogfooded live through the running `pr-triage` daemon on this repo's own
PRs into `chunk/scanner-hardening`.

## Chunk status

| Chunk | Issue | Title | Status |
|-------|-------|-------|--------|
| A | [#81](https://github.com/dustinmays/pr-triage/issues/81) | Test harness & golden fixtures | not started |
| B | [#82](https://github.com/dustinmays/pr-triage/issues/82) | Swift/SwiftBar detection & signals | not started |
| C | [#83](https://github.com/dustinmays/pr-triage/issues/83) | Robustness & edge-case hardening | in progress (dogfood-surfaced fixes landing) |

Sub-issues: A → #84/#85/#86, B → #87/#88/#89, C → #90/#91/#92. Build order A → B → C
(B and C both need A.1's harness first).

## Log

### 2026-08-23 — Dogfood setup surfaced two robustness bugs (Chunk C territory)

Standing up the daemon against `chunk/scanner-hardening` for the first time
immediately exposed two real hardening bugs — which is exactly the value the
scanner-hardening epic is meant to prove out:

1. **`config.Load` didn't merge defaults.** The partial config `init` writes
   (no `signal_tiers`/`routing`/`worktree_ttl`) loaded with an empty `Routing`
   map, so every PR hard-failed to `escalate` via `ErrUnmappedTier` — the
   routine-agent path was dead. Fixed: `Load` now layers the file over
   `DefaultConfig()`. Also corrected stale default model
   `claude-3-5-haiku` → `claude-haiku-4-5`. Regression test added.
2. **`db.Open` didn't create its parent dir.** A fresh `~/.pr-triage` gave the
   cryptic `unable to open database file (14)` — this blocked `init` outright.
   Fixed: `Open` now `MkdirAll`s the DB directory, so `init`/`run`/`status`/
   `checkout` all self-heal on first run.

Both fixes + this knowledge-base scaffold are folded into the first dogfood PR
into `chunk/scanner-hardening`. Deferred (not fixed now): see
[`./deferred/`](./deferred/README.md) — the ignored top-level `config.model`,
the opaque partial config `init` writes, and the two skills we want to build to
codify this very workflow.

### 2026-08-24 — Live daemon run stuck at report_ready → third real bug (report check-run selection)

Started the daemon against `chunk/scanner-hardening`; PR #93 walked to
`report_ready` then **stuck** — no `runs` row, sat past two poll cycles. Root
cause (load-bearing): the poller emitted `report_ready` with an **arbitrary**
check-run ID (`evaluateCheckRuns` returned whichever check run was last in the
array), and the orchestrator fetched the report from *that* check run's output.
The report JSON lives only in the dedicated `pr-prescan-report` check run, so the
fetch hit the wrong check → empty summary → `ErrMissing` → handler returned err →
PR stranded in `report_ready` (the poller won't re-emit for an already-`report_ready`
PR). It only ever worked in tests with a single mocked check run; **in any repo
with multiple checks the daemon could never ingest a report.**

Fixed: added `report.ReportCheckName = "pr-prescan-report"`; the poller now
resolves the report check run *by name* (`reportCheckRunID`) and only emits
`report_ready` once that check exists (otherwise it keeps waiting). Added tests:
picks the report check among many (integration path), and stays pending when
gating is green but the report check is absent. Full suite + lint green.

This coupling to a named check run is itself fragile — captured as deferred:
[report-check-name-coupling-fragile](./deferred/report-check-name-coupling-fragile.md)
(should escalate, not silently `ci_failed`, when the report check never appears)
and [workflow-install-command](./deferred/workflow-install-command.md) (a
`pr-triage workflow` command to install/ensure the pre-scan job). Both fold into
PR #93.

### 2026-08-24 — Routine path reached, then failed: runtime adapter never registered (4th bug)

With the report-fetch fix, the fixed daemon re-triaged #93 correctly: report
ingested, classified **routine**, agent invoked — proving the config + report
fixes work end-to-end. But the run **failed** immediately:
`unregistered runtime "claude-code" ... (registered: [])`. The registry was
empty. The claude-code adapter self-registers in `init()`, but that only runs if
its package is imported into the binary — and **nothing in non-test code imported
`internal/runtime/claudecode`**. Tests import it directly, so they passed; the
real daemon never registered any adapter and every run would fail.

Fixed: blank import `_ ".../internal/runtime/claudecode"` in `cmd/pr-triage/main.go`
(conventional for self-registering adapters; future codex/opencode go there too).
Guard: `cmd/pr-triage/main_test.go` asserts `runtime.Get("claude-code")` succeeds —
it lives in package main, so dropping the blank import fails the test. This push
gives #93 a new SHA, resetting it from the stranded `agent_running` state so the
fixed daemon re-triages it. Expected next: routine → review-agent runs to `done`
with `cost_basis=exact`.

### 2026-08-24 — Routine path GREEN end-to-end; but agent was running half-crippled (5th bug)

Run #2 on PR #93 completed: `state=done`, `runtime=claude-code`, **`cost_basis=exact`,
`cost_usd=$0.1097`, `turns=35`, `stop_reason=end_turn`**. Worktree cleaned up, no
orphans. The full pipeline works: poll → CI wait → report ingest → classify routine
→ registered adapter → agent run → done with exact cost. **Headline routine path
proven live.**

BUT reading run-2.log revealed the agent was effectively **read-only**: it hit
`permission_denials` for `make all`, `golangci-lint run`, `gofmt -l .`, and never
posted its review comment. Cause: the adapter spawned `claude` with no
`--permission-mode`, so it defaulted to `default` with no interactive approver →
non-allowlisted Bash auto-denied. The agent produced an excellent review summary
(as terminal text, not a PR comment) and claimed "all tests pass, approved" — a
claim it could not actually verify because its verification commands were blocked.

Fixed: `BuildArgs` now passes `--permission-mode bypassPermissions` (the daemon is
unattended; the agent works in an isolated worktree and risky changes escalate
before reaching it). Tests updated. Follow-up captured:
[agent-permission-mode-hardening](./deferred/agent-permission-mode-hardening.md)
(tighten to a scoped allowlist / make configurable; agent shouldn't claim
verification it was blocked from running).

**Scorecard: 5 real bugs found+fixed via live dogfood, none caught by the test
suite** — config defaults, db dir, report-check-by-name, adapter registration,
agent permission mode. Next restart should show the agent actually running its
toolchain and posting a comment on the PR.

### 2026-08-24 — Permission fix confirmed; agent verifies for real but doesn't post its review

Run #3 on PR #93: `done`, exact `$0.082`, 30 turns, end_turn. **`permission_denials: []`** —
the agent ran its full toolchain for real (make all/vet/lint/test/test-race/build,
golangci-lint) with zero denials. The permission fix is confirmed working.

Remaining gap (not a pipeline blocker): the agent wrote a thorough review summary
but **never posted it to the PR** — it said it would, wrote the text to
`/tmp/review_summary.md`, then ended without running `gh pr comment`. The review
lives only in the run log; PR #93 shows nothing from pr-triage. Relying on the
model to remember `gh pr comment` is unreliable. Captured as
[orchestrator-should-post-review-comment](./deferred/orchestrator-should-post-review-comment.md):
the orchestrator should post the agent's captured `Result` summary deterministically
via the existing `CreateComment`, idempotently, rather than depending on the agent.

**Pipeline status: fully proven.** Routine path is green end-to-end with exact
cost. Open polish items are all deferred (post-review-comment, permission
allowlist, model-ignored, report-check fragility, install command). Good point to
decide: implement deterministic comment posting next, or merge #93 and move to the
actual Chunk A harness work (#84).

### 2026-08-24 — Deterministic review posting (agent=judgment, harness=delivery)

Decided the "agent didn't post its review" gap is an *architecture* problem, not
an intelligence/instructions/skill problem: any step that must happen every time
must not live in the agent's turn budget. Principle: **agent = judgment; harness =
guaranteed side effects.** Implemented it:

- `runtime.Result.Summary` added; claude-code adapter now populates it from the
  terminal `result` event (it was parsed but thrown away before).
- `orchestrator.postReviewComment` posts the summary on a successful routine run
  via the existing `CreateComment`, tagged `<!-- pr-triage:review -->`,
  best-effort (never fails the run), UTF-8-safe truncation under GitHub's limit.
  Test asserts marker + summary are posted.

Delivery no longer depends on the model remembering `gh pr comment`. Remaining
small follow-up: update-or-create idempotency (needs ListComments/UpdateComment).
Daemon was stopped by the user, so this landed without triggering a run. Not yet
verified live — next live run should show a `<!-- pr-triage:review -->` comment on
the PR.

### 2026-08-24 — Deterministic posting VERIFIED live; agent dedup; #93 ready to merge

Root cause of the earlier silent no-post: the daemon's keyring token was
read-only, so the Go client's `CreateComment` 401'd — and `postReviewComment`
swallowed the error. Fixed both: un-silenced the post (logs to stderr / Terminal 1
on failure); user reset the token to a write-capable fine-grained PAT (Pull
requests RW, Issues RW, Contents RW, Checks R, Metadata R). Run #6 then posted the
**`<!-- pr-triage:review -->`** comment for real — deterministic delivery by the
harness confirmed end-to-end (exact cost $0.027).

Observed along the way: the agent ALSO self-posts via bypassPermissions+gh, but
inconsistently (posted on #4/#5/#6, not #3) — the exact reliability reason the
harness must own delivery. To avoid duplicate comments, stripped the "post a
comment" step from BOTH `agents/review-agent.md` and `.claude/agents/review-agent.md`
(the daemon resolves `--agent` from `.claude/agents/`): the agent now ends with its
summary as its final message and does NOT run gh; the orchestrator is the sole
poster. Deferral path reworded to explain in the final summary rather than
comment.

**Pipeline COMPLETE and verified. #93 is ready for human review + merge into
chunk/scanner-hardening.** 7 bugs found+fixed via live dogfood, none caught by
tests: config defaults, db dir, report-check-by-name, adapter registration, agent
permission mode, review delivery (agent→orchestrator), read-only-token+silent-fail.

### 2026-08-25 — Wave 1 built + live-triaged; escalation dominates infra work

Two autopilot agents built Chunk A/C sub-issues cleanly: **PR #95** (#84 —
prescan test harness + `make prescan-test` + `test_files_deleted` reference) and
**PR #94** (#90 — shellcheck-clean scanner + shellcheck CI job). Both green on all
functional CI. The rebuilt daemon (from this chunk branch) triaged both correctly
and **escalated** them: #94 on `workflow_changed` (adds a CI job), #95 on
`safeguard_config_changed` (edits the Makefile) → `needs-owner-review` label →
`owner-review-gate` red → merge blocked, awaiting owner review.

Key operational insight: this chunk's work IS infra (CI/Makefile/scanner), so
nearly every PR trips an escalate signal and routes to human — the routine
auto-review lane barely applies. Autonomous drive-to-completion is therefore
blocked on owner review (Wave 2 #85/#86 need #84's harness merged first). Paused
here per the "stop on real difficulty" instruction.

Four findings captured to deferred/ from this run:
[per-chunk-triage-config](./deferred/per-chunk-triage-config.md) (high — chunk-owner
signal→tier overlay so expected changes stay routine; belongs in pr-triage itself),
[escalation-comment-lacks-trigger-reason](./deferred/escalation-comment-lacks-trigger-reason.md)
(name the signal+evidence in the escalation comment, not "escalate tripped"),
[escalated-state-overwritten-by-ci-failed](./deferred/escalated-state-overwritten-by-ci-failed.md)
(escalated → ci_failed drift), and
[status-shows-internal-pr-id](./deferred/status-shows-internal-pr-id.md) (status prints
runs.pr_id not the GitHub number).

## Conventions in play

- **STATE.md (this file):** single-writer = chunk owner; curated; updated at
  milestones.
- **[`deferred/`](./deferred/README.md):** one file per finding, descriptive
  kebab slug (never sequential numbers — two agents would race the same number);
  agents only create files, never edit a shared index, so git can't conflict.
  Fixing a finding flips `status:` in its own file.
- **[`transfer-out.md`](./transfer-out.md):** owner-curated checklist of
  conventions/artifacts to graft into the template/seed repo so future projects
  start chunk-ready (distinct from `deferred/`, which is "fix in pr-triage").
