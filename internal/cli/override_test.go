package cli

import (
	"bytes"
	"testing"

	"github.com/dustinmays/pr-triage/internal/db"
)

func TestOverrideCommand_RecordsAndRearms(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome) // DefaultDBPath resolves under ~/.pr-triage

	// Seed a repo + an escalated PR in the same DB the command will open.
	database, err := db.Open(db.DefaultDBPath())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	store := db.NewStore(database)
	repo, err := store.UpsertRepo(&db.Repo{
		Owner: "dustinmays", Name: "pr-triage", BaseRef: "main", PollInterval: "5m",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}
	const prNum = 94
	if _, err := store.UpsertPRState(repo.ID, prNum, "sha-x", nil, "escalated"); err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}
	_ = database.Close()

	// Reset the repeatable flag so a prior test run can't leak values.
	flagOverrideSignals = nil
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"override", "94", "--signal", "workflow_changed", "--reason", "infra chunk"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("override command failed: %v", err)
	}

	// Verify the override row and the re-arm to ci_passed.
	database2, err := db.Open(db.DefaultDBPath())
	if err != nil {
		t.Fatalf("db.Open (verify): %v", err)
	}
	defer func() { _ = database2.Close() }()
	store2 := db.NewStore(database2)

	ov, err := store2.GetActiveOverride(repo.ID, prNum, "sha-x")
	if err != nil {
		t.Fatalf("GetActiveOverride: %v", err)
	}
	if list := ov.WaivedSignalList(); len(list) != 1 || list[0] != "workflow_changed" {
		t.Errorf("waived signals = %v, want [workflow_changed]", list)
	}
	if ov.Reason != "infra chunk" {
		t.Errorf("reason = %q, want 'infra chunk'", ov.Reason)
	}

	pr, err := store2.GetPRState(repo.ID, prNum)
	if err != nil {
		t.Fatalf("GetPRState: %v", err)
	}
	if pr.State != "ci_passed" {
		t.Errorf("PR state = %q, want 'ci_passed' (override must re-arm re-evaluation)", pr.State)
	}
}
