package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/db"
)

func TestCheckoutCommandExecution(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"checkout", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("checkout --help failed: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("checkout")) {
		t.Errorf("expected checkout help text in output")
	}
}
