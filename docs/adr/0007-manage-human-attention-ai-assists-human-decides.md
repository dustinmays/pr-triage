---
title: Manage human attention — deterministic-first, AI assists, the human decides
status: accepted
date: 2026-08-26
tags: [architecture, product, human-in-the-loop, ai, ux, attention]
---

## Context

Making local state authoritative ([[0006-local-state-is-source-of-truth]]) turns
pr-triage from a background assistant into an authority a human must attend to. It
is also, plainly, becoming an AI application: it has **mechanical/deterministic**
pieces (the pre-scan signals, tiers, gates, state machine, sync) and **AI** pieces
(the review/fix agent). That raises product-level questions that will drive many
future feature and bug decisions: what is the app *for*, how should it use AI vs.
deterministic logic, and how should it present information to a human without
distorting their judgment. This ADR commits the *principles*; the detailed
human–AI–computer interface design is deferred to a follow-up epic (see the
placeholder issue) and to [[app-github-boundary-and-experience]].

## Decision

1. **The system's purpose is to manage human attention.** It exists to route a
   person's limited attention to what matters and away from what doesn't. Every
   feature is judged by whether it *reduces* attention spent on the unimportant and
   *sharpens* focus on the important. Reducing information overload is not a nicety;
   it is the point.

2. **Deterministic-first; AI supplements.** Use deterministic systems wherever they
   are appropriate and reliable (signal detection, tiering, gating, state, sync).
   Reach for AI only where genuine judgment is needed — never as the default, and
   never where a deterministic mechanism would be more trustworthy and cheaper.

3. **On escalation, AI is a decision *assistant*, not the decision *maker*.** When a
   change is escalated (judgment required), the app's job is to help a human decide
   — surface the relevant facts and the agent's analysis — not to decide for them or
   auto-act. The human remains the decision-maker; keep the human in the loop.

4. **Present information without skewing the human's read.** Be deliberate about the
   *type and framing* of information shown so it informs rather than biases. Favor
   **evidence over conclusions**; make AI reasoning **inspectable**; clearly
   **distinguish deterministic facts from AI inferences** (and mark AI confidence
   honestly, per the cost-basis-honesty discipline). Never obscure, omit, or frame
   in a way that leads the human to a predetermined answer — the goal is a clear,
   unskewed read of the actual situation.

5. **Progressive disclosure.** Default to the minimal signal a human needs
   ("this needs you / this doesn't"); reveal detail on demand. Curate; do not dump.

6. **Accuracy before experience.** A source of truth that is sometimes wrong is
   worse than none — trust, once lost, sends people back to the raw GitHub view and
   defeats the premise. Lock down state correctness and reliable one-way sync before
   investing in the richer experience layer.

## Consequences

- These principles are the **guiding light** for future decisions: when a feature is
  proposed or a state-dependent bug is found, resolve it by asking "does this manage
  attention, keep the human deciding, present without skewing, and stay
  deterministic where it can?"
- The app owns the curated reviewer experience and attention routing; GitHub remains
  the substrate and the paper trail (a minimal, one-way projection) — see
  [[app-github-boundary-and-experience]].
- Detailed human–AI–computer interface design (views, disclosure, notifications,
  server/interface split) is out of scope for the current epic and is the early work
  of the follow-up "reviewer experience & human–AI interface" epic.
