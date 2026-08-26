// Package db owns SQLite persistence for pr-triage: opening the database
// with the correct pragmas and running the sequential schema migrations.
package db

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// DefaultDBDir returns the default directory (~/.pr-triage) for data storage.
func DefaultDBDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".pr-triage"
	}
	return filepath.Join(home, ".pr-triage")
}

// DefaultDBPath returns ~/.pr-triage/pr-triage.db.
func DefaultDBPath() string {
	return filepath.Join(DefaultDBDir(), "pr-triage.db")
}

// schemaVersion is the target schema version. Bump it by one whenever a new
// migration block is appended to migrate, and never edit a past migration
// block after the fact (see docs/persistence-discipline.md).
const schemaVersion = 3

// Open opens (creating if necessary) the SQLite database at path, applies
// the required pragmas, enforces single-writer discipline, and runs any
// pending migrations up to schemaVersion. path may be a plain filesystem
// path (e.g. "/tmp/foo.db") or a file: DSN; pragma query parameters are
// appended automatically.
func Open(path string) (*sqlx.DB, error) {
	// Ensure the parent directory exists; SQLite returns a cryptic
	// "unable to open database file (14)" if it does not (e.g. a fresh
	// ~/.pr-triage on first run). Skip for in-memory DSNs.
	if path != "" && path != ":memory:" && !strings.Contains(path, "mode=memory") {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("db: create dir %s: %w", dir, err)
			}
		}
	}

	dsn := buildDSN(path)

	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}

	// WAL + a single connection avoids SQLITE_BUSY errors from concurrent
	// writers; see docs/persistence-discipline.md.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db: ping %s: %w", path, err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db: migrate %s: %w", path, err)
	}

	return db, nil
}

// buildDSN appends the pragma settings pr-triage requires to a plain path or
// existing DSN, using modernc.org/sqlite's "_pragma=" query parameter
// convention.
func buildDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	return path + "?" + q.Encode()
}

// migrate reads the current schema version from PRAGMA user_version and
// applies any pending migration blocks in order, inside a single
// transaction, idempotently. To add a migration, append another
// `if version < N { ... }` block below (increment schemaVersion to match)
// and never modify a previously released block.
func migrate(db *sqlx.DB) error {
	var version int
	if err := db.Get(&version, "PRAGMA user_version"); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	if version >= schemaVersion {
		return nil
	}

	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if version < 1 {
		// Minimal bootstrap: a schema_meta table recording when the schema
		// was initialized. Real application tables land in a follow-up
		// sub-issue (see docs/adr/0004-shared-sqlite-schema-repo-id.md).
		if _, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS schema_meta (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				initialized_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
		`); err != nil {
			return fmt.Errorf("migration v1: %w", err)
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_meta (id) VALUES (1);`); err != nil {
			return fmt.Errorf("migration v1: %w", err)
		}
		version = 1
	}

	if version < 2 {
		if _, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS repos (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				owner TEXT NOT NULL,
				name TEXT NOT NULL,
				base_ref TEXT NOT NULL,
				poll_interval TEXT NOT NULL,
				config_path TEXT NOT NULL,
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);

			CREATE TABLE IF NOT EXISTS prs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
				number INTEGER NOT NULL,
				head_sha TEXT NOT NULL,
				last_run_id INTEGER,
				state TEXT NOT NULL,
				updated_at TEXT NOT NULL DEFAULT (datetime('now')),
				UNIQUE(repo_id, number)
			);

			CREATE TABLE IF NOT EXISTS runs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				pr_id INTEGER NOT NULL REFERENCES prs(id) ON DELETE CASCADE,
				head_sha TEXT NOT NULL,
				ci_run_id INTEGER,
				risk_tier TEXT NOT NULL,
				runtime TEXT NOT NULL,
				model TEXT NOT NULL,
				model_source TEXT NOT NULL,
				cost_usd REAL NOT NULL DEFAULT 0.0,
				cost_basis TEXT NOT NULL,
				turns INTEGER NOT NULL DEFAULT 0,
				status TEXT NOT NULL,
				stop_reason TEXT NOT NULL DEFAULT '',
				pid INTEGER,
				log_path TEXT NOT NULL DEFAULT '',
				worktree_path TEXT NOT NULL DEFAULT '',
				started_at TEXT NOT NULL DEFAULT (datetime('now')),
				finished_at TEXT
			);

			CREATE INDEX IF NOT EXISTS idx_prs_repo_id ON prs(repo_id);
			CREATE INDEX IF NOT EXISTS idx_runs_pr_id ON runs(pr_id);
		`); err != nil {
			return fmt.Errorf("migration v2: %w", err)
		}
		version = 2
	}

	if version < 3 {
		// Human overrides for escalated PRs (D.4). A row records that the owner
		// waived specific escalate-tier signals on a PR at a specific head SHA,
		// letting the review agent run instead of escalating. Pinned to head_sha
		// so a new push invalidates it (state-first; see ADR 0006 and
		// docs/epic-80/design/escalation-override.md). waived_signals is a
		// comma-separated list of signal IDs; empty means "waive all escalate-tier
		// signals present at this SHA". consumed_at is set when the override is
		// applied, giving a one-shot audit trail.
		if _, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS overrides (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
				pr_number INTEGER NOT NULL,
				head_sha TEXT NOT NULL,
				waived_signals TEXT NOT NULL DEFAULT '',
				reason TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT (datetime('now')),
				consumed_at TEXT
			);

			CREATE INDEX IF NOT EXISTS idx_overrides_lookup ON overrides(repo_id, pr_number, head_sha);
		`); err != nil {
			return fmt.Errorf("migration v3: %w", err)
		}
		version = 3
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}

	return nil
}
