# 0008 — Purpose: optimize human attention and guarantee behavioral conformance across the work lifecycle

**Status:** Locked (purpose/north-star, signed off by Dustin 2026-08-28; front-of-loop *delivery* remains Expected — see Open)

## Decision

pr-triage exists to **optimize a human's limited attention onto the things that actually
matter, and to guarantee that a large piece of work behaves the way it was agreed to
behave.** It does this across the *whole* work lifecycle — not just at review — by pairing
a **deterministic-first** control plane (signals, tiers, gates, state, sync) with **AI
that assists but never decides**, and by giving the human a **consistent process**
(checklists, task-specific agents, an orchestrator, and tools) that produces quality
code and verifiable behavioral conformance to an agreed specification.

Two halves of one loop:
- **Reactive (built):** a PR exists → deterministic pre-scan tiers it → an AI reviewer
  assists → only what needs a human is escalated. (See [[0007-manage-human-attention-ai-assists-human-decides]].)
- **Proactive (in design):** before code exists → a charter/behavioral contract defines
  the human-verifiable endpoint → tests are written red-first → the implementer makes them
  green → a separate verifier confirms. (See the front-of-loop spike.)

The charter/behavioral contract is the single thread through both halves: it is the
scanner's "expected scope," the reviewer's grading oracle, and the human's definition of
done.

## Why

- **Attention is the scarce resource, not code generation.** Agents produce far more
  change than anyone can read; spending human attention as the primary quality mechanism
  does not scale. The system's job is to route attention *to* what matters and *away* from
  what doesn't — reducing overload is the point, not a nicety.
- **Deterministic-first; AI assists; the human decides.** Use reliable mechanical systems
  wherever they fit (signals, tiers, gates, state, sync); reach for AI only where genuine
  judgment is needed; on escalation the AI surfaces evidence and analysis, never the
  verdict. The human stays the decision-maker.
- **Conformance beats volume.** The failure mode of AI coding is *drift* — confident,
  plausible code that quietly solves the wrong problem. Unit tests can't catch it because
  they assert the code does what the code does. The defense is an agreed, testable
  specification of expected behavior, written before the work and enforced at every gate.
- **The cheapest attention is early attention.** Ratifying a charter and a set of
  behavioral tests at kickoff is one cheap, early decision that prevents an expensive,
  late one (reviewing a drifted PR). This is why the proactive half is worth building.
- **Process is a product feature.** Consistent checklists + task-specific agents + an
  orchestrator + shared tools are what make quality repeatable across people, chunks, and
  *different repos and stacks* — the process and the bar stay constant even when the
  language does not.
- **Evidence over conclusions.** Present facts and inspectable reasoning; distinguish
  deterministic facts from AI inferences; never frame toward a predetermined answer.

## Alternatives considered

- **Full automation / auto-merge (remove the human).** Rejected — judgment is
  irreducible for the risky minority, and a source of truth that is sometimes wrong is
  worse than none; trust, once lost, sends people back to the raw GitHub view.
- **Human-in-the-loop on every PR.** Rejected — does not scale to agent output volume and
  produces exactly the overwhelm the system exists to remove. We are human-*on*-the-loop:
  oversee at the process level, intervene on escalation.
- **"Just write more unit tests."** Rejected — cheap for AI to produce and prove nothing
  about intent; conformance needs behavioral/acceptance contracts and, where judgment is
  involved, evals — held to a higher bar than unit tests.
- **A separate companion app for the front-of-loop.** Rejected — the two halves share
  ~80% of their substrate (agent runtime, orchestration, state, GitHub seam, config) and
  pass the same charter object between them; bundle as a modular monolith and keep a clean
  seam for later extraction (front-of-loop spike §7).

## Current baseline

- **Reactive loop, built:** two-layer model — deterministic pre-scan `signal_tiers` +
  routed AI review agent (`routing.<tier>.{runtime,model,agent_def}`); local SQLite as
  source of truth ([[0006-local-state-is-source-of-truth]]); minimal one-way idempotent
  GitHub projection; runtime registry with self-registering adapters
  (`internal/runtime`, claude-code + opencode today).
- **Governing decisions:** [[0005-risk-tier-routing-in-config]],
  [[0006-local-state-is-source-of-truth]],
  [[0007-manage-human-attention-ai-assists-human-decides]].
- **Proactive loop, designed not built:** `docs/chunk-kickoff-behavioral-testing-spike.md`
  (charter + behavioral contract, human checklist, planning agent, red-first TDD agent,
  stack-agnostic test standard, red→green UI north star) and the dogfood
  `docs/charters/codex-runtime.md`.

## Open

- Front-of-loop **delivery** (the interactive `kickoff` surface, the `internal/charter` /
  `internal/behavioral` modules, the agents) is Expected, not Locked — designed in the
  spike, to be sequenced into the "stateful control plane & reviewer experience" epic.
- Whether this ADR later splits into a short standalone "product purpose" doc if it grows
  beyond ADR scope.
