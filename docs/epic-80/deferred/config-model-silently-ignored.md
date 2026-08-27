---
id: config-model-silently-ignored
title: "Top-level config.model is silently ignored by routing"
kind: bug
severity: medium
area: config, orchestrator
found_by: claude
found_in: chunk/scanner-hardening dogfood setup (2026-08-23)
status: fixed
fixed_in: "#112 — init pins --model into routing.routine.model so the agent runs on it"
related:
  - ../../../internal/orchestrator/orchestrator.go   # uses routing.Model (~line 366, 422)
  - ../../../internal/config/config.go               # DefaultConfig routing table + top-level Model field
  - ../../live-trial-runbook.md                      # runbook tells users to pass --model
---

## What

The `run` orchestrator drives the agent off `routing.<tier>.Model` (see
`internal/orchestrator/orchestrator.go`, `routing.Model` at ~366 and ~422). The
**top-level `model:`** that `init` writes into `.pr-triage/config.yaml` (and the
`--model` flag that sets it) is **never consulted** for agent invocation. It is
decorative.

## Why it matters

A user who runs `init --model claude-opus-4-8` (or edits `model:` in the config)
reasonably expects the routine agent to run on that model. It doesn't — it runs
on `routing.routine.model` from `DefaultConfig` (currently `claude-haiku-4-5`).
Silent no-op on a flag the runbook actively tells people to set.

## Failure scenario

1. `pr-triage init --model claude-opus-4-8 --non-interactive`
2. Config shows `model: claude-opus-4-8`.
3. A routine PR is triaged → agent actually runs on `claude-haiku-4-5`.
4. No error, no warning; cost/behavior silently reflect the wrong model.

## Options (not decided)

- Have `init` write the resolved model into `routing.routine.model` (make the
  flag concrete in the written config), **or**
- Have the orchestrator treat top-level `model` as an override when a tier's
  routing entry doesn't pin one, **or**
- Drop the top-level `model` field entirely and make routing the only source of
  truth (most honest, biggest surface change).

Leaning toward the first (write it into routing) so the config is transparent
about what will actually run.
