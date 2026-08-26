---
id: chunk-setup-agent
title: "Interactive chunk-setup command/agent that tailors BOTH the pre-scan and the agent def"
kind: enhancement
severity: high
area: cli, config, agents, product
found_by: dustinmays
found_in: chunk/scanner-hardening live dogfood (2026-08-25)
status: open
related:
  - ./per-chunk-triage-config.md              # the config surface this produces (deterministic side)
  - ../../../internal/cli/init.go              # could extend init, or a new `setup`/`configure` command
  - ../../../agents/review-agent.md            # the probabilistic side this could tailor
  - ./skill-log-build-state.md
  - ./skill-defer-finding.md
  - ./workflow-install-command.md              # sibling: installs the pre-scan workflow itself
---

## What

A command (extend `init`, or a new `pr-triage setup` / `pr-triage configure`) that
spawns an **agent** to set up triage for a repo/chunk by tailoring **both layers**
pr-triage controls:

1. **The mechanical/deterministic layer** — the pre-scan `signal_tiers` (which
   signals matter, and at what tier) for this repo/chunk. See
   [per-chunk-triage-config](./per-chunk-triage-config.md).
2. **The probabilistic layer** — the review **agent definition** (what the
   reviewer focuses on, what it treats as in-scope-safe vs. defer) for this
   repo/chunk.

## How it would work (owner's vision)

- Start from a **baseline template** config + agent def.
- **Inspect the repository**: detect the stack; notice if the repo is thin/brand-new
  (just a template) vs. mature.
- **Ask the owner questions**: e.g. "Will this chunk primarily be CI/CD work?",
  "What's critical to protect here?", stack/framework confirmations — more
  questions when the repo is too thin to infer from code.
- **Read the chunk's parent issue** (the epic/chunk charter) to understand the
  intended scope and what changes are *expected* (so expected changes stay routine
  instead of all-escalating — the pain observed in this chunk).
- **Emit tailored config**: the per-chunk `signal_tiers` overlay AND a tailored
  agent def, so both the deterministic gate and the LLM reviewer fit this work.

## Why it matters

This is the "early chunk-owner responsibility" made real: triage quality depends
on context, and context is cheapest to capture at chunk kickoff. It generalizes to
every repo adopting pr-triage (each has different "expected" changes and different
critical surfaces). And it's a clean expression of the tool's two-layer model —
one command configures the mechanical scanner and the probabilistic reviewer
together, from the same repo+charter understanding.

## Notes / open questions

- Extend `init` vs. a dedicated command? Leaning dedicated (`setup`/`configure`)
  so it can be re-run per chunk without re-registering the repo.
- Interactive-by-default, every prompt also a flag (matches the existing init
  pattern) so it stays scriptable.
- Overlaps with the [chunk-setup skill](./skill-log-build-state.md) idea — the
  skill could be the agent's playbook; this finding is the command that invokes it.
