package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gh "github.com/google/go-github/v72/github"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/escalate"
	"github.com/dustinmays/pr-triage/internal/orchestrator"
	"github.com/dustinmays/pr-triage/internal/poller"
	"github.com/dustinmays/pr-triage/internal/runtime"
	_ "github.com/dustinmays/pr-triage/internal/runtime/claudecode"
)

// testHeadSHA returns the real HEAD SHA of the repo containing these tests.
// Orchestrator runs create a git worktree at the event's head SHA, so tests
// that reach worktree creation must use a reference that resolves; fake SHAs
// would now hard-fail to escalation instead of being silently swallowed.
func testHeadSHA(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

type mockGHClient struct {
	mu           sync.Mutex
	outputs      map[int64]*gh.CheckRunOutput
	prs          map[int]*gh.PullRequest
	addedLabels  []string
	createdPosts []string
	fetchErr     error
}

func newMockGHClient() *mockGHClient {
	return &mockGHClient{
		outputs: make(map[int64]*gh.CheckRunOutput),
		prs:     make(map[int]*gh.PullRequest),
	}
}

func (m *mockGHClient) FetchCheckRunOutput(ctx context.Context, owner, repo string, checkRunID int64) (*gh.CheckRunOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return m.outputs[checkRunID], nil
}

func (m *mockGHClient) GetPR(ctx context.Context, owner, repo string, number int) (*gh.PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.prs[number], nil
}

func (m *mockGHClient) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addedLabels = append(m.addedLabels, labels...)
	return nil
}

func (m *mockGHClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdPosts = append(m.createdPosts, body)
	return int64(len(m.createdPosts)), nil
}

type fakeMockRuntime struct {
	name       string
	activeRuns int
	maxActive  int
	mu         sync.Mutex
}

func (f *fakeMockRuntime) Name() string { return f.name }
func (f *fakeMockRuntime) Run(ctx context.Context, inv runtime.Invocation, logFile io.Writer) (int, error) {
	f.mu.Lock()
	f.activeRuns++
	if f.activeRuns > f.maxActive {
		f.maxActive = f.activeRuns
	}
	f.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	f.mu.Lock()
	f.activeRuns--
	f.mu.Unlock()

	_, _ = io.WriteString(logFile, `{"total_cost_usd": 0.05, "num_turns": 3, "stop_reason": "completed"}`+"\n")
	return 0, nil
}

func (f *fakeMockRuntime) ParseResult(log io.Reader) (*runtime.Result, error) {
	return &runtime.Result{
		Cost:       0.05,
		CostBasis:  runtime.CostBasisExact,
		Turns:      3,
		StopReason: "completed",
		Summary:    "Automated review: all checks pass. LGTM.",
	}, nil
}

func (f *fakeMockRuntime) ClassifyOutcome(res *runtime.Result, exitCode int) runtime.Outcome {
	return runtime.OutcomeSuccess
}

var (
	registerFakeOnce sync.Once
	mockRT           = &fakeMockRuntime{name: "fake-mock-rt"}
)

func initFakeRuntime() {
	registerFakeOnce.Do(func() {
		runtime.Register(mockRT)
	})
}

// failingMockRuntime simulates a runtime adapter that cannot execute at all
// (e.g. the runtime binary is missing, or the model was rejected before launch).
type failingMockRuntime struct {
	name string
}

func (f *failingMockRuntime) Name() string { return f.name }

func (f *failingMockRuntime) Run(ctx context.Context, inv runtime.Invocation, logFile io.Writer) (int, error) {
	_, _ = io.WriteString(logFile, "boom: runtime not available\n")
	return -1, errors.New("boom: runtime not available")
}

func (f *failingMockRuntime) ParseResult(log io.Reader) (*runtime.Result, error) {
	return nil, errors.New("no result payload in log")
}

func (f *failingMockRuntime) ClassifyOutcome(res *runtime.Result, exitCode int) runtime.Outcome {
	return runtime.OutcomeFailed
}

var (
	registerFailingOnce sync.Once
	failRT              = &failingMockRuntime{name: "fail-mock-rt"}
)

func initFailingRuntime() {
	registerFailingOnce.Do(func() {
		runtime.Register(failRT)
	})
}

func TestOrchestrator_HandleReportReady_ValidAndDone(t *testing.T) {
	initFakeRuntime()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)

	// Create custom config mapping to fake-mock-rt
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
signal_tiers:
  default_tier: routine
  rules: []
routing:
  routine:
    runtime: fake-mock-rt
    model: test-model
    agent_def: test-agent
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   cfgPath,
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	validReport, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "valid.json"))
	if err != nil {
		t.Fatalf("read valid.json: %v", err)
	}

	ghMock := newMockGHClient()
	ghMock.outputs[100] = &gh.CheckRunOutput{
		Title:   gh.Ptr("CI Report"),
		Summary: gh.Ptr(string(validReport)),
	}

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator,
		orchestrator.WithWorktreeDir(filepath.Join(tmpDir, "worktrees")),
		orchestrator.WithLogDir(filepath.Join(tmpDir, "logs")),
	)

	ctx := context.Background()
	event := poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   42,
		HeadSHA:    testHeadSHA(t),
		CheckRunID: 100,
	}

	if err := orch.HandleReportReady(ctx, event); err != nil {
		t.Fatalf("HandleReportReady failed: %v", err)
	}

	// Verify PR state is done
	pr, err := store.GetPRState(repo.ID, 42)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "done" {
		t.Errorf("pr.State = %q, want 'done'", pr.State)
	}

	// Verify run row
	runs, err := store.RunsInState("done")
	if err != nil {
		t.Fatalf("RunsInState failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 done run, got %d", len(runs))
	}
	if runs[0].CostUSD != 0.05 || runs[0].CostBasis != "exact" || runs[0].Turns != 3 {
		t.Errorf("unexpected run metrics: %+v", runs[0])
	}

	// The orchestrator must deterministically post the agent's review summary to
	// the PR — delivery must not depend on the agent running gh itself.
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 review comment posted, got %d", len(ghMock.createdPosts))
	}
	posted := ghMock.createdPosts[0]
	if !strings.Contains(posted, "<!-- pr-triage:review -->") {
		t.Errorf("review comment missing marker: %q", posted)
	}
	if !strings.Contains(posted, "Automated review: all checks pass. LGTM.") {
		t.Errorf("review comment missing agent summary: %q", posted)
	}
}

func TestOrchestrator_HandleReportReady_RunFailureEscalates(t *testing.T) {
	initFailingRuntime()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)

	// Create custom config mapping to fail-mock-rt, a runtime that cannot execute.
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
signal_tiers:
  default_tier: routine
  rules: []
routing:
  routine:
    runtime: fail-mock-rt
    model: test-model
    agent_def: test-agent
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   cfgPath,
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	validReport, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "valid.json"))
	if err != nil {
		t.Fatalf("read valid.json: %v", err)
	}

	ghMock := newMockGHClient()
	ghMock.outputs[100] = &gh.CheckRunOutput{
		Title:   gh.Ptr("CI Report"),
		Summary: gh.Ptr(string(validReport)),
	}

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator,
		orchestrator.WithWorktreeDir(filepath.Join(tmpDir, "worktrees")),
		orchestrator.WithLogDir(filepath.Join(tmpDir, "logs")),
	)

	ctx := context.Background()
	event := poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   42,
		HeadSHA:    testHeadSHA(t),
		CheckRunID: 100,
	}

	// Escalation is a normal, handled outcome — the function must not surface an error.
	if err := orch.HandleReportReady(ctx, event); err != nil {
		t.Fatalf("HandleReportReady failed: %v", err)
	}

	// Verify escalation: label + comment, asserted the same way as the
	// malformed-report escalation test.
	if len(ghMock.addedLabels) != 1 || ghMock.addedLabels[0] != escalate.DefaultEscalationLabel {
		t.Errorf("expected escalation label applied, got %v", ghMock.addedLabels)
	}
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 escalation comment, got %d", len(ghMock.createdPosts))
	}
	body := ghMock.createdPosts[0]
	for _, want := range []string{"fail-mock-rt", "boom: runtime not available"} {
		if !strings.Contains(body, want) {
			t.Errorf("escalation comment missing %q; got:\n%s", want, body)
		}
	}

	// The escalation is the last writer of PR state, so it ends "escalated"
	// (same as the other hard-fail escalation tests) — but the agent run row
	// itself must be recorded as failed with the real stop reason.
	pr, err := store.GetPRState(repo.ID, 42)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated'", pr.State)
	}

	failedRuns, err := store.RunsInState("failed")
	if err != nil {
		t.Fatalf("RunsInState failed: %v", err)
	}
	if len(failedRuns) != 1 {
		t.Fatalf("expected 1 failed run, got %d", len(failedRuns))
	}
	if !strings.Contains(failedRuns[0].StopReason, "failed to execute") {
		t.Errorf("run stop_reason = %q, want it to name the runtime execution failure", failedRuns[0].StopReason)
	}
}

func TestOrchestrator_HandleReportReady_MalformedEscalates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)
	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   "nonexistent.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	malformedReport, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "malformed.json"))

	ghMock := newMockGHClient()
	ghMock.outputs[200] = &gh.CheckRunOutput{
		Title:   gh.Ptr("Malformed Report"),
		Summary: gh.Ptr(string(malformedReport)),
	}

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator)

	ctx := context.Background()
	event := poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   50,
		HeadSHA:    "sha-malformed",
		CheckRunID: 200,
	}

	if err := orch.HandleReportReady(ctx, event); err != nil {
		t.Fatalf("HandleReportReady failed: %v", err)
	}

	// Verify PR state is escalated
	pr, err := store.GetPRState(repo.ID, 50)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated'", pr.State)
	}

	if len(ghMock.addedLabels) != 1 || ghMock.addedLabels[0] != escalate.DefaultEscalationLabel {
		t.Errorf("expected escalation label applied, got %v", ghMock.addedLabels)
	}
}

func TestOrchestrator_ConcurrencyBoundedAtOne(t *testing.T) {
	initFakeRuntime()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
signal_tiers:
  default_tier: routine
routing:
  routine:
    runtime: fake-mock-rt
    model: test-model
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	repo, _ := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   cfgPath,
	})

	validReport, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "valid.json"))

	ghMock := newMockGHClient()
	ghMock.outputs[301] = &gh.CheckRunOutput{Summary: gh.Ptr(string(validReport))}
	ghMock.outputs[302] = &gh.CheckRunOutput{Summary: gh.Ptr(string(validReport))}

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator,
		orchestrator.WithConcurrency(1),
		orchestrator.WithWorktreeDir(filepath.Join(tmpDir, "worktrees")),
		orchestrator.WithLogDir(filepath.Join(tmpDir, "logs")),
	)

	headSHA := testHeadSHA(t)
	eventCh := make(chan poller.ReportReadyEvent, 2)
	eventCh <- poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   101,
		HeadSHA:    headSHA,
		CheckRunID: 301,
	}
	eventCh <- poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   102,
		HeadSHA:    headSHA,
		CheckRunID: 302,
	}
	close(eventCh)

	ctx := context.Background()
	_ = orch.Start(ctx, eventCh)

	mockRT.mu.Lock()
	maxActive := mockRT.maxActive
	mockRT.mu.Unlock()

	if maxActive > 1 {
		t.Errorf("max active concurrent agent runs was %d, want <= 1", maxActive)
	}

	pr1, _ := store.GetPRState(repo.ID, 101)
	pr2, _ := store.GetPRState(repo.ID, 102)
	if pr1.State != "done" || pr2.State != "done" {
		t.Errorf("pr1=%s, pr2=%s, both should be done", pr1.State, pr2.State)
	}
}

func TestOrchestrator_Start_HandleReportReadyError_ReportedViaOnError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)

	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "Bellese",
		Name:         "orgz-seed-template",
		BaseRef:      "chunk/**",
		PollInterval: "5m",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	// Simulate a transient GitHub API failure fetching the check run output -
	// the same "other transient/stale errors -> return for retry" path that
	// used to vanish silently into orchestrator.Start's `continue`.
	ghMock := newMockGHClient()
	ghMock.fetchErr = errors.New("simulated transient GitHub API failure")

	escalator := escalate.New(store, ghMock)

	var mu sync.Mutex
	var gotEvent *poller.ReportReadyEvent
	var gotErr error

	orch := orchestrator.New(store, ghMock, escalator,
		orchestrator.WithWorktreeDir(filepath.Join(tmpDir, "worktrees")),
		orchestrator.WithLogDir(filepath.Join(tmpDir, "logs")),
		orchestrator.WithOnError(func(evt *poller.ReportReadyEvent, err error) {
			mu.Lock()
			defer mu.Unlock()
			gotEvent = evt
			gotErr = err
		}),
	)

	eventCh := make(chan poller.ReportReadyEvent, 1)
	eventCh <- poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   90,
		HeadSHA:    testHeadSHA(t),
		CheckRunID: 999,
	}
	close(eventCh)

	if err := orch.Start(context.Background(), eventCh); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotEvent == nil || gotEvent.PRNumber != 90 {
		t.Fatalf("OnError event = %+v, want PRNumber 90", gotEvent)
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "simulated transient GitHub API failure") {
		t.Errorf("OnError err = %v, want it to contain the underlying fetch error", gotErr)
	}
}

func TestOrchestrator_Recover_StrandedRuns(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)
	repo, _ := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   "test.yaml",
	})

	pr, _ := store.UpsertPRState(repo.ID, 999, "sha-crash", nil, "agent_running")

	fakeWorktreeDir := filepath.Join(tmpDir, "stale-worktree")
	_ = os.MkdirAll(fakeWorktreeDir, 0755)

	run := &db.Run{
		PRID:         pr.ID,
		HeadSHA:      "sha-crash",
		RiskTier:     "routine",
		Runtime:      "fake-mock-rt",
		Model:        "test-model",
		ModelSource:  "routing",
		CostUSD:      0.0,
		CostBasis:    "unavailable",
		Status:       "agent_running",
		WorktreePath: fakeWorktreeDir,
	}
	recordedRun, err := store.RecordRun(run)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	ghMock := newMockGHClient()
	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator)

	ctx := context.Background()
	if err := orch.Recover(ctx); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	runs, err := store.RunsInState("agent_running")
	if err != nil {
		t.Fatalf("RunsInState failed: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 agent_running runs after recovery, got %d", len(runs))
	}

	failedRuns, err := store.RunsInState("failed")
	if err != nil {
		t.Fatalf("RunsInState failed: %v", err)
	}
	if len(failedRuns) != 1 || failedRuns[0].ID != recordedRun.ID {
		t.Fatalf("expected 1 failed recovered run, got %v", failedRuns)
	}
	if failedRuns[0].StopReason != "interrupted by daemon crash/restart" {
		t.Errorf("stop_reason = %q, want 'interrupted by daemon crash/restart'", failedRuns[0].StopReason)
	}
}

func TestOrchestrator_HandleReportReady_HighRiskEscalates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)
	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   "nonexistent.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	highRiskReport, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "high-risk.json"))

	ghMock := newMockGHClient()
	ghMock.outputs[400] = &gh.CheckRunOutput{
		Title:   gh.Ptr("High Risk Report"),
		Summary: gh.Ptr(string(highRiskReport)),
	}

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator)

	ctx := context.Background()
	event := poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   43,
		HeadSHA:    "sha-highrisk",
		CheckRunID: 400,
	}

	if err := orch.HandleReportReady(ctx, event); err != nil {
		t.Fatalf("HandleReportReady failed: %v", err)
	}

	// Verify PR state is escalated
	pr, err := store.GetPRState(repo.ID, 43)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated'", pr.State)
	}

	// D.2: the escalation must name the specific signal(s) that tripped and cite
	// their evidence, not just the tier.
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 escalation comment, got %d", len(ghMock.createdPosts))
	}
	body := ghMock.createdPosts[0]
	for _, want := range []string{"schema_changed_without_migration", "test_files_deleted", "src/schema.ts:42"} {
		if !strings.Contains(body, want) {
			t.Errorf("escalation comment missing %q; got:\n%s", want, body)
		}
	}
	if strings.Contains(body, `risk tier "escalate" triggered escalation`) {
		t.Errorf("escalation comment still uses the generic tier-only reason:\n%s", body)
	}
}

func TestOrchestrator_HandleReportReady_ChunkCompletionEscalates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)
	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   "nonexistent.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	chunkReport, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "chunk-completion.json"))

	ghMock := newMockGHClient()
	ghMock.outputs[500] = &gh.CheckRunOutput{
		Title:   gh.Ptr("Chunk Completion Report"),
		Summary: gh.Ptr(string(chunkReport)),
	}

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator)

	ctx := context.Background()
	event := poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   83,
		HeadSHA:    "sha-chunkcomp",
		CheckRunID: 500,
	}

	if err := orch.HandleReportReady(ctx, event); err != nil {
		t.Fatalf("HandleReportReady failed: %v", err)
	}

	// Verify PR state is escalated
	pr, err := store.GetPRState(repo.ID, 83)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated'", pr.State)
	}

	// D.2: a chunk-completion escalation should explain it was the target_kind,
	// not leave the human guessing.
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 escalation comment, got %d", len(ghMock.createdPosts))
	}
	body := ghMock.createdPosts[0]
	for _, want := range []string{"target_kind", "chunk_completion"} {
		if !strings.Contains(body, want) {
			t.Errorf("escalation comment missing %q; got:\n%s", want, body)
		}
	}
}

// overrideTestConfig keeps the default signal_tiers (all escalate signals) but
// points the routine tier at the in-test fake runtime so a fully-waived PR
// actually runs the agent instead of shelling out to claude.
const overrideTestConfig = `
routing:
  routine:
    runtime: fake-mock-rt
    model: test-model
    agent_def: test-agent
`

func TestOrchestrator_Override_FullWaiver_RunsAgent(t *testing.T) {
	initFakeRuntime()

	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() { _ = database.Close() }()
	store := db.NewStore(database)

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(overrideTestConfig), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repo, err := store.UpsertRepo(&db.Repo{
		Owner: "dustinmays", Name: "pr-triage", BaseRef: "main", PollInterval: "5m", ConfigPath: cfgPath,
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	const prNum = 94
	headSHA := testHeadSHA(t)
	// The PR must exist in state so the override can be recorded/consulted.
	if _, err := store.UpsertPRState(repo.ID, prNum, headSHA, nil, "escalated"); err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}
	// Owner waives all escalate-tier signals for this head SHA.
	if _, err := store.RecordOverride(&db.Override{RepoID: repo.ID, PRNumber: prNum, HeadSHA: headSHA}); err != nil {
		t.Fatalf("RecordOverride: %v", err)
	}

	highRisk, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "high-risk.json"))
	ghMock := newMockGHClient()
	ghMock.outputs[400] = &gh.CheckRunOutput{Summary: gh.Ptr(string(highRisk))}

	orch := orchestrator.New(store, ghMock, escalate.New(store, ghMock),
		orchestrator.WithWorktreeDir(filepath.Join(tmpDir, "wt")),
		orchestrator.WithLogDir(filepath.Join(tmpDir, "logs")),
	)

	event := poller.ReportReadyEvent{Repo: *repo, PRNumber: prNum, HeadSHA: headSHA, CheckRunID: 400}
	if err := orch.HandleReportReady(context.Background(), event); err != nil {
		t.Fatalf("HandleReportReady: %v", err)
	}

	pr, err := store.GetPRState(repo.ID, prNum)
	if err != nil {
		t.Fatalf("GetPRState: %v", err)
	}
	if pr.State != "done" {
		t.Errorf("pr.State = %q, want 'done' (override should route to the agent)", pr.State)
	}
	if len(ghMock.addedLabels) != 0 {
		t.Errorf("expected no escalation label with a full override, got %v", ghMock.addedLabels)
	}
	// The override must be consumed (one-shot).
	if _, err := store.GetActiveOverride(repo.ID, prNum, headSHA); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("expected override consumed, got %v", err)
	}
}

func TestOrchestrator_Override_PartialWaiver_StillEscalates(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() { _ = database.Close() }()
	store := db.NewStore(database)

	repo, err := store.UpsertRepo(&db.Repo{
		Owner: "dustinmays", Name: "pr-triage", BaseRef: "main", PollInterval: "5m", ConfigPath: "nonexistent.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	const prNum = 95
	const headSHA = "sha-highrisk"
	if _, err := store.UpsertPRState(repo.ID, prNum, headSHA, nil, "ci_passed"); err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}
	// Waive only one of the two present escalate signals.
	if _, err := store.RecordOverride(&db.Override{
		RepoID: repo.ID, PRNumber: prNum, HeadSHA: headSHA, WaivedSignals: "schema_changed_without_migration",
	}); err != nil {
		t.Fatalf("RecordOverride: %v", err)
	}

	highRisk, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "high-risk.json"))
	ghMock := newMockGHClient()
	ghMock.outputs[400] = &gh.CheckRunOutput{Summary: gh.Ptr(string(highRisk))}

	orch := orchestrator.New(store, ghMock, escalate.New(store, ghMock))
	event := poller.ReportReadyEvent{Repo: *repo, PRNumber: prNum, HeadSHA: headSHA, CheckRunID: 400}
	if err := orch.HandleReportReady(context.Background(), event); err != nil {
		t.Fatalf("HandleReportReady: %v", err)
	}

	pr, err := store.GetPRState(repo.ID, prNum)
	if err != nil {
		t.Fatalf("GetPRState: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated' (a remaining signal must still escalate)", pr.State)
	}
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 escalation comment, got %d", len(ghMock.createdPosts))
	}
	body := ghMock.createdPosts[0]
	if !strings.Contains(body, "test_files_deleted") {
		t.Errorf("escalation should still name the remaining signal test_files_deleted; got:\n%s", body)
	}
	if !strings.Contains(body, "schema_changed_without_migration") {
		t.Errorf("escalation should note the waived signal; got:\n%s", body)
	}
	// A partial override is not consumed — it stays active for the remaining decision.
	if _, err := store.GetActiveOverride(repo.ID, prNum, headSHA); err != nil {
		t.Errorf("partial override should remain active, got %v", err)
	}
}

func TestHandleReportReady_ReportMissing_Escalates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := db.NewStore(database)
	repo, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
		ConfigPath:   "nonexistent.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	ghMock := newMockGHClient()
	// ghMock has no check run outputs configured, asserting that report fetch is not needed.

	escalator := escalate.New(store, ghMock)
	orch := orchestrator.New(store, ghMock, escalator)

	ctx := context.Background()
	event := poller.ReportReadyEvent{
		Repo:          *repo,
		PRNumber:      77,
		HeadSHA:       "sha-missing-report",
		CheckRunID:    0,
		ReportMissing: true,
	}

	if err := orch.HandleReportReady(ctx, event); err != nil {
		t.Fatalf("HandleReportReady failed: %v", err)
	}

	// Verify PR state is escalated
	pr, err := store.GetPRState(repo.ID, 77)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated'", pr.State)
	}

	if len(ghMock.addedLabels) != 1 || ghMock.addedLabels[0] != escalate.DefaultEscalationLabel {
		t.Errorf("expected escalation label applied, got %v", ghMock.addedLabels)
	}

	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 escalation comment, got %d", len(ghMock.createdPosts))
	}
	if !strings.Contains(ghMock.createdPosts[0], "never appeared within the wait ceiling") {
		t.Errorf("escalation comment missing expected reason; got:\n%s", ghMock.createdPosts[0])
	}
}

// TestUnusedImports suppresses unused compiler warnings.
func TestUnusedImports(t *testing.T) {
	_ = json.Marshal
}
