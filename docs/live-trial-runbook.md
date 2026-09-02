# Live Trial Runbook: pr-triage Dogfooding

This document outlines the end-to-end operational procedure for dogfooding `pr-triage` on live repositories and pull requests, validating the three headline workflows:
1. **Clean Implementation PR**: Autonomous review and scoped fixes with exact cost attribution.
2. **Risky-Signal PR**: Deterministic escalation applying `needs-owner-review` and gating CI until cleared by a human.
3. **Daemon Crash Recovery**: Seamless startup reconciliation of stranded worktrees, orphaned PIDs, and database state.

---

## 1. Prerequisites & Environment Setup

### Required Tools & Credentials
- **Go**: 1.25+ installed (`go version`).
- **GitHub CLI**: Authenticated with PAT or OAuth (`gh auth status`).
- **Claude Code CLI**: Authenticated with Anthropic API key (`claude --version`).
- **SQLite3**: For querying local daemon state (`sqlite3 --version`).

### Required GitHub Token Permissions
Ensure the active GitHub token has access to:
- `repo` (Full repository control: pull requests, comments, labels)
- `checks:read` / `checks:write` (Check run retrieval and publishing)
- `statuses:read` / `actions:read` (Status check rollup querying)

---

## 2. Repository Configuration & Daemon Startup

### Step 2.1: Initialize pr-triage Configuration
Inside the repository root (e.g. `/Users/dustin/repos/pr-triage`):

```bash
# Build binary
make build

# Initialize configuration (registers the repo + writes .pr-triage/config.yaml)
./bin/pr-triage init \
  --base-ref "epic/pr-triage-poc" \
  --github-user "dustinmays" \
  --runtime claude-code \
  --model claude-haiku-4-5 \
  --non-interactive
```

> Note: the flag is `--github-user` (not `--user`). Owner/name are auto-detected
> from the git remote; pass `--owner`/`--name` to override.

This creates `.pr-triage/config.yaml`:
```yaml
base_ref: "epic/pr-triage-poc"
poll_interval: 1m
timeout: 10m
worktree_ttl: 72h
github_user: "dustinmays"
signal_tiers:
  default_tier: routine
  rules:
    - tier: escalate
      signals:
        - migration_sql_added
        - migration_history_rewritten
        - schema_changed_without_migration
        - dependency_manifest_changed
        - install_execution_allowed
        - test_files_deleted
        - tests_skipped_added
        - safeguard_config_changed
        - suppressions_added
        - workflow_changed
        - stack_choice_changed
routing:
  routine:
    runtime: claude-code
    model: claude-haiku-4-5
    agent_def: review-agent
  escalate:
    runtime: escalate
    model: none
    agent_def: escalate
  human:
    runtime: escalate
    model: none
    agent_def: human-review
```

### Step 2.2: Start the Daemon
Run the daemon in the foreground (concurrency is fixed at 1 internally — a single
agent runs at a time, others queue):
```bash
./bin/pr-triage run
```

In another terminal, verify daemon status and registered repositories:
```bash
./bin/pr-triage status
```

---

## 3. Headline Trial Scenarios

---

### Scenario A: Clean Implementation PR (Routine Tier)

**Objective**: Verify that a clean PR is pre-scanned, classified as `routine`, and handled by the review agent in an isolated worktree with exact cost tracking.

1. **Create and Push a Clean PR**:
   ```bash
   git checkout -b feat/trial-clean epic/pr-triage-poc
   # Make a minor change (e.g., improve a comment or add a helper function)
   git commit -am "chore: add helper utility for live trial"
   git push -u origin feat/trial-clean
   gh pr create --base epic/pr-triage-poc --head feat/trial-clean --title "Trial Clean PR" --body "Testing routine triage"
   ```

2. **Observe Layer 2 Pre-Scan**:
   - The `.github/workflows/pr-prescan.yml` workflow triggers on PR creation.
   - It runs `scripts/pr-prescan.sh` and publishes the Check Run `pr-prescan-report` with `conclusion: neutral`.
   - A short comment marked with `<!-- prescan:v1 -->` is posted to the PR, pointing at the workflow run and giving the manual `claude --agent review-agent` one-liner - the full report JSON lives only on the check run, not in the comment.

3. **Observe Daemon Ingestion & Agent Execution**:
   - The poller picks up the `report_ready` event on check completion.
   - Orchestrator classifies the PR as `routine` and routes to `runtime: claude-code` (`agent_def: review-agent`).
   - An isolated worktree is created under `/tmp/pr-triage-worktrees/<owner>-<repo>-<pr>`.
   - PR state moves: `polled` ➔ `agent_running` ➔ `done`.

4. **Verify Database Records**:
   ```bash
   sqlite3 ~/.pr-triage/pr-triage.db "SELECT number, state, head_sha FROM prs ORDER BY id DESC LIMIT 1;"
   sqlite3 ~/.pr-triage/pr-triage.db "SELECT id, risk_tier, runtime, model, cost_usd, cost_basis, turns, status FROM runs ORDER BY id DESC LIMIT 1;"
   ```
   - `status`: `done`
   - `cost_basis`: `exact`
   - `cost_usd`: > 0.00 (from terminal result event)

---

### Scenario B: Risky-Signal PR (Deterministic Escalation)

**Objective**: Verify that a PR introducing a risky signal (e.g., deleting a test file) is immediately escalated, tagged with `needs-owner-review`, and blocked from merge by `owner-review-gate`.

1. **Create and Push a Risky PR**:
   ```bash
   git checkout -b feat/trial-risky epic/pr-triage-poc
   # Simulate risk by deleting a test or adding an unmigrated schema change
   rm internal/auth/auth_test.go
   git commit -am "test: remove auth unit test (simulating test_files_deleted)"
   git push -u origin feat/trial-risky
   gh pr create --base epic/pr-triage-poc --head feat/trial-risky --title "Trial Risky PR" --body "Testing escalation on test_files_deleted"
   ```

2. **Observe Pre-Scan & Escalation**:
   - Pre-scan detects signal `test_files_deleted: present: true`.
   - Orchestrator classifies the report as `escalate`.
   - Escalator immediately applies the label `needs-owner-review` and comments with `@dustinmays` mention.
   - PR state in database transitions to `escalated`.
   - `claude` agent is **NEVER** spawned.

3. **Observe Owner Review Gate**:
   - The `.github/workflows/owner-review-routing.yml` workflow runs on the `labeled` event.
   - Job `owner-review-gate` fails (red status check), blocking merge.

4. **Simulate Human Resolution**:
   - Remove the `needs-owner-review` label via GitHub UI or CLI:
     ```bash
     gh pr edit <PR_NUMBER> --remove-label "needs-owner-review"
     ```
   - `owner-review-gate` reruns and turns green (passed).

---

### Scenario C: Daemon Crash & Startup Recovery

**Objective**: Verify that killing the daemon while an agent run is active results in safe cleanup of orphaned PIDs and worktrees upon restart.

1. **Trigger a Long-Running PR Evaluation**:
   - Submit a PR that initiates an agent run.
   - While PR state is in `agent_running` (visible via `pr-triage status`), abruptly terminate the daemon:
     ```bash
     pkill -9 pr-triage
     ```

2. **Restart the Daemon**:
   ```bash
   ./bin/pr-triage run
   ```

3. **Verify Recovery**:
   - Daemon logs show: `reconciling stranded runs in agent_running state`.
   - Stale worktree under `/tmp/pr-triage-worktrees/` is removed.
   - Database record is reconciled:
     ```bash
     sqlite3 ~/.pr-triage/pr-triage.db "SELECT id, status, stop_reason FROM runs WHERE status='failed';"
     ```
   - `status`: `failed`
   - `stop_reason`: `interrupted by daemon crash/restart`

---

## 4. SwiftBar Menu Bar Integration Verification

To observe live triage state from macOS menu bar:
```bash
# Link SwiftBar plugin script
mkdir -p ~/Library/Application\ Support/SwiftBar/Plugins
ln -sf $(pwd)/scripts/pr-triage.1m.sh ~/Library/Application\ Support/SwiftBar/Plugins/

# Verify menu bar displays:
# - Active agent run indicators (🟢 running, 🟡 idle)
# - Count of PRs in done / escalated state
# - Direct links to open PRs and view logs
```
