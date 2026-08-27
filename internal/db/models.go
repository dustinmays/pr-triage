package db

import (
	"database/sql"
	"strings"
)

// ErrNotFound indicates that a requested database entity was not found.
var ErrNotFound = sql.ErrNoRows

// Repo records a watched GitHub repository.
type Repo struct {
	ID           int64  `db:"id" json:"id"`
	Owner        string `db:"owner" json:"owner"`
	Name         string `db:"name" json:"name"`
	BaseRef      string `db:"base_ref" json:"base_ref"`
	PollInterval string `db:"poll_interval" json:"poll_interval"`
	ConfigPath   string `db:"config_path" json:"config_path"`
	CreatedAt    string `db:"created_at" json:"created_at"`
}

// PR records the tracked state of a pull request.
type PR struct {
	ID        int64  `db:"id" json:"id"`
	RepoID    int64  `db:"repo_id" json:"repo_id"`
	Number    int    `db:"number" json:"number"`
	HeadSHA   string `db:"head_sha" json:"head_sha"`
	LastRunID *int64 `db:"last_run_id" json:"last_run_id"`
	State     string `db:"state" json:"state"`
	UpdatedAt string `db:"updated_at" json:"updated_at"`
}

// Override records a human decision to waive escalate-tier signals on a PR at a
// specific head SHA, letting the review agent run instead of escalating. It is
// pinned to head_sha so a new push invalidates it (state-first; ADR 0006).
type Override struct {
	ID            int64   `db:"id" json:"id"`
	RepoID        int64   `db:"repo_id" json:"repo_id"`
	PRNumber      int     `db:"pr_number" json:"pr_number"`
	HeadSHA       string  `db:"head_sha" json:"head_sha"`
	WaivedSignals string  `db:"waived_signals" json:"waived_signals"`
	Reason        string  `db:"reason" json:"reason"`
	CreatedAt     string  `db:"created_at" json:"created_at"`
	ConsumedAt    *string `db:"consumed_at" json:"consumed_at"`
}

// WaivedSignalList parses WaivedSignals into a slice of signal IDs. An empty
// WaivedSignals returns nil, which callers interpret as "waive all present
// escalate-tier signals".
func (o *Override) WaivedSignalList() []string {
	trimmed := strings.TrimSpace(o.WaivedSignals)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// WaivesAll reports whether the override waives every present escalate-tier
// signal (i.e. no specific signals were named).
func (o *Override) WaivesAll() bool {
	return len(o.WaivedSignalList()) == 0
}

// Run records a single agent execution for a pull request.
type Run struct {
	ID           int64   `db:"id" json:"id"`
	PRID         int64   `db:"pr_id" json:"pr_id"`
	HeadSHA      string  `db:"head_sha" json:"head_sha"`
	CIRunID      *int64  `db:"ci_run_id" json:"ci_run_id"`
	RiskTier     string  `db:"risk_tier" json:"risk_tier"`
	Runtime      string  `db:"runtime" json:"runtime"`
	Model        string  `db:"model" json:"model"`
	ModelSource  string  `db:"model_source" json:"model_source"`
	CostUSD      float64 `db:"cost_usd" json:"cost_usd"`
	CostBasis    string  `db:"cost_basis" json:"cost_basis"`
	Turns        int     `db:"turns" json:"turns"`
	Status       string  `db:"status" json:"status"`
	StopReason   string  `db:"stop_reason" json:"stop_reason"`
	PID          *int    `db:"pid" json:"pid"`
	LogPath      string  `db:"log_path" json:"log_path"`
	WorktreePath string  `db:"worktree_path" json:"worktree_path"`
	StartedAt    string  `db:"started_at" json:"started_at"`
	FinishedAt   *string `db:"finished_at" json:"finished_at"`

	// PRNumber is the GitHub PR number, joined from prs.number for display. It is
	// read-only (not a runs column) and is only populated by queries that join
	// prs (e.g. ListRuns); inserts/updates never write it.
	PRNumber int `db:"pr_number" json:"pr_number"`
}
