package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// initCmd registers the current repo into the shared local store and
// writes its config.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Register the current repo with pr-triage",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "init: not implemented yet")
		return ErrNotImplemented
	},
}
