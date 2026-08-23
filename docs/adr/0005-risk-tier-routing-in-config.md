---
title: Risk-tier routing lives in config
status: accepted
date: 2026-08-23
tags: [architecture, config, risk, escalation]
---

## Context

Not every PR warrants the same runtime/model: a destructive migration or
ADR change is a different review-cost tradeoff than a routine change.
Risk classification comes from the CI/CD report and/or the agent
definition's existing "table of what's important."

## Decision

A routing table of `risk_tier -> {runtime, model, agent_def}` lives in the
daemon's config, not hardcoded in source. An unmapped/unrecognized risk
tier escalates to a human rather than silently falling back to a default
runtime/model.

## Consequences

Risk criteria and available agent definitions can evolve independently of
the daemon's release cycle by editing config. This is the same hard-fail
philosophy as the malformed-report and schema-version-mismatch cases —
see [[hard-fail-philosophy]].
