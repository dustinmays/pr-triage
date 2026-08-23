package db

import (
	"path/filepath"
	"testing"
)

func TestOpen_CreatesSchemaAndIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var journalMode string
	if err := conn.Get(&journalMode, "PRAGMA journal_mode"); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var version int
	if err := conn.Get(&version, "PRAGMA user_version"); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("user_version = %d, want %d", version, schemaVersion)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Re-open the same file: migrations must not re-run and user_version
	// must remain stable.
	conn2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() { _ = conn2.Close() }()

	var version2 int
	if err := conn2.Get(&version2, "PRAGMA user_version"); err != nil {
		t.Fatalf("PRAGMA user_version (reopen): %v", err)
	}
	if version2 != schemaVersion {
		t.Errorf("user_version after reopen = %d, want %d", version2, schemaVersion)
	}
}

func TestForeignKey_EnforcedOnPRS(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Inserting a PR with a non-existent repo_id (e.g. 999) must fail foreign key constraint.
	_, err = conn.Exec(`
		INSERT INTO prs (repo_id, number, head_sha, state)
		VALUES (999, 1, 'abc1234', 'idle');
	`)
	if err == nil {
		t.Fatal("expected foreign key error when inserting PR with non-existent repo_id, got nil")
	}
}

func TestForeignKey_EnforcedOnRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Inserting a Run with a non-existent pr_id (e.g. 999) must fail foreign key constraint.
	_, err = conn.Exec(`
		INSERT INTO runs (pr_id, head_sha, risk_tier, runtime, model, model_source, cost_basis, status)
		VALUES (999, 'abc1234', 'tier-low', 'claude-code', 'claude-3-7-sonnet', 'config', 'exact', 'running');
	`)
	if err == nil {
		t.Fatal("expected foreign key error when inserting Run with non-existent pr_id, got nil")
	}
}

func TestUniqueConstraint_PRDuplicateFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	res, err := conn.Exec(`
		INSERT INTO repos (owner, name, base_ref, poll_interval, config_path)
		VALUES ('dustinmays', 'pr-triage', 'main', '60s', '.pr-triage.yaml');
	`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	repoID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	_, err = conn.Exec(`
		INSERT INTO prs (repo_id, number, head_sha, state)
		VALUES (?, 42, 'sha-1', 'idle');
	`, repoID)
	if err != nil {
		t.Fatalf("first insert pr: %v", err)
	}

	// Duplicate (repo_id, number) must fail due to UNIQUE(repo_id, number)
	_, err = conn.Exec(`
		INSERT INTO prs (repo_id, number, head_sha, state)
		VALUES (?, 42, 'sha-2', 'idle');
	`, repoID)
	if err == nil {
		t.Fatal("expected error on duplicate (repo_id, number) insert, got nil")
	}
}

func TestCostBasis_NotNull(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	res, err := conn.Exec(`
		INSERT INTO repos (owner, name, base_ref, poll_interval, config_path)
		VALUES ('dustinmays', 'pr-triage', 'main', '60s', '.pr-triage.yaml');
	`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	repoID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	res, err = conn.Exec(`
		INSERT INTO prs (repo_id, number, head_sha, state)
		VALUES (?, 1, 'sha-1', 'idle');
	`, repoID)
	if err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	prID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	// Inserting a run without cost_basis (NULL) must fail
	_, err = conn.Exec(`
		INSERT INTO runs (pr_id, head_sha, risk_tier, runtime, model, model_source, status, cost_basis)
		VALUES (?, 'sha-1', 'tier-low', 'claude-code', 'claude-3-7-sonnet', 'config', 'running', NULL);
	`, prID)
	if err == nil {
		t.Fatal("expected NOT NULL error for cost_basis NULL insert, got nil")
	}
}

func TestRuns_RoundTripAllFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	res, err := conn.Exec(`
		INSERT INTO repos (owner, name, base_ref, poll_interval, config_path)
		VALUES ('dustinmays', 'pr-triage', 'main', '30s', '.pr-triage.yaml');
	`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	repoID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	res, err = conn.Exec(`
		INSERT INTO prs (repo_id, number, head_sha, state)
		VALUES (?, 100, 'deadbeef123', 'agent_running');
	`, repoID)
	if err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	prID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	ciRunID := int64(987654321)
	pid := 12345
	finishedAt := "2026-08-23T12:00:00Z"
	startedAt := "2026-08-23T11:55:00Z"

	insertRun := Run{
		PRID:         prID,
		HeadSHA:      "deadbeef123",
		CIRunID:      &ciRunID,
		RiskTier:     "critical",
		Runtime:      "claude-code",
		Model:        "claude-3-7-sonnet",
		ModelSource:  "risk-table",
		CostUSD:      0.1425,
		CostBasis:    "exact",
		Turns:        5,
		Status:       "success",
		StopReason:   "end_turn",
		PID:          &pid,
		LogPath:      "/tmp/run-123.log",
		WorktreePath: "/tmp/worktree-123",
		StartedAt:    startedAt,
		FinishedAt:   &finishedAt,
	}

	runRes, err := conn.NamedExec(`
		INSERT INTO runs (
			pr_id, head_sha, ci_run_id, risk_tier, runtime, model, model_source,
			cost_usd, cost_basis, turns, status, stop_reason, pid, log_path,
			worktree_path, started_at, finished_at
		) VALUES (
			:pr_id, :head_sha, :ci_run_id, :risk_tier, :runtime, :model, :model_source,
			:cost_usd, :cost_basis, :turns, :status, :stop_reason, :pid, :log_path,
			:worktree_path, :started_at, :finished_at
		);
	`, insertRun)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	runID, err := runRes.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	var fetched Run
	if err := conn.Get(&fetched, "SELECT * FROM runs WHERE id = ?", runID); err != nil {
		t.Fatalf("Get run: %v", err)
	}

	if fetched.ID != runID {
		t.Errorf("ID = %d, want %d", fetched.ID, runID)
	}
	if fetched.PRID != insertRun.PRID {
		t.Errorf("PRID = %d, want %d", fetched.PRID, insertRun.PRID)
	}
	if fetched.HeadSHA != insertRun.HeadSHA {
		t.Errorf("HeadSHA = %q, want %q", fetched.HeadSHA, insertRun.HeadSHA)
	}
	if fetched.CIRunID == nil || *fetched.CIRunID != *insertRun.CIRunID {
		t.Errorf("CIRunID = %v, want %v", fetched.CIRunID, insertRun.CIRunID)
	}
	if fetched.RiskTier != insertRun.RiskTier {
		t.Errorf("RiskTier = %q, want %q", fetched.RiskTier, insertRun.RiskTier)
	}
	if fetched.Runtime != insertRun.Runtime {
		t.Errorf("Runtime = %q, want %q", fetched.Runtime, insertRun.Runtime)
	}
	if fetched.Model != insertRun.Model {
		t.Errorf("Model = %q, want %q", fetched.Model, insertRun.Model)
	}
	if fetched.ModelSource != insertRun.ModelSource {
		t.Errorf("ModelSource = %q, want %q", fetched.ModelSource, insertRun.ModelSource)
	}
	if fetched.CostUSD != insertRun.CostUSD {
		t.Errorf("CostUSD = %f, want %f", fetched.CostUSD, insertRun.CostUSD)
	}
	if fetched.CostBasis != insertRun.CostBasis {
		t.Errorf("CostBasis = %q, want %q", fetched.CostBasis, insertRun.CostBasis)
	}
	if fetched.Turns != insertRun.Turns {
		t.Errorf("Turns = %d, want %d", fetched.Turns, insertRun.Turns)
	}
	if fetched.Status != insertRun.Status {
		t.Errorf("Status = %q, want %q", fetched.Status, insertRun.Status)
	}
	if fetched.StopReason != insertRun.StopReason {
		t.Errorf("StopReason = %q, want %q", fetched.StopReason, insertRun.StopReason)
	}
	if fetched.PID == nil || *fetched.PID != *insertRun.PID {
		t.Errorf("PID = %v, want %v", fetched.PID, insertRun.PID)
	}
	if fetched.LogPath != insertRun.LogPath {
		t.Errorf("LogPath = %q, want %q", fetched.LogPath, insertRun.LogPath)
	}
	if fetched.WorktreePath != insertRun.WorktreePath {
		t.Errorf("WorktreePath = %q, want %q", fetched.WorktreePath, insertRun.WorktreePath)
	}
	if fetched.StartedAt != insertRun.StartedAt {
		t.Errorf("StartedAt = %q, want %q", fetched.StartedAt, insertRun.StartedAt)
	}
	if fetched.FinishedAt == nil || *fetched.FinishedAt != *insertRun.FinishedAt {
		t.Errorf("FinishedAt = %v, want %v", fetched.FinishedAt, insertRun.FinishedAt)
	}
}

func TestIndexes_Exist(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	var indexNames []string
	if err := conn.Select(&indexNames, "SELECT name FROM sqlite_master WHERE type = 'index' AND name IN ('idx_prs_repo_id', 'idx_runs_pr_id')"); err != nil {
		t.Fatalf("query sqlite_master for indexes: %v", err)
	}

	found := make(map[string]bool)
	for _, name := range indexNames {
		found[name] = true
	}

	if !found["idx_prs_repo_id"] {
		t.Error("expected index idx_prs_repo_id to exist")
	}
	if !found["idx_runs_pr_id"] {
		t.Error("expected index idx_runs_pr_id to exist")
	}
}
