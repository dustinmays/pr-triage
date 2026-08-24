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
	"sync"
	"time"

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

// HandleReportReady processes a single report_ready event end-to-end.
func (o *Orchestrator) HandleReportReady(ctx context.Context, event poller.ReportReadyEvent) error {
	// 1. Fetch config for repo
	cfg := config.DefaultConfig()
	if event.Repo.ConfigPath != "" {
		if loaded, err := config.Load(event.Repo.ConfigPath); err == nil && loaded != nil {
			cfg = loaded
		}
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
		// Stale / missing report
		return err
	}

	// 3. Classify risk tier
	tier := cfg.Classify(rep)

	if tier == "human" || tier == "escalate" {
		return o.escalator.Escalate(ctx, escalate.Request{
			Repo:       event.Repo,
			PRNumber:   event.PRNumber,
			HeadSHA:    event.HeadSHA,
			Reason:     fmt.Sprintf("risk tier %q triggered escalation", tier),
			CIRunID:    &event.CheckRunID,
			GitHubUser: cfg.GitHubUser,
		})
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
