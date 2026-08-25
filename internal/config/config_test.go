package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/report"
)

func TestConfigLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".pr-triage", "config.yaml")

	cfg := &Config{
		BaseRef:      "release/*",
		PollInterval: "2m",
		Timeout:      "15m",
		GitHubUser:   "alice",
		Runtime:      "claude-code",
		Model:        "sonnet",
	}

	if err := Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.BaseRef != cfg.BaseRef || loaded.PollInterval != cfg.PollInterval || loaded.Timeout != cfg.Timeout || loaded.GitHubUser != cfg.GitHubUser || loaded.Runtime != cfg.Runtime || loaded.Model != cfg.Model {
		t.Fatalf("loaded config %+v does not match saved %+v", loaded, cfg)
	}
}

// TestLoadMergesDefaults guards the fix for the dogfood-surfaced bug where a
// partial config (as written by `init`, lacking signal_tiers/routing/worktree_ttl)
// loaded with empty Routing, causing every PR to hard-fail to escalate via
// ErrUnmappedTier. Load must layer the file over DefaultConfig so absent sections
// keep their defaults.
func TestLoadMergesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Minimal config with no signal_tiers, routing, or worktree_ttl — exactly
	// what `init --non-interactive` writes today.
	partial := []byte("base_ref: chunk/**\npoll_interval: 5m\ntimeout: 10m\ngithub_user: dustinmays\nruntime: claude-code\nmodel: claude-haiku-4-5\n")
	if err := os.WriteFile(cfgPath, partial, 0644); err != nil {
		t.Fatalf("write partial config: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// File values win.
	if loaded.BaseRef != "chunk/**" || loaded.GitHubUser != "dustinmays" {
		t.Errorf("file values not honored: %+v", loaded)
	}
	// Absent sections inherit defaults.
	if loaded.WorktreeTTL != "72h" {
		t.Errorf("WorktreeTTL = %q, want default %q", loaded.WorktreeTTL, "72h")
	}
	if loaded.SignalTiers.DefaultTier != "routine" || len(loaded.SignalTiers.Rules) == 0 {
		t.Errorf("SignalTiers not defaulted: %+v", loaded.SignalTiers)
	}
	// The routine tier must route to a real agent, not ErrUnmappedTier.
	r, err := loaded.Route("routine")
	if err != nil {
		t.Fatalf("Route(routine) after partial load: %v", err)
	}
	if r.Runtime != "claude-code" || r.AgentDef != "review-agent" {
		t.Errorf("routine routing not defaulted: %+v", r)
	}
}

func TestClassifyAndRoute_Fixtures(t *testing.T) {
	cfg := DefaultConfig()

	// 1. Valid report fixture (all present: false) -> routine tier -> review agent
	validData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "valid.json"))
	if err != nil {
		t.Fatalf("read valid.json: %v", err)
	}
	validRep, err := report.ParseAndValidate(validData)
	if err != nil {
		t.Fatalf("parse valid.json: %v", err)
	}

	tier := cfg.Classify(validRep)
	if tier != "routine" {
		t.Errorf("cfg.Classify(validRep) = %q, want %q", tier, "routine")
	}

	routing, err := cfg.Route(tier)
	if err != nil {
		t.Fatalf("cfg.Route(%q) failed: %v", tier, err)
	}
	if routing.Runtime != "claude-code" || routing.Model != "claude-haiku-4-5" || routing.AgentDef != "review-agent" {
		t.Errorf("unexpected routing for routine tier: %+v", routing)
	}

	// 2. High risk report fixture (schema_changed_without_migration present) -> escalate tier
	highRiskData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "high-risk.json"))
	if err != nil {
		t.Fatalf("read high-risk.json: %v", err)
	}
	highRiskRep, err := report.ParseAndValidate(highRiskData)
	if err != nil {
		t.Fatalf("parse high-risk.json: %v", err)
	}

	highTier := cfg.Classify(highRiskRep)
	if highTier != "escalate" {
		t.Errorf("cfg.Classify(highRiskRep) = %q, want %q", highTier, "escalate")
	}

	highRouting, err := cfg.Route(highTier)
	if err != nil {
		t.Fatalf("cfg.Route(%q) failed: %v", highTier, err)
	}
	if highRouting.Runtime != "escalate" {
		t.Errorf("unexpected routing for escalate tier: %+v", highRouting)
	}

	// 3. Chunk completion report fixture (target_kind == chunk_completion) -> human tier
	chunkCompData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "chunk-completion.json"))
	if err != nil {
		t.Fatalf("read chunk-completion.json: %v", err)
	}
	chunkCompRep, err := report.ParseAndValidate(chunkCompData)
	if err != nil {
		t.Fatalf("parse chunk-completion.json: %v", err)
	}

	chunkTier := cfg.Classify(chunkCompRep)
	if chunkTier != "human" {
		t.Errorf("cfg.Classify(chunkCompRep) = %q, want %q", chunkTier, "human")
	}

	chunkRouting, err := cfg.Route(chunkTier)
	if err != nil {
		t.Fatalf("cfg.Route(%q) failed: %v", chunkTier, err)
	}
	if chunkRouting.Runtime != "escalate" || chunkRouting.AgentDef != "human-review" {
		t.Errorf("unexpected routing for human tier: %+v", chunkRouting)
	}

	// 4. Table-driven signal tests for all 10 escalation signals
	signals := []string{
		"migration_sql_added",
		"migration_history_rewritten",
		"schema_changed_without_migration",
		"dependency_manifest_changed",
		"install_execution_allowed",
		"test_files_deleted",
		"tests_skipped_added",
		"safeguard_config_changed",
		"suppressions_added",
		"workflow_changed",
		"stack_choice_changed",
	}

	for _, sigID := range signals {
		t.Run("signal_"+sigID, func(t *testing.T) {
			rep := &report.Report{
				PR: report.PRInfo{
					Number:     99,
					Title:      "Test PR",
					Base:       "chunk/issue-1",
					Head:       "feat/issue-1",
					TargetKind: "implementation",
				},
				CI: report.CIInfo{Status: "passing"},
				Signals: []report.Signal{
					{
						ID:      sigID,
						Present: true,
						Evidence: []report.Evidence{
							{Detail: "test evidence"},
						},
					},
				},
			}
			gotTier := cfg.Classify(rep)
			if gotTier != "escalate" {
				t.Errorf("cfg.Classify with signal %s = %q, want %q", sigID, gotTier, "escalate")
			}
		})
	}

	// 5. Unmapped tier -> ErrUnmappedTier
	_, err = cfg.Route("unknown-tier")
	if !errors.Is(err, ErrUnmappedTier) {
		t.Errorf("expected ErrUnmappedTier for unknown tier, got %v", err)
	}
}

func TestSignalTiers_CustomYAMLReload(t *testing.T) {
	customYAML := `
signal_tiers:
  default_tier: standard
  rules:
    - tier: emergency
      signals:
        - destructive_db_operation
    - tier: caution
      signals:
        - adr_modified

routing:
  standard:
    runtime: opencode
    model: standard-model
    agent_def: standard-agent
  caution:
    runtime: opencode
    model: careful-model
    agent_def: review-agent
  emergency:
    runtime: claude-code
    model: claude-3-7-sonnet
    agent_def: emergency-fixer
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(customYAML), 0644); err != nil {
		t.Fatalf("write custom yaml: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load custom config: %v", err)
	}

	// Test default classification
	repNone := &report.Report{
		Signals: []report.Signal{
			{ID: "other_signal", Present: false},
		},
	}
	if got := cfg.Classify(repNone); got != "standard" {
		t.Errorf("Classify(repNone) = %q, want standard", got)
	}

	// Test emergency classification
	repEmergency := &report.Report{
		Signals: []report.Signal{
			{ID: "destructive_db_operation", Present: true},
		},
	}
	if got := cfg.Classify(repEmergency); got != "emergency" {
		t.Errorf("Classify(repEmergency) = %q, want emergency", got)
	}

	rEmergency, err := cfg.Route("emergency")
	if err != nil {
		t.Fatalf("Route(emergency) error: %v", err)
	}
	if rEmergency.Runtime != "claude-code" || rEmergency.Model != "claude-3-7-sonnet" || rEmergency.AgentDef != "emergency-fixer" {
		t.Errorf("unexpected routing: %+v", rEmergency)
	}

	// Unmapped tier
	if _, err := cfg.Route("nonexistent"); !errors.Is(err, ErrUnmappedTier) {
		t.Errorf("expected ErrUnmappedTier, got %v", err)
	}
}

func TestParseGitRemote(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			url:       "git@github.com:dustinmays/pr-triage.git",
			wantOwner: "dustinmays",
			wantRepo:  "pr-triage",
		},
		{
			url:       "git@github.com:dustinmays/pr-triage",
			wantOwner: "dustinmays",
			wantRepo:  "pr-triage",
		},
		{
			url:       "https://github.com/dustinmays/pr-triage.git",
			wantOwner: "dustinmays",
			wantRepo:  "pr-triage",
		},
		{
			url:       "https://github.com/dustinmays/pr-triage",
			wantOwner: "dustinmays",
			wantRepo:  "pr-triage",
		},
		{
			url:       "ssh://git@github.com/dustinmays/pr-triage.git",
			wantOwner: "dustinmays",
			wantRepo:  "pr-triage",
		},
		{
			url:     "",
			wantErr: true,
		},
		{
			url:     "invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, err := ParseGitRemote(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil (%s/%s)", tt.url, owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.url, err)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Fatalf("ParseGitRemote(%q) = (%q, %q), want (%q, %q)", tt.url, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestDetectRepoFromGit(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo and add remote
	cmd := exec.Command("git", "init", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}

	cmd = exec.Command("git", "-C", tmpDir, "remote", "add", "origin", "git@github.com:testowner/testrepo.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add failed: %v (%s)", err, string(out))
	}

	owner, repo, err := DetectRepoFromGit(tmpDir)
	if err != nil {
		t.Fatalf("DetectRepoFromGit failed: %v", err)
	}
	if owner != "testowner" || repo != "testrepo" {
		t.Fatalf("DetectRepoFromGit = (%q, %q), want (testowner, testrepo)", owner, repo)
	}
}
