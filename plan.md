# PR Triage/Review Agent — Design Doc (draft)

## Inputs to be supplied by the user at project kickoff (not designed here)

- **Agent definition**: already exists, operates off a table of what's
  important (i.e. the escalate-vs-easy-fix judgment/scope boundary raised
  above is already governed by this, external to this tool).
- **CI/CD report-producing script**: already exists; shows the exact shape
  of the JSON report this tool will consume from the CI/CD run.
- Both to be provided when implementation starts — this doc should treat
  the report schema and agent-definition contract as given inputs, not
  design them.

## Goal
Economical, POC-friendly automation that watches PRs, waits for CI/CD to finish,
picks up a deterministic pre-scan JSON report, and hands off to a review agent
(Sonnet/Opus via Claude Code, or Codex/OpenCode) — with human escalation for
anything the deterministic checks flag as critical (destructive migrations,
ADR changes, etc.).

## Pipeline

1. **Poll loop (no LLM)**
   - Cron/scheduled job (not an LLM loop) checks for new/updated PRs against a
     configurable base ref.
   - Deterministic — plain API calls, cheap, reliable.

2. **CI/CD wait**
   - If checks are still running, exponential backoff poll until CI/CD
     workflow completes.
   - Timeout ceiling so a hung workflow doesn't stall the loop forever.

3. **Report retrieval**
   - Pull the JSON report from the **latest CI/CD run** (Checks API output /
     workflow artifact), not from PR comment text — comments can be edited,
     duplicated by reruns, or race with the poll.
   - Error handling required for three cases: report missing, report
     malformed/fails schema validation, and CI run itself failed (report may
     not exist at all in that case — decide whether that's an escalation
     trigger by itself).

4. **Agent invocation (only when CI passed + report present/valid)**
   - Layered/cacheable prompt structure, Docker-layer style:
     1. Static layer: repo-wide conventions, ADR excerpts, shared review
        rubric — must stay byte-identical across calls to hit prompt cache.
     2. Semi-stable layer: parent issue / effort-level context.
     3. Dynamic layer (last): PR-specific diff + JSON report contents.
   - Caching payoff is real but likely modest at low PR volume — worth doing
     for the discipline/structure even if savings are small. Biggest win is
     multiple turns within one PR's review or several PRs landing close
     together sharing the static prefix (cache TTL ~5 min, up to 1hr
     extended).
   - Agent + escalation/labeling behavior already defined elsewhere — not
     part of this design doc's scope.

## Config / CLI arguments (needed)

- `--base-ref` (optional, default: repo's default branch) — lets a user point
  this at their own feature branch instead of main.
- `--agent-runtime` — e.g. `claude-code` / `codex` / `opencode`.
- `--model` — model parameter, passed through to the chosen runtime.
- `--timeout` — optional wall-clock cap on the agent invocation, to stop
  runaway/flailing runs from burning tokens. Hard to standardize across
  providers, so treat as best-effort / provider-specific enforcement.
- `--github-user` (optional) — who to tag/notify on escalation.
- Either a config file or equivalent flags for all of the above — undecided
  which; leaning toward supporting both (flags override config file).

## Open questions
- Malformed report → **hard fail** (decided: escalate immediately, no
  retry-then-escalate).
- Does a failed CI run (no report at all) auto-escalate, or just skip
  silently until it passes? (still open — see state machine below; leaning
  toward "skip, wait for next push" since a failed CI run is the author's
  problem first, not necessarily an escalation)
- Config file format if we go that route (TOML/YAML/JSON)?
- Cost-guardrail enforcement is straightforward for Claude (usage in the
  response) but murkier for Codex/OpenCode — may have to fall back to
  wall-clock timeout as the only lever for those adapters. Per-adapter
  concern, not solved yet.

## Loop-prevention (resolved — no new mechanism needed)
- Not human-vs-bot commit authorship (not a reliable signal — plenty of
  legitimate commits are bot-authored).
- The existing SHA/run-ID-keyed state machine already prevents this: a
  report only counts as a trigger if it's attached to a run for a **head SHA
  not yet processed**. If the CI/CD pipeline later emits a second report
  after the review agent's own push, it's for the *same* SHA (no new human
  commit), so it's a no-op under this keying — regardless of pipeline
  design. Still worth simplifying the CI/CD to emit the report once
  (pre-review) for clarity, but the tool must be defensive either way.

## Runtime adapters
- Target runtimes: **Claude Code**, **Codex CLI**, **OpenCode CLI** — all
  invoked as exec'd subprocesses, avoiding separate API billing.
- Common adapter interface: `Invoke(prompt, timeout) -> (result, error)`,
  with each CLI's argument-passing / output-parsing / exit-code quirks
  isolated behind it.
- Future: an OpenAI-compatible API adapter slots in behind the same
  interface without touching daemon/state-machine logic.
- Cost-guardrail enforcement lives per-adapter (see open questions above).

### Findings from agent-minder (sibling repo, `~/repos/agent-minder`)

Adapters live in `internal/runtime/{claudecode,codex,opencode}/`, sharing a
`runtime.AgentRuntime` interface (`Run`, `ParseResult`, `ClassifyOutcome`,
self-registered via `init()`). Concrete invocation patterns:

- **Claude Code**: `claude --agent <name> -p --output-format stream-json
  --verbose [--model X] -- <prompt>`. Prompt as trailing positional arg.
  Stdout streamed line-by-line JSON; terminal `result` event carries
  `total_cost_usd`, `num_turns`, `is_error`. Timeout via
  `exec.CommandContext`'s ctx only (no adapter-side turn cancellation
  needed — Claude's own `--max-turns` handles it).
- **Codex**: `codex --ask-for-approval never exec --json --cd <worktree>
  [--model X] --sandbox workspace-write <prompt>`. **No terminal result
  event and no dollar cost in output** — must replay the full JSONL log and
  estimate cost from a hardcoded per-model pricing table. **No
  `--max-turns` flag** — adapter self-enforces by watching the stream and
  cancelling the context mid-run when a turn-count threshold is hit.
  Workspace scoping needs extra logic for git-worktree `.git`-file
  indirection (`--add-dir` for the real git dir).
- **OpenCode**: agent-minder does NOT exec it one-shot — `opencode run
  --format json` is called out in their own docs as "undocumented and
  churny across 1.x," so they instead run a persistent `opencode serve`
  process and drive it via the Go SDK + SSE streaming. That buys real cost
  data but costs a shared server + port/lifecycle management +
  shutdown-hook plumbing.

**Decision for this tool (smaller scope than agent-minder)**: skip the
OpenCode-server route. Treat all three as plain exec'd subprocesses
(mirror the Codex/Claude pattern), and accept OpenCode's rougher JSON
output as a known limitation rather than building a server-lifecycle
subsystem to work around it. This also confirms the earlier instinct that
Codex/OpenCode cost-guardrails are harder than Claude's: Codex needs the
pricing-table workaround, OpenCode needs either the SDK route (not doing)
or tolerating flaky plain-CLI JSON (doing).

**Reusable idea, low cost**: write a short "primitives comparison" doc per
CLI (flags, output format, cost reporting, auth, timeout support) *before*
writing its adapter — agent-minder's `design/codex-mapping.md` and
`design/opencode-mapping.md` caught these gotchas early and are worth
mimicking as a lightweight design step.

## State storage & multi-repo

- **SQLite**, shared schema — NOT a table-per-repo. A `repos` table plus a
  `prs`/`runs` table with a `repo_id` foreign key (indexed). `init` run
  inside a repo just inserts a row into `repos`; no per-repo DDL.
  - Reasoning: per-repo dynamic tables mean every schema migration runs N
    times instead of once, and cross-repo status queries get awkward. At
    this scale (dozens of PRs across a handful of repos) `repo_id` costs
    nothing.
- **One daemon**, not one per repo — `init` registers a repo into the shared
  store; the single daemon polls every registered repo.
- Concurrency default: **1 agent invocation at a time** (bounded, queued).

## PR state machine (needed to avoid double-triggering / missed pushes)

A PR isn't just "has report / doesn't" — need explicit state per PR, keyed
off **head SHA** and **CI run ID**, persisted locally so the poll loop is
idempotent:

```
idle -> ci_running -> ci_passed(report pending) -> report_ready -> agent_running -> done | escalated
                   \-> ci_failed(watching head SHA) -/  (new push -> back to ci_running)
```

- Store per-PR: last-processed head SHA + last-processed CI run ID.
- Each poll compares current head SHA / latest run ID against stored values:
  - SHA changed since last check -> treat as a fresh push, re-enter
    `ci_running`, discard old failed state.
  - Same SHA, same run ID already processed -> no-op (idempotency guard,
    prevents re-triggering the agent on repeated polls).
  - New run ID with a passing report attached -> this is the actual trigger
    to invoke the agent.
- Malformed report at `report_ready` -> hard fail -> `escalated`.

## Rate limiting / GitHub API hygiene
- Use conditional requests (ETag / `If-None-Match`) for status/list polling
  — 304 responses don't count against the rate limit; cheaper than just
  widening the poll interval.
- Keep base poll interval generous (minutes) — this is a background POC
  watcher, not a live dashboard.
- Token via a `setup` subcommand that stores the GitHub PAT in macOS
  Keychain (e.g. `go-keyring`), not a plaintext config file.

## Delivery shape (leaning toward)
- **Go CLI tool**, driven by cron/launchd, with a **SwiftBar plugin** for
  at-a-glance "is it running / last result" status in the menu bar.
- Rationale: Go is strong for this (goroutines for the backoff/poll loop,
  good TUI/CLI ergonomics, single static binary, low overhead for a
  background watcher), and SwiftBar integration gives cheap visibility
  without building a full UI.
- **Important split**: the Go binary owns all GitHub polling and writes a
  small local status file (PR, state, last-checked). The SwiftBar plugin
  only *reads* that file on its own refresh cycle — it must not make its own
  GitHub calls, or you end up with two independent, out-of-sync poll loops
  hitting the API.

## Lessons from agent-minder, round 2 (deeper post-mortem)

A second pass through agent-minder's own bug history, not just its code shape.
Highest-value items:

- **Cost-basis honesty (their worst bug)**: minder scraped logs for the
  string `total_cost_usd`, which only Claude's output contains — so Codex
  and OpenCode runs silently recorded cost `0`, and the budget ceiling
  never fired for those two runtimes. Fix: get cost from each adapter's
  structured `ParseResult`, never by scraping, and tag every cost/turn/model
  value with a **basis** field (`exact` / `estimated` / `unavailable` /
  `runtime-defined`) so a genuine zero can never be confused with "didn't
  measure it." This directly confirms the earlier "cost guardrail is harder
  for Codex/OpenCode" concern — the fix is labeling, not a better estimate.
- **Result-shape traps across runtimes** (normalize at the adapter
  boundary, don't compare raw): `num_turns` means different things per
  runtime (don't compare across runtimes — only ratio to that run's own
  limit); Claude's `stop_reason` came back double-quoted; one log file can
  contain multiple runs, summed/first/last differently per runtime — use a
  fresh log per run, never append.
- **Config resolution — build once, before adapters exist**: a single
  ranked resolver (e.g. stage → agent → job → repo config → user config →
  runtime default, most-specific wins), resolved once per run and *stored*
  on the run record — never re-derived in a display path. minder's worst
  config bug (a per-agent model silently dropped, issue #528-style) came
  from not having this.
- **Static capability table, not runtime probing** — declare per-runtime
  differences up front rather than discovering them live:
  - Claude: exact cost, enforces its own max-turns/budget via CLI flags,
    tool allowlist supported, **resume requires passing the working
    directory** (sessions are stored per-project-dir).
  - Codex: estimated cost only, does *not* enforce max-turns/budget itself
    (adapter must self-enforce by watching the stream), no tool allowlist
    (sandbox-only).
  - OpenCode: exact cost, does not enforce turns/budget at all, needs
    `provider/model` form (silently drops a model string with no slash —
    validate at config time), is a shared server so **provider credentials
    are deployment-level, not per-job** (env freezes at server start).
  - Rule of thumb: never advertise a limit (timeout, tool allowlist, budget
    cap) that a given adapter doesn't actually enforce — enforce it or
    don't claim it.
- **Persistence discipline (matches our SQLite plan — confirms it)**: WAL
  mode + single writer (`SetMaxOpenConns(1)`) to avoid lock contention;
  migrations additive-only, one version bump per change, never edit a past
  migration; one durable record per run (resolved model, runtime, session
  id, cost+basis, turns, status, stop reason).
- **GitHub dedup — confirms our design**: dedup on `(PR, head SHA)`,
  exactly matching the SHA-keyed state machine already in this doc.
- **Worktrees — resolved, they're core**: the agent's job is not read-only.
  It should make easy fixes directly (commit/push to the PR branch),
  attempt thorough changes when warranted, and escalate to a human when a
  change is unavoidable but out of its judgment/authority. That means
  worktrees are load-bearing, not optional — confirms the earlier decision,
  overriding minder's "you probably don't need them" note (that note
  assumed a read-only reviewer).
- Also flagged as scope creep to explicitly avoid: dependency-graph
  resolution between jobs, multi-stage pipelines, and any "lesson-learning"
  self-improvement system — none of these belong in a PR-triage-scoped tool.

## Report ownership & schema versioning

- **Decision**: report generation stays in CI/CD, not folded into this
  app. The CI/CD environment already has the full build context (checked
  out repo, migration tooling, linters) that generating the report needs —
  pulling that into the daemon means duplicating analysis logic it
  otherwise doesn't need, and re-couples "detect risk" with "orchestrate
  review" after deliberately splitting them.
- **Contract stability instead of merging systems**: report shape is
  expected to change over time, so give it a `schema_version` field and
  validate against a JSON schema on ingest. The report generator can then
  evolve independently of the daemon. An unrecognized/unsupported
  `schema_version` is a hard-fail (consistent with the malformed-report
  decision above) — never guess at fields on a version mismatch.

## Risk-based agent/model routing

- New scope beyond the original design (which assumed one fixed
  `--agent-runtime`/`--model` pair for every PR). Instead: a routing table
  of `risk_tier -> {runtime, model, agent_def}`, keyed off whatever risk
  classification the report/agent-definition's "table of what's important"
  produces (e.g. destructive migration / ADR change -> higher-capability
  model+agent; routine change -> cheaper/faster one).
- This table belongs in the daemon's **config**, not hardcoded — risk
  criteria and available agent defs will keep evolving independently of
  the daemon's release cycle.
- An unmapped/unrecognized risk tier should **escalate to a human**, not
  silently fall back to a default runtime/model — same hard-fail
  philosophy as the malformed-report and schema-version-mismatch cases.

## Additional considerations (not yet resolved)

- **Git worktrees**: each agent invocation runs in its own isolated
  `git worktree`, not the user's working directory. Stale worktrees from a
  crashed/killed run need cleanup on daemon startup.
  - **Pruning**: worktrees are pruned on an age basis, default **72 hours**,
    configurable (`--worktree-ttl` or config equivalent). Daemon should
    sweep for worktrees past TTL on a periodic basis (not just at startup),
    since it's long-running. Prune via `git worktree remove` (falling back
    to `--force` + `git worktree prune` if the dir was already hand-deleted)
    rather than a raw `rm -rf`, to keep git's internal worktree metadata
    consistent.
- **Agent process tracking**: the daemon tracks each agent invocation as a
  real child process (PID, start time, log path) in the same SQLite store —
  this is what makes `--timeout` enforceable (SIGTERM the tracked PID) and
  lets a `status` command show live progress (e.g. "agent running 4m on
  PR #52").
- **Crash/restart recovery**: on daemon restart, detect any PR that was
  `agent_running` when the process died and decide retry vs. mark-failed,
  rather than silently losing track of it.
- **Logs**: headless + long-running (launchd-managed) → needs rotated logs
  and a `logs` / `status --verbose` command, since debugging launchd's
  stdout redirection directly is unpleasant.
- **`init` wizard**: run from inside a target repo (`cd repo && review-triage
  init`). Interactive by default, but every prompt should also be settable
  via flags for non-interactive/scripted setup — base-ref (supports a
  glob/pattern, not just an exact branch), poll interval, timeout, etc.
  Registers the repo into the shared SQLite store (see above); does not
  start its own process. Writes config as YAML.
  - **Agent-assisted setup, layered on top (not a replacement)**: a plain
    flag/prompt-driven `init` should ship first and always work with zero
    dependencies. An `init --assisted` (or separate `config agent`
    subcommand) can layer an agent-run wizard on top later — it does a
    repo inventory first (existing `.claude/agents/`, `AGENTS.md`,
    `.opencode/agent/` -> which runtime's already in use; `.github/workflows/`
    -> likely CI report location; an ADR folder if present -> confirms the
    ADR-change signal), pre-fills answers from that, asks only what it
    couldn't infer, and writes the same YAML the plain path would.
  - **Bootstrapping wrinkle**: the assisted wizard needs *some* working
    runtime adapter to run itself, but `init`'s job is partly to configure
    that runtime — resolve via PATH-detection of an installed CLI (claude /
    codex / opencode) as its first, self-answered question, or fall back to
    asking the user directly which one they have installed.
- **GitHub identity**: personal PAT is fine for solo use; a GitHub App/bot
  account is cleaner once this stops being solo-use (PR comments read as
  "the bot," finer-scoped permissions) — not needed for the POC.
- **Reference project**: `agent-minder` (sibling repo, `~/repos/agent-minder`
  or similar) is a much more elaborate version of this same
  poll-repo/trigger-agent/track-state shape, built up over months. Worth
  mining for lessons (state machine edge cases, worktree handling, process
  tracking) — but this tool should stay intentionally smaller in scope:
  PR-triage-only, not a general agentic workflow platform.

## Future: alerting/GUI beyond SwiftBar

- Motivation: SwiftBar was chosen mainly for cheap native macOS
  alerting/monitoring. Longer-term interest in a proper GUI (web or native
  macOS) fed by server-sent events instead of a polled status file.
- Recommended shape: don't build the SSE/API server now — instead, structure
  the Go daemon's internal state-change handling as a single **event
  emitter**, with the file-writer (for SwiftBar) as one subscriber. An SSE
  endpoint (`net/http` + `Flush()`, no extra deps in Go) becomes a second
  subscriber later without needing to rework the poller/state machine.
- Tradeoff: an SSE/API server is a long-running process with a bound port —
  more commitment than "cron writes a file." Worth it once a live GUI is
  actually being built, not for the initial POC.

