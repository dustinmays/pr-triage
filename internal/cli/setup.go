package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dustinmays/pr-triage/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var setupTokenFlag string

func init() {
	setupCmd.Flags().StringVarP(&setupTokenFlag, "token", "t", "", "GitHub Personal Access Token")
}

// setupCmd handles one-time setup, e.g. storing a GitHub PAT in the OS keychain.
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "One-time setup (e.g. storing GitHub credentials)",
	RunE: func(cmd *cobra.Command, args []string) error {
		token := strings.TrimSpace(setupTokenFlag)

		if token == "" {
			// Check if standard input is a terminal.
			inFd := int(os.Stdin.Fd())
			if term.IsTerminal(inFd) {
				fmt.Fprint(cmd.OutOrStdout(), "Enter GitHub token: ")
				raw, err := term.ReadPassword(inFd)
				fmt.Fprintln(cmd.OutOrStdout()) // Newline after hidden password entry
				if err != nil {
					return fmt.Errorf("read token: %w", err)
				}
				token = strings.TrimSpace(string(raw))
			} else {
				scanner := bufio.NewScanner(cmd.InOrStdin())
				if scanner.Scan() {
					token = strings.TrimSpace(scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("read token from stdin: %w", err)
				}
			}
		}

		if token == "" {
			return fmt.Errorf("no GitHub token provided")
		}

		if err := auth.SetToken(token); err != nil {
			return fmt.Errorf("store token in keyring: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "GitHub token stored successfully in OS keyring.")
		return nil
	},
}
