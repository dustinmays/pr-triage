package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// checkoutCmd checks out a PR's worktree for local inspection.
var checkoutCmd = &cobra.Command{
	Use:   "checkout",
	Short: "Check out a PR's git worktree for local inspection",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "checkout: not implemented yet")
		return ErrNotImplemented
	},
}
