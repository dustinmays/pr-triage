// Package cli implements the pr-triage command-line interface.
package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// ErrNotImplemented is returned by stubbed subcommands that have not yet
// been implemented. It signals a clean, non-zero exit rather than a panic.
var ErrNotImplemented = errors.New("not implemented yet")

// rootCmd is the base command for the pr-triage CLI.
var rootCmd = &cobra.Command{
	Use:   "pr-triage",
	Short: "pr-triage watches GitHub PRs, waits for CI, and routes review agents by risk",
	Long: `pr-triage is a Go CLI daemon that watches GitHub pull requests, waits for
CI/CD to finish, ingests a pre-scan report, routes the PR by risk, runs a
review agent in an isolated git worktree, and escalates hard-fails to a
human.`,
	SilenceUsage: true,
}

// Execute runs the root command and returns any error encountered.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(checkoutCmd)
}
