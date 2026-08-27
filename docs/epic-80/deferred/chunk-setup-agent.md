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
  - ../../../internal/cli/init.go              # `setup` extends/wraps this for registration
  - ../../../agents/review-agent.md            # the probabilistic side this could tailor
  - ./workflow-install-command.md              # sibling: installs the pre-scan workflow itself
  - ../transfer-out.md                        # STATE.md/deferred/ convention lives in the template repo, not here
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
- **Emit tailored config**: the per-chunk `signal_tiers` overlay (per
  [per-chunk-triage-config](./per-chunk-triage-config.md)) AND a tailored agent
  def, so both the deterministic gate and the LLM reviewer fit this work.

## Scope boundary (decided 2026-08-26)

`pr-triage setup` owns exactly two artifacts, both pr-triage-native:
1. The repo/base-ref registration (extends what `init` already does —
   `internal/cli/init.go`).
2. The `.pr-triage/chunks/<base-ref>.yaml` signal-tier overlay + a tailored
   `.claude/agents/review-agent.md`.

It does **not** scaffold `STATE.md`, `deferred/`, or `transfer-out.md`. That
build-state/knowledge-base convention is owned by the chunk owner and their
orchestrating agent, living under `docs/<epic-or-chunk>/` **in the target repo**,
and is expected to be seeded by a skill in the template/seed repo (see
`transfer-out.md`), not by this tool. pr-triage stays a reviewer/gate; it doesn't
own the human/agent collaboration record. `chunk-setup-agent` may *read*
STATE.md's frontmatter (chunk charter, expected scope) as input if present, but
never writes it.

Also decided: pr-triage tracks **one active base_ref per repo** for now (the
`repos` table is keyed on `(owner, name)`, and `UpsertRepo` overwrites
`base_ref` — there's no multi-chunk-concurrent tracking today). Re-running
`setup` against a new chunk branch repoints the existing registration rather
than adding a second tracked line of work. Concurrent multi-chunk tracking
(schema change to key on `(owner, name, base_ref)`) is out of scope here and
would be its own deferred item if ever needed.

## Why it matters

This is the "early chunk-owner responsibility" made real: triage quality depends
on context, and context is cheapest to capture at chunk kickoff. It generalizes to
every repo adopting pr-triage (each has different "expected" changes and different
critical surfaces). And it's a clean expression of the tool's two-layer model —
one command configures the mechanical scanner and the probabilistic reviewer
together, from the same repo+charter understanding.

## Notes / open questions

- Extend `init` vs. wrap it from a new `setup` command? `init` already does the
  registration + config write; `setup`'s distinct job is the interactive
  tailoring (stack detection, charter read, overlay + agent-def generation).
  Could be `init` with a new flag, or `setup` calling `init`'s registration
  internally — not yet decided.
- Interactive-by-default, every prompt also a flag (matches the existing init
  pattern) so it stays scriptable.
- The template repo's own chunk-kickoff skill (STATE.md/deferred/ scaffolding)
  is a separate, sequenced step — likely run before or alongside `pr-triage
  setup`, not by it. See the scope boundary above.
