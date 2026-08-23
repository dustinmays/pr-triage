package db

import "database/sql"

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
}
