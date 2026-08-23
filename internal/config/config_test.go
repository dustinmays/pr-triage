package config

import (
	"os/exec"
	"path/filepath"
	"testing"
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
