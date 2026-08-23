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

func TestSchema_TablesAndConstraints(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var fk int
	if err := conn.Get(&fk, "PRAGMA foreign_keys"); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	res, err := conn.Exec(`INSERT INTO repos (owner, name, base_ref) VALUES (?, ?, ?)`,
		"acme", "widgets", "main")
	if err != nil {
		t.Fatalf("insert repos: %v", err)
	}
	repoID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("repos LastInsertId: %v", err)
	}

	prRes, err := conn.Exec(`INSERT INTO prs (repo_id, number, state) VALUES (?, ?, ?)`,
		repoID, 42, "open")
	if err != nil {
		t.Fatalf("insert prs: %v", err)
	}
	prID, err := prRes.LastInsertId()
	if err != nil {
		t.Fatalf("prs LastInsertId: %v", err)
	}

	// Duplicate (repo_id, number) must violate the UNIQUE constraint.
	if _, err := conn.Exec(`INSERT INTO prs (repo_id, number, state) VALUES (?, ?, ?)`,
		repoID, 42, "open"); err == nil {
		t.Fatalf("expected UNIQUE(repo_id, number) violation, got nil error")
	}

	// A run referencing a nonexistent pr_id must violate the FK constraint.
	if _, err := conn.Exec(`INSERT INTO runs (pr_id, cost_basis) VALUES (?, ?)`,
		999999, "estimated"); err == nil {
		t.Fatalf("expected FK violation on runs.pr_id, got nil error")
	}

	// A well-formed run should round-trip all fields, including cost_basis.
	started := "2024-01-01T00:00:00Z"
	finished := "2024-01-01T00:05:00Z"
	if _, err := conn.Exec(`
		INSERT INTO runs (
			pr_id, head_sha, ci_run_id, risk_tier, runtime, model, model_source,
			cost_usd, cost_basis, turns, status, stop_reason, pid, log_path,
			worktree_path, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		prID, "deadbeef", 123, "high", "docker", "claude-opus", "config",
		1.23, "measured", 5, "completed", "done", 4242, "/tmp/log",
		"/tmp/worktree", started, finished,
	); err != nil {
		t.Fatalf("insert runs: %v", err)
	}

	type run struct {
		ID           int64   `db:"id"`
		PRID         int64   `db:"pr_id"`
		HeadSHA      string  `db:"head_sha"`
		CIRunID      int64   `db:"ci_run_id"`
		RiskTier     string  `db:"risk_tier"`
		Runtime      string  `db:"runtime"`
		Model        string  `db:"model"`
		ModelSource  string  `db:"model_source"`
		CostUSD      float64 `db:"cost_usd"`
		CostBasis    string  `db:"cost_basis"`
		Turns        int     `db:"turns"`
		Status       string  `db:"status"`
		StopReason   string  `db:"stop_reason"`
		PID          int64   `db:"pid"`
		LogPath      string  `db:"log_path"`
		WorktreePath string  `db:"worktree_path"`
		StartedAt    string  `db:"started_at"`
		FinishedAt   string  `db:"finished_at"`
	}

	var got run
	if err := conn.Get(&got, `SELECT * FROM runs WHERE pr_id = ?`, prID); err != nil {
		t.Fatalf("select runs: %v", err)
	}

	if got.CostBasis != "measured" {
		t.Errorf("cost_basis = %q, want %q", got.CostBasis, "measured")
	}
	if got.HeadSHA != "deadbeef" {
		t.Errorf("head_sha = %q, want %q", got.HeadSHA, "deadbeef")
	}
	if got.CostUSD != 1.23 {
		t.Errorf("cost_usd = %v, want %v", got.CostUSD, 1.23)
	}
	if got.Turns != 5 {
		t.Errorf("turns = %d, want %d", got.Turns, 5)
	}
	if got.StartedAt != started || got.FinishedAt != finished {
		t.Errorf("started_at/finished_at = %q/%q, want %q/%q", got.StartedAt, got.FinishedAt, started, finished)
	}
}
