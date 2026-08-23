package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/tui"
)

// checkoutCmd opens the interactive Bubble Tea TUI for inspecting and acting on triage runs.
var checkoutCmd = &cobra.Command{
	Use:   "checkout",
	Short: "Open interactive TUI to inspect and manage PR triage runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.Open(db.DefaultDBPath())
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer func() { _ = database.Close() }()

		store := db.NewStore(database)
		model := tui.New(store)

		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("tui runtime error: %w", err)
		}

		return nil
	},
}
