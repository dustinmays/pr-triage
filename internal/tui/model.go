package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dustinmays/pr-triage/internal/db"
)

// Store defines persistence operations required by the TUI.
type Store interface {
	ListRuns(limit int) ([]db.Run, error)
}

// Model is the Bubble Tea model for the run list view.
type Model struct {
	store    Store
	runs     []db.Run
	cursor   int
	quitting bool
	width    int
	height   int
}

// New creates a Model backed by store.
func New(store Store) Model {
	m := Model{
		store:  store,
		runs:   make([]db.Run, 0),
		cursor: 0,
	}
	if store != nil {
		if runs, err := store.ListRuns(50); err == nil {
			m.runs = runs
		}
	}
	return m
}

// NewWithRuns creates a Model pre-populated with runs (useful for testing).
func NewWithRuns(runs []db.Run) Model {
	return Model{
		runs:   runs,
		cursor: 0,
	}
}

// Init initializes the Bubble Tea model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages and updates model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.runs)-1 {
				m.cursor++
			}
		}
	}

	return m, nil
}

// View renders the current UI state to a string.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("pr-triage — Runs"))
	b.WriteString("\n")

	if len(m.runs) == 0 {
		b.WriteString(emptyStyle.Render("No triage runs found in store.\nRun 'pr-triage run' to begin watching repositories."))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("q/esc: quit"))
		return b.String()
	}

	// Column headers
	header := fmt.Sprintf("  %-6s %-8s %-14s %-10s %-18s %-12s %s",
		"RUN", "PR", "STATUS", "TIER", "MODEL", "COST", "AGE")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	for i, r := range m.runs {
		cursorMark := "  "
		if i == m.cursor {
			cursorMark = "▶ "
		}

		var statusStyled string
		switch r.Status {
		case "done":
			statusStyled = statusDoneStyle.Render(fmt.Sprintf("%-14s", "done"))
		case "failed", "timeout":
			statusStyled = statusFailedStyle.Render(fmt.Sprintf("%-14s", r.Status))
		case "agent_running":
			statusStyled = statusRunningStyle.Render(fmt.Sprintf("%-14s", "running"))
		case "escalated":
			statusStyled = statusEscalatedStyle.Render(fmt.Sprintf("%-14s", "escalated"))
		default:
			statusStyled = fmt.Sprintf("%-14s", r.Status)
		}

		age := formatAge(r.StartedAt)
		costStr := fmt.Sprintf("$%.4f", r.CostUSD)
		if r.CostBasis != "exact" && r.CostBasis != "" {
			costStr += " (" + r.CostBasis[:min(3, len(r.CostBasis))] + ")"
		}

		row := fmt.Sprintf("%s%-6d #%-7d %s %-10s %-18s %-12s %s",
			cursorMark,
			r.ID,
			r.PRID,
			statusStyled,
			r.RiskTier,
			r.Model,
			costStr,
			age,
		)

		if i == m.cursor {
			b.WriteString(selectedRowStyle.Render(row))
		} else {
			b.WriteString(normalRowStyle.Render(row))
		}
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("↑/k: up • ↓/j: down • q: quit"))
	return b.String()
}

// SelectedRun returns the currently selected run, or nil if empty.
func (m Model) SelectedRun() *db.Run {
	if len(m.runs) == 0 || m.cursor < 0 || m.cursor >= len(m.runs) {
		return nil
	}
	cp := m.runs[m.cursor]
	return &cp
}

func formatAge(timestamp string) string {
	if timestamp == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", timestamp)
		if err != nil {
			return timestamp
		}
	}
	d := time.Since(t).Round(time.Minute)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
