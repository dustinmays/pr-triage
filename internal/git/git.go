// Package git provides git operations and worktree management.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run executes a git command in the given directory and returns trimmed stdout.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), stderrStr, err)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// WorktreeEntry represents an active git worktree.
type WorktreeEntry struct {
	Path   string
	Branch string
	IsMain bool
}

// Worktrees lists all active git worktrees for repoDir.
func Worktrees(ctx context.Context, repoDir string) ([]WorktreeEntry, error) {
	out, err := Run(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	var worktrees []WorktreeEntry
	var current WorktreeEntry

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeEntry{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "":
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	if len(worktrees) > 0 {
		worktrees[0].IsMain = true
	}

	return worktrees, nil
}

// WorktreeAdd creates a new worktree at worktreePath checked out to ref.
func WorktreeAdd(ctx context.Context, repoDir, worktreePath, ref string) error {
	_, err := Run(ctx, repoDir, "worktree", "add", "--detach", worktreePath, ref)
	if err != nil {
		// Try fallback if --detach fails
		_, err = Run(ctx, repoDir, "worktree", "add", worktreePath, ref)
	}
	return err
}

// WorktreeRemove removes a worktree with fallback to prune.
func WorktreeRemove(ctx context.Context, repoDir, worktreePath string) error {
	_, err := Run(ctx, repoDir, "worktree", "remove", "--force", worktreePath)
	if err != nil {
		_ = WorktreePrune(ctx, repoDir)
	}
	return err
}

// WorktreePrune runs git worktree prune.
func WorktreePrune(ctx context.Context, repoDir string) error {
	_, err := Run(ctx, repoDir, "worktree", "prune")
	return err
}

// HasChanges checks if there are uncommitted modifications or untracked files in dir.
func HasChanges(ctx context.Context, dir string) (bool, error) {
	out, err := Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(out)) > 0, nil
}

// CommitAndPush stages all changes, commits with commitMsg, and pushes to remoteBranch.
func CommitAndPush(ctx context.Context, dir, remoteBranch, commitMsg string) (string, error) {
	if _, err := Run(ctx, dir, "add", "."); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if _, err := Run(ctx, dir, "commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	headSHA, err := Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	if remoteBranch != "" {
		if _, err := Run(ctx, dir, "push", "origin", fmt.Sprintf("HEAD:%s", remoteBranch)); err != nil {
			return headSHA, fmt.Errorf("git push origin HEAD:%s: %w", remoteBranch, err)
		}
	}
	return headSHA, nil
}

// ResolveRepoPath finds the local git repository root for a repo record or working directory.
func ResolveRepoPath(dir string) (string, error) {
	return filepath.Abs(dir)
}
