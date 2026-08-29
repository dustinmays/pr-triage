# Architecture Decision Records

This folder holds the project's ADRs — one file per decision, numbered and immutable.

**An ADR is immutable once merged.** Do not edit a merged ADR to change its meaning or status. If a decision changes, write a *new* ADR that supersedes the old one and note the supersession here in the index. Status tracking lives in the index table below, **not** inside the ADR files.

## Index

This table is the source of truth for ADR status. Keep it in sync when an ADR is added or superseded.

| # | Title | Status |
|---|-------|--------|
| [0000](0000-template.md) | ADR template | Template |
| [0001](0001-go-cli-sqlite-launchd.md) | Delivery shape: Go CLI + SQLite + launchd | Accepted |
| [0002](0002-report-stays-in-cicd.md) | The report stays in CI/CD | Accepted |
| [0003](0003-exec-subprocess-adapters.md) | Adapters exec subprocesses | Accepted |
| [0004](0004-shared-sqlite-schema-repo-id.md) | Shared SQLite schema with repo_id, not table-per-repo | Accepted |
| [0005](0005-risk-tier-routing-in-config.md) | Risk-tier routing lives in config | Accepted |
| [0006](0006-local-state-is-source-of-truth.md) | Local app state is the source of truth; GitHub is a projection | Accepted |
| [0007](0007-manage-human-attention-ai-assists-human-decides.md) | Manage human attention: deterministic-first, AI assists, the human decides | Accepted |
| [0008](0008-portable-agent-definitions.md) | Portable agent definitions: one neutral source, generated per tool | Locked |
| [0009](0009-runtime-adapter-kit.md) | Runtime adapter kit: shared exec, declared capabilities, conformance harness, doctor | Expected |

Status legend: **Locked** — signed off, immutable. **Expected** — pending finalization (note who signs off). **Accepted** — older ADRs merged under the prior status model; treat as Locked. **Template** — the scaffold, not a decision. **Superseded by NNNN** — replaced; keep the file, add the pointer.

## Writing a new ADR

- **Filename:** `NNNN-topic-slug.md` — a zero-padded four-digit number followed by a short slug. Use the next unused number.
- **Title (H1):** combines the number, the topic, and the decision made — e.g. `# 0008 — Portable agent definitions: one neutral source, generated per tool`.
- **Status:** `Locked` (immutable, signed off) or `Expected (<reason and who signs off>)` (pending finalization).
- **Sections:** `Decision` (the choice, plainly), `Why` (bullets, each led by a **bolded claim**), `Alternatives considered` (what was rejected and honestly why), `Current baseline` (concrete file paths / versions so a reader can verify it), and an optional `Open` (deliberately-undecided items; delete if empty).
- Keep it concise. State the decision and the reasoning that would let a future reader understand *why*, not a narrative of how you got there.
- After merging, add the ADR to the index table above. Never rewrite a merged ADR — supersede it with a new one.

## Template

```
# NNNN — <Topic>: <the choice made>

**Status:** Locked | Expected (<reason and who signs off>)

## Decision

<One or two sentences. State the choice plainly.>

## Why

- **<Bolded claim>.** <Explanation.>
- **<Bolded claim>.** <Explanation.>

## Alternatives considered

- **<Option rejected>** — <why, honestly.>

## Current baseline

<Versions, file paths, enough detail for verification.>

## Open

<Optional. Anything deliberately left undecided. Delete if empty.>
```
