package escalate_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/escalate"
)

type mockGitHubClient struct {
	mu           sync.Mutex
	addedLabels  []string
	createdPosts []string
}

func (m *mockGitHubClient) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addedLabels = append(m.addedLabels, labels...)
	return nil
}

func (m *mockGitHubClient) CreateComment(ctx context.Context, owner, repo string, number int, body string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdPosts = append(m.createdPosts, body)
	return int64(len(m.createdPosts)), nil
}

func TestEscalator_Escalate_IdempotencyAndPersistence(t *testing.T) {
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
		ConfigPath:   "pr-triage.yml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	ghMock := &mockGitHubClient{}
	escalator := escalate.New(store, ghMock)
	ctx := context.Background()

	ciRunID := int64(9876)
	req := escalate.Request{
		Repo:       *repo,
		PRNumber:   42,
		HeadSHA:    "sha-malformed-report",
		Reason:     "malformed report: missing required field 'signals'",
		CIRunID:    &ciRunID,
		GitHubUser: "dustinmays",
	}

	// 1. Initial escalation
	if err := escalator.Escalate(ctx, req); err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}

	if len(ghMock.addedLabels) != 1 || ghMock.addedLabels[0] != escalate.DefaultEscalationLabel {
		t.Errorf("expected label %q, got %v", escalate.DefaultEscalationLabel, ghMock.addedLabels)
	}
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(ghMock.createdPosts))
	}
	expectedCommentSubstr := "@dustinmays"
	if !containsSubstring(ghMock.createdPosts[0], expectedCommentSubstr) {
		t.Errorf("comment %q should contain %q", ghMock.createdPosts[0], expectedCommentSubstr)
	}

	// Verify DB state
	pr, err := store.GetPRState(repo.ID, 42)
	if err != nil {
		t.Fatalf("GetPRState failed: %v", err)
	}
	if pr.State != "escalated" {
		t.Errorf("pr.State = %q, want 'escalated'", pr.State)
	}

	runs, err := store.RunsInState("escalated")
	if err != nil {
		t.Fatalf("RunsInState failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 escalated run, got %d", len(runs))
	}
	if runs[0].StopReason != req.Reason {
		t.Errorf("run.StopReason = %q, want %q", runs[0].StopReason, req.Reason)
	}

	// 2. Repeat escalation (Idempotency test)
	if err := escalator.Escalate(ctx, req); err != nil {
		t.Fatalf("Repeat Escalate failed: %v", err)
	}

	if len(ghMock.addedLabels) != 1 {
		t.Errorf("expected no duplicate labels on repeat, got %d labels", len(ghMock.addedLabels))
	}
	if len(ghMock.createdPosts) != 1 {
		t.Errorf("expected no duplicate comments on repeat, got %d comments", len(ghMock.createdPosts))
	}
}

func TestEscalator_CustomLabelAndNoUser(t *testing.T) {
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
		ConfigPath:   "pr-triage.yml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo failed: %v", err)
	}

	ghMock := &mockGitHubClient{}
	escalator := escalate.New(store, ghMock)
	ctx := context.Background()

	req := escalate.Request{
		Repo:     *repo,
		PRNumber: 100,
		HeadSHA:  "sha-unknown-version",
		Reason:   "unsupported schema version: 99",
		Label:    "needs-human-review",
	}

	if err := escalator.Escalate(ctx, req); err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}

	if len(ghMock.addedLabels) != 1 || ghMock.addedLabels[0] != "needs-human-review" {
		t.Errorf("expected custom label, got %v", ghMock.addedLabels)
	}
	if len(ghMock.createdPosts) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(ghMock.createdPosts))
	}
	if containsSubstring(ghMock.createdPosts[0], "@") {
		t.Errorf("comment should not contain @ mention when user is empty: %q", ghMock.createdPosts[0])
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
