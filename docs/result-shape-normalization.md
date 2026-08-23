---
title: Result-shape normalization
tags: [adapters, runtime, correctness, logging]
related: [[cost-basis-honesty]], [[runtime-capability-table]]
source: agent-minder
---

Normalize runtime output at the adapter boundary — don't compare raw
values across runtimes. `num_turns` means something different per
runtime, so only the ratio of a run's turns to that same run's own limit
is meaningful; never compare turn counts across runtimes directly.

Related traps: Claude's `stop_reason` came back double-quoted in raw
output. One log file can contain multiple runs, summed/first/last
differently per runtime — always write a fresh log file per run, never
append.
