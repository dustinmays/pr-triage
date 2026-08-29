---
id: largest-file-empty-path-on-zero-line-diffs
title: "diff.largest_file is {path:\"\",changed:0} for pure-rename / pure-binary diffs"
kind: question
severity: low
area: scanner
found_by: dustinmays
found_in: chunk/scanner-hardening — C.2 edge-case fixtures PR (2026-08-26)
related:
  - ../../../scripts/pr-prescan.sh   # DIFF_JSON awk block, max_changed/max_path
  - ../../../scripts/prescan-test/fixtures/edge_rename/golden.json
  - ../../../scripts/prescan-test/fixtures/edge_binary/golden.json
status: resolved
fixed_in: "#126 — DIFF_JSON awk `BEGIN { max_changed = -1 }` so all-zero-line diffs report a real `largest_file.path` (edge_binary/edge_rename goldens regenerated)"
---

## What

When every file in a diff has zero line-changes (a pure rename with identical
content, or a binary-only change — numstat reports `-`/`0`), the `DIFF_JSON`
awk block never updates `max_path` because `changed (=0)` is never strictly
greater than the initial `max_changed (=0)`. The result is:

```json
"largest_file": { "path": "", "changed": 0 }
```

i.e. a non-null `largest_file` whose `path` is the empty string. Surfaced by
the C.2 edge fixtures `edge_rename` and `edge_binary` (both goldens record it).

## Why it matters (mild)

It is valid JSON and a valid `schema_version:1` report — not a crash, not
malformed — so it does not fail the "never crash" C.2 guarantee. But an empty
`path` is a slightly degenerate value: a consumer reading
`diff.largest_file.path` gets `""` rather than either a real path or `null`.
When `files_changed > 0` but no file changed any lines, arguably `largest_file`
should still name one of the changed files (with `changed:0`), or be `null`.

## Options

- In the awk, initialize `max_changed = -1` (or track "any file seen") so the
  first file always becomes `max_path`, giving a real path with `changed:0`.
- Or emit `largest_file: null` when `max_changed == 0`.
- Or accept it and document that an all-zero-line diff yields an empty path.

Low priority; fold into the next scanner-semantics pass (natural fit alongside
the other Chunk C edge-case work). Decide deliberately rather than leave it an
awk-initialization accident.

## Resolution

Chose the first option: the `DIFF_JSON` awk block now initializes
`max_changed = -1` in a `BEGIN` block, so the first changed file always becomes
`max_path` and an all-zero-line diff yields a real changed-file path (with
`changed:0`) instead of an empty string. The `if (files > 0)` guard is unchanged,
so a genuinely empty diff still emits `largest_file: null`. The `edge_binary` and
`edge_rename` goldens were regenerated (the only field that changed is
`largest_file.path`: `""` → `assets/logo.png` / `internal/util/{helper.go =>
helpers.go}`).
