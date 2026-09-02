package db

import (
	"errors"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewStore(conn)
}

func TestUpsertRepo_CreateAndUpdate(t *testing.T) {
	store := setupTestDB(t)

	// Nil check
	if _, err := store.UpsertRepo(nil); err == nil {
		t.Fatal("expected error when upserting nil repo, got nil")
	}

	// 1. Initial insert
	repo1 := &Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	}

	created, err := store.UpsertRepo(repo1)
	if err != nil {
		t.Fatalf("UpsertRepo (create) error = %v", err)
	}
	if created.ID <= 0 {
		t.Errorf("created.ID = %d, want > 0", created.ID)
	}
	if created.Owner != repo1.Owner || created.Name != repo1.Name {
		t.Errorf("created repo = %s/%s, want %s/%s", created.Owner, created.Name, repo1.Owner, repo1.Name)
	}
	if created.BaseRef != "main" {
		t.Errorf("created.BaseRef = %q, want %q", created.BaseRef, "main")
	}
	if created.CreatedAt == "" {
		t.Error("expected non-empty created_at")
	}

	// 2. Re-running with modified fields must update in-place without duplicating
	repoUpdate := &Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "epic/pr-triage-poc",
		PollInterval: "30s",
		ConfigPath:   "custom-config.yaml",
	}

	updated, err := store.UpsertRepo(repoUpdate)
	if err != nil {
		t.Fatalf("UpsertRepo (update) error = %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("updated.ID = %d, want %d (should not change ID)", updated.ID, created.ID)
	}
	if updated.BaseRef != "epic/pr-triage-poc" {
		t.Errorf("updated.BaseRef = %q, want %q", updated.BaseRef, "epic/pr-triage-poc")
	}
	if updated.PollInterval != "30s" {
		t.Errorf("updated.PollInterval = %q, want %q", updated.PollInterval, "30s")
	}
	if updated.ConfigPath != "custom-config.yaml" {
		t.Errorf("updated.ConfigPath = %q, want %q", updated.ConfigPath, "custom-config.yaml")
	}
	if updated.CreatedAt != created.CreatedAt {
		t.Errorf("updated.CreatedAt = %q, want %q (created_at should be preserved)", updated.CreatedAt, created.CreatedAt)
	}

	// Verify count is 1
	repos, err := store.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos error = %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
}

func TestListRepos(t *testing.T) {
	store := setupTestDB(t)

	// Empty store returns empty slice (not nil)
	repos, err := store.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos on empty DB error = %v", err)
	}
	if repos == nil || len(repos) != 0 {
		t.Errorf("ListRepos on empty DB = %v, want empty slice", repos)
	}

	// Add 2 repos
	_, err = store.UpsertRepo(&Repo{
		Owner:        "owner-a",
		Name:         "repo-a",
		BaseRef:      "main",
		PollInterval: "1m",
		ConfigPath:   "a.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo repo-a: %v", err)
	}

	_, err = store.UpsertRepo(&Repo{
		Owner:        "owner-b",
		Name:         "repo-b",
		BaseRef:      "develop",
		PollInterval: "2m",
		ConfigPath:   "b.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo repo-b: %v", err)
	}

	repos, err = store.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("len(repos) = %d, want 2", len(repos))
	}
	if repos[0].Name != "repo-a" || repos[1].Name != "repo-b" {
		t.Errorf("repos order = [%s, %s], want [repo-a, repo-b]", repos[0].Name, repos[1].Name)
	}
}

func TestUpsertPRState_Idempotent(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	// 1. Initial PR state insert
	pr, err := store.UpsertPRState(repo.ID, 42, "sha-1", nil, "ci_running")
	if err != nil {
		t.Fatalf("UpsertPRState error = %v", err)
	}
	if pr.ID <= 0 {
		t.Errorf("pr.ID = %d, want > 0", pr.ID)
	}
	if pr.RepoID != repo.ID || pr.Number != 42 || pr.HeadSHA != "sha-1" || pr.State != "ci_running" {
		t.Errorf("pr state mismatch: %+v", pr)
	}
	if pr.LastRunID != nil {
		t.Errorf("pr.LastRunID = %v, want nil", pr.LastRunID)
	}

	// 2. Second upsert on same (repo_id, number) updates in-place
	runID := int64(10)
	prUpdated, err := store.UpsertPRState(repo.ID, 42, "sha-2", &runID, "agent_running")
	if err != nil {
		t.Fatalf("UpsertPRState update error = %v", err)
	}
	if prUpdated.ID != pr.ID {
		t.Errorf("prUpdated.ID = %d, want %d (should retain same ID)", prUpdated.ID, pr.ID)
	}
	if prUpdated.HeadSHA != "sha-2" {
		t.Errorf("prUpdated.HeadSHA = %q, want %q", prUpdated.HeadSHA, "sha-2")
	}
	if prUpdated.LastRunID == nil || *prUpdated.LastRunID != runID {
		t.Errorf("prUpdated.LastRunID = %v, want %d", prUpdated.LastRunID, runID)
	}
	if prUpdated.State != "agent_running" {
		t.Errorf("prUpdated.State = %q, want %q", prUpdated.State, "agent_running")
	}

	// 3. Foreign key violation when repoID is invalid
	if _, err := store.UpsertPRState(99999, 1, "sha-3", nil, "idle"); err == nil {
		t.Fatal("expected foreign key error on non-existent repoID, got nil")
	}
}

func TestGetPRState(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	// Non-existent PR returns ErrNotFound
	_, err = store.GetPRState(repo.ID, 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPRState non-existent: err = %v, want ErrNotFound", err)
	}

	// Insert PR and retrieve
	created, err := store.UpsertPRState(repo.ID, 101, "head-sha-101", nil, "report_ready")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	fetched, err := store.GetPRState(repo.ID, 101)
	if err != nil {
		t.Fatalf("GetPRState: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("fetched.ID = %d, want %d", fetched.ID, created.ID)
	}
	if fetched.Number != 101 {
		t.Errorf("fetched.Number = %d, want 101", fetched.Number)
	}
	if fetched.State != "report_ready" {
		t.Errorf("fetched.State = %q, want %q", fetched.State, "report_ready")
	}
}

func TestDeletePRState(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	// Deleting a PR that was never tracked returns ErrNotFound.
	if err := store.DeletePRState(repo.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeletePRState non-existent: err = %v, want ErrNotFound", err)
	}

	if _, err := store.UpsertPRState(repo.ID, 101, "head-sha-101", nil, "ci_failed"); err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	if err := store.DeletePRState(repo.ID, 101); err != nil {
		t.Fatalf("DeletePRState: %v", err)
	}

	if _, err := store.GetPRState(repo.ID, 101); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPRState after delete: err = %v, want ErrNotFound", err)
	}

	// Re-upserting after a delete behaves like a brand-new PR.
	recreated, err := store.UpsertPRState(repo.ID, 101, "head-sha-101", nil, "ci_running")
	if err != nil {
		t.Fatalf("UpsertPRState after delete: %v", err)
	}
	if recreated.State != "ci_running" {
		t.Errorf("recreated.State = %q, want %q", recreated.State, "ci_running")
	}
}

func TestListRuns_PopulatesGitHubPRNumber(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "5m",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	// Use a GitHub PR number that is deliberately different from the internal
	// pr_id (autoincrement starting at 1) so a regression to pr_id is caught.
	const githubNumber = 94
	pr, err := store.UpsertPRState(repo.ID, githubNumber, "sha-1", nil, "escalated")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}
	if pr.ID == githubNumber {
		t.Fatalf("test precondition: internal pr_id (%d) must differ from GitHub number (%d)", pr.ID, githubNumber)
	}

	if _, err := store.RecordRun(&Run{
		PRID:      pr.ID,
		HeadSHA:   "sha-1",
		RiskTier:  "escalated",
		Runtime:   "none",
		Model:     "none",
		CostBasis: "unavailable",
		Status:    "escalated",
	}); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	runs, err := store.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].PRNumber != githubNumber {
		t.Errorf("run.PRNumber = %d, want the GitHub PR number %d (not internal pr_id %d)",
			runs[0].PRNumber, githubNumber, runs[0].PRID)
	}
}

func TestOverrides_RecordGetConsumeAndSHAPinning(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner: "dustinmays", Name: "pr-triage", BaseRef: "main", PollInterval: "5m",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	// No active override initially.
	if _, err := store.GetActiveOverride(repo.ID, 42, "sha-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	ov, err := store.RecordOverride(&Override{
		RepoID: repo.ID, PRNumber: 42, HeadSHA: "sha-a",
		WaivedSignals: "workflow_changed,safeguard_config_changed", Reason: "infra chunk",
	})
	if err != nil {
		t.Fatalf("RecordOverride: %v", err)
	}

	got, err := store.GetActiveOverride(repo.ID, 42, "sha-a")
	if err != nil {
		t.Fatalf("GetActiveOverride: %v", err)
	}
	if got.ID != ov.ID || len(got.WaivedSignalList()) != 2 || got.WaivesAll() {
		t.Errorf("unexpected override: %+v (waived=%v)", got, got.WaivedSignalList())
	}

	// SHA pinning: a different head SHA has no active override.
	if _, err := store.GetActiveOverride(repo.ID, 42, "sha-b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected no override for a new head SHA, got %v", err)
	}

	// Consuming makes it inactive (one-shot).
	if err := store.MarkOverrideConsumed(ov.ID); err != nil {
		t.Fatalf("MarkOverrideConsumed: %v", err)
	}
	if _, err := store.GetActiveOverride(repo.ID, 42, "sha-a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected consumed override to be inactive, got %v", err)
	}

	// An empty waiver list means "waive all".
	all, err := store.RecordOverride(&Override{RepoID: repo.ID, PRNumber: 43, HeadSHA: "sha-c"})
	if err != nil {
		t.Fatalf("RecordOverride(all): %v", err)
	}
	if !all.WaivesAll() {
		t.Errorf("expected WaivesAll for empty waiver, got %v", all.WaivedSignalList())
	}
}

func TestRecordRun_AndCostHonesty(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	pr, err := store.UpsertPRState(repo.ID, 1, "sha-1", nil, "idle")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	// Nil run check
	if _, err := store.RecordRun(nil); err == nil {
		t.Fatal("expected error on RecordRun(nil), got nil")
	}

	// Empty cost basis must be rejected
	if _, err := store.RecordRun(&Run{
		PRID:      pr.ID,
		HeadSHA:   "sha-1",
		RiskTier:  "low",
		Runtime:   "codex",
		Model:     "gpt-4o",
		CostUSD:   0.0,
		CostBasis: "", // empty!
		Status:    "running",
	}); err == nil {
		t.Fatal("expected error when cost_basis is empty, got nil")
	}

	// Valid run with CostBasis = "unavailable" and 0.0 cost
	runInput := &Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "low",
		Runtime:     "codex",
		Model:       "gpt-4o",
		ModelSource: "config",
		CostUSD:     0.0,
		CostBasis:   "unavailable",
		Turns:       2,
		Status:      "running",
		StopReason:  "",
		LogPath:     "/tmp/codex.log",
	}

	recorded, err := store.RecordRun(runInput)
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if recorded.ID <= 0 {
		t.Errorf("recorded.ID = %d, want > 0", recorded.ID)
	}
	if recorded.CostUSD != 0.0 || recorded.CostBasis != "unavailable" {
		t.Errorf("cost honesty check: got cost=%f basis=%q, want 0.0 and 'unavailable'", recorded.CostUSD, recorded.CostBasis)
	}

	// Confirm pr.last_run_id was updated
	updatedPR, err := store.GetPRState(repo.ID, 1)
	if err != nil {
		t.Fatalf("GetPRState: %v", err)
	}
	if updatedPR.LastRunID == nil || *updatedPR.LastRunID != recorded.ID {
		t.Errorf("updatedPR.LastRunID = %v, want %d", updatedPR.LastRunID, recorded.ID)
	}
}

func TestUpdateRun(t *testing.T) {
	store := setupTestDB(t)

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	pr, err := store.UpsertPRState(repo.ID, 1, "sha-1", nil, "idle")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	run, err := store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "high",
		Runtime:     "claude-code",
		Model:       "claude-3-7-sonnet",
		ModelSource: "table",
		CostUSD:     0.0,
		CostBasis:   "exact",
		Turns:       0,
		Status:      "agent_running",
	})
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	// Update fields
	finishedAt := "2026-08-23T15:00:00Z"
	pid := 4567
	run.CostUSD = 0.42
	run.Turns = 7
	run.Status = "done"
	run.StopReason = "end_turn"
	run.PID = &pid
	run.FinishedAt = &finishedAt

	if err := store.UpdateRun(run); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}

	// Fetch via ListRuns and confirm
	runs, err := store.ListRuns(1)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	r := runs[0]
	if r.CostUSD != 0.42 {
		t.Errorf("CostUSD = %f, want 0.42", r.CostUSD)
	}
	if r.Turns != 7 {
		t.Errorf("Turns = %d, want 7", r.Turns)
	}
	if r.Status != "done" {
		t.Errorf("Status = %q, want %q", r.Status, "done")
	}
	if r.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", r.StopReason, "end_turn")
	}
	if r.PID == nil || *r.PID != 4567 {
		t.Errorf("PID = %v, want 4567", r.PID)
	}
	if r.FinishedAt == nil || *r.FinishedAt != finishedAt {
		t.Errorf("FinishedAt = %v, want %s", r.FinishedAt, finishedAt)
	}

	// Update non-existent run ID must return ErrNotFound
	nonExistentRun := *run
	nonExistentRun.ID = 999999
	if err := store.UpdateRun(&nonExistentRun); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateRun non-existent: err = %v, want ErrNotFound", err)
	}
}

func TestListRuns_NewestFirstAndLimit(t *testing.T) {
	store := setupTestDB(t)

	// Empty store returns empty slice
	runs, err := store.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns empty DB error = %v", err)
	}
	if runs == nil || len(runs) != 0 {
		t.Errorf("ListRuns empty = %v, want empty slice", runs)
	}

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	pr, err := store.UpsertPRState(repo.ID, 1, "sha-1", nil, "idle")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	// Insert 3 runs sequentially
	run1, err := store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "low",
		Runtime:     "claude-code",
		Model:       "sonnet",
		ModelSource: "config",
		CostUSD:     0.01,
		CostBasis:   "exact",
		Status:      "done",
	})
	if err != nil {
		t.Fatalf("RecordRun 1: %v", err)
	}

	run2, err := store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "medium",
		Runtime:     "claude-code",
		Model:       "sonnet",
		ModelSource: "config",
		CostUSD:     0.02,
		CostBasis:   "exact",
		Status:      "done",
	})
	if err != nil {
		t.Fatalf("RecordRun 2: %v", err)
	}

	run3, err := store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "critical",
		Runtime:     "claude-code",
		Model:       "opus",
		ModelSource: "risk-table",
		CostUSD:     0.15,
		CostBasis:   "exact",
		Status:      "agent_running",
	})
	if err != nil {
		t.Fatalf("RecordRun 3: %v", err)
	}

	// ListRuns(0) or ListRuns(-1) should return all 3, newest first
	allRuns, err := store.ListRuns(0)
	if err != nil {
		t.Fatalf("ListRuns(0): %v", err)
	}
	if len(allRuns) != 3 {
		t.Fatalf("len(allRuns) = %d, want 3", len(allRuns))
	}
	if allRuns[0].ID != run3.ID || allRuns[1].ID != run2.ID || allRuns[2].ID != run1.ID {
		t.Errorf("allRuns order = [%d, %d, %d], want [%d, %d, %d] (newest first)",
			allRuns[0].ID, allRuns[1].ID, allRuns[2].ID, run3.ID, run2.ID, run1.ID)
	}

	// ListRuns with limit=2 should return newest 2
	limitedRuns, err := store.ListRuns(2)
	if err != nil {
		t.Fatalf("ListRuns(2): %v", err)
	}
	if len(limitedRuns) != 2 {
		t.Fatalf("len(limitedRuns) = %d, want 2", len(limitedRuns))
	}
	if limitedRuns[0].ID != run3.ID || limitedRuns[1].ID != run2.ID {
		t.Errorf("limitedRuns order = [%d, %d], want [%d, %d]", limitedRuns[0].ID, limitedRuns[1].ID, run3.ID, run2.ID)
	}
}

func TestRunsInState(t *testing.T) {
	store := setupTestDB(t)

	// Empty store returns empty slice
	runs, err := store.RunsInState("agent_running")
	if err != nil {
		t.Fatalf("RunsInState empty DB: %v", err)
	}
	if runs == nil || len(runs) != 0 {
		t.Errorf("RunsInState empty = %v, want empty slice", runs)
	}

	repo, err := store.UpsertRepo(&Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "60s",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	pr, err := store.UpsertPRState(repo.ID, 1, "sha-1", nil, "idle")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	// 1 done run, 2 agent_running runs
	_, _ = store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "low",
		Runtime:     "claude-code",
		Model:       "sonnet",
		ModelSource: "config",
		CostBasis:   "exact",
		Status:      "done",
	})
	runRunning1, _ := store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "medium",
		Runtime:     "claude-code",
		Model:       "sonnet",
		ModelSource: "config",
		CostBasis:   "exact",
		Status:      "agent_running",
	})
	runRunning2, _ := store.RecordRun(&Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-1",
		RiskTier:    "critical",
		Runtime:     "claude-code",
		Model:       "opus",
		ModelSource: "table",
		CostBasis:   "exact",
		Status:      "agent_running",
	})

	runningRuns, err := store.RunsInState("agent_running")
	if err != nil {
		t.Fatalf("RunsInState: %v", err)
	}
	if len(runningRuns) != 2 {
		t.Fatalf("len(runningRuns) = %d, want 2", len(runningRuns))
	}
	// Ordered newest first
	if runningRuns[0].ID != runRunning2.ID || runningRuns[1].ID != runRunning1.ID {
		t.Errorf("runningRuns order = [%d, %d], want [%d, %d]",
			runningRuns[0].ID, runningRuns[1].ID, runRunning2.ID, runRunning1.ID)
	}

	doneRuns, err := store.RunsInState("done")
	if err != nil {
		t.Fatalf("RunsInState(done): %v", err)
	}
	if len(doneRuns) != 1 {
		t.Fatalf("len(doneRuns) = %d, want 1", len(doneRuns))
	}
}
