---
title: "App ↔ GitHub boundary & the reviewer experience (exploratory)"
status: exploratory
date: 2026-08-26
tags: [design, boundaries, ux, server, state, direction]
related:
  - ../../adr/0006-local-state-is-source-of-truth.md
  - ../../adr/0007-manage-human-attention-ai-assists-human-decides.md
  - ./escalation-override.md
  - ../deferred/chunk-setup-agent.md
---

# App ↔ GitHub boundary & the reviewer experience

**Status: exploratory / thinking-out-loud.** Captures a direction discussed while
writing ADR 0006; not yet committed work. GitHub remains the sole target (no
provider-portability push yet) — this is about drawing a maintainable seam and
about what makes the tool worth attending to.

## The premise (and its cost)

Making local state the source of truth ([[0006-local-state-is-source-of-truth]])
changes the tool's character: it moves from "runs in the background, optional
assistant" to "the authority a human must attend to." That's a real cost — if the
app demands attention, its UX has to *earn* it. The justification: done well, the
app **reduces total attention** by curating what a human sees, rather than adding
one more surface to check. The tool's job becomes **attention routing**, not just
automation.

## The boundary: who owns what

**The app owns (authoritative, interactive):**
- Triage decisions and their rationale (why escalated, which signal, evidence).
- The reviewer's working view: a clean, curated PR readout (diff summary, pre-scan
  facts, agent findings, what needs attention and why).
- Attention routing / progressive disclosure — "this needs you, this doesn't."
- Run history, cost, agent outputs, override/approval decisions.
- (Later) notifications and prioritization.

**GitHub owns (substrate the app defers to):**
- Identity, auth, permissions (who may push/merge).
- The canonical code, the PR, and merge mechanics (the merge button, branch
  protection, required checks).
- CI execution.
- The durable, public, multi-party record — the paper trail collaborators and
  other tooling rely on.

**The seam (sync contract), one-directional:**
- **Inbound (GitHub → app):** ingest PR metadata, CI status, the pre-scan report,
  and human signals; interpret them into local state. GitHub is never the app's
  memory.
- **Outbound (app → GitHub):** project a **minimal, sufficient** set of artifacts
  for others — a status label, the merge gate, a paper-trail comment. Notifications
  and records, reconciled *from* state, idempotent, never merged back.

## Maintainability principle

Keep the **outbound projection minimal and stable**. The richer the experience we
put *in* the app, the smaller the footprint we should keep *in* GitHub — ideally a
handful of well-defined artifacts (one status label, one gate, one summary comment
for the trail). Every extra projected artifact is more two-place consistency to
maintain. Rule: **rich experience in the app; minimal-but-sufficient, one-way,
idempotent projection to GitHub.** GitHub is the app's public API surface for
humans-not-running-the-app and for merge mechanics — not where the reviewer works.

## The experience vision

- A **curated PR view in the app**: simplified, cleaned-up — the diff that matters,
  the pre-scan results, the escalation *reason* (which signal + evidence), the
  agent's findings — so the reviewer doesn't bounce between the app's readouts and
  GitHub's "big dumb page."
- **Progressive disclosure**: the app decides what rises to the human's attention
  and what stays collapsed; reduce information overload, prioritize.
- GitHub comments/labels continue **for the paper trail** (others' benefit), but the
  primary, seamless experience lives in the app.

## Server / interface separation → and team scale

This is the natural shape:
- **Server (state authority + reconciler + local API):** the daemon already emits
  events and writes a status file (Chunk 6) — the seed of this. Harden it into the
  authoritative state service exposing a clean read/command API (read state; issue
  commands like `override`).
- **Interfaces (thin, read the API):** the current TUI is a fine starting point;
  richer UIs and notifications come later. No interface re-derives from GitHub.

**Team/sustainability angle:** today the daemon is single-user-per-machine (solo
PAT, local SQLite). A **team** needs a shared state authority — which is exactly the
server. So the server/interface split isn't only UX; it's the path to multi-user.
The boundary discipline now (minimal one-way GitHub projection; app owns the working
view) is what makes a team-shared server feasible later without a rewrite. Accuracy
must be locked down first — a source of truth that's wrong is worse than none — while
always reconciling GitHub to reflect the reality the app defines.

## Immediate scope decision (2026-08-26)

**Swift/SwiftBar (Chunk B, #82/#87/#88/#89) is de-scoped/deprioritized.** Focus:
finish scanner hardening (Chunk A + C) and set the TUI + server foundation. The
state-first control-plane work ([[escalation-override]], per-chunk-triage-config,
chunk-setup-agent) plus this experience direction are candidates for a dedicated
follow-up epic (working name: "stateful control plane & reviewer experience").
