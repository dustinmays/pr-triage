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
	UpsertPRState(repoID int64, number int, headSHA string, runID *int64, state string) (*db.PR, error)
	ListRepos() ([]db.Repo, error)
}

// viewState represents the current view screen in the TUI.
type viewState int

const (
	viewList viewState = iota
	viewActionMenu
	viewLog
	viewConfirmRetrigger
)

// ActionItem represents a selectable menu option.
type ActionItem int

const (
	ActionOpenBrowser ActionItem = iota
	ActionViewLog
	ActionRetrigger
	ActionBack
)

var actionLabels = []string{
	"🌐 Open PR in Browser",
	"📄 View Run Log",
	"🔄 Re-trigger Agent Review",
	"⬅️  Back to List",
}

// Model is the Bubble Tea model for the run list and action menu view.
type Model struct {
	store         Store
	runs          []db.Run
	cursor        int
	menuCursor    int
	state         viewState
	quitting      bool
	width         int
	height        int
	logContent    string
	logScroll     int
	statusMessage string
	browserOpener BrowserOpener
	repos         []db.Repo
}

// New creates a Model backed by store.
func New(store Store) Model {
	m := Model{
		store:         store,
		runs:          make([]db.Run, 0),
		cursor:        0,
		menuCursor:    0,
		state:         viewList,
		browserOpener: DefaultOpenBrowser,
	}
	if store != nil {
		if runs, err := store.ListRuns(50); err == nil {
			m.runs = runs
		}
		if repos, err := store.ListRepos(); err == nil {
			m.repos = repos
		}
	}
	return m
}

// NewWithRuns creates a Model pre-populated with runs (useful for testing).
func NewWithRuns(runs []db.Run) Model {
	return Model{
		runs:          runs,
		cursor:        0,
		menuCursor:    0,
		state:         viewList,
		browserOpener: func(url string) error { return nil },
	}
}

// SetBrowserOpener sets a custom browser opener function (useful for mock/unit tests).
func (m *Model) SetBrowserOpener(fn BrowserOpener) {
	m.browserOpener = fn
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
		key := msg.String()

		switch m.state {
		case viewList:
			switch key {
			case "q", "ctrl+c":
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

			case "enter":
				if len(m.runs) > 0 {
					m.state = viewActionMenu
					m.menuCursor = 0
					m.statusMessage = ""
				}
			}

		case viewActionMenu:
			switch key {
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit

			case "esc", "b":
				m.state = viewList
				m.statusMessage = ""

			case "up", "k":
				if m.menuCursor > 0 {
					m.menuCursor--
				}

			case "down", "j":
				if m.menuCursor < len(actionLabels)-1 {
					m.menuCursor++
				}

			case "enter":
				return m.handleActionSelect()
			}

		case viewLog:
			switch key {
			case "esc", "q", "b", "enter":
				m.state = viewActionMenu

			case "up", "k":
				if m.logScroll > 0 {
					m.logScroll--
				}

			case "down", "j":
				m.logScroll++
			}

		case viewConfirmRetrigger:
			switch key {
			case "y", "enter":
				m.executeRetrigger()
				m.state = viewList

			case "n", "esc", "q":
				m.state = viewActionMenu
			}
		}
	}

	return m, nil
}

func (m *Model) handleActionSelect() (tea.Model, tea.Cmd) {
	selected := m.SelectedRun()
	if selected == nil {
		m.state = viewList
		return *m, nil
	}

	switch ActionItem(m.menuCursor) {
	case ActionOpenBrowser:
		prURL := m.getPRURL(selected)
		if m.browserOpener != nil {
			if err := m.browserOpener(prURL); err != nil {
				m.statusMessage = fmt.Sprintf("Error opening browser: %v", err)
			} else {
				m.statusMessage = fmt.Sprintf("Opened %s in browser", prURL)
			}
		}
		return *m, nil

	case ActionViewLog:
		logContent, err := ReadLogFile(selected.LogPath)
		if err != nil {
			m.statusMessage = err.Error()
			return *m, nil
		}
		m.logContent = logContent
		m.logScroll = 0
		m.state = viewLog
		return *m, nil

	case ActionRetrigger:
		m.state = viewConfirmRetrigger
		return *m, nil

	case ActionBack:
		m.state = viewList
		m.statusMessage = ""
		return *m, nil
	}

	return *m, nil
}

func (m *Model) executeRetrigger() {
	selected := m.SelectedRun()
	if selected == nil || m.store == nil {
		return
	}

	var repoID int64 = 1
	if len(m.repos) > 0 {
		repoID = m.repos[0].ID
	}

	_, _ = m.store.UpsertPRState(repoID, int(selected.PRID), selected.HeadSHA, nil, "report_ready")
	m.statusMessage = fmt.Sprintf("✅ Re-triggered review for PR #%d", selected.PRID)
}

func (m *Model) getPRURL(r *db.Run) string {
	owner := "owner"
	repo := "repo"
	if len(m.repos) > 0 {
		owner = m.repos[0].Owner
		repo = m.repos[0].Name
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, r.PRID)
}

// View renders the current UI state to a string.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	switch m.state {
	case viewActionMenu:
		return m.viewActionMenu()
	case viewLog:
		return m.viewLog()
	case viewConfirmRetrigger:
		return m.viewConfirmRetrigger()
	default:
		return m.viewRunList()
	}
}

func (m Model) viewRunList() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("pr-triage — Runs"))
	b.WriteString("\n")

	if m.statusMessage != "" {
		b.WriteString(statusRunningStyle.Render(m.statusMessage))
		b.WriteString("\n\n")
	}

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

	b.WriteString(helpStyle.Render("↑/k: up • ↓/j: down • ↵: actions • q: quit"))
	return b.String()
}

func (m Model) viewActionMenu() string {
	var b strings.Builder
	selected := m.SelectedRun()

	b.WriteString(titleStyle.Render(fmt.Sprintf("Actions for Run #%d (PR #%d)", selected.ID, selected.PRID)))
	b.WriteString("\n")

	if m.statusMessage != "" {
		b.WriteString(statusRunningStyle.Render(m.statusMessage))
		b.WriteString("\n\n")
	}

	for i, label := range actionLabels {
		cursor := "  "
		if i == m.menuCursor {
			cursor = "▶ "
			b.WriteString(selectedRowStyle.Render(cursor + label))
		} else {
			b.WriteString(normalRowStyle.Render(cursor + label))
		}
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("↑/k: up • ↓/j: down • ↵: select • esc/b: back"))
	return b.String()
}

func (m Model) viewLog() string {
	var b strings.Builder
	selected := m.SelectedRun()

	b.WriteString(titleStyle.Render(fmt.Sprintf("Log Output: Run #%d (PR #%d)", selected.ID, selected.PRID)))
	b.WriteString("\n\n")

	lines := strings.Split(m.logContent, "\n")
	start := m.logScroll
	if start > len(lines)-1 {
		start = max(0, len(lines)-1)
	}
	end := min(len(lines), start+20)

	for _, l := range lines[start:end] {
		b.WriteString(l)
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("↑/k: scroll up • ↓/j: scroll down • esc/b: back"))
	return b.String()
}

func (m Model) viewConfirmRetrigger() string {
	var b strings.Builder
	selected := m.SelectedRun()

	b.WriteString(titleStyle.Render(fmt.Sprintf("Confirm Re-trigger PR #%d", selected.PRID)))
	b.WriteString("\n\n")
	b.WriteString("Are you sure you want to reset PR state and re-enqueue review agent?\n\n")
	b.WriteString(selectedRowStyle.Render(" [Y] Yes, re-trigger review ") + "   " + normalRowStyle.Render(" [N] Cancel "))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("y/↵: confirm • n/esc: cancel"))
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
