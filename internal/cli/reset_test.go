package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/dustinmays/pr-triage/internal/db"
)

func TestResetCommand_ClearsTrackedPR(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // DefaultDBPath resolves under ~/.pr-triage

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
	const prNum = 92
	if _, err := store.UpsertPRState(repo.ID, prNum, "sha-abc123", nil, "ci_failed"); err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}
	_ = database.Close()

	flagResetRepo = ""
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"reset", "92"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("reset command failed: %v", err)
	}

	database2, err := db.Open(db.DefaultDBPath())
	if err != nil {
		t.Fatalf("db.Open (verify): %v", err)
	}
	defer func() { _ = database2.Close() }()
	store2 := db.NewStore(database2)

	if _, err := store2.GetPRState(repo.ID, prNum); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetPRState after reset: err = %v, want ErrNotFound", err)
	}
}

func TestResetCommand_UnknownPRErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	database, err := db.Open(db.DefaultDBPath())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if _, err := db.NewStore(database).UpsertRepo(&db.Repo{
		Owner: "dustinmays", Name: "pr-triage", BaseRef: "main", PollInterval: "5m",
	}); err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}
	_ = database.Close()

	flagResetRepo = ""
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"reset", "404"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for an untracked PR number, got nil")
	}
}

func TestResetCommand_AmbiguousAcrossReposRequiresFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	database, err := db.Open(db.DefaultDBPath())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	store := db.NewStore(database)
	repoA, err := store.UpsertRepo(&db.Repo{Owner: "acme", Name: "widgets", BaseRef: "main", PollInterval: "5m"})
	if err != nil {
		t.Fatalf("UpsertRepo A: %v", err)
	}
	repoB, err := store.UpsertRepo(&db.Repo{Owner: "acme", Name: "gadgets", BaseRef: "main", PollInterval: "5m"})
	if err != nil {
		t.Fatalf("UpsertRepo B: %v", err)
	}
	const prNum = 7
	if _, err := store.UpsertPRState(repoA.ID, prNum, "sha-a", nil, "ci_failed"); err != nil {
		t.Fatalf("UpsertPRState A: %v", err)
	}
	if _, err := store.UpsertPRState(repoB.ID, prNum, "sha-b", nil, "ci_failed"); err != nil {
		t.Fatalf("UpsertPRState B: %v", err)
	}
	_ = database.Close()

	flagResetRepo = ""
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"reset", "7"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected ambiguity error when PR number is tracked in multiple repos, got nil")
	}

	// Disambiguating with --repo should succeed and only clear that repo's PR.
	rootCmd.SetArgs([]string{"reset", "7", "--repo", "acme/widgets"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("reset with --repo failed: %v", err)
	}

	database2, err := db.Open(db.DefaultDBPath())
	if err != nil {
		t.Fatalf("db.Open (verify): %v", err)
	}
	defer func() { _ = database2.Close() }()
	store2 := db.NewStore(database2)

	if _, err := store2.GetPRState(repoA.ID, prNum); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetPRState repoA after reset: err = %v, want ErrNotFound", err)
	}
	if _, err := store2.GetPRState(repoB.ID, prNum); err != nil {
		t.Fatalf("GetPRState repoB should be untouched: %v", err)
	}
}
