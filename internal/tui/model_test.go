package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/tui"
)

type mockStore struct {
	runs     []db.Run
	repos    []db.Repo
	prStates map[string]string
}

func (m *mockStore) ListRuns(limit int) ([]db.Run, error) {
	return m.runs, nil
}

func (m *mockStore) ListRepos() ([]db.Repo, error) {
	return m.repos, nil
}

func (m *mockStore) UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*db.PR, error) {
	if m.prStates == nil {
		m.prStates = make(map[string]string)
	}
	m.prStates[headSHA] = state
	return &db.PR{
		RepoID:  repoID,
		Number:  number,
		HeadSHA: headSHA,
		State:   state,
	}, nil
}

func TestTUI_EmptyState(t *testing.T) {
	m := tui.NewWithRuns([]db.Run{})
	view := m.View()

	if !strings.Contains(view, "No triage runs found") {
		t.Errorf("expected empty state message, got: %s", view)
	}
	if !strings.Contains(view, "pr-triage — Runs") {
		t.Errorf("expected header title, got: %s", view)
	}
}

func TestTUI_NavigationAndSelection(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	runs := []db.Run{
		{
			ID:        1,
			PRID:      11, // internal pr_id, distinct from the GitHub number
			PRNumber:  101,
			HeadSHA:   "sha-1",
			RiskTier:  "critical",
			Runtime:   "claude-code",
			Model:     "opus",
			CostUSD:   0.25,
			CostBasis: "exact",
			Status:    "done",
			StartedAt: now,
		},
		{
			ID:        2,
			PRID:      12, // internal pr_id, distinct from the GitHub number
			PRNumber:  102,
			HeadSHA:   "sha-2",
			RiskTier:  "routine",
			Runtime:   "claude-code",
			Model:     "haiku",
			CostUSD:   0.01,
			CostBasis: "exact",
			Status:    "failed",
			StartedAt: now,
		},
	}

	m := tui.NewWithRuns(runs)
	view := m.View()

	if !strings.Contains(view, "#101") || !strings.Contains(view, "#102") {
		t.Errorf("expected run list to contain PR numbers, got: %s", view)
	}

	// Check initial selected run
	sel := m.SelectedRun()
	if sel == nil || sel.ID != 1 {
		t.Fatalf("expected selected run ID 1, got %v", sel)
	}

	// Press 'down'
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(tui.Model)

	sel = m.SelectedRun()
	if sel == nil || sel.ID != 2 {
		t.Fatalf("expected selected run ID 2 after moving down, got %v", sel)
	}

	// Press 'down' again (should stay at bound 2)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(tui.Model)
	if m.SelectedRun().ID != 2 {
		t.Fatalf("expected selected run ID to stay at 2, got %v", m.SelectedRun())
	}

	// Press 'up'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(tui.Model)
	if m.SelectedRun().ID != 1 {
		t.Fatalf("expected selected run ID 1 after moving up, got %v", m.SelectedRun())
	}

	// Press 'q'
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Errorf("expected tea.Quit command on 'q'")
	}
	m = updated.(tui.Model)
	if m.View() != "" {
		t.Errorf("expected empty view after quitting, got %q", m.View())
	}
}

func TestTUI_StoreBacked(t *testing.T) {
	store := &mockStore{
		runs: []db.Run{
			{
				ID:        10,
				PRID:      50,
				HeadSHA:   "sha-50",
				RiskTier:  "medium",
				Model:     "sonnet",
				Status:    "agent_running",
				CostUSD:   0.0,
				CostBasis: "unavailable",
			},
		},
	}

	m := tui.New(store)
	if m.SelectedRun() == nil || m.SelectedRun().ID != 10 {
		t.Fatalf("expected store backed model to load run ID 10")
	}
}

func TestTUI_ActionMenu_EnterActions(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "agent.log")
	_ = os.WriteFile(logPath, []byte("agent run log details line 1\nline 2\n"), 0644)

	store := &mockStore{
		repos: []db.Repo{
			{ID: 1, Owner: "dustinmays", Name: "pr-triage"},
		},
		runs: []db.Run{
			{
				ID:       5,
				PRID:     7, // internal pr_id, distinct from the GitHub number
				PRNumber: 77,
				HeadSHA:  "sha-77",
				RiskTier: "routine",
				Model:    "sonnet",
				Status:   "done",
				LogPath:  logPath,
			},
		},
	}

	var openedURL string
	m := tui.New(store)
	m.SetBrowserOpener(func(url string) error {
		openedURL = url
		return nil
	})

	// 1. Press Enter to enter Action Menu
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)
	menuView := m.View()

	if !strings.Contains(menuView, "Actions for Run #5") {
		t.Errorf("expected action menu header, got: %s", menuView)
	}

	// 2. Action 0: Open PR in Browser
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)
	if openedURL != "https://github.com/dustinmays/pr-triage/pull/77" {
		t.Errorf("expected browser to open PR URL, got %q", openedURL)
	}

	// 3. Move down to Action 1: View Run Log
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(tui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)
	logView := m.View()

	if !strings.Contains(logView, "agent run log details line 1") {
		t.Errorf("expected log view content, got: %s", logView)
	}

	// Exit log view with 'esc'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(tui.Model)

	// 4. Move down from Action 1 to Action 2: Re-trigger Agent Review
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(tui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)
	confirmView := m.View()

	if !strings.Contains(confirmView, "Confirm Re-trigger PR #77") {
		t.Errorf("expected confirm re-trigger view, got: %s", confirmView)
	}

	// Confirm with 'y'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(tui.Model)

	if store.prStates["sha-77"] != "report_ready" {
		t.Errorf("expected prState to be reset to report_ready, got %s", store.prStates["sha-77"])
	}
}
