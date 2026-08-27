package orchestrator

// White-box test: buildReviewPrompt is unexported, so this file lives in
// package orchestrator (the black-box suite stays in orchestrator_test.go).

import (
	"strings"
	"testing"

	gh "github.com/google/go-github/v72/github"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/poller"
	"github.com/dustinmays/pr-triage/internal/report"
)

func TestBuildReviewPrompt(t *testing.T) {
	event := poller.ReportReadyEvent{
		Repo: db.Repo{
			Owner: "dustinmays",
			Name:  "pr-triage",
		},
		PRNumber: 123,
		HeadSHA:  "abcdef1234567890",
	}

	repWithSignals := &report.Report{
		PR: report.PRInfo{
			Number: 123,
			Title:  "Fix deterministic prompt issue",
			Base:   "main",
			Head:   "feature-123",
		},
		Signals: []report.Signal{
			{
				ID:      "schema_changed_without_migration",
				Present: true,
				Evidence: []report.Evidence{
					{
						File:   "src/schema.ts",
						Line:   gh.Ptr(42),
						Detail: "schema modified without migration script",
					},
				},
			},
			{
				ID:      "test_files_deleted",
				Present: true,
				Evidence: []report.Evidence{
					{
						File:   "tests/auth_test.go",
						Detail: "test file deleted",
					},
				},
			},
			{
				ID:      "workflow_changed",
				Present: false,
			},
		},
	}

	prompt := buildReviewPrompt(event, repWithSignals)

	if !strings.Contains(prompt, "PR #123") {
		t.Errorf("prompt missing PR number 'PR #123'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Fix deterministic prompt issue") {
		t.Errorf("prompt missing PR title; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "dustinmays/pr-triage") {
		t.Errorf("prompt missing repo 'dustinmays/pr-triage'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "schema_changed_without_migration") {
		t.Errorf("prompt missing signal 'schema_changed_without_migration'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "test_files_deleted") {
		t.Errorf("prompt missing signal 'test_files_deleted'; got:\n%s", prompt)
	}
	// A non-present signal must not appear as a bullet.
	if strings.Contains(prompt, "- workflow_changed") {
		t.Errorf("prompt should not list non-present signal 'workflow_changed'; got:\n%s", prompt)
	}
	// Evidence is rendered as "file:line — detail".
	if !strings.Contains(prompt, "src/schema.ts:42 — schema modified without migration script") {
		t.Errorf("prompt missing rendered evidence line; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Stay in this worktree") {
		t.Errorf("prompt missing 'Stay in this worktree'; got:\n%s", prompt)
	}
	// Diff hint uses the report's base branch.
	if !strings.Contains(prompt, "git diff main...HEAD") {
		t.Errorf("prompt missing base-aware diff hint 'git diff main...HEAD'; got:\n%s", prompt)
	}

	// A report with no present signals states so explicitly.
	repNoSignals := &report.Report{
		PR:      report.PRInfo{Number: 123, Title: "docs tweak", Base: "main"},
		Signals: []report.Signal{{ID: "workflow_changed", Present: false}},
	}
	if p := buildReviewPrompt(event, repNoSignals); !strings.Contains(p, "flagged no risk signals") {
		t.Errorf("prompt missing 'flagged no risk signals'; got:\n%s", p)
	}
}
