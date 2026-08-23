package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the per-repository or global pr-triage configuration.
type Config struct {
	BaseRef      string `yaml:"base_ref,omitempty"`
	PollInterval string `yaml:"poll_interval,omitempty"`
	Timeout      string `yaml:"timeout,omitempty"`
	GitHubUser   string `yaml:"github_user,omitempty"`
	Runtime      string `yaml:"runtime,omitempty"`
	Model        string `yaml:"model,omitempty"`
}

// DefaultConfig returns default configuration values.
func DefaultConfig() *Config {
	return &Config{
		BaseRef:      "main",
		PollInterval: "5m",
		Timeout:      "10m",
	}
}

// Load loads configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes configuration to a YAML file, creating parent directories if needed.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// ParseGitRemote extracts the owner and repository name from a git remote URL.
// Supports HTTPS, SSH, and SCP-like URL styles.
func ParseGitRemote(rawURL string) (string, string, error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return "", "", fmt.Errorf("empty git remote url")
	}

	raw = strings.TrimSuffix(raw, ".git")

	// SCP-like: git@github.com:owner/repo
	if strings.Contains(raw, "@") && strings.Contains(raw, ":") && !strings.Contains(raw, "://") {
		parts := strings.SplitN(raw, ":", 2)
		pathParts := strings.Split(parts[1], "/")
		if len(pathParts) == 2 && pathParts[0] != "" && pathParts[1] != "" {
			return pathParts[0], pathParts[1], nil
		}
	}

	// URL-like: https://github.com/owner/repo or ssh://git@github.com/owner/repo
	if strings.Contains(raw, "://") {
		parts := strings.SplitN(raw, "://", 2)
		pathAndHost := parts[1]
		slashIdx := strings.Index(pathAndHost, "/")
		if slashIdx != -1 {
			pathParts := strings.Split(strings.TrimPrefix(pathAndHost[slashIdx+1:], "/"), "/")
			if len(pathParts) >= 2 && pathParts[0] != "" && pathParts[1] != "" {
				return pathParts[0], pathParts[1], nil
			}
		}
	}

	// Fallback regex pattern
	re := regexp.MustCompile(`[:/]([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)$`)
	matches := re.FindStringSubmatch(raw)
	if len(matches) == 3 {
		return matches[1], matches[2], nil
	}

	return "", "", fmt.Errorf("could not parse git remote: %s", rawURL)
}

// DetectRepoFromGit inspects git remotes in dir to discover owner and repository name.
func DetectRepoFromGit(dir string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Fallback to git config
		cmd2 := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get", "remote.origin.url")
		stdout.Reset()
		cmd2.Stdout = &stdout
		if err2 := cmd2.Run(); err2 != nil {
			return "", "", fmt.Errorf("detect git remote origin in %s: %w (%s)", dir, err, strings.TrimSpace(stderr.String()))
		}
	}

	remoteURL := strings.TrimSpace(stdout.String())
	return ParseGitRemote(remoteURL)
}
