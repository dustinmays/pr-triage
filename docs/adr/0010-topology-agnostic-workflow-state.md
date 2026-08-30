# 0010 — Chunk workflow state is topology-agnostic; GitHub is a projection

**Status:** Expected (drafted from the issue #129 dogfood; pending sign-off by Dustin)

## Decision

Represent the chunk lifecycle as explicit, provider-independent control-plane state.
GitHub branches, pull requests, draft status, comments, and CI checks are projections and
evidence for that state; they are not the only place the state lives and must not define
the workflow's semantics.

The durable record must be able to identify the chunk and work item, the charter and
behavioral-contract version, ratification and protected-artifact checkpoints, the current
phase and whether RED or GREEN is expected, work ownership/dependencies, independent
verification, and reactive-review/escalation outcomes. Provider references such as PR
number, base/head refs, and check-run IDs attach to that record without becoming the
record itself.

Do not require one Git topology. A draft feature/chunk PR to `main` with child issue PRs
targeting the feature branch is a useful projection: the parent can begin visibly RED and
progress toward GREEN as children merge. A pre-PR RED → implementation → GREEN flow is
also valid. AgentMinder/Trigger or a successor may select and orchestrate either shape;
the front-of-loop contract and pr-triage's reactive boundary must work with both.

At the GitHub adapter boundary, discovery must eventually support a set of base-ref
selectors (including exact names, globs, or all) rather than forcing one base ref per
repository. Review intent must not depend solely on a branch-name convention when an
explicit chunk/work-item role is available. The exact configuration syntax and ownership
of the broader orchestration record remain open.

## Why

- **Expected RED is workflow state, not generic breakage.** A failing draft can be an
  excellent visible cue during contract-first development, but only an explicit phase and
  ratified checkpoint distinguish “failing for the agreed reason” from a regression.
- **GitHub is useful without becoming mandatory.** Draft PRs, stacked child PRs, and CI
  provide a strong shared paper trail, while a local or future provider workflow can
  preserve the same contract, gates, and evidence without reconstructing truth from
  GitHub. This extends [[0006-local-state-is-source-of-truth]] across the chunk lifecycle.
- **Topology is a team choice.** The current two-level feature stack fits chunk work well,
  but baking it into charter, test, discovery, or review semantics would unnecessarily
  constrain other teams and future AgentMinder/Trigger workflows.
- **The front and back need a stable interface.** The front produces a ratified charter,
  protected RED checkpoint, and phase evidence; the back consumes scope, contract
  integrity, verification, and review intent. The middle may change without forcing those
  boundaries to change.
- **Multiple watched bases are a real operational need.** A daemon may need to observe
  both a default-branch completion PR and child implementation PRs targeting a named
  feature/chunk branch. “Watch everything” is a useful temporary mode, not always an
  acceptable permanent substitute.
- **Attention remains bounded.** The orchestrator can consume state transitions and
  bounded verdicts instead of importing agent transcripts or duplicating routine code
  review, consistent with [[0007-manage-human-attention-ai-assists-human-decides]].

## Alternatives considered

- **Make the two-level draft PR stack the required architecture** — rejected. It is a
  productive current workflow and should be supported well, but it is a projection rather
  than the durable contract.
- **Treat GitHub PRs and CI as the workflow database** — rejected. Expected-RED state,
  ratification history, protected checkpoints, and agent dependencies cannot be inferred
  reliably from a changing PR head, and provider availability should not determine
  whether the process knows its own state.
- **Keep one `base_ref` and reconfigure it between phases** — acceptable as a temporary
  operator workaround, but it creates discovery gaps and makes concurrent parent/child
  observation impossible.
- **Always watch every open PR** — acceptable for the current dogfood, but potentially
  noisy and costly in a repository with unrelated work.
- **Encode several bases in a clever glob or comma-delimited string** — not selected.
  The capability should be an explicit collection; serialization syntax can be decided
  with the configuration/schema change.

## Current baseline

- [[0006-local-state-is-source-of-truth]] already makes pr-triage's SQLite state
  authoritative and GitHub a one-way projection for the reactive loop. This ADR extends
  that separation to the front/middle/back chunk lifecycle and its expected-RED state.
- `internal/config.Config.BaseRef`, `internal/db.Repo.BaseRef`, and
  `internal/poller.GitHubClient.ListOpenPRs` accept one string. The GitHub client treats it
  as one exact ref or `path.Match` glob; an empty string returns all open PRs.
- `internal/db.UpsertRepo` keys registrations by `(owner, name)`, so registering the same
  repository with another base ref overwrites the existing selection.
- `scripts/pr-prescan.sh` currently infers `chunk_completion` from a `main` base and
  `implementation` from other bases, coupling review role to topology.
- `docs/chunk-kickoff-behavioral-testing-spike.md` already calls the one-active-base-ref
  constraint an open concurrent-multi-chunk question.
- The issue #129 experiment used a ratified RED commit followed by a GREEN implementation
  commit before opening PR #133; the human's normal workflow also supports a draft parent
  PR that remains visibly RED while child implementations land. See
  `docs/experiments/issue-129-front-of-loop-report.md`.

## Open

- Which durable fields live in pr-triage versus AgentMinder/Trigger, and what narrow
  interface carries charter/checkpoint/phase/review state between them?
- What configuration shape represents multiple base selectors, and should selection be
  per repository, per workflow registration, or both?
- How is explicit review intent conveyed when branch names are not authoritative?
- How should an expected-RED phase appear in local status, CI, and human-facing
  projections without weakening ordinary GREEN merge gates?
