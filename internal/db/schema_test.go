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
