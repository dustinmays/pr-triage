package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestGit_WorktreeSweep(t *testing.T) {
	repoDir := initTestGitRepo(t)
	ctx := context.Background()

	staleDir := filepath.Join(t.TempDir(), "wt-stale")
	if err := git.WorktreeAdd(ctx, repoDir, staleDir, "HEAD"); err != nil {
		t.Fatalf("WorktreeAdd failed: %v", err)
	}

	// Change modtime of staleDir to 5 days ago
	oldTime := time.Now().Add(-5 * 24 * time.Hour)
	_ = os.Chtimes(staleDir, oldTime, oldTime)

	swept, err := git.WorktreeSweep(ctx, repoDir, 72*time.Hour)
	if err != nil {
		t.Fatalf("WorktreeSweep failed: %v", err)
	}
	if len(swept) != 1 {
		t.Fatalf("expected 1 swept worktree, got %v", swept)
	}

	staleReal, _ := filepath.EvalSymlinks(staleDir)
	sweptReal, _ := filepath.EvalSymlinks(swept[0])
	if staleReal != sweptReal {
		t.Fatalf("expected swept %s, got %s", staleReal, sweptReal)
	}

	// Confirm worktree was removed
	worktrees, _ := git.Worktrees(ctx, repoDir)
	for _, wt := range worktrees {
		if wt.Path == staleDir {
			t.Errorf("stale worktree was not removed")
		}
	}
}
