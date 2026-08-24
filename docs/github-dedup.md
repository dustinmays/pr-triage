---
title: GitHub dedup
tags: [github, rate-limiting, state-machine]
related: [[persistence-discipline]], [[hard-fail-philosophy]]
source: plan.md
---

Dedup on `(PR, head SHA)` — a report only counts as a trigger if it's
attached to a run for a head SHA not yet processed for that PR. This is
the same key the PR state machine uses to prevent double-triggering the
agent on repeated polls or on a re-emitted report for an already-processed
SHA.

Use conditional requests (ETag / `If-None-Match`) for status/list polling:
304 responses don't count against the GitHub API rate limit, which is
cheaper than just widening the poll interval.
