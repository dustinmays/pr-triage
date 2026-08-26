package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Store wraps a database connection and provides persistence operations.
type Store struct {
	db *sqlx.DB
}

// NewStore creates a new Store wrapping db.
func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying sqlx.DB connection.
func (s *Store) DB() *sqlx.DB {
	return s.db
}

// UpsertRepo inserts a new repository or updates an existing repository
// matching (owner, name).
func (s *Store) UpsertRepo(repo *Repo) (*Repo, error) {
	return UpsertRepo(s.db, repo)
}

// ListRepos returns all tracked repositories ordered by ID ascending.
func (s *Store) ListRepos() ([]Repo, error) {
	return ListRepos(s.db)
}

// UpsertPRState inserts or updates the tracked state of a pull request
// identified by (repoID, number).
func (s *Store) UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*PR, error) {
	return UpsertPRState(s.db, repoID, number, headSHA, runID, state)
}

// GetPRState retrieves the current state of a pull request by repo ID and number.
func (s *Store) GetPRState(repoID int64, number int) (*PR, error) {
	return GetPRState(s.db, repoID, number)
}

// RecordRun inserts a new agent execution run record.
func (s *Store) RecordRun(run *Run) (*Run, error) {
	return RecordRun(s.db, run)
}

// UpdateRun updates an existing agent execution run record.
func (s *Store) UpdateRun(run *Run) error {
	return UpdateRun(s.db, run)
}

// ListRuns returns runs ordered by newest first (descending ID), up to limit records.
// If limit <= 0, all runs are returned.
func (s *Store) ListRuns(limit int) ([]Run, error) {
	return ListRuns(s.db, limit)
}

// RunsInState returns all runs matching the given status, ordered newest first.
func (s *Store) RunsInState(state string) ([]Run, error) {
	return RunsInState(s.db, state)
}

// RecordOverride inserts a human override waiving escalate-tier signals on a PR
// at a specific head SHA.
func (s *Store) RecordOverride(ov *Override) (*Override, error) {
	return RecordOverride(s.db, ov)
}

// GetActiveOverride returns the most recent unconsumed override for a PR at the
// given head SHA, or ErrNotFound when none is active.
func (s *Store) GetActiveOverride(repoID int64, prNumber int, headSHA string) (*Override, error) {
	return GetActiveOverride(s.db, repoID, prNumber, headSHA)
}

// MarkOverrideConsumed stamps consumed_at on an override so it applies once.
func (s *Store) MarkOverrideConsumed(id int64) error {
	return MarkOverrideConsumed(s.db, id)
}

// UpsertRepo inserts a new repository or updates an existing repository
// matching (owner, name). It returns the inserted or updated Repo.
func UpsertRepo(db *sqlx.DB, repo *Repo) (*Repo, error) {
	if repo == nil {
		return nil, fmt.Errorf("db: repo cannot be nil")
	}

	var existing Repo
	err := db.Get(&existing, "SELECT id, owner, name, base_ref, poll_interval, config_path, created_at FROM repos WHERE owner = ? AND name = ?", repo.Owner, repo.Name)
	if err == nil {
		_, err = db.Exec(
			"UPDATE repos SET base_ref = ?, poll_interval = ?, config_path = ? WHERE id = ?",
			repo.BaseRef, repo.PollInterval, repo.ConfigPath, existing.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("db: update repo: %w", err)
		}
		var updated Repo
		if err := db.Get(&updated, "SELECT id, owner, name, base_ref, poll_interval, config_path, created_at FROM repos WHERE id = ?", existing.ID); err != nil {
			return nil, fmt.Errorf("db: get updated repo: %w", err)
		}
		return &updated, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("db: query repo: %w", err)
	}

	res, err := db.Exec(
		"INSERT INTO repos (owner, name, base_ref, poll_interval, config_path) VALUES (?, ?, ?, ?, ?)",
		repo.Owner, repo.Name, repo.BaseRef, repo.PollInterval, repo.ConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("db: insert repo: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("db: last insert id: %w", err)
	}

	var created Repo
	if err := db.Get(&created, "SELECT id, owner, name, base_ref, poll_interval, config_path, created_at FROM repos WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("db: get created repo: %w", err)
	}
	return &created, nil
}

// ListRepos returns all tracked repositories ordered by ID ascending.
func ListRepos(db *sqlx.DB) ([]Repo, error) {
	repos := make([]Repo, 0)
	err := db.Select(&repos, "SELECT id, owner, name, base_ref, poll_interval, config_path, created_at FROM repos ORDER BY id ASC")
	if err != nil {
		return nil, fmt.Errorf("db: list repos: %w", err)
	}
	return repos, nil
}

// UpsertPRState inserts or updates the tracked state of a pull request
// identified by (repoID, number). It is idempotent on (repo_id, number).
func UpsertPRState(db *sqlx.DB, repoID int64, number int, headSHA string, runID *int64, state string) (*PR, error) {
	query := `
		INSERT INTO prs (repo_id, number, head_sha, last_run_id, state, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(repo_id, number) DO UPDATE SET
			head_sha = excluded.head_sha,
			last_run_id = excluded.last_run_id,
			state = excluded.state,
			updated_at = excluded.updated_at
	`
	_, err := db.Exec(query, repoID, number, headSHA, runID, state)
	if err != nil {
		return nil, fmt.Errorf("db: upsert pr state: %w", err)
	}

	var pr PR
	if err := db.Get(&pr, "SELECT id, repo_id, number, head_sha, last_run_id, state, updated_at FROM prs WHERE repo_id = ? AND number = ?", repoID, number); err != nil {
		return nil, fmt.Errorf("db: get upserted pr: %w", err)
	}
	return &pr, nil
}

// GetPRState retrieves the current state of a pull request by repo ID and number.
// If the PR is not found, it returns ErrNotFound.
func GetPRState(db *sqlx.DB, repoID int64, number int) (*PR, error) {
	var pr PR
	err := db.Get(&pr, "SELECT id, repo_id, number, head_sha, last_run_id, state, updated_at FROM prs WHERE repo_id = ? AND number = ?", repoID, number)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: get pr state: %w", err)
	}
	return &pr, nil
}

// RecordRun inserts a new agent execution run record and updates the
// associated PR's last_run_id. It validates that cost_basis is non-empty.
func RecordRun(db *sqlx.DB, run *Run) (*Run, error) {
	if run == nil {
		return nil, fmt.Errorf("db: run cannot be nil")
	}
	if run.CostBasis == "" {
		return nil, fmt.Errorf("db: cost_basis is required")
	}

	query := `
		INSERT INTO runs (
			pr_id, head_sha, ci_run_id, risk_tier, runtime, model, model_source,
			cost_usd, cost_basis, turns, status, stop_reason, pid, log_path,
			worktree_path, started_at, finished_at
		) VALUES (
			:pr_id, :head_sha, :ci_run_id, :risk_tier, :runtime, :model, :model_source,
			:cost_usd, :cost_basis, :turns, :status, :stop_reason, :pid, :log_path,
			:worktree_path, COALESCE(NULLIF(:started_at, ''), datetime('now')), :finished_at
		)
	`
	res, err := db.NamedExec(query, run)
	if err != nil {
		return nil, fmt.Errorf("db: record run: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("db: last insert id: %w", err)
	}

	// Update associated PR last_run_id
	_, _ = db.Exec("UPDATE prs SET last_run_id = ?, updated_at = datetime('now') WHERE id = ?", id, run.PRID)

	var recorded Run
	if err := db.Get(&recorded, "SELECT id, pr_id, head_sha, ci_run_id, risk_tier, runtime, model, model_source, cost_usd, cost_basis, turns, status, stop_reason, pid, log_path, worktree_path, started_at, finished_at FROM runs WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("db: get recorded run: %w", err)
	}
	return &recorded, nil
}

// UpdateRun updates an existing agent execution run record.
// If the run is not found, it returns ErrNotFound.
func UpdateRun(db *sqlx.DB, run *Run) error {
	if run == nil {
		return fmt.Errorf("db: run cannot be nil")
	}
	if run.ID <= 0 {
		return fmt.Errorf("db: cannot update run without valid ID")
	}
	if run.CostBasis == "" {
		return fmt.Errorf("db: cost_basis is required")
	}

	query := `
		UPDATE runs SET
			pr_id = :pr_id,
			head_sha = :head_sha,
			ci_run_id = :ci_run_id,
			risk_tier = :risk_tier,
			runtime = :runtime,
			model = :model,
			model_source = :model_source,
			cost_usd = :cost_usd,
			cost_basis = :cost_basis,
			turns = :turns,
			status = :status,
			stop_reason = :stop_reason,
			pid = :pid,
			log_path = :log_path,
			worktree_path = :worktree_path,
			started_at = :started_at,
			finished_at = :finished_at
		WHERE id = :id
	`
	res, err := db.NamedExec(query, run)
	if err != nil {
		return fmt.Errorf("db: update run %d: %w", run.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("db: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("db: update run %d: %w", run.ID, ErrNotFound)
	}

	return nil
}

// ListRuns returns runs ordered by newest first (descending ID), up to limit records.
// If limit <= 0, all runs are returned.
func ListRuns(db *sqlx.DB, limit int) ([]Run, error) {
	runs := make([]Run, 0)
	var err error
	if limit > 0 {
		query := `
			SELECT r.id, r.pr_id, p.number AS pr_number, r.head_sha, r.ci_run_id, r.risk_tier,
			       r.runtime, r.model, r.model_source, r.cost_usd, r.cost_basis, r.turns,
			       r.status, r.stop_reason, r.pid, r.log_path, r.worktree_path,
			       r.started_at, r.finished_at
			FROM runs r
			LEFT JOIN prs p ON p.id = r.pr_id
			ORDER BY r.id DESC
			LIMIT ?
		`
		err = db.Select(&runs, query, limit)
	} else {
		query := `
			SELECT r.id, r.pr_id, p.number AS pr_number, r.head_sha, r.ci_run_id, r.risk_tier,
			       r.runtime, r.model, r.model_source, r.cost_usd, r.cost_basis, r.turns,
			       r.status, r.stop_reason, r.pid, r.log_path, r.worktree_path,
			       r.started_at, r.finished_at
			FROM runs r
			LEFT JOIN prs p ON p.id = r.pr_id
			ORDER BY r.id DESC
		`
		err = db.Select(&runs, query)
	}
	if err != nil {
		return nil, fmt.Errorf("db: list runs: %w", err)
	}
	return runs, nil
}

// RecordOverride inserts a human override row and returns it with its assigned ID.
func RecordOverride(db *sqlx.DB, ov *Override) (*Override, error) {
	if ov == nil {
		return nil, fmt.Errorf("db: override cannot be nil")
	}
	if ov.PRNumber <= 0 || ov.HeadSHA == "" {
		return nil, fmt.Errorf("db: override requires a pr_number and head_sha")
	}
	res, err := db.Exec(
		`INSERT INTO overrides (repo_id, pr_number, head_sha, waived_signals, reason)
		 VALUES (?, ?, ?, ?, ?)`,
		ov.RepoID, ov.PRNumber, ov.HeadSHA, ov.WaivedSignals, ov.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("db: record override: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("db: override last insert id: %w", err)
	}
	ov.ID = id
	return ov, nil
}

// GetActiveOverride returns the most recent unconsumed override matching
// (repoID, prNumber, headSHA). Pinning on head SHA means a new push, which
// changes the SHA, leaves no active override — the PR re-escalates rather than
// riding a stale waiver. Returns ErrNotFound when none is active.
func GetActiveOverride(db *sqlx.DB, repoID int64, prNumber int, headSHA string) (*Override, error) {
	var ov Override
	err := db.Get(&ov,
		`SELECT id, repo_id, pr_number, head_sha, waived_signals, reason, created_at, consumed_at
		 FROM overrides
		 WHERE repo_id = ? AND pr_number = ? AND head_sha = ? AND consumed_at IS NULL
		 ORDER BY id DESC
		 LIMIT 1`,
		repoID, prNumber, headSHA,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: get active override: %w", err)
	}
	return &ov, nil
}

// MarkOverrideConsumed sets consumed_at on an override, making it one-shot.
func MarkOverrideConsumed(db *sqlx.DB, id int64) error {
	if _, err := db.Exec(
		`UPDATE overrides SET consumed_at = datetime('now') WHERE id = ? AND consumed_at IS NULL`,
		id,
	); err != nil {
		return fmt.Errorf("db: mark override consumed: %w", err)
	}
	return nil
}

// RunsInState returns all runs matching the given status, ordered newest first.
func RunsInState(db *sqlx.DB, state string) ([]Run, error) {
	runs := make([]Run, 0)
	query := `
		SELECT id, pr_id, head_sha, ci_run_id, risk_tier, runtime, model, model_source,
		       cost_usd, cost_basis, turns, status, stop_reason, pid, log_path,
		       worktree_path, started_at, finished_at
		FROM runs
		WHERE status = ?
		ORDER BY id DESC
	`
	if err := db.Select(&runs, query, state); err != nil {
		return nil, fmt.Errorf("db: runs in state %q: %w", state, err)
	}
	return runs, nil
}
