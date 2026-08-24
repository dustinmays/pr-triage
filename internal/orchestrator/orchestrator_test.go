package orchestrator_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

type mockGHClient struct {
	mu           sync.Mutex
	outputs      map[int64]*gh.CheckRunOutput
	prs          map[int]*gh.PullRequest
	addedLabels  []string
	createdPosts []string
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
		HeadSHA:    "sha-valid-42",
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

	eventCh := make(chan poller.ReportReadyEvent, 2)
	eventCh <- poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   101,
		HeadSHA:    "sha-101",
		CheckRunID: 301,
	}
	eventCh <- poller.ReportReadyEvent{
		Repo:       *repo,
		PRNumber:   102,
		HeadSHA:    "sha-102",
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
}

// TestUnusedImports suppresses unused compiler warnings.
func TestUnusedImports(t *testing.T) {
	_ = json.Marshal
}
