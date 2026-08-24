package runtime

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/db"
)

type stubRuntime struct {
	name string
}

func (s *stubRuntime) Name() string { return s.name }
func (s *stubRuntime) Run(_ context.Context, _ Invocation, _ io.Writer) (int, error) {
	return 0, nil
}
func (s *stubRuntime) ParseResult(_ io.Reader) (*Result, error) {
	return &Result{Cost: 0, CostBasis: CostBasisExact, Turns: 1, StopReason: "end_turn"}, nil
}
func (s *stubRuntime) ClassifyOutcome(_ *Result, _ int) Outcome {
	return OutcomeSuccess
}

func resetRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	registry = map[string]AgentRuntime{}
	registryMu.Unlock()

	Register(&stubRuntime{name: NameClaudeCode})
	Register(&stubRuntime{name: NameCodex})
	Register(&stubRuntime{name: NameOpenCode})
}

func TestResolveTableHierarchy(t *testing.T) {
	tests := []struct {
		name       string
		explicit   string
		repoCfg    string
		userCfg    string
		wantValue  string
		wantSource string
	}{
		{
			name:       "explicit flag overrides repo and user",
			explicit:   NameCodex,
			repoCfg:    "runtime: claude-code\n",
			userCfg:    "runtime: opencode\n",
			wantValue:  NameCodex,
			wantSource: "flag",
		},
		{
			name:       "repo config overrides user and default",
			explicit:   "",
			repoCfg:    "runtime: opencode\n",
			userCfg:    "runtime: codex\n",
			wantValue:  NameOpenCode,
			wantSource: "repo",
		},
		{
			name:       "user config overrides default when repo config absent",
			explicit:   "",
			repoCfg:    "",
			userCfg:    "runtime: codex\n",
			wantValue:  NameCodex,
			wantSource: "user",
		},
		{
			name:       "built-in default when no explicit, repo, or user config",
			explicit:   "",
			repoCfg:    "",
			userCfg:    "",
			wantValue:  DefaultName,
			wantSource: "default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetRegistry(t)

			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := t.TempDir()

			if tc.userCfg != "" {
				userDir := filepath.Join(home, ".pr-triage")
				if err := os.MkdirAll(userDir, 0o755); err != nil {
					t.Fatalf("mkdir user dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(userDir, ConfigFileName), []byte(tc.userCfg), 0o644); err != nil {
					t.Fatalf("write user config: %v", err)
				}
			}

			if tc.repoCfg != "" {
				repoDir := filepath.Join(repo, ".pr-triage")
				if err := os.MkdirAll(repoDir, 0o755); err != nil {
					t.Fatalf("mkdir repo dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(repoDir, ConfigFileName), []byte(tc.repoCfg), 0o644); err != nil {
					t.Fatalf("write repo config: %v", err)
				}
			}

			got, err := Resolve(tc.explicit, repo)
			if err != nil {
				t.Fatalf("Resolve(%q, %q) returned error: %v", tc.explicit, repo, err)
			}
			if got.Value != tc.wantValue {
				t.Errorf("got.Value = %q, want %q", got.Value, tc.wantValue)
			}
			if got.Source != tc.wantSource {
				t.Errorf("got.Source = %q, want %q", got.Source, tc.wantSource)
			}
			if got.Name() != tc.wantValue {
				t.Errorf("got.Name() = %q, want %q", got.Name(), tc.wantValue)
			}
		})
	}
}

func TestResolvePersistedOnRunRow(t *testing.T) {
	resetRegistry(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlxDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() { _ = sqlxDB.Close() }()

	store := db.NewStore(sqlxDB)

	repoRecord, err := store.UpsertRepo(&db.Repo{
		Owner:        "dustinmays",
		Name:         "pr-triage",
		BaseRef:      "main",
		PollInterval: "1m",
		ConfigPath:   ".pr-triage.yaml",
	})
	if err != nil {
		t.Fatalf("UpsertRepo: %v", err)
	}

	prRecord, err := store.UpsertPRState(repoRecord.ID, 42, "abc1234", nil, "open")
	if err != nil {
		t.Fatalf("UpsertPRState: %v", err)
	}

	// 1. Resolve configuration across ranks
	repoDir := t.TempDir()
	cfgDir := filepath.Join(repoDir, ".pr-triage")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, ConfigFileName), []byte("runtime: claude-code\nmodel: claude-3-7-sonnet\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	runtimeRes, err := Resolve("", repoDir)
	if err != nil {
		t.Fatalf("Resolve runtime: %v", err)
	}
	if runtimeRes.Source != "repo" || runtimeRes.Value != "claude-code" {
		t.Fatalf("Resolve = %+v, want claude-code/repo", runtimeRes)
	}

	modelRes, err := ResolveModel("", repoDir)
	if err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if modelRes.Source != "repo" || modelRes.Value != "claude-3-7-sonnet" {
		t.Fatalf("ResolveModel = %+v, want claude-3-7-sonnet/repo", modelRes)
	}

	// 2. Caller writes resolved model and model_source once onto the run row
	run := &db.Run{
		PRID:         prRecord.ID,
		HeadSHA:      "abc1234",
		RiskTier:     "tier-low",
		Runtime:      runtimeRes.Value,
		Model:        modelRes.Value,
		ModelSource:  modelRes.Source,
		CostUSD:      0.05,
		CostBasis:    "exact",
		Turns:        2,
		Status:       "success",
		StopReason:   "end_turn",
		WorktreePath: "/tmp/worktree",
	}

	recordedRun, err := store.RecordRun(run)
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	// 3. Confirm the stored run row matches the resolved values
	runs, err := store.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	stored := runs[0]
	if stored.ID != recordedRun.ID {
		t.Errorf("stored.ID = %d, want %d", stored.ID, recordedRun.ID)
	}
	if stored.Runtime != runtimeRes.Value {
		t.Errorf("stored.Runtime = %q, want %q", stored.Runtime, runtimeRes.Value)
	}
	if stored.Model != modelRes.Value {
		t.Errorf("stored.Model = %q, want %q", stored.Model, modelRes.Value)
	}
	if stored.ModelSource != modelRes.Source {
		t.Errorf("stored.ModelSource = %q, want %q", stored.ModelSource, modelRes.Source)
	}
}

func TestResolveModelHierarchy(t *testing.T) {
	tests := []struct {
		name       string
		explicit   string
		repoCfg    string
		userCfg    string
		wantValue  string
		wantSource string
	}{
		{
			name:       "explicit model overrides repo and user",
			explicit:   "gpt-4o",
			repoCfg:    "model: claude-3-7-sonnet\n",
			userCfg:    "model: claude-3-5-sonnet\n",
			wantValue:  "gpt-4o",
			wantSource: "flag",
		},
		{
			name:       "repo model overrides user and default",
			explicit:   "",
			repoCfg:    "model: claude-3-5-sonnet\n",
			userCfg:    "model: gpt-4o\n",
			wantValue:  "claude-3-5-sonnet",
			wantSource: "repo",
		},
		{
			name:       "user model overrides default when repo absent",
			explicit:   "",
			repoCfg:    "",
			userCfg:    "model: gpt-4o\n",
			wantValue:  "gpt-4o",
			wantSource: "user",
		},
		{
			name:       "built-in default model when no explicit, repo, or user config",
			explicit:   "",
			repoCfg:    "",
			userCfg:    "",
			wantValue:  DefaultModel,
			wantSource: "default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := t.TempDir()

			if tc.userCfg != "" {
				userDir := filepath.Join(home, ".pr-triage")
				if err := os.MkdirAll(userDir, 0o755); err != nil {
					t.Fatalf("mkdir user dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(userDir, ConfigFileName), []byte(tc.userCfg), 0o644); err != nil {
					t.Fatalf("write user config: %v", err)
				}
			}

			if tc.repoCfg != "" {
				repoDir := filepath.Join(repo, ".pr-triage")
				if err := os.MkdirAll(repoDir, 0o755); err != nil {
					t.Fatalf("mkdir repo dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(repoDir, ConfigFileName), []byte(tc.repoCfg), 0o644); err != nil {
					t.Fatalf("write repo config: %v", err)
				}
			}

			got, err := ResolveModel(tc.explicit, repo)
			if err != nil {
				t.Fatalf("ResolveModel(%q, %q) returned error: %v", tc.explicit, repo, err)
			}
			if got.Value != tc.wantValue {
				t.Errorf("got.Value = %q, want %q", got.Value, tc.wantValue)
			}
			if got.Source != tc.wantSource {
				t.Errorf("got.Source = %q, want %q", got.Source, tc.wantSource)
			}
		})
	}
}

func TestResolveRejectsUnknownRuntime(t *testing.T) {
	resetRegistry(t)

	// Explicit unknown runtime
	if _, err := Resolve("unknown-runtime", t.TempDir()); err == nil {
		t.Fatal("expected Resolve to reject unknown explicit runtime")
	}

	// Repo config with unknown runtime
	repo := t.TempDir()
	repoCfgDir := filepath.Join(repo, ".pr-triage")
	if err := os.MkdirAll(repoCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoCfgDir, ConfigFileName), []byte("runtime: unknown-runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Resolve("", repo); err == nil {
		t.Fatal("expected Resolve to reject unknown runtime in repo config")
	}

	// User config with unknown runtime
	home := t.TempDir()
	t.Setenv("HOME", home)
	userCfgDir := filepath.Join(home, ".pr-triage")
	if err := os.MkdirAll(userCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userCfgDir, ConfigFileName), []byte("runtime: bad-runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Resolve("", t.TempDir()); err == nil {
		t.Fatal("expected Resolve to reject unknown runtime in user config")
	}
}

func TestResolveMalformedConfigFile(t *testing.T) {
	resetRegistry(t)

	repo := t.TempDir()
	repoCfgDir := filepath.Join(repo, ".pr-triage")
	if err := os.MkdirAll(repoCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoCfgDir, ConfigFileName), []byte("runtime: [invalid yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Resolve("", repo); err == nil {
		t.Fatal("expected Resolve to return error on malformed YAML")
	}
}
