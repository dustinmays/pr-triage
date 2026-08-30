# Issue #129 Codex runtime front-of-loop dogfood report

Working, instrumented report for the manual charter → RED-first behavioral tests →
implementation → independent verification → reactive-review experiment. Entries marked
**instrumented** use command/session evidence captured during the run; entries marked
**estimated** are retrospective estimates.

## A. Header

- **Chunk / issue:** Codex runtime adapter / #129
- **PR(s):** #133 — implementation; #134 — disposable full-diff Codex runtime dogfood
- **Dates:** 2026-08-29 through 2026-08-30
- **Implementation orchestration runtime / model:** Codex session (exact host model ID not surfaced)
- **Delegated RED-test runtime / model:** OpenCode 1.18.23 / `openrouter/z-ai/glm-5.3-flash` (interrupted session `ses_fb0917065ffe2Hsf0SotS5xFwn`) → `openrouter/google/gemini-3.7-flash`
- **Delegated production implementation runtime / model:** OpenCode 1.18.23 / `openrouter/google/gemini-3.7-flash` (`ses_fb05e14f0ffeiW5anBVgJUWjJo`)
- **Reactive triage:** deterministic `chunk_completion` route automatically escalated to the human; no review runtime/model was invoked
- **Independent reactive-review runtime / model:** OpenCode 1.18.23 / `openrouter/deepseek/deepseek-v4-pro-0813`; run 25 on PR #134, ~2m47s, 17 turns, exact cost $0.070297876; clean/no edits
- **Codex runtime smoke:** PR #134 / `codex` / `gpt-5.6-sol`; completed in ~6m06s, one turn, estimated cost $1.3591696; proved auth, pre-scan readability, workspace writes, push, parsing, metrics, and review-comment delivery
- **Outcome:** success — Codex adapter implemented; original 32-test contract ratified and preserved; two human-approved additive bindings bring the suite to 34; PR #133 and CI are GREEN; deterministic triage escalated the chunk-completion PR as designed; Codex smoke succeeded; independent DeepSeek review found no remaining in-scope defects

## B. Scorecard

- **Implementation attempts / first-pass success:** 1 / yes at the ratified GREEN gate. The production pass added the adapter, binary registration, and doctor Git initialization; targeted contract tests and the agent's full suite passed without implementation-stage rework.
- **Rework cycles:** 1 post-GREEN cycle, found by Codex smoke/review: envelope writes ignored errors and short writes, allowing launch without the model metadata required for honest cost. The reviewer added two negative bindings to the existing envelope scenario and a production fix; the human explicitly approved the additive contract change. One separate reporting correction: the implementation agent summarized “19 adapter tests,” while deterministic inventory showed 32 at ratification; no code changed for that correction.
- **Reactive triage:** 1 run / deterministic human escalation as designed. Evidence: `needs-owner-review` plus an escalation comment naming `target_kind "chunk_completion"`; no LLM reviewer was invoked.
- **Runtime dogfood:** 1 run / success on `codex` + `gpt-5.6-sol`; one turn, ~6m06s, $1.3591696 estimated. The agent edited and the orchestrator pushed commit `b577eaa` to disposable PR #134.
- **Independent review:** 1 run / clean, no edits. DeepSeek verified the envelope fix and repeated the non-blocking shared-timeout observation; it also raised a future reasoning-token pricing question.

### Things flushed during testing and smoke

| # | What | Found by | Would a plain unit test have caught it? |
| --- | --- | --- | --- |
| 1 | Runtime-doctor probe directories must be initialized as Git repositories because production Codex invocation intentionally does not bypass the repository check. | RED test | Y |
| 2 | Ratified test bodies and goldens are not mechanically protected: changed assertions are not signaled and generic `testdata/` paths are excluded from pre-scan. | Human at RED gate | N |
| 3 | `base_ref` filters PR destination, only one selector is persisted, and a stale `main` registration made the disposable implementation PR invisible. | Human + smoke | N |
| 4 | Reusing one head SHA across two PRs can mix same-named report check runs and make report selection ambiguous. | Smoke | N |
| 5 | Invocation-envelope write errors and short writes allowed launch without the model identity required for honest cost attribution. | Codex reactive review | Y |
| 6 | Shared deadline termination can be classified as failure instead of `OutcomeTimeout`; this is cross-adapter and deferred from #129. | Codex + DeepSeek reactive reviews | Y |
| 7 | Review comments linked to temporary worktree paths that became dead after cleanup. | Smoke | N |
| 8 | Codex reports reasoning-output tokens separately, while current price calculation does not have a separately ratified rate for them. | DeepSeek reactive review | N |

**Findings invisible to plain unit tests:** 5/8 = **62.5%**, versus the OpenCode
baseline's approximately 40%. The percentage moved upward because grounding and contract
tests removed ordinary adapter unknowns early, leaving a smaller set concentrated at
GitHub identity/configuration, governance, and human-attention seams. This is not evidence
that unit tests regressed; three unit-catchable defects were in fact made executable.

### Things not considered ahead of time

This bucket counts findings first surfaced after charter ratification; grounding findings
that were incorporated into the ratified charter are intentionally excluded.

| Bucket | Count | Items |
| --- | ---: | --- |
| Tool-contract unknowns | 1 | Whether separately reported reasoning-output tokens require a distinct pricing rule |
| Unplanned scope/gaps | 2 | Invocation-envelope write completeness; shared timeout classification |
| Environment surprises | 2 | Persisted destination-base filter mismatch; same-SHA/multiple-PR check-run ambiguity |
| Other | 1 | Ephemeral worktree links in durable review comments |
| **Total** | **6** | |

Tool-contract unknowns fell to **1/6**, compared with the baseline's **5/9**. Grounding
converted the larger Codex CLI unknowns—auth, sandbox, Git requirement, JSONL shape,
model-identity absence, usage semantics, and real failure events—into ratified decisions
before implementation. The six residual findings were mostly integration and projection
behavior that a runtime-only charter could not settle.

### Deviations

| Class | Count | What changed |
| --- | ---: | --- |
| Did more (good) | 5 | Companion-terminal/readability playbook; manual protected-contract checkpoint/audit; ADR 0010; disposable full-diff runtime smoke PR; additive-test governance refinement |
| Did different but necessary | 4 | GLM session replaced by Gemini after no output; disposable PR used because the real `main` PR deterministically escalates; unique marker SHA isolated checks; reviewer additions entered through a human contract gate |
| Did less/nothing (bad) | 0 | None |
| Product-side unplanned | 5 | Missing protected-contract signal; single destination-base selector; same-SHA report identity ambiguity; ephemeral review links; shared timeout classification |

Counts are instrumented from the timeline; category placement is a synthesis judgment.

### Headline versus the OpenCode baseline

| Measure | This run | Baseline | Movement and likely cause |
| --- | ---: | ---: | --- |
| Implementation attempts / first-pass GREEN | 1 / yes | comparison shape retained | Strong: grounded CLI contracts and RED bindings gave the implementer a stable target. |
| Post-GREEN rework cycles | 1 | comparison shape retained | Small but nonzero: reactive smoke added a missing negative boundary without changing ratified behavior. |
| Things not considered ahead | 6 | 9 | Down 3: grounding absorbed most runtime tool-contract questions before coding. |
| Tool-contract unknowns | 1/6 | 5/9 | Materially down: empirical probes plus ADR/runtime-kit reading were the main cause. |
| Findings invisible to plain unit tests | 62.5% | ~40% | Up: the residual set shifted toward workflow topology, GitHub identity, governance, and durable presentation. |

## C. Front-of-loop process evaluation

### Grounding ROI — preliminary

Grounding materially changed the ratification candidate. The draft charter did not know
about the now-shipped adapter kit, assumed a shared model/price definition that does not
exist, named a stale automation credential, and described a production event channel that
has types/tests but no emission site. The empirical Codex JSONL probe also exposed an
interface mismatch: usage is emitted, but model identity is not, while `ParseResult` receives
only the log. These are all issues that would otherwise have been discovered during
implementation or smoke testing.

### Ratification delta — what was changed/missing in the original charter

The human ratified the grounded charter at **2026-08-29 15:23 MDT**. Relative to the
original draft, ratification recorded as binding: kit-native construction (ExecRun +
Capabilities + runtimetest — the draft predated ADR 0009); the exact `codex exec
--json --ephemeral --sandbox workspace-write` invocation contract (inline prompt, cwd
worktree, exact `-m` passthrough, no slash validation, no production
`--skip-git-repo-check`); ParseResult semantics on captured JSONL (agent_message →
Summary, terminal turn events → runtime-local turns) with known priced models
estimated from usage and unknown models Cost=0/CostBasisUnavailable; the
adapter-written namespaced invocation envelope (usage lacks model identity and
ParseResult is reader-only — missing from the draft entirely); saved `codex login` /
invocation-scoped `CODEX_API_KEY` auth (the draft's `OPENAI_API_KEY` credential was
stale); runtime-doctor Git-repo initialization; the narrowed shared-event scenario
(production has no emitter wiring — the draft's scenario could not literally pass);
timeout-only v1 reaffirmed; the stdlib Go test standard with the config-selection
scenario exempt from all-new-tests-red; and final smoke separately proving auth,
pre-scan readability, and workspace writing.

### Test Suite Architecture, Envelope Schema, Pricing, and Governance

- **Adapter Test Count & Coverage:** 32 tests were ratified at Gate 2. The current suite has 34 tests in `internal/runtime/codex`: `capabilities_test.go` (6), `parse_result_test.go` (13), `run_test.go` (10), `classify_test.go` (4), and `conformance_test.go` (1). The two additions are human-approved negative bindings to the already-ratified invocation-envelope scenario.
- **Envelope Schema:** Because Codex CLI 0.151.0 NDJSON events emit token usage but not the model identifier, the adapter writes an initial namespaced structured invocation envelope:
  `{"pr_triage_codex":{"version":1,"kind":"invocation","model":"gpt-5.6-sol"}}`
  Exact schema: root key `pr_triage_codex`, `version: 1`, `kind: "invocation"`, `model: "<string>"`.
- **Pricing Source and 0.0409608 Calculation:** Current official OpenAI pricing verified 2026-08-29 at `https://developers.openai.com/api/docs/models/compare` for `gpt-5.6-sol`:
  - Uncached input: USD 4.00 per 1M tokens ($0.000004/token)
  - Cached input: USD 0.40 per 1M tokens ($0.0000004/token)
  - Output: USD 20.00 per 1M tokens ($0.00002/token)
  Captured usage from empirical `codex-0.151.0-success.jsonl` fixture: 16,426 input tokens (6,912 cached input tokens) and 7 output tokens.
  Expected cost calculation:
  `uncached_input = 16426 - 6912 = 9514`
  `uncached_cost  = 9514 * 4.00 / 1e6   = 0.038056`
  `cached_cost    = 6912 * 0.40 / 1e6   = 0.0027648`
  `output_cost    = 7    * 20.00 / 1e6  = 0.000140`
  `total_cost     = 0.038056 + 0.0027648 + 0.000140 = 0.0409608 USD`
- **Fixture Governance and Normalization:** The two JSONL fixtures (`codex-0.151.0-success.jsonl` and `codex-0.151.0-failed.jsonl`) preserve the empirically captured Codex 0.151.0 event shapes and token counts; only stable thread/item IDs and safe error wording were normalized. Human Gate 2 ratified them as golden contracts at 2026-08-29 16:25 MDT.
- **Relied-Upon Generic Orchestrator Coverage:**
  - `TestOrchestrator_HandleReportReady_ValidAndDone` proves report fetch to routing/runtime to metrics and posted summary.
  - `TestBuildReviewPrompt` proves pre-scan content reaches the runtime prompt.

### Process Friction and Rework (RED Test Suite Delegation)

The first RED OpenCode session was `ses_fb0917065ffe2Hsf0SotS5xFwn` on `openrouter/z-ai/glm-5.3-flash`; it spent a long planning pass and wrote nothing, so it was interrupted. At the human suggestion the same task resumed on the available catalog ID `openrouter/google/gemini-3.7-flash`, which immediately authored the test suite. This is recorded as observed process friction/rework, not a product implementation attempt.

### Human Gate 2 companion-terminal playbook

The RED gate needs a human-readable inspection path, not only an agent's test summary.
Run these commands from the isolated worktree:

```bash
cd /private/tmp/pr-triage-codex-runtime
git status --short
git diff --stat
git ls-files --others --exclude-standard
```

Important: ordinary `git diff` and `git diff --stat` omit untracked files. At this
gate that includes the new `internal/runtime/codex` package and its golden fixtures,
so `git status` plus `git ls-files --others` are required for a complete inventory.

Read the behavioral intent from broadest to narrowest:

```bash
bat docs/charters/codex-runtime.md
rg -n '^func Test' internal/runtime/codex/*_test.go
bat internal/runtime/codex/capabilities_test.go
bat internal/runtime/codex/run_test.go
bat internal/runtime/codex/parse_result_test.go
bat internal/runtime/codex/classify_test.go
bat internal/runtime/codex/conformance_test.go
bat internal/runtime/codex/testdata/*.jsonl
```

The file-to-intent map is: `capabilities_test.go` declares honest limitations;
`run_test.go` pins the CLI boundary; `testdata` plus `parse_result_test.go` pin the
golden event contract and normalized result; `classify_test.go` pins outcomes;
`conformance_test.go` applies the shared runtime kit; and the tests under `cmd` and
`internal/cli` pin binary registration, doctor setup, and effective config routing.
Each of the original 35 behavioral/external-gate tests has a two-line reader breadcrumb immediately
above it: the first line states the scenario and expected behavior; the second explains
why that contract matters to the system or operator. The two smoke-added tests follow the
same format, bringing the current total to 37.

Run the evidence in progressively smaller slices:

```bash
# Inventory without execution.
go test -list . ./internal/runtime/codex

# Entire adapter suite: expect 32 RED tests on the absent Codex registration.
GOCACHE=/private/tmp/pr-triage-codex-runtime-red-gocache \
  go test -count=1 -v ./internal/runtime/codex/...

# One representative contract at a time.
GOCACHE=/private/tmp/pr-triage-codex-runtime-red-gocache \
  go test -count=1 -v ./internal/runtime/codex \
  -run '^TestRunInvokesCodexExecWithRatifiedBaseFlags$'

# External wiring: first two should be RED; config-show is the GREEN exception.
GOCACHE=/private/tmp/pr-triage-codex-runtime-red-gocache \
  go test -count=1 -v ./cmd/pr-triage -run '^TestCodexRuntimeRegistered$'
GOCACHE=/private/tmp/pr-triage-codex-runtime-red-gocache \
  go test -count=1 -v ./internal/cli \
  -run '^TestRuntimeCheckInitializesProbeWorkdirAsGitRepository$'
GOCACHE=/private/tmp/pr-triage-codex-runtime-red-gocache \
  go test -count=1 -v ./internal/cli \
  -run '^TestInitRuntimePinsCodexRoutineRoutingAndConfigShow$'
```

Useful local viewers detected in this environment are `bat` for syntax-highlighted,
paged files; `git diff --color=always | less -R` for tracked diffs; and `lazygit` for
an interactive status/diff browser. For an untracked file without staging it, use
`git diff --no-index -- /dev/null internal/runtime/codex/run_test.go | less -R` (exit
status 1 means “different,” not a command failure).

The ratifying human should decide five things: whether the test names read as the
intended behavior; whether the captured success/failure fixtures are credible and
minimally normalized; whether the namespaced invocation envelope is an acceptable
adapter-local contract; whether the known/unknown/no-usage cost semantics are honest;
and whether any charter behavior is missing or over-specified. Approval turns the two
fixtures into golden contracts; subsequent fixture changes become an escalation.

### Contract integrity and attention-budget findings

The gate surfaced a gap between the design and the current product. Section 5.3 says a
ratified scenario or golden-fixture diff becomes a high-tier deterministic signal, but
the scanner does not implement that signal yet. It flags deleted Go test files and new
`t.Skip` calls, but not modified assertions or expected values. More importantly,
`scripts/pr-prescan.sh` deliberately excludes every `testdata/` path from all signal
evaluation, so changes to the two Codex goldens would currently be invisible to the
reactive escalation mechanism. Ratification is therefore social/manual today, not
mechanically protected.

A watched path by itself is also insufficient for a single-PR red-first workflow. The
normal pre-scan compares the PR merge base with its final head; it cannot tell that a test
was ratified at an intermediate commit and then rewritten later in the same PR. The
minimum durable mechanism needs both:

1. A ratification checkpoint outside the implementer's mutable diff: ideally the
   contract commit is already on the target `chunk/*` branch; otherwise pr-triage records
   an immutable checkpoint commit/tree SHA at the ratification action.
2. A protected-contract manifest containing scenario IDs plus identities/digests for each
   ratified test body and each golden oracle. The scanner compares those checkpoint
   objects with PR head objects. Weakening, changing, skipping, renaming, or removing an
   existing binding—or changing a golden, the manifest, or its checkpoint pointer—emits a
   `ratified_contract_changed` signal with evidence. Protected goldens override the
   scanner's generic `testdata/` exclusion. File-level hashes alone are too coarse because
   they cannot distinguish laundering from a useful additive test.

That signal should map to `escalate` by default. On chunk/epic implementation PRs the
existing escalation path applies `needs-owner-review`, and the existing
`owner-review-gate` fails until a human clears it. The contract is allowed to evolve;
the change is surfaced rather than silently accepted or absolutely frozen. Until this
product mechanism exists, this experiment should take a separate ratified test-only
checkpoint and manually compare all protected files against it before accepting an
implementation or fixture change.

The smoke refined the governance decision. The owner's concern is silent weakening or
removal, not freezing learning after kickoff. A new test may be added when it traces to an
existing ratified scenario and does not modify an existing binding; it is recorded as an
additive contract refinement rather than blocked. A new observable requirement, a changed
expected value/oracle, or any weakening/removal still returns to the human. In this run,
the reviewer added two envelope-write failure tests to `run_test.go`; the manual audit
proved zero existing ratified test lines and zero goldens changed, and the human approved
the additions before they entered PR #133.

The human-attention surface and executable-test surface must not be conflated:

| Layer | Primary reviewer | Count policy | Governance |
| --- | --- | --- | --- |
| Charter scenarios | Human | Target about 12 at most | Human ratifies the observable must/must-not behavior |
| Contract-test bindings and goldens | Independent TDD/verifier agents | As many as needed; 32 at ratification, 34 after smoke is acceptable here | Existing binding/golden changes escalate; traced additive bindings are recorded and allowed |
| Unit/regression/implementation tests | Implementer and reviewer agents | Unbounded by the charter gate | May evolve normally outside the protected manifest |

The original Codex suite should remain intact: its 32 ratified adapter tests bind only nine
charter scenarios, and the current 34-test suite adds two bindings without changing those
32. The usability error was presenting every binding as an equal human decision.
Future Gate 2 output should show the roughly dozen scenario cards, their RED evidence,
and a scenario-to-test count/map; full tests remain progressive-disclosure detail.

For future chunks with 5–12 sub-issues, the four-role autonomous flow can be: an issue
drafter traces the issue to existing charter scenario IDs; a separate TDD author writes
the RED bindings and an independent verifier confirms the right failure and registers
the checkpoint; an implementer works without authority to silently move protected
artifacts; and a reviewer receives the contract-integrity signal plus GREEN evidence.
Issue-level contracts can proceed without a new human gate when they only refine an
already-ratified scenario. A new observable behavior, changed oracle/golden, missing
charter trace, or protected-artifact diff routes back to the human.

### Trunk-orchestrator operating model

This section preserves adjacent workflow observations for AgentMinder/Trigger; it is not
a proposal to expand the front-of-loop or pr-triage scope. The experiment's product focus
is the beginning of a chunk (grounding, charter, behavioral contract, RED evidence, and
ratification) and the end (pre-scan, reactive review, escalation, and the final human
decision). Worktree lifecycle, agent dispatch, concurrency, and routine PR creation belong
to the middle orchestration system except where their handoff affects those two boundaries.
The durable architectural decision extracted from these observations is recorded in
[[0010-topology-agnostic-workflow-state]].

The trunk/chunk orchestrator should be a stateful coordinator, not another code reviewer.
The two-level topology used in the human's current workflow is one useful projection:

```text
main <- draft feature/chunk PR <- issue worktree PRs
```

It gives the human a continuously updated draft diff and CI cue while isolated child PRs
land into the feature branch. It is not a required topology for the front-of-loop or
reactive review. The durable state belongs in AgentMinder/Trigger: chunk identity, charter
version, ratification events, protected checkpoint, expected RED/GREEN phase, work-item
ownership/dependencies, verifier verdict, and reactive-review outcome. GitHub branches,
PRs, and checks are useful projections and evidence, not the only place from which that
state can be recovered.

When this topology is used, each independent sub-issue gets an isolated worktree/branch
from the feature/chunk base and a small OpenCode context packet: issue, traced charter
scenario IDs, ratified checkpoint/protected paths, owned files, and required checks.
Independent issue PRs can run concurrently; dependencies remain explicit in the external
chunk ledger.

Within any selected GitHub topology, the destination branch is part of the task contract:
the context packet includes the exact expected head and base refs; the agent must report
the resulting PR URL; and the orchestrator verifies GitHub's actual `headRefName` and
`baseRefName` before accepting the handoff or starting triage. A mismatch is a cheap
deterministic stop-and-correct condition. This is an adapter-boundary validation that
AgentMinder/Trigger may own, not a mandate for a particular branch hierarchy.

When a PR is used as the handoff and audit artifact, full agent transcripts stay in files
or the agent runtime and are not streamed into the trunk context. The orchestrator waits
for a terminal handoff (PR URL or bounded failure summary) and normally reads only:

| Evidence | Orchestrator attention |
| --- | --- |
| PR/base/head and changed-file summary | Track ownership, dependency, and scope |
| CI/check status | Advance on deterministic GREEN; route persistent failure |
| Pre-scan present signals and evidence | Escalate only the named risk/contract facts |
| Protected-contract integrity result | Stop immediately on drift |
| Reactive review verdict and findings | Act on findings; do not duplicate routine review |

The orchestrator should not tail agent reasoning, reread an un-escalated diff, or launch a
second general review “for confidence.” Those duplicate the reactive pipeline and consume
the same attention the product is meant to save. Its high-value work is maintaining the
chunk state/traceability ledger, deciding what may run in parallel, enforcing gates,
summarizing evidence, and resolving only cross-issue conflicts, repeated blockers,
contract changes, or review escalations. In the current two-level workflow the draft
feature/chunk PR to `main` is the batched human decision point; another workflow may
project the same durable gates differently.

This experiment also exposed a topology constraint at reactive-review time. PR #133 was
opened directly against `main`, so pre-scan correctly classified it as
`chunk_completion`. The current orchestrator treats target-kind escalation as
non-waivable; even a clean pre-scan cannot route that PR to a review runtime. The expected
dogfood run therefore escalates to the human by construction. Future experiments that
need an organic agent-review pass must either target a watched non-`main` base or provide
another explicit way to declare implementation-review intent. This exposes undesirable
coupling between branch topology and review role; it does not establish one required
topology. For this already-open PR, branch restructuring or a one-off manual OpenCode
review is a human choice, not an orchestration assumption.

The watcher adds a separate constraint: `base_ref` matches the PR's destination branch,
not its source/head branch. The current config and poller accept one exact base or one
glob; the repo row is keyed by owner/name, so registering another base replaces the first.
There is no union such as `main` plus `feature/*` today. An empty base reaches the GitHub
client's “all open PRs” behavior, but that is broader than the desired multi-base watch.
A future list/repeated-selector form is a product option, not a decided syntax.
This was already noted as the one-active-base limitation in the kickoff design and remains
a useful Trigger/pr-triage interface finding, not work to add to issue #129.

Two RED/CI projections are valid. The design's original flow ratifies RED locally/on the
issue branch, records an immutable checkpoint, makes that branch GREEN, and verifies it
before a PR exists; CI sees only the GREEN head. In the human's current two-level workflow,
the parent PR to `main` is deliberately a draft and may remain RED while ratified tests are
present and child implementations progressively make it GREEN. That failing draft is a
useful visual cue, but the durable state must still say “expected RED at checkpoint X” so
the process is not dependent on GitHub or unable to distinguish expected RED from breakage.
Neither projection should be baked into the charter/TDD contract itself.

### Codex runtime smoke

Disposable PR #134 presented the complete #133 diff to the routine lane while targeting
`chunk/codex-runtime-review-base`. A first head reused #133's SHA and exposed a GitHub
check-run identity hazard: check runs attach to commits, and `reportCheckRunID` selects the
first check named `pr-prescan-report`; two PRs sharing one head SHA can therefore mix
reports. An empty marker commit gave #134 a unique SHA without changing its tree.

The first observation window found no run because the persisted registration still
watched `main`, despite earlier discussion of “all PRs.” Bounded `config show` / `status`
diagnostics surfaced that fact without reading runtime logs. After the human changed the
registration to `chunk/**`, the daemon classified #134 as routine and invoked `codex` with
`gpt-5.6-sol`. The run completed in about 6m06s with one runtime-local turn and estimated
cost $1.3591696. It proved the daemon's saved auth, Codex workspace-write sandbox, ability
to read the full pre-scan/review prompt, file editing, commit/push, JSONL parsing, cost
accounting, and deterministic review-comment delivery.

The reviewer found one PR-local issue: `Adapter.Run` ignored invocation-envelope write
errors and short writes, yet that envelope is the only model-identity source for honest
cost reporting. It added two negative tests and fixed the adapter, and the orchestrator
pushed commit `b577eaa` to #134. The review comment's temporary-worktree file links became
dead after cleanup even though the commit survived; future review output should link to
GitHub paths/commits or omit ephemeral absolute paths. It also deferred a shared timeout
classification issue: deadline-terminated `ExecRun` processes can be classified as
failure rather than `OutcomeTimeout`; that cross-adapter change is not silently folded
into #129.

### Charter, TDD, governance, and gate assessment

- **Charter:** It held after grounding. The implementation did not drift from its nine
  observable scenarios. One reviewer finding required two additive negative bindings to
  an existing envelope scenario, not a new product behavior. The charter cut escalation
  noise by explicitly excluding named-agent-definition parity and self-enforced limits.
- **Grounding ROI:** High. It surfaced the runtime kit, lack of a shared price table,
  absent production event emitter, stale credential name, Git/sandbox requirements,
  actual JSONL success/failure shapes, and missing model identity before ratification.
  This directly moved tool-contract unknowns from the baseline's 5/9 to 1/6 here.
- **RED-first TDD:** Natural once the stalled GLM delegation was replaced. The resolution
  was mostly correct: scenario-level intent remained readable while 32 stack bindings
  supplied concrete executable detail. Input-validation/CLI-boundary tests covered flags,
  model passthrough, auth environment, and workdir; calculation-to-golden tests covered
  JSONL normalization and cost; background/process tests covered execution, doctor setup,
  and result delivery. No UI or TUI endpoint existed in this chunk.
- **Golden governance:** Neither golden fixture changed after ratification, and the manual
  checkpoint audit found no altered or removed original binding. Laundering did not occur,
  but the reviewer additions demonstrated why additions must be distinguishable from
  weakening. The human approved the two traced additions; changed or removed bindings and
  changed oracles remain escalation-worthy.
- **Two gates:** The charter gate was cheap and high leverage. The RED gate was too heavy
  when presented as 32 equal tests; the human reasonably spot-checked rather than reading
  all of them. Future gates should present at most about 12 scenario cards, RED evidence,
  and a scenario-to-binding map, with full tests available progressively.
- **Builder != verifier:** Enforceable and useful: Gemini built, Codex smoke found the one
  rework item, and DeepSeek independently found no remaining in-scope defect. Friction came
  from model availability, branch/config projection, and the cost of importing verbose
  transcripts—not from the role separation itself.

### Independent non-Codex review

DeepSeek run 25 reviewed the complete adapter and all 34 current adapter tests at
`b577eaa` in about 2m47s over 17 turns for an exact $0.070297876. It made no edits and
confirmed the envelope-write fix. It repeated the shared timeout-classification concern
and added the non-blocking reasoning-token pricing question. Its prose summary said “32
new Codex tests” even though deterministic inventory showed 34; as with the implementer's
earlier “19” count, machine inventory is authoritative over agent summary counts.

## D. Deliverables to PM

### What worked

- Empirical tool probing and ADR/domain grounding turned unstable CLI assumptions into a
  ratified, executable contract before implementation.
- A separately committed RED checkpoint made the original bindings and goldens auditable.
- One-behavior tests, concrete values, breadcrumbs, and a progressive companion-terminal
  flow made the suite inspectable without requiring the human to trust an agent summary.
- The builder/reviewer split found a real negative-boundary defect; the independent
  non-Codex pass then closed the loop without redundant implementation edits.
- Deterministic routing correctly reserved the `main` chunk-completion decision for the
  human, while a disposable routine PR exercised the new runtime end to end.

### What did not work

- Asking the human to ratify 32 bindings as if each were a separate product decision
  exceeded the attention budget.
- The first delegated RED author planned without producing artifacts, and streaming its
  verbose output unnecessarily consumed orchestrator context.
- Contract protection is currently social/manual; test assertion and golden changes can
  evade deterministic pre-scan.
- Review discovery and identity depend too heavily on GitHub projection details: one
  destination-base selector, target kind inferred from base branch, and same-SHA checks.
- Durable review comments contained temporary local paths.

### What could improve

- Make Gate 2 scenario-first: no more than about 12 human decision cards, each mapped to
  one or more machine-owned bindings and a concise right-reason RED result.
- Persist a protected-contract checkpoint and manifest of scenario IDs, test identities,
  and golden digests; deterministically escalate weakening/removal/oracle changes while
  allowing recorded additions traced to an existing scenario.
- Give the orchestrator bounded terminal handoffs—PR identity, checks, protected-contract
  verdict, and review findings—rather than agent transcripts or duplicate code review.
- Decouple review intent and workflow phase from branch topology; support explicit
  multiple base selectors at the GitHub boundary.
- Render durable source links against a repository commit, never an ephemeral worktree.

### Proposed kickoff checklist

1. Create isolated chunk/work-item state and record the exact base commit, intended PR
   head/base (if any), owner, and expected RED/GREEN phase.
2. Read impacted ADRs, the nearest adapter/domain implementation, runtime-authoring guide,
   and repository test style; inventory unresolved tool contracts.
3. Probe only unresolved external contracts with minimal deterministic examples and retain
   sanitized fixtures/evidence.
4. Draft at most about 12 observable scenario cards, explicitly listing non-goals and
   known limitations.
5. **Gate 1 — human charter ratification:** approve behaviors, omissions, and escalation
   boundaries; record charter version and signer.
6. A separate TDD author maps scenarios to concrete RED bindings/goldens; an independent
   verifier proves each fails for the intended reason.
7. Present scenario cards, binding counts/map, fixture provenance, and concise RED evidence;
   keep full code as progressive disclosure.
8. **Gate 2 — human contract ratification:** approve scenario coverage and any golden
   oracles; record immutable checkpoint/manifest.
9. Implement without authority to weaken protected bindings; allow traced additive tests,
   but route new behavior or oracle changes to the human.
10. Verify GREEN independently, including protected-contract integrity and full repository
    checks; then open/advance the PR and verify its actual head/base references.
11. Run deterministic pre-scan and reactive review on a working, non-builder runtime;
    surface only findings, deviations, and contract changes for human attention.
12. Human decides on escalations and the final chunk-completion merge.

### What a future charter/chunk agent should know and do

Treat the charter as the human-owned observable boundary, not a request for exhaustive
test enumeration. Trace every issue and binding to a stable scenario ID; disclose tool
unknowns and probe them before ratification; preserve fixture provenance; report exact
head/base/checkpoint identities; never alter a protected binding or oracle silently; and
return a bounded evidence packet instead of a reasoning transcript. Additive learning is
welcome when it strengthens an existing scenario and is recorded. New behavior, changed
expected values, missing traceability, or weakened coverage returns to the human.

### Testing-flow recommendations

Set up an isolated worktree and a writable Go cache, show `git status` plus untracked-file
inventory, and start with the scenario-to-test map. Deliver evidence progressively:
scenario card → sentence-style test name and breadcrumb → one representative right-reason
RED output → per-scenario summary → full suite and fixtures on demand. Preserve the RED
checkpoint, then show the same inventory GREEN without changing protected meaning. Use
deterministic test counts and diffs instead of model-reported counts. Give a busy human a
copyable companion-terminal sequence and five or fewer explicit decisions at each gate.

### Open questions and risks

- Where should the protected-contract manifest/checkpoint live, and which component owns
  its integrity signal and human override record?
- What explicit field replaces topology-derived review intent, and how should multiple
  base selectors be configured without making “watch all” the default?
- How should expected RED appear in CI/status without weakening ordinary merge gates?
- Should reasoning-output tokens receive distinct Codex pricing, and how is that price
  versioned with captured usage fixtures?
- Should shared `ExecRun` deadline termination become `OutcomeTimeout` across all adapters,
  and what compatibility tests are required before changing it?
- How should temporary/disposable review PRs and branches be retained or cleaned while
  preserving durable evidence?

## E. Evidence appendix — running timeline

| Local time | Kind | Evidence / observation | Attribution |
| --- | --- | --- | --- |
| 2026-08-29 09:23 MDT | discovery | Isolated worktree was clean on `feature/codex-runtime-adapter` at `305152b`, matching the separately checked-out local `main`. | instrumented: `git status`, `git worktree list` |
| 2026-08-29 09:24 MDT | environment surprise | Sandboxed `git fetch` could not write the linked-worktree `FETCH_HEAD`; the human-approved escalated retry succeeded. | instrumented: first command failure + approved retry |
| 2026-08-29 09:25 MDT | discovery | `origin/main` advanced from `305152b` to `c1d5611`; the isolated feature branch fast-forwarded cleanly and gained ADR 0009 plus the shared executor, declared capabilities, conformance harness, runtime doctor, and runtime-authoring guide. | instrumented: fetch/merge output |
| 2026-08-29 09:27 MDT | discovery | Official Codex CLI documentation confirms `codex exec` is stable, supports newline-delimited JSON via `--json`, a per-run model override, workspace selection, sandbox policy, and non-interactive stdin/positional prompts. | instrumented: official OpenAI docs lookup |
| 2026-08-29 09:28 MDT | discovery | Issue #129 remains open and its five acceptance scenarios match the draft charter at a high level. | instrumented: `gh issue view 129` |
| 2026-08-29 09:29 MDT | environment surprise | The first sandboxed `opencode models` call failed because OpenCode could not open its user log outside the workspace; an approved retry found the requested model as `openrouter/z-ai/glm-5.3-flash`. | instrumented: command failure + approved retry |
| 2026-08-29 09:31 MDT | delegation | Started a distinct read-only OpenCode grounding session with `openrouter/z-ai/glm-5.3-flash`, requesting file/line-cited charter deltas, test-standard findings, ambiguities, and Codex tool-contract unknowns. | instrumented: exact CLI model flag and prompt |
| 2026-08-29 09:31 MDT | discovery | The repository's runtime tests use Go's standard `testing` package, table-driven subtests, inline/captured NDJSON fixtures, concrete assertions, `t.TempDir`, and fake executable scripts; no testify dependency/style is used in the impacted packages. | instrumented: impacted tests and import scan |
| 2026-08-29 09:31 MDT | conflict surfaced by grounding | `docs/runtime-capability-table.md` still says Codex must self-enforce turn/budget by watching the stream, while the charter and newer ADR 0009 explicitly accept timeout-only v1. This must be resolved/acknowledged at charter ratification rather than silently copied into code. | instrumented: cited docs comparison |
| 2026-08-29 09:34 MDT | delegation | OpenCode grounding session `ses_fb1db30c0ffewvWBBxFa9Tn7aH` completed in about 3m07s and independently surfaced the kit-native requirement, missing cost-source decision, event-scenario over-specification, turn-unit ambiguity, and doctor/sandbox questions. | instrumented: OpenCode JSON events; exact model `openrouter/z-ai/glm-5.3-flash` |
| 2026-08-29 09:36 MDT | unplanned scope/gap | No shared model-price table exists in `internal/`; the charter's recent-sync note does not hold for pricing. | instrumented: repository-wide price/model search |
| 2026-08-29 09:36 MDT | unplanned scope/gap | `internal/events` defines normalized agent lifecycle fields, but production code has no `Emitter` construction or `EventAgentStarted` / `EventAgentFinished` emission site. The charter's progress-channel scenario cannot literally pass without generic orchestrator work or narrower wording. | instrumented: production-code reference search |
| 2026-08-29 09:38 MDT | environment surprise | The installed `codex` launcher resolves `/opt/homebrew/bin/node`, whose `llhttp` dylib is broken; the bundled native Codex 0.151.0 binary itself works and reports ChatGPT login active. | instrumented: launcher failure, path inspection, native `--version` / `login status` |
| 2026-08-29 09:40 MDT | tool-contract discovery | One minimal authenticated `gpt-5.6-sol` run produced the real JSONL contract: `thread.started`, `turn.started`, `item.completed` with `agent_message`, and `turn.completed` with input/cached/output/reasoning token usage. It consumed 16,426 input and 7 output tokens, showing even a tiny Codex smoke has non-trivial fixed prompt cost. | instrumented: Codex 0.151.0 native JSONL output |
| 2026-08-29 09:40 MDT | unplanned scope/gap | Codex's terminal usage event contains token counts but no model identifier. Since `ParseResult` receives only a log reader, known-model price estimation needs either a namespaced structured invocation envelope written by the adapter or a shared interface change. | instrumented: live JSONL + `AgentRuntime.ParseResult` signature |
| 2026-08-29 09:42 MDT | tool-contract discovery | Current official automation guidance uses `CODEX_API_KEY` (or saved `codex login`), not the charter's `OPENAI_API_KEY`; `codex exec` defaults to read-only sandbox and requires a Git repo unless explicitly bypassed. | instrumented: official OpenAI non-interactive docs + installed CLI help |
| 2026-08-29 09:43 MDT | tool-contract discovery | Invalid-model probe yielded `item.completed(type=error)`, top-level `error`, and terminal `turn.failed`, then exited 1 without model-token usage. This supplies a real negative fixture shape. | instrumented: Codex 0.151.0 native JSONL output |
| 2026-08-29 15:17 MDT | environment surprise | The first baseline `go test ./...` was invalid under the sandbox: Go cache writes, localhost `httptest` listeners, and linked-worktree creation were denied. | instrumented: two sandboxed test attempts |
| 2026-08-29 15:19 MDT | verification | The permission-correct baseline `go test ./...` passed on `c1d5611` before any implementation or behavioral-test code. | instrumented: full Go suite exit 0 |
| 2026-08-29 15:23 MDT | ratification gate | Human ratified the grounded charter with binding clarifications: kit-native (ExecRun + Capabilities + runtimetest); invocation contract `codex exec --json --ephemeral --sandbox workspace-write` with exact `-m` passthrough, inline prompt, cwd worktree, no slash validation, no production `--skip-git-repo-check`; ParseResult on captured JSONL (agent_message → Summary, terminal turn events → runtime-local turns), known priced models estimated from usage, unknown model Cost=0/CostBasisUnavailable; adapter writes a namespaced structured invocation envelope to its log; auth = saved `codex login` or invocation-scoped `CODEX_API_KEY`; generic runtime doctor initializes its temp workdir as a Git repo; shared-event scenario narrowed to a fully populated normalized Result; timeout-only v1; stdlib Go test standard with config-selection scenario exempt from all-new-tests-red; final smoke separately proves auth, pre-scan readability, workspace writing. | instrumented: human ratification + charter/capability-table updates in this task |
| 2026-08-29 15:30 MDT | process friction / rework | First RED OpenCode session `ses_fb0917065ffe2Hsf0SotS5xFwn` on `openrouter/z-ai/glm-5.3-flash` spent a long planning pass and wrote nothing, so it was interrupted; resumed on `openrouter/google/gemini-3.7-flash` which immediately authored the suite (observed process friction/rework, not a product implementation attempt). | instrumented: session interruption + resumption |
| 2026-08-29 15:35 MDT | test-suite construction | Authored golden fixtures `internal/runtime/codex/testdata/codex-0.151.0-success.jsonl` and `codex-0.151.0-failed.jsonl` (preserving empirical event shapes and token counts with normalized IDs/safe error wording) and unit/contract suite with exact envelope schema (`pr_triage_codex` version 1 kind `invocation` model string), 0.0409608 cost calculation based on 2026-08-29 official pricing, one behavior per test, and relied on existing generic coverage (`TestOrchestrator_HandleReportReady_ValidAndDone`, `TestBuildReviewPrompt`). | instrumented: file creations and edits |
| 2026-08-29 15:40 MDT | red-first verification | Executed `go test ./internal/runtime/codex/...`, `go test ./cmd/pr-triage/...`, `go test ./internal/cli -run TestRuntimeCheckInitializesProbeWorkdirAsGitRepository`, and `go test ./internal/cli -run TestInitRuntimePinsCodexRoutineRoutingAndConfigShow`. Verified all 32/32 Codex adapter tests fail visibly RED on unregistered runtime, main registration test fails RED on missing import, doctor temp-workdir git-init test fails RED on non-repo temp directory, while config show verification (`TestInitRuntimePinsCodexRoutineRoutingAndConfigShow`) passes GREEN under pre-existing wiring exemption. | instrumented: test execution outputs |
| 2026-08-29 15:49 MDT | attention/tooling friction | Polling the OpenCode process streamed verbose NDJSON, including reasoning metadata, into the orchestration context. The human identified the unwanted context cost. Future delegated sessions will redirect full logs to files and expose only process status plus a bounded final summary/test extract. | instrumented: tool output volume + human feedback |
| 2026-08-29 15:52 MDT | environment surprise / verification | Independent reruns reproduced all intended RED/GREEN outcomes. A first generic orchestrator-coverage run falsely escalated because the sandbox denied linked-worktree metadata writes; the permission-correct rerun passed both `TestBuildReviewPrompt` and `TestOrchestrator_HandleReportReady_ValidAndDone`. | instrumented: sandboxed failure diagnosis + permission-correct targeted test exit 0 |
| 2026-08-29 15:55 MDT | gate usability / human feedback | Human asked how a busy reviewer can inspect the RED suite, understand intent, see untracked files, and reproduce evidence in a companion terminal. Added a progressive inspection/test playbook and explicit five-question ratification checklist; this surfaced that `git diff` alone hides the new untracked adapter suite. | instrumented: human feedback + report edit |
| 2026-08-29 16:09 MDT | gate readability / human feedback | Human found sentence-style test names close to pseudocode but still wanted breadcrumbs for junior readers. A bounded Gemini OpenCode pass added a scenario/why comment above all 35 new behavioral tests without changing bodies, fixtures, or production code; orchestration then corrected one comment that conflated ephemeral state with sandboxing. | instrumented: human feedback + comment-only edit audit |
| 2026-08-29 16:22 MDT | governance gap / human feedback | Human identified that an implementer can launder a bug by editing ratified contract tests or goldens and asked for deterministic pre-scan escalation. Inspection confirmed modified assertions are not signaled and all `testdata/` paths are excluded; §5.3 is aspirational in the current product. | instrumented: human feedback + scanner/config inspection |
| 2026-08-29 16:22 MDT | attention-budget decision | Human explicitly kept the current 32-test suite but set future guidance at roughly a dozen human-reviewed charter behaviors. The current charter already has nine scenarios; the 32 tests are stack-local bindings to be agent-verified and available through progressive disclosure, not 32 equal human decisions. | instrumented: human decision + scenario/test counts |
| 2026-08-29 16:25 MDT | ratification gate | Human spot-checked a subset rather than all 32 Go bindings and approved proceeding. Gate 2 ratifies the nine charter scenarios, their 32-test executable binding, the invocation-envelope schema, pricing semantics, and both Codex 0.151.0 golden fixtures. | instrumented: explicit human approval |
| 2026-08-29 20:27 MDT | orchestration/process correction | Human clarified that separate-model credits were not the issue; the problem was the trunk orchestrator importing too much sub-agent context and duplicating routine code review. Adopted PR/check-driven coordination: issue agents work in isolated branches/worktrees, commit/push/open PRs to the chunk branch, and the orchestrator attends only to state, deterministic gates, and escalations. | instrumented: human feedback + operating-model update |
| 2026-08-29 16:25 MDT | contract checkpoint | Committed the ratified charter, 32-test binding, external gate tests, and golden fixtures separately as `080a36e`, establishing the manual protected-contract baseline for this experiment. | instrumented: Git commit |
| 2026-08-29 20:27 MDT | implementation / first-pass GREEN | Gemini OpenCode session `ses_fb05e14f0ffeiW5anBVgJUWjJo` implemented only `internal/runtime/codex/codex.go`, binary registration, and doctor Git initialization. It reported full-suite/vet GREEN and zero protected-file diff; independent deterministic slices confirmed all 32 adapter tests and external gates GREEN. | instrumented: production diff + test outputs + checkpoint diff |
| 2026-08-29 20:28 MDT | reporting discrepancy | Implementation-agent handoff incorrectly reported 19 adapter tests despite 32 top-level test functions. Deterministic inventory/output corrected the number; this was a summary-quality issue, not implementation rework. | instrumented: handoff text vs `rg`/Go test output |
| 2026-08-29 20:31 MDT | PR / verification | Opened PR #133 from `feature/codex-runtime-adapter` to `main`. All required build, test, vet, lint, race, shell, agent, and pre-scan checks completed GREEN; the pre-scan report had no present signals. | instrumented: GitHub PR/check results |
| 2026-08-29 20:36 MDT | process/topology discovery | Because PR #133 targets `main`, pre-scan classifies it as `chunk_completion`; the current orchestrator explicitly forbids overrides of target-kind escalation. The running dogfood binary is therefore expected to request human review rather than invoke a reactive agent, despite clean signals. | instrumented: pre-scan result + `applyOverride` control flow |
| 2026-08-29 20:38 MDT | process decision | Human noted that the current PR can be retargeted in GitHub after review and required future OpenCode sub-agents to target the correct chunk branch. Added a deterministic handoff rule: issue task packets name exact head/base refs, and the orchestrator verifies the created PR's actual refs before accepting it or invoking triage. | instrumented: human direction + operating-model update |
| 2026-08-29 20:47 MDT | scope boundary | Human clarified that middle-of-chunk dispatch, worktree, concurrency, and PR-creation mechanics are likely AgentMinder/Trigger responsibilities. Preserve findings as interface notes, but evaluate this experiment on front-of-chunk charter/TDD quality and back-of-chunk pre-scan/reactive-review quality. | instrumented: human direction + report framing |
| 2026-08-29 20:49 MDT | dogfood configuration / correction | Human found that the dogfood registration watched a chunk base and changed it to a feature pattern, then caught that `base_ref` filters the PR destination rather than the head. PR #133 targets `main`, so neither chunk nor feature base filters discover it. This corrects the orchestrator's initial mistaken interpretation of the filter direction. | instrumented: human correction + `ListOpenPRs` inspection |
| 2026-08-29 20:53 MDT | product limitation | Confirmed one exact/glob `base_ref` per owner/repo registration; re-registration overwrites it. Empty means all open PRs at the GitHub-client seam, but there is no configured multi-base union such as default branch plus active chunk. The kickoff design already lists concurrent multi-chunk watching as open. | instrumented: config, DB upsert, GitHub client, and existing design note |
| 2026-08-29 20:53 MDT | process clarification | Human surfaced the risk that a test-only RED PR would keep parent CI failing. The intended front-of-loop avoids this: ratify and checkpoint RED on the issue branch, implement to GREEN, independently verify, and only then open the PR; ordinary CI evaluates the GREEN head while commit history preserves RED evidence. | instrumented: human question + §4.2/§4.3 design flow + current experiment history |
| 2026-08-29 21:03 MDT | workflow/generalization | Human described the current two-level stack: a draft feature/chunk PR to `main` stays visibly RED while child worktree PRs merge into the feature branch and progressively make it GREEN. Kept this as a useful GitHub projection, but removed it as a presumed durable topology: AgentMinder/Trigger should own phase/checkpoint/work-item/verdict state, and the front/back contracts should remain usable with other team workflows. | instrumented: human workflow description + report correction |
| 2026-08-29 21:05 MDT | reactive triage | After the human configured the watcher to include PRs targeting `main`, pr-triage discovered PR #133 and automatically applied `needs-owner-review` plus an escalation comment: `target_kind "chunk_completion" requires human review`. This is the expected deterministic terminal route; no review runtime was invoked. | instrumented: human report + GitHub label/comment verification |
| 2026-08-29 21:06 MDT | durable decision | Human requested that the topology/state finding outlive the experiment report. Drafted ADR 0010: chunk lifecycle state is provider-independent and authoritative outside GitHub; PR stacks/draft CI are optional projections; expected RED is an explicit phase; multiple base selectors are a required future capability without choosing syntax or orchestration ownership here. | instrumented: human direction + ADR/index/report changes |
| 2026-08-30 07:55 MDT | runtime dogfood setup | Opened disposable PR #134 with the complete #133 diff against `chunk/codex-runtime-review-base`, so CI/pre-scan run and target kind is `implementation` without changing the real `main` PR. | instrumented: Git refs + GitHub PR/pre-scan |
| 2026-08-30 07:57 MDT | smoke surprise | Reusing #133's exact head SHA caused GitHub to expose both PRs' check runs on one commit; the poller selects the first named report check and cannot associate it with a PR. Added an empty marker commit solely to give #134 an isolated SHA. | instrumented: `gh pr checks` mixed output + poller code inspection |
| 2026-08-30 08:06 MDT | configuration diagnosis | Five minutes of no result led to bounded state inspection: daemon and Codex routing were healthy, but the persisted repo registration still watched `main`, so #134 was invisible. Human changed it to `chunk/**`; no runtime transcript was needed. | instrumented: `config show`, `status --json`, human correction |
| 2026-08-30 08:11 MDT | Codex smoke | Daemon started routine run 24 for #134 on `codex` / `gpt-5.6-sol` at unique SHA `7186aa4`; state showed `agent_running` with no early failure/escalation. | instrumented: persisted run state |
| 2026-08-30 08:17 MDT | Codex smoke success | Run 24 completed `done` / `turn.completed` in ~6m06s, one turn, estimated cost $1.3591696. The review summary was parsed and posted; the agent edited its sandbox worktree and the orchestrator pushed automated-fix commit `b577eaa`. | instrumented: run state + GitHub comment/commit |
| 2026-08-30 08:18 MDT | reactive finding | Codex found that envelope write errors/short writes were ignored, allowing launch without required model metadata. It added two negative tests and a fail-before-launch fix. It separately deferred cross-adapter timeout misclassification; its review comment also contained ephemeral absolute file links that died after worktree cleanup. | instrumented: bounded review summary + two-file automated commit |
| 2026-08-30 08:20 MDT | contract-governance gate | Because the reviewer added tests inside ratified `run_test.go`, adoption stopped for human attention. Human clarified the policy: additive tests/findings are useful and allowed; the core concern is silently changing, weakening, or removing existing charter tests. Human approved the two additions. | instrumented: human decision |
| 2026-08-30 08:23 MDT | rework / verification | Applied the approved two-test envelope refinement and production fix to PR #133 as commit `9722f8e`. Manual audit found no existing protected-test line removed/changed and no golden diff; both new tests, all 34 adapter tests, full `go test ./...`, `go vet ./...`, and `git diff --check` passed. | instrumented: diff audit + command outputs |
| 2026-08-30 08:31 MDT | independent review | DeepSeek run 25 started on disposable PR #134 at `b577eaa` using OpenCode 1.18.23 / `openrouter/deepseek/deepseek-v4-pro-0813`; it received the complete adapter diff and current 34-test suite without importing its transcript into the trunk context. | instrumented: persisted run state |
| 2026-08-30 08:34 MDT | independent review success | Run 25 finished clean/no edits in ~2m47s over 17 turns for exact cost $0.070297876. It confirmed the envelope fix, repeated the non-blocking shared-timeout concern, and raised reasoning-token pricing as a future contract question. | instrumented: bounded review summary + run metrics |
| 2026-08-30 08:34 MDT | reporting discrepancy | DeepSeek summarized the current suite as 32 tests although deterministic inventory showed 34. Agent prose counts remain advisory; executable inventory is authoritative. | instrumented: review summary vs Go test inventory |
| 2026-08-30 08:42 MDT | ADR ratification | Human approved ADR 0010; status changed from Expected to Locked and the ADR index was updated. | instrumented: explicit human sign-off |
