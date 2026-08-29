# Issue #129 Codex runtime front-of-loop dogfood report

Working, instrumented report for the manual charter → RED-first behavioral tests →
implementation → independent verification → reactive-review experiment. Entries marked
**instrumented** use command/session evidence captured during the run; entries marked
**estimated** are retrospective estimates.

## A. Header

- **Chunk / issue:** Codex runtime adapter / #129
- **PR(s):** pending
- **Dates:** started 2026-08-29
- **Implementation orchestration runtime / model:** Codex session (exact host model ID not surfaced)
- **Delegated RED-test runtime / model:** OpenCode 1.18.23 / `openrouter/z-ai/glm-5.3-flash` (interrupted session `ses_fb0917065ffe2Hsf0SotS5xFwn`) → `openrouter/google/gemini-3.7-flash`
- **Reactive-review runtime / model:** pending; must not be Codex
- **Outcome:** in progress — charter **ratified** 2026-08-29 15:23 MDT; nine-scenario behavioral contract / 32-test RED binding **ratified** 2026-08-29 16:25 MDT after human spot-check; implementation authorized

## B. Scorecard

Pending implementation and verification.

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

- **Adapter Test Count & Coverage:** 32 tests in `internal/runtime/codex` across `capabilities_test.go` (6 tests), `parse_result_test.go` (13 tests), `run_test.go` (8 tests), `classify_test.go` (4 tests), and `conformance_test.go` (1 test), strictly enforcing one behavior per test.
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
Each of the 35 behavioral tests also has a two-line reader breadcrumb immediately
above it: the first line states the scenario and expected behavior; the second explains
why that contract matters to the system or operator.

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
2. A protected-artifact manifest containing scenario IDs and the contract-test/golden
   paths or digests. The scanner compares the checkpoint objects with PR head objects.
   Changing a protected artifact, the manifest, or its checkpoint pointer emits a
   `ratified_contract_changed` signal with file evidence. Protected goldens override the
   scanner's generic `testdata/` exclusion.

That signal should map to `escalate` by default. On chunk/epic implementation PRs the
existing escalation path applies `needs-owner-review`, and the existing
`owner-review-gate` fails until a human clears it. The contract is allowed to evolve;
the change is surfaced rather than silently accepted or absolutely frozen. Until this
product mechanism exists, this experiment should take a separate ratified test-only
checkpoint and manually compare all protected files against it before accepting an
implementation or fixture change.

The human-attention surface and executable-test surface must not be conflated:

| Layer | Primary reviewer | Count policy | Governance |
| --- | --- | --- | --- |
| Charter scenarios | Human | Target about 12 at most | Human ratifies the observable must/must-not behavior |
| Contract-test bindings and goldens | Independent TDD/verifier agents | As many as needed; 32 is acceptable here | Registered as protected artifacts; any later diff escalates |
| Unit/regression/implementation tests | Implementer and reviewer agents | Unbounded by the charter gate | May evolve normally outside the protected manifest |

The current Codex suite should remain intact: its 32 adapter tests bind only nine charter
scenarios. The usability error was presenting all 32 bindings as equal human decisions.
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

## D. Deliverables to PM

Pending completion.

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
