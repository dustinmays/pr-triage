package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/db"
)

func TestStatusAndLogsCommands(t *testing.T) {
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
	pr, _ := store.UpsertPRState(repo.ID, 42, "sha-status-test", nil, "done")

	logPath := filepath.Join(tmpDir, "run-1.log")
	_ = os.WriteFile(logPath, []byte("line 1\nline 2\nline 3\n"), 0644)

	run := &db.Run{
		PRID:        pr.ID,
		HeadSHA:     "sha-status-test",
		RiskTier:    "routine",
		Runtime:     "claude-code",
		Model:       "sonnet",
		ModelSource: "default",
		CostUSD:     0.02,
		CostBasis:   "exact",
		Status:      "done",
		LogPath:     logPath,
	}
	_, _ = store.RecordRun(run)

	// Test Status command output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"status"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status command failed: %v", err)
	}

	out := buf.String()
	if len(out) == 0 {
		t.Fatalf("expected non-empty status output")
	}

	// Test Status --json output
	buf.Reset()
	rootCmd.SetArgs([]string{"status", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}

	// Test Logs command output
	buf.Reset()
	rootCmd.SetArgs([]string{"logs", "--lines", "10"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("logs command failed: %v", err)
	}
}
