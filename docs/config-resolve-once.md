---
title: Config resolve-once
tags: [config, architecture, correctness]
related: [[runtime-capability-table]], [[persistence-discipline]]
source: agent-minder
---

Build a single ranked config resolver (e.g. stage -> agent -> job -> repo
config -> user config -> runtime default, most-specific wins), resolve it
once per run, and store the resolved values on the run record. Never
re-derive configuration in a display path or at read time.

agent-minder's worst config bug (a per-agent model silently dropped,
issue #528-style) came from not having this: without a single resolve-once
point, different code paths could re-derive different answers for "what
model is this run using."
