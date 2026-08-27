// Package poller implements the deterministic poll loop, PR state machine,
// and exponential backoff CI wait for watched repositories.
package poller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	gh "github.com/google/go-github/v72/github"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/report"
)

// PR state machine state constants matching the plan diagram:
// idle -> ci_running -> ci_passed -> report_ready -> agent_running -> done | escalated
//
//	\-> ci_failed(watching head SHA) -/  (new push -> back to ci_running)
const (
	StateIdle         = "idle"
	StateCIRunning    = "ci_running"
	StateCIPassed     = "ci_passed"
	StateCIFailed     = "ci_failed"
	StateReportReady  = "report_ready"
	StateAgentRunning = "agent_running"
	StateDone         = "done"
	StateEscalated    = "escalated"
)

// Common poller errors.
var (
	ErrCITimeout     = errors.New("poller: CI wait timed out ceiling reached")
	ErrPollerStopped = errors.New("poller: stopped")
	ErrAlreadyRun    = errors.New("poller: already running")
)

// Default configuration parameters.
const (
	DefaultPollInterval   = 5 * time.Minute
	DefaultInitialBackoff = 30 * time.Second
	DefaultMaxBackoff     = 5 * time.Minute
	DefaultBackoffFactor  = 2.0
	DefaultTimeoutCeiling = 10 * time.Minute
)

// Store represents the persistence interface required by the poller.
type Store interface {
	ListRepos() ([]db.Repo, error)
	GetPRState(repoID int64, number int) (*db.PR, error)
	UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*db.PR, error)
}

// GitHubClient represents the GitHub API interface required by the poller.
type GitHubClient interface {
	ListOpenPRs(ctx context.Context, owner, repo, baseRef string) ([]*gh.PullRequest, error)
	ListCheckRunsForSHA(ctx context.Context, owner, repo, sha string) ([]*gh.CheckRun, error)
}

// ReportReadyEvent is emitted when CI passes for a PR head SHA, signalling
// Chunk 3 report processing without fetching the report blob itself.
type ReportReadyEvent struct {
	Repo       db.Repo
	PRNumber   int
	HeadSHA    string
	CheckRunID int64
}

// SleepFunc is a hook for context-aware sleeping (customizable for deterministic tests).
type SleepFunc func(ctx context.Context, d time.Duration) error

// Options configures the Poller instance.
type Options struct {
	PollInterval   time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	TimeoutCeiling time.Duration
	ReadyChan      chan ReportReadyEvent
	Sleep          SleepFunc
}

// Option is a functional option for Poller.
type Option func(*Options)

// WithPollInterval sets the default polling interval between ticker sweeps.
func WithPollInterval(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.PollInterval = d
		}
	}
}

// WithInitialBackoff sets the initial CI-wait sleep duration.
func WithInitialBackoff(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.InitialBackoff = d
		}
	}
}

// WithMaxBackoff sets the maximum CI-wait sleep interval ceiling.
func WithMaxBackoff(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.MaxBackoff = d
		}
	}
}

// WithBackoffFactor sets the multiplier for exponential backoff during CI wait.
func WithBackoffFactor(factor float64) Option {
	return func(o *Options) {
		if factor > 1.0 {
			o.BackoffFactor = factor
		}
	}
}

// WithTimeoutCeiling sets the maximum duration to wait for CI to complete.
func WithTimeoutCeiling(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.TimeoutCeiling = d
		}
	}
}

// WithReportReadyChan sets a custom channel for ReportReadyEvent emission.
func WithReportReadyChan(ch chan ReportReadyEvent) Option {
	return func(o *Options) {
		o.ReadyChan = ch
	}
}

// WithSleepFunc sets a custom sleep function (useful for mock/unit tests).
func WithSleepFunc(fn SleepFunc) Option {
	return func(o *Options) {
		o.Sleep = fn
	}
}

// Poller runs the ticker-driven repository watcher and PR state machine.
type Poller struct {
	store   Store
	client  GitHubClient
	opts    Options
	readyCh chan ReportReadyEvent
	stopCh  chan struct{}
	doneCh  chan struct{}
	mu      sync.Mutex
	running bool
}

// New creates a new Poller instance with the given store, GitHub client, and options.
func New(store Store, client GitHubClient, opts ...Option) *Poller {
	options := Options{
		PollInterval:   DefaultPollInterval,
		InitialBackoff: DefaultInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
		BackoffFactor:  DefaultBackoffFactor,
		TimeoutCeiling: DefaultTimeoutCeiling,
		Sleep:          defaultSleep,
	}

	for _, opt := range opts {
		opt(&options)
	}

	readyCh := options.ReadyChan
	if readyCh == nil {
		readyCh = make(chan ReportReadyEvent, 64)
	}

	return &Poller{
		store:   store,
		client:  client,
		opts:    options,
		readyCh: readyCh,
	}
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ReportReadyEvents returns the receive-only channel of report_ready signals.
func (p *Poller) ReportReadyEvents() <-chan ReportReadyEvent {
	return p.readyCh
}

// Start runs the ticker loop over registered repos until ctx is cancelled or Stop is called.
func (p *Poller) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return ErrAlreadyRun
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		close(p.doneCh)
	}()

	// Run initial sweep immediately
	_ = p.PollOnce(ctx)

	ticker := time.NewTicker(p.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.stopCh:
			return nil
		case <-ticker.C:
			if err := p.PollOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				// Continue loop even if one iteration encountered an error
				continue
			}
		}
	}
}

// Stop terminates a running Poller loop and waits for shutdown completion.
func (p *Poller) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	close(p.stopCh)
	p.mu.Unlock()

	<-p.doneCh
}

// PollOnce executes a single poll pass across all registered repositories.
func (p *Poller) PollOnce(ctx context.Context) error {
	repos, err := p.store.ListRepos()
	if err != nil {
		return fmt.Errorf("poller: list repos: %w", err)
	}

	for _, repo := range repos {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := p.PollRepo(ctx, repo); err != nil {
			// Log/continue with other repos rather than aborting the entire pass
			continue
		}
	}

	return nil
}

// PollRepo polls all open PRs matching the base ref for a single repository.
func (p *Poller) PollRepo(ctx context.Context, repo db.Repo) error {
	prs, err := p.client.ListOpenPRs(ctx, repo.Owner, repo.Name, repo.BaseRef)
	if err != nil {
		return fmt.Errorf("poller: list open prs for %s/%s: %w", repo.Owner, repo.Name, err)
	}

	for _, pr := range prs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := p.ProcessPR(ctx, repo, pr); err != nil {
			continue
		}
	}

	return nil
}

// ProcessPR evaluates and advances the state machine for a single pull request.
func (p *Poller) ProcessPR(ctx context.Context, repo db.Repo, pr *gh.PullRequest) error {
	if pr == nil {
		return nil
	}
	number := pr.GetNumber()
	headSHA := pr.GetHead().GetSHA()
	if number == 0 || headSHA == "" {
		return nil
	}

	existing, err := p.store.GetPRState(repo.ID, number)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return fmt.Errorf("poller: get pr state %s/%s#%d: %w", repo.Owner, repo.Name, number, err)
	}

	// Case 1: Fresh/new PR not yet in store.
	if errors.Is(err, db.ErrNotFound) || existing == nil {
		if _, err := p.store.UpsertPRState(repo.ID, number, headSHA, nil, StateCIRunning); err != nil {
			return fmt.Errorf("poller: init pr state: %w", err)
		}
		return p.pollCI(ctx, repo, number, headSHA)
	}

	// Case 2: New push detected (head SHA changed).
	// Reset to ci_running and discard stale failed/terminal state.
	if existing.HeadSHA != headSHA {
		if _, err := p.store.UpsertPRState(repo.ID, number, headSHA, nil, StateCIRunning); err != nil {
			return fmt.Errorf("poller: reset pr state on new push: %w", err)
		}
		return p.pollCI(ctx, repo, number, headSHA)
	}

	// Case 3: Same head SHA already in terminal / active state (idempotency guard).
	switch existing.State {
	case StateReportReady, StateAgentRunning, StateDone:
		// Already processed for this SHA -> no-op.
		return nil
	case StateEscalated:
		// Escalated is human-owned terminal state (ADR 0006: local state is the
		// source of truth). Once a PR is escalated at a head SHA, only a human
		// action (the override) or a new push (handled by Case 2's SHA change)
		// may leave it. Re-polling must not re-run CI evaluation here, or the PR
		// would re-escalate or be overwritten with a stale ci_failed on the next
		// sweep. No-op until the head SHA changes.
		return nil
	case StateCIFailed:
		// Stale failed state watching head SHA -> no-op until new push.
		return nil
	case StateCIPassed:
		// Already passed -> advance to report_ready if not emitted.
		return p.emitReportReady(ctx, repo, number, headSHA, existing.LastRunID)
	case StateCIRunning, StateIdle:
		// In-flight CI -> continue polling CI.
		return p.pollCI(ctx, repo, number, headSHA)
	default:
		return p.pollCI(ctx, repo, number, headSHA)
	}
}

// checkRunStatus represents evaluated check run state.
type checkRunState int

const (
	checkRunPending checkRunState = iota
	checkRunPassed
	checkRunFailed
)

// pollCI executes exponential backoff CI polling until completion or timeout ceiling.
func (p *Poller) pollCI(ctx context.Context, repo db.Repo, number int, headSHA string) error {
	backoff := p.opts.InitialBackoff
	start := time.Now()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		checkRuns, err := p.client.ListCheckRunsForSHA(ctx, repo.Owner, repo.Name, headSHA)
		if err != nil {
			// On API error, treat as pending and back off
			checkRuns = nil
		}

		status, runID := evaluateCheckRuns(checkRuns)

		// The pre-scan report lives in a dedicated check run (report.ReportCheckName),
		// not the arbitrary gating run evaluateCheckRuns happens to return. Resolve
		// it by name so the orchestrator fetches the report from the right place.
		reportID := reportCheckRunID(checkRuns)

		// If gating otherwise passed but the report check run hasn't registered
		// yet, keep waiting instead of emitting report_ready with a wrong ID.
		if status == checkRunPassed && reportID == 0 {
			status = checkRunPending
		}

		switch status {
		case checkRunPassed:
			// CI passed -> persist report_ready and emit internal signal
			return p.emitReportReady(ctx, repo, number, headSHA, &reportID)

		case checkRunFailed:
			// CI failed -> persist ci_failed (watching head SHA)
			if _, err := p.store.UpsertPRState(repo.ID, number, headSHA, runID, StateCIFailed); err != nil {
				return fmt.Errorf("poller: save ci_failed state: %w", err)
			}
			return nil

		case checkRunPending:
			// CI is still in progress or no check runs have registered yet
			if time.Since(start)+backoff > p.opts.TimeoutCeiling {
				// Timeout ceiling reached -> mark as ci_failed
				_, _ = p.store.UpsertPRState(repo.ID, number, headSHA, runID, StateCIFailed)
				return ErrCITimeout
			}

			if err := p.opts.Sleep(ctx, backoff); err != nil {
				return err
			}

			// Apply exponential backoff multiplier capped at MaxBackoff
			nextBackoff := time.Duration(float64(backoff) * p.opts.BackoffFactor)
			if nextBackoff > p.opts.MaxBackoff {
				nextBackoff = p.opts.MaxBackoff
			}
			backoff = nextBackoff
		}
	}
}

// reportCheckRunID returns the ID of the pre-scan report check run among runs,
// identified by name, or 0 if it is not present. The report JSON lives only in
// this check run's output; the other checks (lint, test, build, …) do not carry it.
func reportCheckRunID(runs []*gh.CheckRun) int64 {
	for _, run := range runs {
		if run != nil && run.GetName() == report.ReportCheckName && run.GetID() != 0 {
			return run.GetID()
		}
	}
	return 0
}

// evaluateCheckRuns inspects check runs for a commit SHA.
func evaluateCheckRuns(runs []*gh.CheckRun) (checkRunState, *int64) {
	if len(runs) == 0 {
		return checkRunPending, nil
	}

	var latestRunID *int64
	allCompleted := true
	hasFailure := false

	for _, run := range runs {
		if run == nil {
			continue
		}
		id := run.GetID()
		if id != 0 {
			runIDVal := id
			latestRunID = &runIDVal
		}

		status := run.GetStatus()
		conclusion := run.GetConclusion()

		if status != "completed" {
			allCompleted = false
		}

		switch conclusion {
		case "failure", "timed_out", "cancelled", "action_required":
			hasFailure = true
			if id != 0 {
				runIDVal := id
				latestRunID = &runIDVal
			}
		}
	}

	if !allCompleted {
		return checkRunPending, latestRunID
	}

	if hasFailure {
		return checkRunFailed, latestRunID
	}

	return checkRunPassed, latestRunID
}

func (p *Poller) emitReportReady(ctx context.Context, repo db.Repo, number int, headSHA string, runID *int64) error {
	var runIDVal int64
	if runID != nil {
		runIDVal = *runID
	}

	if _, err := p.store.UpsertPRState(repo.ID, number, headSHA, runID, StateReportReady); err != nil {
		return fmt.Errorf("poller: update report_ready state: %w", err)
	}

	event := ReportReadyEvent{
		Repo:       repo,
		PRNumber:   number,
		HeadSHA:    headSHA,
		CheckRunID: runIDVal,
	}

	select {
	case p.readyCh <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Non-blocking fallback if buffer is full so poller does not deadlock
		select {
		case p.readyCh <- event:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	}
}
