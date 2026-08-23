package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// setupCmd handles one-time setup, e.g. storing a GitHub PAT in the OS
// keychain.
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "One-time setup (e.g. storing GitHub credentials)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "setup: not implemented yet")
		return ErrNotImplemented
	},
}
