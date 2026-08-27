// Package orchestrator coordinates the end-to-end pipeline: ingesting reports on
// report_ready triggers, classifying risk tiers, routing to agents, executing runs
// in isolated worktrees with a concurrency bound of 1, and escalating hard fails.
package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	gh "github.com/google/go-github/v72/github"

	"github.com/dustinmays/pr-triage/internal/config"
	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/escalate"
	"github.com/dustinmays/pr-triage/internal/git"
	"github.com/dustinmays/pr-triage/internal/poller"
	"github.com/dustinmays/pr-triage/internal/report"
	"github.com/dustinmays/pr-triage/internal/runtime"
)

// GitHubClient defines the GitHub operations needed by the orchestrator.
type GitHubClient interface {
	FetchCheckRunOutput(ctx context.Context, owner, repo string, checkRunID int64) (*gh.CheckRunOutput, error)
	GetPR(ctx context.Context, owner, repo string, number int) (*gh.PullRequest, error)
	AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error
	CreateComment(ctx context.Context, owner, repo string, number int, body string) (int64, error)
}

// Escalator defines the escalation interface.
type Escalator interface {
	Escalate(ctx context.Context, req escalate.Request) error
}

// Store defines persistence operations required by the orchestrator.
type Store interface {
	GetPRState(repoID int64, number int) (*db.PR, error)
	UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*db.PR, error)
	RecordRun(run *db.Run) (*db.Run, error)
	UpdateRun(run *db.Run) error
	RunsInState(state string) ([]db.Run, error)
	GetActiveOverride(repoID int64, prNumber int, headSHA string) (*db.Override, error)
	MarkOverrideConsumed(id int64) error
}

// ConfigLoader loads repository or global configuration.
type ConfigLoader interface {
	Load(configPath string) (*config.Config, error)
}

// RecoveryPolicy determines how stranded runs are handled on startup.
type RecoveryPolicy string

const (
	RecoveryMarkFailed RecoveryPolicy = "mark-failed"
	RecoveryRetry      RecoveryPolicy = "retry"
)

// Options configures the Orchestrator.
type Options struct {
	Concurrency    int
	WorktreeDir    string
	WorktreeTTL    time.Duration
	LogDir         string
	AgentPrompt    string
	RecoveryPolicy RecoveryPolicy
}

// Option is a functional option for Orchestrator.
type Option func(*Options)

// WithConcurrency sets the maximum number of concurrent agent invocations (default: 1).
func WithConcurrency(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.Concurrency = n
		}
	}
}

// WithWorktreeTTL sets the maximum lifetime before stale worktrees are swept.
func WithWorktreeTTL(ttl time.Duration) Option {
	return func(o *Options) {
		if ttl > 0 {
			o.WorktreeTTL = ttl
		}
	}
}

// WithRecoveryPolicy sets the recovery policy for stranded runs on restart.
func WithRecoveryPolicy(p RecoveryPolicy) Option {
	return func(o *Options) {
		if p != "" {
			o.RecoveryPolicy = p
		}
	}
}

// WithWorktreeDir sets the parent directory where worktrees are created.
func WithWorktreeDir(dir string) Option {
	return func(o *Options) {
		o.WorktreeDir = dir
	}
}

// WithLogDir sets the directory where run log files are stored.
func WithLogDir(dir string) Option {
	return func(o *Options) {
		o.LogDir = dir
	}
}

// Orchestrator manages the execution queue and agent lifecycles.
type Orchestrator struct {
	store     Store
	client    GitHubClient
	escalator Escalator
	sem       chan struct{}
	opts      Options
	mu        sync.Mutex
	stopCh    chan struct{}
	doneCh    chan struct{}
	running   bool
}

// New creates a new Orchestrator instance.
func New(store Store, client GitHubClient, escalator Escalator, opts ...Option) *Orchestrator {
	options := Options{
		Concurrency: 1,
		WorktreeDir: filepath.Join(os.TempDir(), "pr-triage-worktrees"),
		WorktreeTTL: 72 * time.Hour,
		LogDir:      filepath.Join(os.TempDir(), "pr-triage-logs"),
		AgentPrompt: "Review and fix issues identified in the CI report.",
	}

	for _, opt := range opts {
		opt(&options)
	}

	return &Orchestrator{
		store:     store,
		client:    client,
		escalator: escalator,
		sem:       make(chan struct{}, options.Concurrency),
		opts:      options,
	}
}

// SweepStaleWorktrees sweeps worktrees in repoDir older than WorktreeTTL.
func (o *Orchestrator) SweepStaleWorktrees(ctx context.Context, repoDir string) ([]string, error) {
	if repoDir == "" {
		localPath, _ := filepath.Abs(".")
		repoDir = localPath
	}
	return git.WorktreeSweep(ctx, repoDir, o.opts.WorktreeTTL)
}

// Start listens to report_ready events from eventCh until ctx is cancelled or Stop is called.
func (o *Orchestrator) Start(ctx context.Context, eventCh <-chan poller.ReportReadyEvent) error {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return errors.New("orchestrator is already running")
	}
	o.running = true
	o.stopCh = make(chan struct{})
	o.doneCh = make(chan struct{})
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.running = false
		o.mu.Unlock()
		close(o.doneCh)
	}()

	// Reconcile and recover any stranded runs left in agent_running state on startup
	_ = o.Recover(ctx)

	// Periodic worktree sweep ticker
	sweepTicker := time.NewTicker(1 * time.Hour)
	defer sweepTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-o.stopCh:
			return nil
		case <-sweepTicker.C:
			localRepo, _ := filepath.Abs(".")
			_, _ = o.SweepStaleWorktrees(ctx, localRepo)
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if err := o.HandleReportReady(ctx, event); err != nil {
				continue
			}
		}
	}
}

// Stop halts the orchestrator loop.
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	if !o.running {
		o.mu.Unlock()
		return
	}
	close(o.stopCh)
	o.mu.Unlock()

	<-o.doneCh
}

// Recover cleans up stale worktrees/processes and reconciles runs left in agent_running state
// after a daemon crash or abrupt kill.
func (o *Orchestrator) Recover(ctx context.Context) error {
	strandedRuns, err := o.store.RunsInState(poller.StateAgentRunning)
	if err != nil {
		return fmt.Errorf("orchestrator: query stranded runs: %w", err)
	}

	for i := range strandedRuns {
		run := strandedRuns[i]

		// 1. Terminate orphaned PID if still running
		if run.PID != nil && *run.PID > 0 {
			if proc, err := os.FindProcess(*run.PID); err == nil && proc != nil {
				_ = proc.Kill()
			}
		}

		// 2. Cleanup stale worktree if present
		if run.WorktreePath != "" {
			if _, err := os.Stat(run.WorktreePath); err == nil {
				localRepoPath, _ := filepath.Abs(".")
				_ = git.WorktreeRemove(ctx, localRepoPath, run.WorktreePath)
			}
		}

		// 3. Reconcile database state based on recovery policy
		now := time.Now().UTC().Format(time.RFC3339)
		run.FinishedAt = &now

		if o.opts.RecoveryPolicy == RecoveryRetry {
			run.Status = "failed"
			run.StopReason = "interrupted by daemon crash/restart (re-enqueued)"
			_ = o.store.UpdateRun(&run)
		} else {
			run.Status = "failed"
			run.StopReason = "interrupted by daemon crash/restart"
			_ = o.store.UpdateRun(&run)
		}
	}

	return nil
}

// escalationReason renders a human-facing explanation of why a PR was escalated,
// naming the specific signal(s) that tripped the tier and citing their evidence.
// Per ADR 0007 it reports the deterministic pre-scan facts only — signal IDs and
// their file:line evidence — with no AI phrasing or editorializing, so the human's
// read is not skewed. Falls back to naming the tier when no per-signal detail
// exists (default/routing-forced escalations).
func escalationReason(rep *report.Report, class config.Classification) string {
	switch {
	case class.ByTargetKind:
		return fmt.Sprintf("PR target_kind %q requires human review (classified as %q tier).",
			class.TargetKind, class.Tier)

	case len(class.MatchedSignals) > 0:
		var b strings.Builder
		fmt.Fprintf(&b, "Pre-scan signal(s) %s tripped the %q tier.",
			strings.Join(quoteAll(class.MatchedSignals), ", "), class.Tier)

		evidence := evidenceBySignal(rep, class.MatchedSignals)
		for _, sigID := range class.MatchedSignals {
			fmt.Fprintf(&b, "\n\n**%s**", sigID)
			lines := evidence[sigID]
			if len(lines) == 0 {
				b.WriteString("\n- (no evidence detail in report)")
				continue
			}
			for _, ln := range lines {
				fmt.Fprintf(&b, "\n- %s", ln)
			}
		}
		return b.String()

	default:
		return fmt.Sprintf("risk tier %q triggered escalation", class.Tier)
	}
}

// evidenceBySignal collects the formatted evidence lines for the given signal IDs
// from the report, keyed by signal ID. Each line is "file:line — detail" (line and
// file omitted when absent).
func evidenceBySignal(rep *report.Report, ids []string) map[string][]string {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := make(map[string][]string, len(ids))
	if rep == nil {
		return out
	}
	for _, sig := range rep.Signals {
		if !sig.Present || !want[sig.ID] {
			continue
		}
		for _, ev := range sig.Evidence {
			out[sig.ID] = append(out[sig.ID], formatEvidence(ev))
		}
	}
	return out
}

// formatEvidence renders a single Evidence as "file:line — detail", omitting
// parts that are absent.
func formatEvidence(ev report.Evidence) string {
	loc := ev.File
	if ev.Line != nil {
		if loc != "" {
			loc = fmt.Sprintf("%s:%d", loc, *ev.Line)
		} else {
			loc = fmt.Sprintf("line %d", *ev.Line)
		}
	}
	switch {
	case loc != "" && ev.Detail != "":
		return fmt.Sprintf("%s — %s", loc, ev.Detail)
	case loc != "":
		return loc
	default:
		return ev.Detail
	}
}

// quoteAll wraps each string in double quotes for readable inline lists.
func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// applyOverride consults for an active human override before a PR is escalated.
// It returns (routeToAgent, reason):
//   - routeToAgent=true  -> the escalation is fully waived; the caller should run
//     the review agent instead of escalating. The override is marked consumed.
//   - routeToAgent=false -> escalate; reason explains why (naming any signals
//     that remain after a partial waiver, or the unmodified escalation reason
//     when there is no override).
//
// target_kind-driven escalations (e.g. chunk_completion) are intentional human
// gates and are never waived here — only signal-driven escalations are.
func (o *Orchestrator) applyOverride(
	event poller.ReportReadyEvent,
	rep *report.Report,
	class config.Classification,
) (bool, string) {
	baseReason := escalationReason(rep, class)

	// Only signal-driven escalations are waivable.
	if class.ByTargetKind || len(class.MatchedSignals) == 0 {
		return false, baseReason
	}

	ov, err := o.store.GetActiveOverride(event.Repo.ID, event.PRNumber, event.HeadSHA)
	if err != nil || ov == nil {
		return false, baseReason
	}

	// Determine which of the matched (escalate-tier, present) signals remain
	// after applying the waiver. An empty waiver list waives all of them.
	waived := make(map[string]bool)
	if !ov.WaivesAll() {
		for _, s := range ov.WaivedSignalList() {
			waived[s] = true
		}
	}
	var remaining []string
	for _, s := range class.MatchedSignals {
		if ov.WaivesAll() || waived[s] {
			continue
		}
		remaining = append(remaining, s)
	}

	if len(remaining) == 0 {
		// Fully waived: consume the override (one-shot) and route to the agent.
		_ = o.store.MarkOverrideConsumed(ov.ID)
		return true, ""
	}

	// Partial waiver: still escalate, but only for the signals that remain.
	reason := fmt.Sprintf(
		"Escalation partially overridden: %s waived by owner; still requires review for signal(s) %s.",
		strings.Join(quoteAll(ov.WaivedSignalList()), ", "),
		strings.Join(quoteAll(remaining), ", "),
	)
	return false, reason
}

// HandleReportReady processes a single report_ready event end-to-end.
func (o *Orchestrator) HandleReportReady(ctx context.Context, event poller.ReportReadyEvent) error {
	// 1. Fetch config for repo
	cfg := config.DefaultConfig()
	if event.Repo.ConfigPath != "" {
		if loaded, err := config.Load(event.Repo.ConfigPath); err == nil && loaded != nil {
			cfg = loaded
		}
	}

	// The poller flags ReportMissing when gating CI passed but the pre-scan
	// report check never appeared. There is no report to fetch — escalate so a
	// human is pinged instead of silently dropping the PR.
	if event.ReportMissing {
		return o.escalator.Escalate(ctx, escalate.Request{
			Repo:       event.Repo,
			PRNumber:   event.PRNumber,
			HeadSHA:    event.HeadSHA,
			Reason:     fmt.Sprintf("gating CI passed but the pre-scan report check %q never appeared within the wait ceiling", report.ReportCheckName),
			CIRunID:    nil,
			GitHubUser: cfg.GitHubUser,
		})
	}

	// 2. Fetch and validate CI report
	rep, err := report.FetchAndValidate(ctx, o.client, event.Repo.Owner, event.Repo.Name, event.CheckRunID)
	if err != nil {
		if errors.Is(err, report.ErrMalformed) || errors.Is(err, report.ErrUnsupportedVersion) {
			// Hard fail -> escalate
			return o.escalator.Escalate(ctx, escalate.Request{
				Repo:       event.Repo,
				PRNumber:   event.PRNumber,
				HeadSHA:    event.HeadSHA,
				Reason:     err.Error(),
				CIRunID:    &event.CheckRunID,
				GitHubUser: cfg.GitHubUser,
			})
		}
		// Missing report payload (report check present but empty/absent) ->
		// escalate so a human is pinged, rather than silently returning.
		if errors.Is(err, report.ErrMissing) {
			return o.escalator.Escalate(ctx, escalate.Request{
				Repo:       event.Repo,
				PRNumber:   event.PRNumber,
				HeadSHA:    event.HeadSHA,
				Reason:     fmt.Sprintf("pre-scan report check %q produced no report payload", report.ReportCheckName),
				CIRunID:    &event.CheckRunID,
				GitHubUser: cfg.GitHubUser,
			})
		}
		// Other transient/stale errors -> return for retry on the next poll.
		return err
	}

	// 3. Classify risk tier
	class := cfg.ClassifyWithReason(rep)
	tier := class.Tier

	if tier == "human" || tier == "escalate" {
		// Consult a human override (D.4) before escalating. This is the
		// state-first escape hatch: the owner can waive the specific signal(s)
		// that would escalate — pinned to this head SHA — and let the review
		// agent run instead. No daemon restart required; state is read per event.
		routeToAgent, reason := o.applyOverride(event, rep, class)
		if !routeToAgent {
			return o.escalator.Escalate(ctx, escalate.Request{
				Repo:       event.Repo,
				PRNumber:   event.PRNumber,
				HeadSHA:    event.HeadSHA,
				Reason:     reason,
				CIRunID:    &event.CheckRunID,
				GitHubUser: cfg.GitHubUser,
			})
		}
		// Fully waived -> fall through to the default (routine) tier and route.
		tier = cfg.SignalTiers.DefaultTier
		if tier == "" {
			tier = "routine"
		}
	}

	// 4. Route risk tier
	routing, err := cfg.Route(tier)
	if err != nil {
		if errors.Is(err, config.ErrUnmappedTier) {
			// Hard fail -> escalate
			return o.escalator.Escalate(ctx, escalate.Request{
				Repo:       event.Repo,
				PRNumber:   event.PRNumber,
				HeadSHA:    event.HeadSHA,
				Reason:     fmt.Sprintf("unmapped risk tier %q: %v", tier, err),
				CIRunID:    &event.CheckRunID,
				GitHubUser: cfg.GitHubUser,
			})
		}
		return err
	}

	if routing.Runtime == "escalate" {
		return o.escalator.Escalate(ctx, escalate.Request{
			Repo:       event.Repo,
			PRNumber:   event.PRNumber,
			HeadSHA:    event.HeadSHA,
			Reason:     fmt.Sprintf("routing configured to escalate for tier %q", tier),
			CIRunID:    &event.CheckRunID,
			GitHubUser: cfg.GitHubUser,
		})
	}

	// 5. Acquire concurrency slot (bounded at 1 by default)
	select {
	case o.sem <- struct{}{}:
		defer func() { <-o.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// 6. Execute agent invocation in worktree
	return o.executeRun(ctx, event, cfg, rep, tier, routing)
}

func (o *Orchestrator) executeRun(
	ctx context.Context,
	event poller.ReportReadyEvent,
	cfg *config.Config,
	rep *report.Report,
	tier string,
	routing config.Routing,
) error {
	// Update PR state to agent_running in DB
	prRecord, err := o.store.UpsertPRState(event.Repo.ID, event.PRNumber, event.HeadSHA, &event.CheckRunID, poller.StateAgentRunning)
	if err != nil {
		return fmt.Errorf("orchestrator: upsert pr agent_running: %w", err)
	}

	// Create run record in DB
	runRecord := &db.Run{
		PRID:        prRecord.ID,
		HeadSHA:     event.HeadSHA,
		CIRunID:     &event.CheckRunID,
		RiskTier:    tier,
		Runtime:     routing.Runtime,
		Model:       routing.Model,
		ModelSource: "routing",
		CostUSD:     0.0,
		CostBasis:   "unavailable",
		Status:      poller.StateAgentRunning,
	}
	runRecord, err = o.store.RecordRun(runRecord)
	if err != nil {
		return fmt.Errorf("orchestrator: record run: %w", err)
	}

	// Prepare directories
	_ = os.MkdirAll(o.opts.WorktreeDir, 0755)
	_ = os.MkdirAll(o.opts.LogDir, 0755)

	worktreePath := filepath.Join(o.opts.WorktreeDir, fmt.Sprintf("%s-%s-%d-%s", event.Repo.Owner, event.Repo.Name, event.PRNumber, event.HeadSHA[:min(7, len(event.HeadSHA))]))
	logPath := filepath.Join(o.opts.LogDir, fmt.Sprintf("run-%d.log", runRecord.ID))

	runRecord.WorktreePath = worktreePath
	runRecord.LogPath = logPath
	_ = o.store.UpdateRun(runRecord)

	// Create git worktree
	localRepoPath, _ := filepath.Abs(".")
	_ = git.WorktreeAdd(ctx, localRepoPath, worktreePath, event.HeadSHA)
	defer func() {
		_ = git.WorktreeRemove(ctx, localRepoPath, worktreePath)
	}()

	// Lookup agent runtime adapter
	adapter, err := runtime.Get(routing.Runtime)
	if err != nil {
		// Hard fail unknown runtime -> escalate
		runRecord.Status = "failed"
		runRecord.StopReason = fmt.Sprintf("unregistered runtime %q: %v", routing.Runtime, err)
		_ = o.store.UpdateRun(runRecord)
		return o.escalator.Escalate(ctx, escalate.Request{
			Repo:       event.Repo,
			PRNumber:   event.PRNumber,
			HeadSHA:    event.HeadSHA,
			Reason:     runRecord.StopReason,
			CIRunID:    &event.CheckRunID,
			GitHubUser: cfg.GitHubUser,
		})
	}

	timeout := 10 * time.Minute
	if cfg.Timeout != "" {
		if parsed, err := time.ParseDuration(cfg.Timeout); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	inv := runtime.Invocation{
		AgentName: routing.AgentDef,
		Model:     routing.Model,
		Prompt:    o.opts.AgentPrompt,
		Workdir:   worktreePath,
		Limits: runtime.Limits{
			Timeout: timeout,
		},
		PIDCallback: func(pid int) {
			runRecord.PID = &pid
			_ = o.store.UpdateRun(runRecord)
		},
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		logFile = nil
	}
	defer func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}()

	var logBuf bytes.Buffer
	exitCode, runErr := adapter.Run(ctx, inv, &logBuf)
	if logFile != nil {
		_, _ = logFile.Write(logBuf.Bytes())
	}

	parsedRes, parseErr := adapter.ParseResult(bytes.NewReader(logBuf.Bytes()))
	if parseErr != nil || parsedRes == nil {
		parsedRes = &runtime.Result{
			Cost:       0.0,
			CostBasis:  runtime.CostBasisUnavailable,
			StopReason: "failed to parse agent log",
			IsError:    runErr != nil,
		}
	}

	outcome := adapter.ClassifyOutcome(parsedRes, exitCode)
	now := time.Now().UTC().Format(time.RFC3339)
	runRecord.FinishedAt = &now
	runRecord.CostUSD = parsedRes.Cost
	runRecord.CostBasis = string(parsedRes.CostBasis)
	runRecord.Turns = parsedRes.Turns
	runRecord.StopReason = parsedRes.StopReason

	if outcome == runtime.OutcomeSuccess && runErr == nil {
		// Check for fixes made in worktree
		if hasChanges, _ := git.HasChanges(ctx, worktreePath); hasChanges {
			prBranch := rep.PR.Head
			_, _ = git.CommitAndPush(ctx, worktreePath, prBranch, fmt.Sprintf("fix: automated triage fix for PR #%d", event.PRNumber))
		}

		runRecord.Status = "done"
		_ = o.store.UpdateRun(runRecord)
		_, _ = o.store.UpsertPRState(event.Repo.ID, event.PRNumber, event.HeadSHA, &event.CheckRunID, poller.StateDone)

		// Deterministically post the agent's review to the PR. Delivery must not
		// depend on the agent remembering to run `gh pr comment` — it often does
		// not (observed on haiku). The adapter already captured the agent's final
		// summary in parsedRes.Summary; posting it here guarantees the review is
		// visible on the PR and costs zero agent turns.
		o.postReviewComment(ctx, event, parsedRes.Summary)
		return nil
	}

	// Execution failed or timed out
	runRecord.Status = "failed"
	if outcome == runtime.OutcomeTimeout {
		runRecord.Status = "timeout"
	}
	_ = o.store.UpdateRun(runRecord)
	_, _ = o.store.UpsertPRState(event.Repo.ID, event.PRNumber, event.HeadSHA, &event.CheckRunID, poller.StateCIFailed)
	return nil
}

// reviewCommentMarker tags orchestrator-posted review comments so they can be
// recognized later (e.g. for future update-or-create idempotency).
const reviewCommentMarker = "<!-- pr-triage:review -->"

// postReviewComment posts the agent's review summary to the PR. It is
// best-effort: a failure to comment must not fail the run (the review is also in
// the run log). Create-only for now; update-or-create idempotency is a tracked
// follow-up (docs/epic-80/deferred/orchestrator-should-post-review-comment.md).
func (o *Orchestrator) postReviewComment(ctx context.Context, event poller.ReportReadyEvent, summary string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}

	body := reviewCommentMarker + "\n\n## 🤖 pr-triage review\n\n" + summary

	// Stay under GitHub's ~65536-char issue-comment limit, truncating on a valid
	// UTF-8 boundary.
	const maxBody = 60000
	if len(body) > maxBody {
		b := body[:maxBody]
		for len(b) > 0 && !utf8.ValidString(b) {
			b = b[:len(b)-1]
		}
		body = b + "\n\n_…(truncated by pr-triage; full review in the run log)_"
	}

	if _, err := o.client.CreateComment(ctx, event.Repo.Owner, event.Repo.Name, event.PRNumber, body); err != nil {
		// Best-effort: a comment failure must not fail the run, but it must not
		// be silent either — a swallowed error here hid a real problem during
		// dogfooding. The review is still in the run log.
		fmt.Fprintf(os.Stderr, "pr-triage: failed to post review comment on %s/%s#%d: %v\n",
			event.Repo.Owner, event.Repo.Name, event.PRNumber, err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
