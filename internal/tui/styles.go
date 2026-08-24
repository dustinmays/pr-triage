// Package tui implements the interactive Bubble Tea terminal user interface
// for inspecting past runs, navigating triage results, and executing actions.
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#BBBBBB")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("#444444")).
			MarginBottom(1)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#3B4252"))

	normalRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D8DEE9"))

	statusDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A3BE8C")).
			Bold(true)

	statusFailedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#BF616A")).
				Bold(true)

	statusRunningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EBCB8B")).
				Bold(true)

	statusEscalatedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B48EAD")).
				Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626880")).
			MarginTop(1)

	emptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true).
			Padding(2, 4)
)
