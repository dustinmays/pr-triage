---
id: skill-defer-finding
title: "Build a `defer-finding` skill for filing deferred findings"
kind: tooling
severity: n/a
area: workflow, agents
found_by: dustinmays
found_in: chunk/scanner-hardening dogfood setup (2026-08-23)
status: open
related:
  - ./README.md              # the convention this skill would encode
  - ../STATE.md              # the sibling build-state-log convention
  - ./skill-log-build-state.md
---

## What

A Claude Code / agent **skill** that encodes how any agent files a deferred
finding, so the convention is followed consistently across Claude, Gemini, and
OpenCode instead of living only in a human's head.

## Trigger

Ad-hoc, any agent, mid-work: "I found something broken / worth improving but it's
out of scope right now."

## Behavior the skill should encode

- Create a **new** file `docs/epic-<N>/deferred/<kebab-slug>.md` — never edit an
  existing finding's file, never edit a shared index (git-collision-free).
- Use a **descriptive** slug, not a sequential number (two agents would race the
  same number). Same issue → same slug → natural dedup.
- Fill the frontmatter schema: `id, title, kind, severity, area, found_by,
  found_in, status, related`.
- `related:` should link to the actual source files and to any relevant
  `docs/` / `docs/adr/` context so a future reader has the trail.
- Do NOT fix the thing now; just record it.
- Regenerate `deferred/README.md` from the directory (a script, so no contention).

## Why a skill (not just docs)

Encodes the workflow as a repeatable checklist agents load on demand, and fits
the chunk-ownership model: any agent can defer; the owner triages. Build only
after we've used the convention manually enough to trust its shape.
