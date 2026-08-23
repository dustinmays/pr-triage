package db

// Repo records a watched GitHub repository.
type Repo struct {
	ID           int64  `db:"id"`
	Owner        string `db:"owner"`
	Name         string `db:"name"`
	BaseRef      string `db:"base_ref"`
	PollInterval string `db:"poll_interval"`
	ConfigPath   string `db:"config_path"`
	CreatedAt    string `db:"created_at"`
}

// PR records the tracked state of a pull request.
type PR struct {
	ID        int64  `db:"id"`
	RepoID    int64  `db:"repo_id"`
	Number    int    `db:"number"`
	HeadSHA   string `db:"head_sha"`
	LastRunID *int64 `db:"last_run_id"`
	State     string `db:"state"`
	UpdatedAt string `db:"updated_at"`
}

// Run records a single agent execution for a pull request.
type Run struct {
	ID           int64   `db:"id"`
	PRID         int64   `db:"pr_id"`
	HeadSHA      string  `db:"head_sha"`
	CIRunID      *int64  `db:"ci_run_id"`
	RiskTier     string  `db:"risk_tier"`
	Runtime      string  `db:"runtime"`
	Model        string  `db:"model"`
	ModelSource  string  `db:"model_source"`
	CostUSD      float64 `db:"cost_usd"`
	CostBasis    string  `db:"cost_basis"`
	Turns        int     `db:"turns"`
	Status       string  `db:"status"`
	StopReason   string  `db:"stop_reason"`
	PID          *int    `db:"pid"`
	LogPath      string  `db:"log_path"`
	WorktreePath string  `db:"worktree_path"`
	StartedAt    string  `db:"started_at"`
	FinishedAt   *string `db:"finished_at"`
}
