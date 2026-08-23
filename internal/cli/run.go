package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// runCmd starts the daemon's poll loop.
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the pr-triage daemon poll loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "run: not implemented yet")
		return ErrNotImplemented
	},
}
