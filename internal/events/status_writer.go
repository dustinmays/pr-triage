package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dustinmays/pr-triage/internal/db"
)

// ActiveRunStatus represents an in-flight agent run.
type ActiveRunStatus struct {
	RepoOwner string    `json:"repo_owner"`
	RepoName  string    `json:"repo_name"`
	PRNumber  int       `json:"pr_number"`
	HeadSHA   string    `json:"head_sha"`
	Runtime   string    `json:"runtime"`
	Model     string    `json:"model"`
	StartedAt time.Time `json:"started_at"`
}

// PRStatusSummary represents current state for a tracked PR.
type PRStatusSummary struct {
	RepoOwner string    `json:"repo_owner"`
	RepoName  string    `json:"repo_name"`
	PRNumber  int       `json:"pr_number"`
	HeadSHA   string    `json:"head_sha"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StatusFile is the JSON document written to ~/.pr-triage/status.json.
type StatusFile struct {
	UpdatedAt  time.Time         `json:"updated_at"`
	DaemonPID  int               `json:"daemon_pid"`
	ActiveRuns []ActiveRunStatus `json:"active_runs"`
	RecentPRs  []PRStatusSummary `json:"recent_prs"`
	LastEvent  *Event            `json:"last_event,omitempty"`
}

// DefaultStatusPath returns ~/.pr-triage/status.json.
func DefaultStatusPath() string {
	return filepath.Join(db.DefaultDBDir(), "status.json")
}

// StatusFileWriter listens to events and persists the latest status to a JSON file atomically.
type StatusFileWriter struct {
	mu         sync.Mutex
	filePath   string
	activeRuns map[string]ActiveRunStatus
	recentPRs  map[string]PRStatusSummary
	lastEvent  *Event
}

// NewStatusFileWriter creates a StatusFileWriter writing to filePath.
func NewStatusFileWriter(filePath string) *StatusFileWriter {
	if filePath == "" {
		filePath = DefaultStatusPath()
	}
	return &StatusFileWriter{
		filePath:   filePath,
		activeRuns: make(map[string]ActiveRunStatus),
		recentPRs:  make(map[string]PRStatusSummary),
	}
}

// HandleEvent updates the internal state and writes the updated status file.
func (w *StatusFileWriter) HandleEvent(event Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	cp := event
	w.lastEvent = &cp

	key := fmt.Sprintf("%s/%s#%d", event.RepoOwner, event.RepoName, event.PRNumber)

	if event.PRNumber > 0 {
		w.recentPRs[key] = PRStatusSummary{
			RepoOwner: event.RepoOwner,
			RepoName:  event.RepoName,
			PRNumber:  event.PRNumber,
			HeadSHA:   event.HeadSHA,
			State:     event.State,
			UpdatedAt: event.Timestamp,
		}
	}

	switch event.Type {
	case EventAgentStarted:
		if event.PRNumber > 0 {
			w.activeRuns[key] = ActiveRunStatus{
				RepoOwner: event.RepoOwner,
				RepoName:  event.RepoName,
				PRNumber:  event.PRNumber,
				HeadSHA:   event.HeadSHA,
				Runtime:   event.Runtime,
				Model:     event.Model,
				StartedAt: event.Timestamp,
			}
		}
	case EventAgentFinished, EventEscalated:
		delete(w.activeRuns, key)
	}

	_ = w.writeAtomicLocked()
}

// ReadStatus reads and parses the status file from filePath.
func ReadStatus(filePath string) (*StatusFile, error) {
	if filePath == "" {
		filePath = DefaultStatusPath()
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read status file %s: %w", filePath, err)
	}

	var status StatusFile
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parse status file %s: %w", filePath, err)
	}

	return &status, nil
}

func (w *StatusFileWriter) writeAtomicLocked() error {
	dir := filepath.Dir(w.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create status dir: %w", err)
	}

	activeList := make([]ActiveRunStatus, 0, len(w.activeRuns))
	for _, ar := range w.activeRuns {
		activeList = append(activeList, ar)
	}

	recentList := make([]PRStatusSummary, 0, len(w.recentPRs))
	for _, pr := range w.recentPRs {
		recentList = append(recentList, pr)
	}

	statusDoc := StatusFile{
		UpdatedAt:  time.Now().UTC(),
		DaemonPID:  os.Getpid(),
		ActiveRuns: activeList,
		RecentPRs:  recentList,
		LastEvent:  w.lastEvent,
	}

	data, err := json.MarshalIndent(statusDoc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status doc: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", w.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp status file: %w", err)
	}

	if err := os.Rename(tmpPath, w.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp status file: %w", err)
	}

	return nil
}
