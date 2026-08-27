package poller_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gh "github.com/google/go-github/v72/github"

	"github.com/dustinmays/pr-triage/internal/db"
	internalgh "github.com/dustinmays/pr-triage/internal/github"
	"github.com/dustinmays/pr-triage/internal/poller"
)

// mockStore provides an in-memory db.Store implementation for unit tests.
type mockStore struct {
	mu    sync.Mutex
	repos []db.Repo
	prs   map[string]*db.PR
}

func newMockStore() *mockStore {
	return &mockStore{
		repos: make([]db.Repo, 0),
		prs:   make(map[string]*db.PR),
	}
}

func (s *mockStore) ListRepos() ([]db.Repo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]db.Repo, len(s.repos))
	copy(res, s.repos)
	return res, nil
}

func (s *mockStore) GetPRState(repoID int64, number int) (*db.PR, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := prKey(repoID, number)
	pr, ok := s.prs[key]
	if !ok {
		return nil, db.ErrNotFound
	}
	cp := *pr
	return &cp, nil
}

func (s *mockStore) UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*db.PR, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := prKey(repoID, number)
	existing, ok := s.prs[key]
	if !ok {
		existing = &db.PR{
			ID:     int64(len(s.prs) + 1),
			RepoID: repoID,
			Number: number,
		}
		s.prs[key] = existing
	}
	existing.HeadSHA = headSHA
	existing.LastRunID = runID
	existing.State = state
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	cp := *existing
	return &cp, nil
}

func prKey(repoID int64, number int) string {
	return string(rune(repoID)) + ":" + string(rune(number))
}

// mockGitHubClient provides simulated GitHub responses.
type mockGitHubClient struct {
	mu              sync.Mutex
	prs             map[string][]*gh.PullRequest
	checkRuns       map[string][]*gh.CheckRun
	checkRunCalls   int
	checkRunSeqFunc func(sha string, callCount int) []*gh.CheckRun
}

func newMockGitHubClient() *mockGitHubClient {
	return &mockGitHubClient{
		prs:       make(map[string][]*gh.PullRequest),
		checkRuns: make(map[string][]*gh.CheckRun),
	}
}

func (c *mockGitHubClient) ListOpenPRs(ctx context.Context, owner, repo, baseRef string) ([]*gh.PullRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := owner + "/" + repo
	prs, ok := c.prs[key]
	if !ok {
		return []*gh.PullRequest{}, nil
	}
	return prs, nil
}

func (c *mockGitHubClient) ListCheckRunsForSHA(ctx context.Context, owner, repo, sha string) ([]*gh.CheckRun, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkRunCalls++
	if c.checkRunSeqFunc != nil {
		return c.checkRunSeqFunc(sha, c.checkRunCalls), nil
	}
	runs, ok := c.checkRuns[sha]
	if !ok {
		return []*gh.CheckRun{}, nil
	}
	return runs, nil
}

func TestPoller_NewPR_CIPasses(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:           1,
		Owner:        "owner",
		Name:         "repo",
		BaseRef:      "main",
		PollInterval: "5m",
	})

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(42),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-pass")},
			Base:   &gh.PullRequestBranch{Ref: gh.Ptr("main")},
		},
	}
	client.checkRuns["sha-pass"] = []*gh.CheckRun{
		{
			ID:         gh.Ptr(int64(999)),
			Name:       gh.Ptr("pr-prescan-report"),
			Status:     gh.Ptr("completed"),
			Conclusion: gh.Ptr("success"),
		},
	}

	p := poller.New(store, client,
		poller.WithInitialBackoff(10*time.Millisecond),
		poller.WithTimeoutCeiling(1*time.Second),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	prState, err := store.GetPRState(1, 42)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateReportReady {
		t.Errorf("prState.State = %q, want %q", prState.State, poller.StateReportReady)
	}
	if prState.HeadSHA != "sha-pass" {
		t.Errorf("prState.HeadSHA = %q, want sha-pass", prState.HeadSHA)
	}
	if prState.LastRunID == nil || *prState.LastRunID != 999 {
		t.Errorf("prState.LastRunID = %v, want 999", prState.LastRunID)
	}

	select {
	case evt := <-p.ReportReadyEvents():
		if evt.PRNumber != 42 || evt.HeadSHA != "sha-pass" || evt.CheckRunID != 999 {
			t.Errorf("unexpected event: %+v", evt)
		}
	default:
		t.Fatal("expected ReportReadyEvent, got none")
	}
}

// TestPoller_GatingGreenNoReportCheck_StaysPending guards the fix for the
// dogfood-surfaced bug where the poller emitted report_ready with an arbitrary
// check-run ID. When every gating check is green but the pr-prescan-report check
// run is absent, the poller must NOT emit report_ready (which would make the
// orchestrator fetch the report from the wrong check run); it should keep waiting
// until the ceiling, then mark ci_failed.
func TestPoller_GatingGreenNoReportCheck_StaysPending(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(7),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-noreport")},
			Base:   &gh.PullRequestBranch{Ref: gh.Ptr("main")},
		},
	}
	// All gating checks pass, but none is the pr-prescan-report check.
	client.checkRuns["sha-noreport"] = []*gh.CheckRun{
		{ID: gh.Ptr(int64(11)), Name: gh.Ptr("lint"), Status: gh.Ptr("completed"), Conclusion: gh.Ptr("success")},
		{ID: gh.Ptr(int64(12)), Name: gh.Ptr("test"), Status: gh.Ptr("completed"), Conclusion: gh.Ptr("success")},
	}

	p := poller.New(store, client,
		poller.WithInitialBackoff(10*time.Millisecond),
		poller.WithTimeoutCeiling(50*time.Millisecond),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	// Expected to hit the timeout ceiling rather than emit report_ready.
	_ = p.PollOnce(ctx)

	prState, err := store.GetPRState(1, 7)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State == poller.StateReportReady {
		t.Errorf("state = report_ready, but no pr-prescan-report check exists; want it to keep waiting/ci_failed")
	}

	select {
	case evt := <-p.ReportReadyEvents():
		t.Fatalf("unexpected ReportReadyEvent emitted without a report check run: %+v", evt)
	default:
	}
}

func TestPoller_NewPR_CIFails(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(43),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-fail")},
			Base:   &gh.PullRequestBranch{Ref: gh.Ptr("main")},
		},
	}
	client.checkRuns["sha-fail"] = []*gh.CheckRun{
		{
			ID:         gh.Ptr(int64(1001)),
			Status:     gh.Ptr("completed"),
			Conclusion: gh.Ptr("failure"),
		},
	}

	p := poller.New(store, client,
		poller.WithInitialBackoff(10*time.Millisecond),
		poller.WithTimeoutCeiling(1*time.Second),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	prState, err := store.GetPRState(1, 43)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateCIFailed {
		t.Errorf("prState.State = %q, want %q", prState.State, poller.StateCIFailed)
	}
	if prState.LastRunID == nil || *prState.LastRunID != 1001 {
		t.Errorf("prState.LastRunID = %v, want 1001", prState.LastRunID)
	}

	select {
	case evt := <-p.ReportReadyEvents():
		t.Fatalf("expected no ReportReadyEvent on CI failure, got %+v", evt)
	default:
		// OK
	}
}

func TestPoller_Idempotency_SameSHA_NoOp(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})
	runID := int64(888)
	_, _ = store.UpsertPRState(1, 50, "sha-same", &runID, poller.StateReportReady)

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(50),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-same")},
		},
	}

	p := poller.New(store, client,
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	if client.checkRunCalls != 0 {
		t.Errorf("expected 0 check run calls for already processed PR, got %d", client.checkRunCalls)
	}

	select {
	case evt := <-p.ReportReadyEvents():
		t.Fatalf("expected no ReportReadyEvent for already processed PR, got %+v", evt)
	default:
		// OK
	}
}

func TestPoller_NewPush_ResetsToCIRunning(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})
	oldRunID := int64(100)
	_, _ = store.UpsertPRState(1, 60, "sha-old", &oldRunID, poller.StateCIFailed)

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(60),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-new")},
		},
	}
	client.checkRuns["sha-new"] = []*gh.CheckRun{
		{
			ID:         gh.Ptr(int64(200)),
			Name:       gh.Ptr("pr-prescan-report"),
			Status:     gh.Ptr("completed"),
			Conclusion: gh.Ptr("success"),
		},
	}

	p := poller.New(store, client,
		poller.WithInitialBackoff(10*time.Millisecond),
		poller.WithTimeoutCeiling(1*time.Second),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	prState, err := store.GetPRState(1, 60)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateReportReady {
		t.Errorf("prState.State = %q, want %q", prState.State, poller.StateReportReady)
	}
	if prState.HeadSHA != "sha-new" {
		t.Errorf("prState.HeadSHA = %q, want sha-new", prState.HeadSHA)
	}
	if prState.LastRunID == nil || *prState.LastRunID != 200 {
		t.Errorf("prState.LastRunID = %v, want 200", prState.LastRunID)
	}

	select {
	case evt := <-p.ReportReadyEvents():
		if evt.PRNumber != 60 || evt.HeadSHA != "sha-new" || evt.CheckRunID != 200 {
			t.Errorf("unexpected event: %+v", evt)
		}
	default:
		t.Fatal("expected ReportReadyEvent, got none")
	}
}

func TestPoller_Escalated_SameSHA_NoOp(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})
	runID := int64(777)
	_, _ = store.UpsertPRState(1, 55, "sha-esc", &runID, poller.StateEscalated)

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(55),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-esc")},
		},
	}
	// A failing check run is present for the escalated head SHA. If the poller
	// were to re-evaluate CI here, it would overwrite the human-owned escalated
	// state with ci_failed. It must not: escalated is terminal until a new push.
	client.checkRuns["sha-esc"] = []*gh.CheckRun{
		{
			ID:         gh.Ptr(int64(1001)),
			Status:     gh.Ptr("completed"),
			Conclusion: gh.Ptr("failure"),
		},
	}

	p := poller.New(store, client,
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	if client.checkRunCalls != 0 {
		t.Errorf("expected 0 check run calls for escalated PR, got %d", client.checkRunCalls)
	}

	prState, err := store.GetPRState(1, 55)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateEscalated {
		t.Errorf("prState.State = %q, want %q (escalated must survive re-poll)", prState.State, poller.StateEscalated)
	}

	select {
	case evt := <-p.ReportReadyEvents():
		t.Fatalf("expected no ReportReadyEvent for escalated PR, got %+v", evt)
	default:
		// OK
	}
}

func TestPoller_Escalated_NewPush_ResetsToCIRunning(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})
	oldRunID := int64(300)
	_, _ = store.UpsertPRState(1, 56, "sha-old-esc", &oldRunID, poller.StateEscalated)

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(56),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-new-esc")},
		},
	}
	client.checkRuns["sha-new-esc"] = []*gh.CheckRun{
		{
			ID:         gh.Ptr(int64(400)),
			Name:       gh.Ptr("pr-prescan-report"),
			Status:     gh.Ptr("completed"),
			Conclusion: gh.Ptr("success"),
		},
	}

	p := poller.New(store, client,
		poller.WithInitialBackoff(10*time.Millisecond),
		poller.WithTimeoutCeiling(1*time.Second),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	prState, err := store.GetPRState(1, 56)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	// A new push must pull the PR out of the human-owned escalated state and
	// re-enter the CI lifecycle for the new SHA.
	if prState.State != poller.StateReportReady {
		t.Errorf("prState.State = %q, want %q after new push", prState.State, poller.StateReportReady)
	}
	if prState.HeadSHA != "sha-new-esc" {
		t.Errorf("prState.HeadSHA = %q, want sha-new-esc", prState.HeadSHA)
	}
}

func TestPoller_CIWait_Backoff_PendingToSuccess(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(70),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-wait")},
		},
	}

	// 1st call: in_progress
	// 2nd call: in_progress
	// 3rd call: completed success
	client.checkRunSeqFunc = func(sha string, callCount int) []*gh.CheckRun {
		if callCount < 3 {
			return []*gh.CheckRun{
				{
					ID:     gh.Ptr(int64(700)),
					Status: gh.Ptr("in_progress"),
				},
			}
		}
		return []*gh.CheckRun{
			{
				ID:         gh.Ptr(int64(700)),
				Name:       gh.Ptr("pr-prescan-report"),
				Status:     gh.Ptr("completed"),
				Conclusion: gh.Ptr("success"),
			},
		}
	}

	var sleepDurations []time.Duration
	var mu sync.Mutex

	p := poller.New(store, client,
		poller.WithInitialBackoff(50*time.Millisecond),
		poller.WithBackoffFactor(2.0),
		poller.WithMaxBackoff(500*time.Millisecond),
		poller.WithTimeoutCeiling(5*time.Second),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			sleepDurations = append(sleepDurations, d)
			mu.Unlock()
			return nil
		}),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sleepDurations) != 2 {
		t.Fatalf("expected 2 sleep backoff iterations, got %d (%v)", len(sleepDurations), sleepDurations)
	}
	if sleepDurations[0] != 50*time.Millisecond {
		t.Errorf("1st backoff = %v, want 50ms", sleepDurations[0])
	}
	if sleepDurations[1] != 100*time.Millisecond {
		t.Errorf("2nd backoff = %v, want 100ms", sleepDurations[1])
	}

	prState, err := store.GetPRState(1, 70)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateReportReady {
		t.Errorf("prState.State = %q, want %q", prState.State, poller.StateReportReady)
	}
}

func TestPoller_CIWait_TimeoutCeiling(t *testing.T) {
	store := newMockStore()
	store.repos = append(store.repos, db.Repo{
		ID:      1,
		Owner:   "owner",
		Name:    "repo",
		BaseRef: "main",
	})

	client := newMockGitHubClient()
	client.prs["owner/repo"] = []*gh.PullRequest{
		{
			Number: gh.Ptr(80),
			Head:   &gh.PullRequestBranch{SHA: gh.Ptr("sha-hung")},
		},
	}
	// Always in_progress
	client.checkRuns["sha-hung"] = []*gh.CheckRun{
		{
			ID:     gh.Ptr(int64(800)),
			Status: gh.Ptr("in_progress"),
		},
	}

	timeoutCeiling := 200 * time.Millisecond

	p := poller.New(store, client,
		poller.WithInitialBackoff(100*time.Millisecond),
		poller.WithBackoffFactor(2.0),
		poller.WithTimeoutCeiling(timeoutCeiling),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error {
			time.Sleep(d)
			return nil
		}),
	)

	ctx := context.Background()
	_ = p.PollOnce(ctx)

	prState, err := store.GetPRState(1, 80)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateCIFailed {
		t.Errorf("prState.State = %q, want %q on timeout ceiling", prState.State, poller.StateCIFailed)
	}
}

func TestPoller_StartStop(t *testing.T) {
	store := newMockStore()
	client := newMockGitHubClient()

	p := poller.New(store, client,
		poller.WithPollInterval(20*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	p.Stop()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("unexpected error on stop: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("poller failed to stop in time")
	}
}

func TestPoller_RealDBAndHTTPClient(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)
	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "org",
		Name:         "project",
		BaseRef:      "main",
		PollInterval: "1m",
		ConfigPath:   "pr-triage.yml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	prsResp := []map[string]any{
		{
			"number": 12,
			"head": map[string]any{
				"sha": "sha-real-integration",
			},
			"base": map[string]any{
				"ref": "main",
			},
		},
	}

	checkRunsResp := map[string]any{
		"total_count": 2,
		"check_runs": []map[string]any{
			{
				"id":         4444,
				"name":       "ci/test",
				"head_sha":   "sha-real-integration",
				"status":     "completed",
				"conclusion": "success",
			},
			{
				"id":         5555,
				"name":       "pr-prescan-report",
				"head_sha":   "sha-real-integration",
				"status":     "completed",
				"conclusion": "success",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/org/project/pulls":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(prsResp)
		case "/repos/org/project/commits/sha-real-integration/check-runs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(checkRunsResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ghClient := internalgh.NewClientWithBaseURL("test-token", srv.URL)

	p := poller.New(store, ghClient,
		poller.WithInitialBackoff(10*time.Millisecond),
		poller.WithTimeoutCeiling(1*time.Second),
		poller.WithSleepFunc(func(ctx context.Context, d time.Duration) error { return nil }),
	)

	ctx := context.Background()
	if err := p.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	prState, err := store.GetPRState(repo.ID, 12)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if prState.State != poller.StateReportReady {
		t.Errorf("prState.State = %q, want %q", prState.State, poller.StateReportReady)
	}
	if prState.HeadSHA != "sha-real-integration" {
		t.Errorf("prState.HeadSHA = %q, want sha-real-integration", prState.HeadSHA)
	}
	if prState.LastRunID == nil || *prState.LastRunID != 5555 {
		t.Errorf("prState.LastRunID = %v, want 5555", prState.LastRunID)
	}

	select {
	case evt := <-p.ReportReadyEvents():
		if evt.PRNumber != 12 || evt.HeadSHA != "sha-real-integration" || evt.CheckRunID != 5555 {
			t.Errorf("unexpected event: %+v", evt)
		}
	default:
		t.Fatal("expected ReportReadyEvent, got none")
	}
}
