package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/git"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	ctx := context.Background()
	if _, err := git.Run(ctx, dir, "init", "-b", "main"); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if _, err := git.Run(ctx, dir, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}
	if _, err := git.Run(ctx, dir, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}

	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := git.Run(ctx, dir, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(ctx, dir, "commit", "-m", "initial commit"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	return dir
}

func TestGit_WorktreeAddRemoveList(t *testing.T) {
	repoDir := initTestGitRepo(t)
	ctx := context.Background()

	wtDir := filepath.Join(t.TempDir(), "wt-test")
	if err := git.WorktreeAdd(ctx, repoDir, wtDir, "HEAD"); err != nil {
		t.Fatalf("WorktreeAdd failed: %v", err)
	}

	worktrees, err := git.Worktrees(ctx, repoDir)
	if err != nil {
		t.Fatalf("Worktrees list failed: %v", err)
	}
	if len(worktrees) < 2 {
		t.Fatalf("expected at least 2 worktrees, got %d", len(worktrees))
	}

	// Make changes in worktree
	subFile := filepath.Join(wtDir, "fix.txt")
	if err := os.WriteFile(subFile, []byte("fixed\n"), 0644); err != nil {
		t.Fatalf("write subfile: %v", err)
	}

	hasChanges, err := git.HasChanges(ctx, wtDir)
	if err != nil {
		t.Fatalf("HasChanges failed: %v", err)
	}
	if !hasChanges {
		t.Errorf("expected HasChanges to be true")
	}

	// Commit changes in worktree
	sha, err := git.CommitAndPush(ctx, wtDir, "", "apply automated fix")
	if err != nil {
		t.Fatalf("CommitAndPush failed: %v", err)
	}
	if sha == "" {
		t.Errorf("expected non-empty commit sha")
	}

	// Remove worktree
	if err := git.WorktreeRemove(ctx, repoDir, wtDir); err != nil {
		t.Fatalf("WorktreeRemove failed: %v", err)
	}
}
