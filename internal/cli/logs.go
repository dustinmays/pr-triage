package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// logsCmd shows daemon/agent logs.
var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show pr-triage daemon and agent logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "logs: not implemented yet")
		return ErrNotImplemented
	},
}
