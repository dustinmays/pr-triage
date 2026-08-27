---
id: skill-log-build-state
title: "Build a `log-build-state` skill for maintaining STATE.md"
kind: tooling
severity: n/a
area: workflow, agents
found_by: dustinmays
found_in: chunk/scanner-hardening dogfood setup (2026-08-23)
status: open
related:
  - ../STATE.md              # the file this skill maintains
  - ./skill-defer-finding.md # the sibling skill; shares one reference doc
  - ./README.md
---

## What

A Claude Code / agent **skill** that encodes how the chunk owner (or their one
designated lead agent) maintains the per-epic `STATE.md` build-state log.

## Trigger

Milestone-driven, owner only: a sub-issue lands, a chunk opens/closes, a key
decision is made.

## Behavior the skill should encode

- Enforce **single-writer** discipline: only the chunk owner / lead agent edits
  STATE.md; worker agents read it, never write it.
- Keep the required sections current: goal, chunk-status table, dated log
  entries, conventions.
- Convert relative dates to absolute; keep `updated:` frontmatter fresh.
- Maintain `related:` links to the relevant source, docs, and ADRs.
- Cross-reference the `deferred/` backlog rather than duplicating it.

## Why a skill (not just docs)

Makes "one person owns a chunk" operational: the skill is the owner's checklist
for keeping shared state trustworthy. Pairs with
[`skill-defer-finding`](./skill-defer-finding.md) — same convention, opposite
write pattern (curated single-writer vs. append-only many-writer). Build both
together, sharing one reference doc, after the manual convention proves out.
