---
id: schema-sql-matches-migration-regex
title: "Editing internal/db/schema.sql also trips migration_history_rewritten"
kind: question
severity: low
area: scanner
found_by: dustinmays
found_in: chunk/scanner-hardening — A.2 golden fixtures PR #106 (2026-08-27)
related:
  - ../../../scripts/pr-prescan.sh   # MIGRATION_RE and schema_changed_without_migration blocks
status: open
---

## What

`MIGRATION_RE` is broad: `(^|/)(internal/db/migrations/|internal/db/|migrations/|db/migrations/).*[.]sql$`.
The `internal/db/` alternative matches **any** `.sql` under `internal/db/`,
including `internal/db/schema.sql`. So editing `schema.sql` (an M-status change)
trips `migration_history_rewritten` in addition to the intended
`schema_changed_without_migration`.

Surfaced by the A.2 fixtures: `schema_changed_without_migration`'s golden records
BOTH signals present (see the cross-signal check in the #106 review). The golden
is correct — it pins the scanner's actual behavior — but the behavior itself is
questionable.

## Why it matters (mild)

`migration_history_rewritten` is meant to flag rewriting an existing *migration*
file (a genuinely dangerous "someone edited already-applied history" event).
A schema-snapshot edit is a different thing. Double-flagging isn't harmful today
(both are escalate-tier, so routing is unaffected), but it muddies the evidence a
human reads on escalation and would matter if the two signals ever routed
differently or fed per-signal overrides (see [per-chunk-triage-config](./per-chunk-triage-config.md)
and [override-command-state-first](./override-command-state-first.md), whose
`--signal` waivers assume each signal means one distinct thing).

## Options

- Exclude the schema snapshot from the migration-history rule (e.g. subtract
  `schema.sql` from `migration_history_rewritten`'s file set, so a schema edit
  trips only `schema_changed_without_migration`).
- Or accept it and document that `internal/db/*.sql` is intentionally treated as
  migration history. Decide deliberately rather than by regex accident.

Low priority; capture the decision when scanner semantics next get attention
(Chunk C edge-case work is a natural home).
