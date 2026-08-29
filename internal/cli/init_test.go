package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/config"
	"github.com/dustinmays/pr-triage/internal/db"
)

func TestInitCommandNonInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	repoDir := filepath.Join(tmpDir, "my-repo")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{
		"init",
		"--non-interactive",
		"--owner", "dustinmays",
		"--name", "my-repo",
		"--base-ref", "main",
		"--poll-interval", "5m",
		"--timeout", "12m",
		"--github-user", "dustinmays",
		"--db-path", dbPath,
		"--repo-dir", repoDir,
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error: %v", err)
	}

	// 1. Verify config file written
	cfgPath := filepath.Join(repoDir, ".pr-triage", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load(%s) error: %v", cfgPath, err)
	}
	if cfg.BaseRef != "main" || cfg.PollInterval != "5m" || cfg.Timeout != "12m" || cfg.GitHubUser != "dustinmays" {
		t.Fatalf("unexpected config content: %+v", cfg)
	}

	// 2. Verify repos table in DB
	dbConn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open error: %v", err)
	}
	defer func() { _ = dbConn.Close() }()

	store := db.NewStore(dbConn)
	repos, err := store.ListRepos()
	if err != nil {
		t.Fatalf("store.ListRepos error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo row, got %d", len(repos))
	}
	if repos[0].Owner != "dustinmays" || repos[0].Name != "my-repo" || repos[0].BaseRef != "main" || repos[0].PollInterval != "5m" {
		t.Fatalf("unexpected repo row: %+v", repos[0])
	}

	// 3. Re-run with updated base-ref -> should update existing row without duplicate
	buf.Reset()
	rootCmd.SetArgs([]string{
		"init",
		"--non-interactive",
		"--owner", "dustinmays",
		"--name", "my-repo",
		"--base-ref", "release/v2",
		"--poll-interval", "10m",
		"--db-path", dbPath,
		"--repo-dir", repoDir,
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("re-run rootCmd.Execute() error: %v", err)
	}

	reposAfter, err := store.ListRepos()
	if err != nil {
		t.Fatalf("store.ListRepos error: %v", err)
	}
	if len(reposAfter) != 1 {
		t.Fatalf("expected 1 repo row after update, got %d", len(reposAfter))
	}
	if reposAfter[0].BaseRef != "release/v2" || reposAfter[0].PollInterval != "10m" {
		t.Fatalf("expected updated repo row, got: %+v", reposAfter[0])
	}
}

func TestInitModelPinsRoutineRouting(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	repoDir := filepath.Join(tmpDir, "my-repo")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{
		"init",
		"--non-interactive",
		"--owner", "dustinmays",
		"--name", "my-repo",
		"--base-ref", "main",
		"--model", "claude-opus-4-8",
		"--db-path", dbPath,
		"--repo-dir", repoDir,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error: %v", err)
	}

	cfgPath := filepath.Join(repoDir, ".pr-triage", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load(%s) error: %v", cfgPath, err)
	}
	got := cfg.Routing["routine"].Model
	if got != "claude-opus-4-8" {
		t.Fatalf("routing.routine.model = %q, want %q", got, "claude-opus-4-8")
	}
	// The routine routing entry must stay otherwise intact (runtime + agent def).
	if cfg.Routing["routine"].Runtime != "claude-code" || cfg.Routing["routine"].AgentDef != "review-agent" {
		t.Fatalf("routine routing lost its runtime/agent def: %+v", cfg.Routing["routine"])
	}
}

func TestInitRuntimePinsRoutineRouting(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	repoDir := filepath.Join(tmpDir, "my-repo")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{
		"init",
		"--non-interactive",
		"--owner", "dustinmays",
		"--name", "my-repo",
		"--base-ref", "main",
		"--runtime", "opencode",
		"--model", "openrouter/z-ai/glm-5.3-flash",
		"--db-path", dbPath,
		"--repo-dir", repoDir,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error: %v", err)
	}

	cfgPath := filepath.Join(repoDir, ".pr-triage", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load(%s) error: %v", cfgPath, err)
	}
	if got := cfg.Routing["routine"].Runtime; got != "opencode" {
		t.Fatalf("routing.routine.runtime = %q, want %q", got, "opencode")
	}
	if got := cfg.Routing["routine"].Model; got != "openrouter/z-ai/glm-5.3-flash" {
		t.Fatalf("routing.routine.model = %q, want %q", got, "openrouter/z-ai/glm-5.3-flash")
	}
	// The routine routing entry must stay otherwise intact (agent def).
	if cfg.Routing["routine"].AgentDef != "review-agent" {
		t.Fatalf("routine routing lost its agent def: %+v", cfg.Routing["routine"])
	}
}
