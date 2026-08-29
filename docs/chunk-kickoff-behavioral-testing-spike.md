---
title: "Chunk kickoff & behavioral testing — front-of-loop design spike"
status: exploratory
date: 2026-08-27
tags: [design, spike, behavioral-testing, spec-driven, human-in-the-loop, architecture, agents, attention]
related:
  - ./adr/0007-manage-human-attention-ai-assists-human-decides.md
  - ./epic-80/design/app-github-boundary-and-experience.md
  - ./epic-80/deferred/chunk-setup-agent.md
  - ./epic-80/deferred/per-chunk-triage-config.md
  - ./scope-guardrails.md
---

# Chunk kickoff & behavioral testing — front-of-loop design spike

**Status: exploratory / research spike.** Ideation + a recommendation, not committed
work. Commissioned to research emerging best practices for *behavioral* testing in
agentic coding, then design how pr-triage should help a human set up a chunk
charter/contract and behavioral tests at the **start** of work — and to decide whether
that lives in a companion app or bundled in this codebase.

---

## TL;DR (the decision, stated first)

1. **The gap is real and worth building.** pr-triage today is the *reactive* half of
   the loop (review + gate a PR that already exists). The high-leverage missing half is
   the *proactive* front: turning a chunk into a **charter/contract** and a set of
   **behavioral acceptance tests** that become the human-verifiable oracle the whole
   loop converges on. Every credible 2026 practice — spec-driven development,
   eval-driven development, adversarial validation — says the cheapest place to prevent
   drift is *before* code, by writing down expected behavior in a testable form.

2. **Recommendation on packaging: bundle it — same codebase, distinct surface. Do not
   start a separate companion app.** Build the front-of-loop as new modules and a new
   interface *inside* pr-triage, sharing the core (agent runtime, orchestration, state
   store, GitHub seam, the two-layer config model). Keep a clean internal seam so it
   *could* be extracted into its own deployable later — but distribution is a cost you
   pay only when team-scale or independent release cadence forces it, and nothing does
   yet. This is the modular-monolith path; see [Architecture](#architecture-companion-app-vs-bundled).

3. **The unit of behavioral testing here is not "more unit tests." It is an
   executable, human-readable behavioral contract** (Given/When/Then acceptance
   scenarios) plus a small set of **eval-style behavioral checks** for the judgment
   behaviors that programmatic asserts can't capture — graded by a *separate* verifier
   from the one that wrote the code.

The rest of this doc is the evidence and the design behind those three claims.

---

## 1. The gap this spike addresses

The system's stated purpose is to **manage human attention** — route a person's limited
attention to what matters, keep the human the decision-maker, present evidence not
conclusions ([[0007-manage-human-attention-ai-assists-human-decides]]). Today that
purpose is served only *reactively*: a PR exists, the deterministic pre-scan tiers it,
the agent reviews it, and escalation surfaces the ones that need a human.

But the loop has a front end that pr-triage doesn't own yet:

> A chunk is human-reviewable/human-verifiable **at its endpoint**; the intermediate
> pieces are escalated only as the rules require. For the endpoint to *be* verifiable,
> "verifiable" has to be defined up front — as expected behavior, written down, before
> the agent starts.

Without that, three failure modes recur (all observed or anticipated in the project's
own notes):

- **Drift** — "confident, plausible code that quietly solves the wrong problem"
  because nothing grounded the work in a real spec. Unit tests can't catch this; they
  assert the code does what the code does.
- **Escalation noise** — the scanner escalates *everything* because it doesn't know
  what changes were *expected* for this chunk (the exact pain that motivated
  [[chunk-setup-agent]] and [[per-chunk-triage-config]]).
- **Cheap-but-hollow tests** — AI writes unit tests readily, and "agent-generated tests
  can validate flawed logic, creating false confidence." Volume of tests ≠ evidence of
  correct behavior.

The front-of-loop artifact — a charter + behavioral contract — is what fixes all three:
it is the reviewer's oracle, the scanner's "expected scope" input, and the tester's
pass/fail authority.

---

## 2. What the field says (2026 synthesis)

Five converging threads from current practice. They agree more than they disagree, and
they line up almost exactly with this project's existing principles.

### 2.1 Spec-Driven Development (SDD) — the spec is the source of truth

SDD went mainstream in 2026 because "AI agents are great at writing code and terrible at
guessing what you meant." The spec declares intent; code is a *generated, verifiable
artifact*. A good spec carries **outcomes, scope boundaries, constraints, prior
decisions, task breakdown, and verification criteria**. Tooling (GitHub Spec Kit, AWS
Kiro, BMAD) converges on a three-document shape: **requirements → design → tasks**.
Acceptance criteria are written in **EARS** notation — five sentence patterns
(`WHEN <trigger> THE SYSTEM SHALL <behavior>`, plus ubiquitous/state-driven/
unwanted-behavior/optional) that make requirements unambiguous and testable. The claimed
payoff: SDD "catches architectural violations and API contract drift that unit tests
structurally cannot."

**For us:** the chunk charter *is* a spec. The `chunk-setup-agent` already proposes
reading the parent issue's charter to tailor triage — SDD says: don't just read a
charter, help *produce a good one*, in a form testable enough to enforce.

### 2.2 Behavioral tests as executable, human-readable contracts (BDD/Gherkin)

A Gherkin `Given/When/Then` scenario is "simultaneously a description of intent and an
executable test." Its structural advantage: **making the spec executable forces
validation** — some ambiguity has to be resolved before the scenario can even run. The
scenario becomes a shared authority both non-technical and technical stakeholders can
read: if the scenario is wrong, you change it by explicit agreement; if the code is
wrong, you fix the code. Caveat from the field: without explicit rules, AI-generated
Gherkin "drifts into vague Then steps, UI-heavy scripts, and multi-behavior scenarios."
So the behavioral-test *authoring* is itself an agent task that needs guardrails.

**For us:** this is the "beyond superficial unit tests" the owner is after — a behavioral
contract that a human can read in 30 seconds and that a machine can execute.

### 2.3 Eval-Driven Development (EDD) — for the judgment behaviors asserts can't reach

Some expected behavior isn't a clean pass/fail assertion (tone, instruction-following,
"did it actually do the thing the ticket asked"). The eval discipline (per Anthropic's
guidance and EDD practice):

- **Start small, from real failures** — 20–50 unambiguous tasks; "two domain experts
  would independently reach the same pass/fail verdict."
- **Combine graders** — code-based (fast, objective, for deterministic outcomes) +
  model-based **LLM-as-judge** (for open-ended behavior, with a rubric and an "Unknown"
  escape hatch) + occasional human grading as the gold-standard calibrator.
- **Grade the output, not the path** — don't over-constrain *how* the agent got there.
- **Design against gaming** — passing must require solving the problem, not exploiting a
  loophole; watch for saturation (≈100% pass = no signal) and 0% (usually a broken
  task, not an incapable agent).
- **Read the transcripts** — you don't know your grader works until you review many
  trials.

**For us:** a small per-chunk eval set complements the Gherkin contract for the parts of
"expected behavior" that are judgment calls — and pr-triage already has an agent runtime
that can host both the runs and the LLM-judge.

### 2.4 Adversarial validation — the builder cannot judge itself

The strongest single principle across the sources: **"the agent that built the code
CANNOT judge it."** When the agent controls its own verification, "it's no longer
verification but self-confirmation." AWS SpecShip operationalizes this as a `validate`
phase where up to seven *independent* subagents (code review, security, integration,
browser QA, design, alignment, load) each return a **typed verdict with evidence**, and
aggregation decides merge/recover/escalate. Anti-slop gates (typecheck + tests + build
must pass before a milestone advances) sit underneath.

**For us:** pr-triage's reviewer/fixer agents are *already* the separate verifier —
that's the architecture's latent superpower. The front-of-loop should preserve the
separation: the **implementer** writes code, a **different tester** grades it against the
contract locally, and the reviewer agent grades it again at the PR. Never let the
implementer sign off on its own behavior.

### 2.5 Human-on-the-loop, not human-in-the-loop — spend attention where being wrong hurts

Teams adopting agents report ~98% more PRs and ~91% more review time — "code emerges
faster than anyone can review it." The paradigm shift is **human-on-the-loop**: the
human oversees at the process level and intervenes on escalation, rather than reading
every diff. Concretely:

- **Risk-based triage** (P0 auth/payments/migrations → full review; P3 formatting →
  auto-approve on green CI). "Gating only the riskiest 20% of PRs captures 69% of total
  review effort."
- **Automate the mechanical, reserve humans for judgment** — intent verification
  ("does this match the *requirement*, not just the prompt?"), architectural fit,
  security. This is the "review sandwich."
- **Structure for reviewability** — small PRs (reject > ~250 changed lines), stacked
  changesets, validate the spec *before* generation so requirements aren't discovered
  during review.

**For us:** this *is* pr-triage's thesis, already committed in ADR-0007. The research
just confirms it and extends it *upstream*: the cheapest attention-saving move is a good
charter + behavioral contract at kickoff, because it prevents the drift that would
otherwise cost a human escalation later.

---

## 3. How this maps onto pr-triage's existing commitments

The research isn't a new direction; it's the *front half* of the direction already
chosen. The alignment is unusually clean:

| Field practice (2026) | pr-triage already has / believes |
| --- | --- |
| Spec is source of truth; code is a verifiable artifact | Local state is source of truth ([[0006]]); GitHub is substrate |
| Deterministic gates + probabilistic judgment | Two-layer model: pre-scan tiers (mechanical) + review agent (AI) |
| Builder ≠ verifier (adversarial validation) | Separate reviewer/fixer agents; agents run in isolated worktrees |
| Typed verdict with evidence | Report contract; "evidence over conclusions" (ADR-0007 §4) |
| Human-on-the-loop, risk-based triage | Risk-tier routing in config ([[0005]]); escalate-only-when-needed |
| Progressive disclosure; avoid overwhelm | ADR-0007 §5; "manage human attention" is the whole point |
| Per-project tailoring of what's "expected" | `chunk-setup-agent` + `per-chunk-triage-config` |

**Takeaway:** the front-of-loop is not a pivot. It's the same attention-management loop,
extended to its natural starting point. That fact is also the core of the packaging
recommendation in §6.

---

## 4. The concept: a "Chunk Charter + Behavioral Contract" front-of-loop

A kickoff flow (a **human checklist** + interactive agent + view) that, for a chunk,
produces and maintains the artifacts below and wires them into the existing reactive
loop. The human works *from the checklist, with the agent* — the checklist is the
human's spine; the agent assists at each step and the human ratifies.

### 4.1 Artifacts (what gets written down)

0. **Human kickoff checklist** — the human-facing spine of the flow: a short, ordered
   list of decisions the owner must make/ratify to open a chunk (charter drafted →
   grounding reviewed → charter ratified → behavioral contract ratified → tests confirmed
   red → implement). The agent drives each item; the human checks it off. It exists so
   the human always knows *where they are* and *what decision is theirs next* — attention
   routing applied to kickoff itself, not just review.

1. **Chunk Charter / Contract** — the spec, in the SDD six-element shape: outcomes,
   scope boundaries (what is explicitly *out*), constraints, prior decisions, task
   breakdown, verification criteria. This is also the `chunk-setup-agent`'s input for
   tailoring the scanner's "expected scope" so intermediate changes stay routine instead
   of all-escalating.

2. **Behavioral Contract** — a small set of `Given/When/Then` acceptance scenarios in
   the chunk owner's language, executable where possible. This is the human-verifiable
   endpoint definition. One behavior per scenario; concrete Then; no UI-scripting.

3. **Behavioral Eval Set** *(only where asserts can't reach)* — 5–20 judgment-behavior
   checks graded by an LLM-judge with a rubric, for "did it actually do what the ticket
   meant," instruction-following, and negative cases ("must NOT do X").

All three live in the target repo under the chunk's `docs/<chunk>/` knowledge-base
convention (owned by the chunk owner, *not* projected into GitHub as noise — consistent
with the minimal one-way projection rule in [[app-github-boundary-and-experience]]).
pr-triage *reads* them; it authors them collaboratively but the human ratifies.

### 4.2 Agents (who does what — and the separation that matters)

- **Charter / planning agent** — interviews the owner (fewer questions when the
  repo/issue is rich, more when it's thin — exactly the `chunk-setup-agent` heuristic),
  drafts the charter, and helps **splash out the issues** for the chunk. Its first job
  is **grounding**: before proposing anything, it reads the knowledge base and points the
  charter at the *specific prior decisions that bind this work* — exactly the way this
  spike's Codex example cited [[cost-basis-honesty]], [[hard-fail-philosophy]], and the
  capability table as constraints. Concretely it should surface:
    - **Impacted ADRs** — which accepted decisions this chunk touches or must not violate
      (the highest-value grounding; an ADR conflict is a design smell to raise *now*).
    - **Relevant domain / knowledge-base facts** — the single-fact docs under `docs/`
      that constrain this area (cost basis, result-shape normalization, dedup, etc.).
    - **Current build state, if resuming** — `STATE.md` and `deferred/` for the chunk.
      (Less important when *starting* a chunk; the domain facts and ADRs matter more.)
    - **The repo's test standard** — detect (or ask for) the local convention for
      acceptance/charter-level tests and record it as a charter field, so the TDD agent
      binds to the house style instead of inventing one (see §5.1).
  The output isn't just a charter — it's a charter whose "constraints" and "prior
  decisions" sections are *linked to real files*, so the contract is enforceable and the
  human can see what it rests on. *Assists; the human ratifies.*
- **TDD behavioral-testing agent** — the heart of the flow. Takes the ratified charter's
  behavioral contract and **implements the tests, then shows them all failing (red)
  before any implementation exists.** Two non-negotiables:
    1. **Red-first** — the human sees the tests fail for the right reason (behavior
       absent), confirming the tests actually encode the intended behavior before a line
       of implementation is written. Red that turns green *is the evidence*.
    2. **Human-readable** — good test names, clear assertions, one behavior per test, and
       expectations a human can read at a glance (`expects X … does not do Y`). The human
       reviews the *tests themselves* as the ratification act; if the test is unreadable,
       it can't serve as the contract. (Guard against the field's failure mode: vague
       Then-steps, multi-behavior tests, happy-path-only.)
  *Assists; the human ratifies the red tests before implementation begins.*
- **Implementer agent** — does the work against the now-ratified, red contract (this is
  the "implementer" the owner already names). Its target is simply: make the red go green.
- **Tester/verifier agent** — runs the behavioral contract + evals **locally, before a
  PR exists**, and returns a typed verdict with evidence. **Must be a different agent
  than the implementer** (adversarial validation, §2.4). Its verdict is what decides
  whether a PR is even opened.

Then the existing reactive loop takes over: pre-scan tiers the PR (now with accurate
"expected scope"), the review agent grades against the same contract, escalation
surfaces only what needs a human.

### 4.3 The loop, end to end

```
        ┌──────────────── front-of-loop (new) ─────────────────┐
Issue → Charter/planning agent → [human ratifies charter]       │
        (grounds in impacted ADRs + domain facts)               │
      → TDD agent writes tests → all RED → [human ratifies red tests]
      → Implementer makes them GREEN → Tester agent (≠ implementer)
                            │                     │
                            │              typed verdict + evidence
                            ▼                     ▼  (pass)
        └──────────────────────────────────── open PR ─────────┘
                                                  │
        ┌──────────── reactive loop (exists today) ────────────┐
                    pre-scan tiers ─► review agent grades vs contract
                                             │
                              escalate only what needs a human
        └───────────────────────────────────────────────────────┘
```

The charter/contract is the single thread running through both halves — authored up
front, enforced at every gate, and the thing the human actually verifies at the end.

---

## 5. Behavioral testing taxonomy for this tool (what "beyond unit tests" means)

Four layers, cheapest/most-deterministic first (deterministic-first, per ADR-0007 §2):

| Layer | Question it answers | Grader | Human role |
| --- | --- | --- | --- |
| **Unit tests** | Do the parts compute correctly? | code | none (CI gate) |
| **Contract/acceptance (Given/When/Then)** | Does the system do the expected *behavior*? | code where executable | ratify scenarios; read failures |
| **Integration / behavioral evals** | Did it do what the *ticket meant* (judgment, negative cases)? | LLM-judge + rubric | ratify rubric; spot-check transcripts |
| **Charter alignment** | Is this the right thing, built the expected way? | reviewer agent + human | decide on escalation |

The owner's instinct is correct: unit tests are "cheap and easy for AI to write but
don't test whether it does what it's supposed to." The value is concentrated in the
middle two layers — the behavioral contract and the evals — which are exactly the layers
current unit-test-only workflows omit.

**Guardrail (from §2.3/§2.5):** more tests is not the goal; *evidence* is. Watch for
saturation (all-green tells you nothing), grade the output not the path, and periodically
have a human read transcripts to confirm the grader is honest.

### 5.1 The charter-scenario test standard (stack-agnostic)

pr-triage runs against many repos — Go here, Node/TypeScript next, Python after that. The
language, test runner, and infrastructure will all change; **the process and the bar for
charter tests must not.** So we standardize *properties*, not a framework. The key
separation that makes this portable:

> **The scenario is the durable, portable artifact. The test is its stack-local
> binding.** The Given/When/Then scenario is written once, in the owner's language, and
> survives across stacks. The TDD agent *binds* it to whatever the repo actually uses
> (Go `testing`+testify, Vitest/Jest, pytest, Catch2, Playwright for UI, a TUI driver…).
> The standard below governs the binding so it stays readable and rigorous everywhere.

**Level-set early (a charter step, not an afterthought).** Before writing any test, the
charter/planning agent must **detect or ask for the repo's existing test standard** —
specifically for acceptance/charter-level tests, which often differ from unit-test
convention:
- Existing test layout, runner, and assertion/matcher library.
- Any house style for acceptance/e2e/golden tests (naming, fixtures, snapshot policy).
- The local tool for each endpoint type the chunk needs (golden-file, component, TUI,
  visual).
Record the answer as a charter field — **"Test standard (this repo)"** — so the TDD agent
obeys the house style instead of inventing one. If none exists, the agent proposes one
from this standard and the human ratifies it (that itself becomes a small ADR-like
decision for the repo).

**Charter tests are held to a higher bar than unit tests — on two axes:**

1. **Readability — approaching pseudocode.** A junior engineer should read the test and
   say *"yes, this expects X and specifically not Y"* without reading the implementation.
   This is a solved problem *if you use the stack's most expressive assertion layer* — the
   way C++ went from cryptic `assert(a==b)` to Catch2/Hamcrest matchers that read like
   prose (`REQUIRE_THAT(result, Equals(expected))`). General rule: **prefer the most
   expressive matcher/assertion library the stack offers**, name the test as a sentence
   about behavior, keep one behavior per test, use named fixtures and concrete expected
   values, and make the failure message say what was expected vs. observed.

2. **Resolution — it tests something *vital to acceptance*, at the endpoint.** Charter
   tests assert observable behavior and output *contracts*, not internal wiring. They
   must be falsifiable (fail red for the right reason), prefer precise **golden/fixture
   contracts** where an output is exact, and carry balanced must-do / must-not-do cases.
   A charter test that can't distinguish "did the chunk's promised thing" from "didn't"
   is not a charter test.

**Grader choice follows the endpoint (deterministic-first).** Use code/golden graders for
anything precise; reserve the LLM-judge for genuinely judgment behaviors (e.g. "is the
error message clear?") — never for something a fixture could pin exactly.

### 5.2 Endpoint-type playbook

A chunk's endpoint takes different shapes; each maps to a scenario + a stack-local test.
The *shape of the scenario* is portable; only the binding tool changes per repo.

| Endpoint kind | Portable scenario shape | Grader / evidence | Watch out for |
| --- | --- | --- | --- |
| **Input validation** (e.g. input from a prior chunk) | Given valid/invalid input → accepts, or rejects **with the specific reason** | code; boundary + negative cases mandatory | happy-path-only; vague rejection ("just fails") |
| **Calculation → output contract** (calc lands in a report) | Given input fixture → output **exactly matches** the golden fixture/contract | golden file / snapshot of *data* | drifting the fixture to match a bug; over-broad snapshots |
| **UI component** | Given interaction → behaves like X (state/DOM) **and** looks like Y | component test (behavior) + visual/snapshot (appearance) | snapshot brittleness; asserting pixels not behavior |
| **TUI interaction** | Given keystrokes/input → expected rendered frames / state transitions | TUI driver + frame/golden assertion | terminal nondeterminism (size, color, timing) |
| **Background logic / side effect** | Given trigger → observable effect (event emitted, state written, message sent) | assert at the boundary (the emitted event/row), not internals | testing implementation instead of the observable effect |

The **calculation → golden output contract** is the strongest and most portable kind:
it's deterministic, cheap, precise, and reads as *"take this input, run the process, get
exactly this output."* Reach for it whenever the endpoint is a value or a document. The
fuzzier the endpoint (UI look, message tone), the more you lean on snapshots + a narrow
LLM-judge — and the more explicit the human ratification of *what "correct" means* has to
be, because there's no exact fixture to hide behind.

---

## 6. Attention design for the front-of-loop (so it doesn't become overwhelm)

The front-of-loop *adds* a surface, so it must earn its attention the same way the
reviewer view does. Apply ADR-0007 directly:

- **Progressive disclosure.** Default output at kickoff is the minimal decision:
  *"Here's the charter and N behavioral scenarios I propose — ratify, edit, or expand."*
  Detail (full rubric, per-scenario reasoning, repo inspection notes) reveals on demand.
  Curate; do not dump the whole spec.
- **Evidence over conclusions; mark AI vs. deterministic.** When the tester agent
  returns a verdict, show the failing scenario + the observed behavior, not just
  "failed." Distinguish "deterministic assert failed" from "LLM-judge scored 2/5, here's
  the rubric line."
- **The human ratifies, the AI assists.** Charter and contract are *proposed*, never
  auto-committed. The human remains the decision-maker — the point where intent is
  verified is at *ratification*, cheaply, before any code exists.
- **Batch the human's touchpoints.** Two natural approval gates (charter, contract), not
  a stream of micro-approvals. Everything between them runs on-the-loop.
- **Don't skew the read.** Present balanced scenarios (behaviors that *should* and
  *shouldn't* happen), so ratification isn't rubber-stamping a one-sided happy path.
- **The human checklist is the attention spine.** The kickoff checklist (§4.1, item 0)
  is itself an attention-routing device: it shows the human exactly where they are and
  which decision is theirs next, so kickoff never feels like an open-ended interrogation.

### 6.1 UI north star: watching red turn green

The behavioral contract makes a chunk's progress *visible as state*, which is the seed of
a much better UI than a diff. A future higher-def TUI/GUI could show a chunk as its list
of behavioral scenarios and **render them going red → green** as the implementer works —
the human watches *behavior being satisfied*, not lines changing. This is the natural,
motivating surface the daemon's event stream already makes possible (the same
`internal/events` channel every runtime emits through). It's explicitly *later* work —
the plumbing (contract-as-state + events) must be right first — but it's the north star
worth designing the state model toward now: **a chunk is a set of behaviors, each with a
red/green status and its evidence.** Keep that shape in the state layer and the UI comes
almost for free.

The whole design goal: convert one expensive, late attention event (reviewing a drifted
PR) into one cheap, early one (ratifying a charter), and let determinism carry the
middle.

---

## 7. Architecture: companion app vs. bundled

This is the section the owner asked to *learn* from, so it's written to teach the
reasoning, not just state the verdict.

### 7.1 The real question

"Companion app vs. bundled" is really: **where do you draw the module boundary, and
where do you draw the *deployment* boundary?** Those are two different decisions, and
conflating them is the classic mistake. You can have strong internal modularity *without*
a second deployable. The interesting choice is almost never "one giant blob vs. many
services" — it's **modular monolith vs. distributed system**, and the default should be
modular monolith until a specific force pushes you off it.

### 7.2 What the two halves actually share

The front-of-loop and the reactive loop share almost their entire core:

- **Agent runtime** (the OpenCode/Claude adapter, model routing, isolated worktrees).
- **Orchestration** (spawning agents, collecting typed verdicts, the state machine).
- **State store** (local SQLite as source of truth — the charter/contract/verdicts are
  just more state).
- **GitHub seam** (one-way, idempotent projection — same discipline for both halves).
- **The two-layer config model** (mechanical tiers + probabilistic agent def) — the
  front-of-loop *produces* config the reactive loop *consumes*.
- **The charter/contract itself** — literally the same object, authored in front,
  enforced in back.

When two features share ~80% of their substrate and pass a shared object directly between
them, they are **highly cohesive**. High cohesion is the signal to keep them in one
codebase. A separate companion app would force that shared core to become a *published
interface* between two repos — you'd either duplicate it (two-place drift, the exact
thing ADR-0007 warns about with GitHub) or extract it into a shared library you now
version and release independently. Both are real, ongoing costs.

### 7.3 The server/interface split already anticipated this

[[app-github-boundary-and-experience]] already commits to a **server (state authority +
reconciler + local API)** with **thin interfaces that read the API**. That is the correct
seam and it *already exists in intent*. The front-of-loop is just:

- new **domain modules** in the core (charter, behavioral-contract, eval-runner), and
- a new **interface** (a kickoff command + view) that reads/writes through the same
  local API.

No new deployable is required to get there. The "companion app" instinct is really a
wish for a *distinct interface surface* — and you get that from the interface layer, not
from a second server.

### 7.4 Recommendation

**Bundle it. Build the front-of-loop as modules + a new interface inside pr-triage,
behind a clean internal seam, sharing the core.** Concretely:

- Put front-of-loop domain logic in its own `internal/` packages (e.g.
  `internal/charter`, `internal/behavioral`) with a **narrow, explicit API** to the rest
  of the core — so the module boundary is real even though the deployment boundary is
  not.
- Expose it as a **distinct command/subcommand and view** (e.g. `pr-triage kickoff`),
  not a separate binary/server.
- Keep the GitHub projection for these artifacts **minimal or zero** — the charter and
  contract live in the target repo's `docs/`, not as GitHub comments.

**Why this is the right default:**

- *Cohesion/coupling* — the two halves share their substrate and exchange a shared
  object; splitting them creates a distributed-systems tax (versioned interfaces, two
  deploys, cross-process consistency) with no offsetting benefit today.
- *Distribution is a cost, not a feature* — you take on network/versioning/operational
  complexity the moment you split. Pay it only when a force demands it.
- *The modular seam keeps extraction cheap later* — if a force *does* arrive, a
  well-bounded `internal/charter` package with a narrow API extracts into its own service
  in an afternoon. You lose nothing by waiting; you lose weeks by splitting early.

### 7.5 When to revisit (the forces that would justify a separate app)

Split into a companion deployable only when one of these becomes true:

- **Team scale / shared authority** — multiple humans need a *shared* state authority
  (the doc already notes today's daemon is single-user/local SQLite). A shared server is
  the trigger — but note that's a reason to grow *the server*, not to fork a second app.
- **Independent release cadence** — the kickoff surface needs to ship on a wildly
  different schedule or to a different audience (e.g. non-engineers authoring charters in
  a browser) than the reviewer daemon.
- **Different runtime/host** — the front-of-loop wants to run somewhere the daemon can't
  (a hosted web app, a different security boundary).

None of these hold now. Ship it bundled; keep the seam clean; extract on evidence.

### 7.6 One-slide mental model

> **Cohesion decides the *module* boundary. A concrete force decides the *deployment*
> boundary.** Default to a modular monolith. Let interfaces (not servers) give you
> distinct surfaces. Only distribute when team-scale, release cadence, or runtime
> forces you to — and design the module seam now so that day is cheap.

---

## 8. Risks & anti-patterns to design against

- **Charter theater** — a beautiful charter nobody enforces. Mitigation: the same
  charter must feed the scanner's expected-scope *and* be the reviewer's grading oracle,
  or it's decorative.
- **Self-confirmation** — implementer grading its own behavior. Mitigation: enforce
  builder ≠ tester ≠ reviewer (§2.4).
- **Test volume as vanity metric** — lots of green unit tests, no behavioral coverage.
  Mitigation: taxonomy in §5 weights the middle layers; watch saturation.
- **Approval fatigue** — too many micro-ratifications. Mitigation: two batched gates
  (charter, contract), everything else on-the-loop.
- **Gherkin drift** — vague Then steps, multi-behavior scenarios. Mitigation: explicit
  authoring guardrails for the behavioral-test agent.
- **Premature distribution** — building the companion app first. Mitigation: §7.

---

## 9. Recommended next steps (cheap experiments before committing an epic)

1. **Paper spike on one real chunk.** Hand-write a charter + 5 Given/When/Then scenarios
   for a chunk you're about to do; run the existing reactive loop with them as the
   reviewer's oracle. Measure: did escalation noise drop? Did the reviewer catch drift it
   otherwise wouldn't? (This tests the *value* with zero code.)
2. **Prototype the behavioral-test author agent** as a skill/prompt only, over the
   ratified charter, with the BDD guardrails from §2.2 — evaluate scenario quality by
   hand.
3. **Prototype the tester agent** running those scenarios locally and returning a typed
   verdict — confirm the builder≠tester separation is enforceable in the current runtime.
4. **Define the `internal/charter` module API on paper** — the narrow seam from §7.4 —
   before writing it, to lock in the modular-monolith boundary.
5. Fold the results into the "stateful control plane & reviewer experience" epic
   candidate named in [[app-github-boundary-and-experience]]; the front-of-loop is its
   natural companion workstream.

---

## 10. Open questions

**Resolved (2026-08-28) — scenario vs. test, and cross-stack portability.** The scenario
is the durable, portable artifact; the test is its stack-local binding (§5.1). We
standardize the *properties* of a charter test (readability approaching pseudocode; higher
resolution than unit tests; golden contracts where precise), not a framework — because
the stack varies per repo. The repo's own test standard is detected/ratified early as a
charter field. This resolves the earlier "Go tests vs. Gherkin / who owns the language"
tension: the owner reads and ratifies the **scenario**; the agent binds it to the repo's
tooling under the readability bar.

- **EARS vs. plain Given/When/Then** as the charter's acceptance-criteria notation — EARS
  is more precise but heavier; Gherkin is more readable. Possibly EARS for constraints,
  Gherkin for scenarios.
- **Where do evals run** — inside the existing worktree/runtime, or a dedicated harness?
  (Isolation/clean-state matters for eval stability, per §2.3.)
- **How much of the charter, if any, is ever projected to GitHub** — default is nothing;
  but a one-line "chunk endpoint defined" trail marker might aid collaborators. Decide
  against the minimal-projection rule.
- **Relationship to `chunk-setup-agent`** — is the charter agent an *extension* of it, or
  a sibling that runs first and feeds it? (Leaning: charter agent runs first and produces
  the input `chunk-setup-agent` already wants to read.)
- **Concurrent multi-chunk** — the front-of-loop makes the one-active-base_ref-per-repo
  limit (noted in [[chunk-setup-agent]]) more visible; may need revisiting sooner.

---

## Sources

- [Spec-Driven Development in 2026 (DEV Community)](https://dev.to/krlz/spec-driven-development-in-2026-what-it-is-the-tooling-and-how-teams-actually-use-it-2fk2)
- [Spec + TDD: Shippable AI Code (Augment Code)](https://www.augmentcode.com/guides/spec-tdd-shippable-ai-generated-code)
- [What Is Spec-Driven Development? (Augment Code)](https://www.augmentcode.com/guides/what-is-spec-driven-development)
- [Spec-Driven Development: AI-Native Engineering (Microsoft for Developers)](https://developer.microsoft.com/blog/spec-driven-development-ai-native-engineering/)
- [Spec-Driven Development: The Definitive 2026 Guide (bcms)](https://www.thebcms.com/blog/spec-driven-development/)
- [Comprehensive Guide to SDD: Kiro, Spec Kit, BMAD (Medium)](https://medium.com/@visrow/comprehensive-guide-to-spec-driven-development-kiro-github-spec-kit-and-bmad-method-5d28ff61b9b1)
- [aws-samples/sample-specship (GitHub)](https://github.com/aws-samples/sample-specship)
- [Kiro Agentic AI IDE — Spec Driven (AWS re:Post)](https://repost.aws/articles/AROjWKtr5RTjy6T2HbFJD_Mw/)
- [Demystifying evals for AI agents (Anthropic)](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)
- [Eval Driven Development (DeepEval)](https://deepeval.com/blog/eval-driven-development)
- [Automating Eval-Driven Development for Agentic Applications (Fiddler AI)](https://www.fiddler.ai/blog/automating-eval-driven-development-agentic-applications)
- [The Human Review Bottleneck (Daniel Vaughan / Codex KB)](https://codex.danielvaughan.com/2026/05/24/human-review-bottleneck-code-review-strategies-agent-output/)
- [Reviewing AI-Generated Code: A Verification Discipline (Augment Code)](https://www.augmentcode.com/guides/reviewing-ai-generated-code)
- [Agentic Code Review (Addy Osmani)](https://addyo.substack.com/p/agentic-code-review)
- [Code Review Is Dead: Verification, Not Approval (Codacy)](https://blog.codacy.com/code-review-is-dead-why-ai-generated-code-needs-verification-not-human-approval)
- [BDD Gherkin Guidelines for AI Coding and Testing (Automation Panda)](https://automationpanda.com/2026/04/27/bdd-gherkin-guidelines-for-ai-coding-and-testing/)
- [Gherkin User Stories & Acceptance Criteria: 2026 Guide (TestQuality)](https://testquality.com/gherkin-user-stories-acceptance-criteria-guide/)
- [Spec-Driven Development: From Code to Contract (arXiv 2602.00180)](https://arxiv.org/html/2602.00180v1)
