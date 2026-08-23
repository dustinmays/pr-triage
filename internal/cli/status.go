package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// statusCmd reports current PR/agent state.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current PR and agent status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "status: not implemented yet")
		return ErrNotImplemented
	},
}
