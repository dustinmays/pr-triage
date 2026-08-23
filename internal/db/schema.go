// Package db owns SQLite persistence for pr-triage: opening the database
// with the correct pragmas and running the sequential schema migrations.
package db

import (
	"fmt"
	"net/url"

	"github.com/jmoiron/sqlx"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// schemaVersion is the target schema version. Bump it by one whenever a new
// migration block is appended to migrate, and never edit a past migration
// block after the fact (see docs/persistence-discipline.md).
const schemaVersion = 1

// Open opens (creating if necessary) the SQLite database at path, applies
// the required pragmas, enforces single-writer discipline, and runs any
// pending migrations up to schemaVersion. path may be a plain filesystem
// path (e.g. "/tmp/foo.db") or a file: DSN; pragma query parameters are
// appended automatically.
func Open(path string) (*sqlx.DB, error) {
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

	// Example for the next migration:
	// if version < 2 {
	// 	if _, err := tx.Exec(`...`); err != nil {
	// 		return fmt.Errorf("migration v2: %w", err)
	// 	}
	// 	version = 2
	// }

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}

	return nil
}
