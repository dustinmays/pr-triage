---
title: Hard-fail philosophy
tags: [error-handling, escalation, schema]
related: [[github-dedup]], [[scope-guardrails]]
source: plan.md
---

Malformed report, unknown/unsupported `schema_version`, and an
unmapped/unrecognized risk tier all hard-fail into escalation to a
human — never retry-then-guess and never silently fall back to a
default. The report generator can evolve its schema independently of the
daemon precisely because a version mismatch is a hard stop, not a
best-effort parse.
