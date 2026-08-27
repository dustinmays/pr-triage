---
id: init-writes-opaque-partial-config
title: "init writes a partial config that hides the active routing rules"
kind: enhancement
severity: low
area: config, cli
found_by: claude
found_in: chunk/scanner-hardening dogfood setup (2026-08-23)
status: fixed
fixed_in: "#114 — pr-triage config show prints the effective merged config (signal_tiers + routing) read-only"
related:
  - ../../../internal/cli/init.go        # writes .pr-triage/config.yaml
  - ../../../internal/config/config.go    # Load now merges over DefaultConfig
  - ./config-model-silently-ignored.md    # related: model would become visible if routing were written out
---

## What

`init` writes only `base_ref`, `poll_interval`, `timeout`, `github_user`,
`runtime`, `model`. It omits `signal_tiers` and `routing`. This is now
*functionally* fine — `config.Load` layers the file over `DefaultConfig()`
(fixed this session) — but the written file is **opaque**: a human opening
`.pr-triage/config.yaml` can't see which signals escalate or where each tier
routes.

## Why it matters

The runbook and the whole design lean on operators being able to read and tune
the signal→tier and routing tables. An empty config hides them. First-run users
have no discoverable surface to edit.

## Options (not decided)

- `init` expands and writes the full `DefaultConfig` (self-documenting, but
  verbose and can drift from code defaults), **or**
- keep the minimal file but add a `pr-triage config show --effective` command
  that prints the merged config, **or**
- write the minimal file plus a commented-out reference block.

Low priority — the merge fix removed the correctness bite; this is transparency.
