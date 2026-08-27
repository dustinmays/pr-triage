---
id: scanner-scans-its-own-test-fixtures
title: "Scanner trips signals on its own test-fixture files (paths matched anywhere via (^|/))"
kind: bug
severity: medium
area: scanner, poller, orchestrator
found_by: dustinmays
found_in: chunk/scanner-hardening — reviewing the A.2 golden-fixture PR #106 (2026-08-27)
status: open
related:
  - ../../../scripts/pr-prescan.sh          # signal path regexes use (^|/), not ^-anchored
  - ../../../scripts/prescan-test/fixtures/  # fixtures embed internal/db/, Makefile, go.mod, etc.
  - ./per-chunk-triage-config.md            # related: infra/test PRs escalate wholesale
---

## What

Most signal path patterns in `scripts/pr-prescan.sh` match a path **anywhere in
the tree**, not just at the repo root, because they are written as
`(^|/)internal/db/...`, `(^|/)(\.golangci|Makefile|lefthook)...`,
`(^|/)go\.(mod|sum)$`, etc. (the `(^|/)` alternation).

The A.2 test fixtures deliberately embed those very paths as fixture data — e.g.
`scripts/prescan-test/fixtures/migration_sql_added/_base/internal/db/migrations/0002_add_table.sql`,
`.../safeguard_config_changed/_base/Makefile`, `.../stack_choice_changed/_base/go.mod`.
So when the daemon scans a PR that **adds those fixtures** (like #106), the scanner
matches them and trips `migration_sql_added`, `safeguard_config_changed`,
`dependency_manifest_changed`, `stack_choice_changed`, … → the PR escalates.

(Root-`^`-anchored patterns like `workflow_changed`'s `^\.github/workflows/` do NOT
have this problem — the fixture path `scripts/.../_base/.github/workflows/ci.yml`
does not start with `.github/`. Only the `(^|/)`-style patterns leak.)

### Confirmed live (PR #106, 2026-08-27)

The daemon escalated #106 in exactly this way, tripping `migration_sql_added`,
`dependency_manifest_changed`, `install_execution_allowed`, and
`safeguard_config_changed`. (This was also a clean validation of D.2 — the
escalation comment named every signal and cited the offending fixture paths.)

The live evidence exposed a **second layer**: the scanner matches not only fixture
`_base/` data but the fixture *scaffolding itself* — `apply.sh` and `golden.json`.
The content-scanning signals (`install_execution_allowed`, `suppressions_added`,
`tests_skipped_added`, …) grep added lines for a pattern regardless of file role,
so:

- `install_execution_allowed/apply.sh` tripped on its own `//go:generate` example
  line, and `install_execution_allowed/golden.json` tripped on the `"detail":
  "+ //go:generate ..."` string it recorded as expected evidence;
- `install_execution_allowed_negative/apply.sh` tripped too — because its comment
  literally reads "add a comment that contains 'generate' but is NOT a
  `//go:generate` directive", which contains the pattern.

So even the *negative* fixture escalates, purely from describing the thing it's
testing. Any exclusion must therefore skip the whole fixture directory
(`apply.sh` / `golden.json` / `pr.json` / `_base/`), not just `_base/`.

## Why it matters

A PR whose *only* real change is adding scanner test data reads to the daemon as a
high-risk migration/dependency/safeguard change. It's a false positive driven by
the scanner scanning representations of risky paths rather than actual risky
changes. It inflates escalations (every future A.2/edge-case fixture PR trips it)
and muddies the signal the whole tool depends on.

## Fix options

- **Exclude known test-fixture/testdata dirs from scanning**: have the scanner
  skip paths under `scripts/prescan-test/fixtures/**` (and likely `testdata/**`)
  when evaluating signals. Cheapest, most targeted.
- **Tighten anchoring**: reconsider whether `(^|/)internal/db/` should really match
  mid-path; many of these are meant to be repo-root-relative. Riskier (could miss
  genuine nested modules).
- **Configurable ignore globs**: a scanner-level `ignore:` list (pairs well with
  [per-chunk-triage-config](./per-chunk-triage-config.md)).

Prefer the explicit fixture/testdata exclusion for now. Note this is exactly the
kind of expected-but-unwanted escalation the new `pr-triage override` is designed
to wave through in the meantime.
