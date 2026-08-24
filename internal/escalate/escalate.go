// Package escalate provides a single, idempotent escalation mechanism
// for hard-fail conditions (malformed report, unknown schema version, unmapped risk tier).
package escalate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dustinmays/pr-triage/internal/db"
)

// DefaultEscalationLabel is applied to PRs that require human intervention.
const DefaultEscalationLabel = "needs-owner-review"

// GitHubClient defines the GitHub operations needed for escalation.
type GitHubClient interface {
	AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error
	CreateComment(ctx context.Context, owner, repo string, number int, body string) (int64, error)
}

// Store defines the SQLite persistence operations needed for escalation.
type Store interface {
	GetPRState(repoID int64, number int) (*db.PR, error)
	UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*db.PR, error)
	RecordRun(run *db.Run) (*db.Run, error)
}

// Request holds all parameters needed to escalate a pull request.
type Request struct {
	Repo       db.Repo
	PRNumber   int
	HeadSHA    string
	Reason     string
	CIRunID    *int64
	GitHubUser string
	Label      string
}

// Escalator executes idempotent pull request escalations.
type Escalator struct {
	store  Store
	client GitHubClient
}

// New creates a new Escalator instance.
func New(store Store, client GitHubClient) *Escalator {
	return &Escalator{
		store:  store,
		client: client,
	}
}

// Escalate applies the escalation label, posts an optional notification comment,
// and records the escalated state and reason in the database idempotently.
func (e *Escalator) Escalate(ctx context.Context, req Request) error {
	if req.PRNumber <= 0 || req.HeadSHA == "" {
		return errors.New("escalate: valid pr number and head sha are required")
	}

	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = DefaultEscalationLabel
	}

	// Idempotency check: if already escalated on this head SHA, return without duplicate actions.
	existing, err := e.store.GetPRState(req.Repo.ID, req.PRNumber)
	if err == nil && existing != nil && existing.State == "escalated" && existing.HeadSHA == req.HeadSHA {
		return nil
	}

	// 1. Apply escalation label to GitHub PR
	if err := e.client.AddLabels(ctx, req.Repo.Owner, req.Repo.Name, req.PRNumber, []string{label}); err != nil {
		return fmt.Errorf("escalate: add label %q: %w", label, err)
	}

	// 2. Post notification comment if user or reason provided
	var commentBody string
	user := strings.TrimPrefix(strings.TrimSpace(req.GitHubUser), "@")
	if user != "" {
		commentBody = fmt.Sprintf("⚠️ **pr-triage escalation**\n\nCc @%s\n\n**Reason:** %s", user, req.Reason)
	} else {
		commentBody = fmt.Sprintf("⚠️ **pr-triage escalation**\n\n**Reason:** %s", req.Reason)
	}

	if _, err := e.client.CreateComment(ctx, req.Repo.Owner, req.Repo.Name, req.PRNumber, commentBody); err != nil {
		return fmt.Errorf("escalate: create comment: %w", err)
	}

	// 3. Persist state in SQLite
	pr, err := e.store.UpsertPRState(req.Repo.ID, req.PRNumber, req.HeadSHA, req.CIRunID, "escalated")
	if err != nil {
		return fmt.Errorf("escalate: persist pr state: %w", err)
	}

	// 4. Record run row for audit trail
	run := &db.Run{
		PRID:        pr.ID,
		HeadSHA:     req.HeadSHA,
		CIRunID:     req.CIRunID,
		RiskTier:    "escalated",
		Runtime:     "none",
		Model:       "none",
		ModelSource: "escalation",
		CostUSD:     0.0,
		CostBasis:   "unavailable",
		Status:      "escalated",
		StopReason:  req.Reason,
	}
	if _, err := e.store.RecordRun(run); err != nil {
		return fmt.Errorf("escalate: record run row: %w", err)
	}

	return nil
}
