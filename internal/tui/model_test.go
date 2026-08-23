package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/tui"
)

type mockStore struct {
	runs []db.Run
}

func (m *mockStore) ListRuns(limit int) ([]db.Run, error) {
	return m.runs, nil
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
			PRID:      101,
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
			PRID:      102,
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
