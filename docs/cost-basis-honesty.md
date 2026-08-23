---
title: Cost-basis honesty
tags: [cost, adapters, runtime, correctness]
related: [[runtime-capability-table]], [[result-shape-normalization]]
source: agent-minder
---

Never scrape logs for cost — get cost from the adapter's structured result
(e.g. Claude's terminal `result` event `total_cost_usd`), never by grepping
stdout/log text for a string like `total_cost_usd` that only one runtime's
output happens to contain. Tag every cost/turn/model value with a **basis**
field (`exact` / `estimated` / `unavailable` / `runtime-defined`) so a
genuine `0` can never be confused with "didn't measure it."

This was agent-minder's worst bug: log-scraping for `total_cost_usd` meant
Codex and OpenCode runs silently recorded cost `0`, and their budget
ceiling never fired for those two runtimes.
