---
title: Persistence discipline
tags: [sqlite, persistence, migrations]
related: [[config-resolve-once]], [[github-dedup]]
source: plan.md
---

Use WAL mode plus a single writer (`SetMaxOpenConns(1)`) to avoid lock
contention on SQLite. Migrations are additive-only: one version bump per
change, and a past migration is never edited after the fact. Store one
durable record per run (resolved model, runtime, session id, cost+basis,
turns, status, stop reason) rather than reconstructing that state from
logs later.
