package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dustinmays/pr-triage/internal/auth"
	"github.com/dustinmays/pr-triage/internal/daemon"
	"github.com/dustinmays/pr-triage/internal/github"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var setupTokenFlag string

func init() {
	setupCmd.Flags().StringVarP(&setupTokenFlag, "token", "t", "", "GitHub Personal Access Token")
}

// tokenValidator is the minimal GitHub client surface setup needs to sanity-check
// a token before storing it. A package-level var (rather than calling
// github.NewClient directly) so tests can substitute a stub and avoid a real
// network call.
type tokenValidator interface {
	ValidateToken(ctx context.Context) (string, error)
}

var newTokenValidator = func(token string) tokenValidator {
	return github.NewClient(token)
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

		// Reject a bad/expired/mistyped token up front instead of reporting
		// success and only failing later, silently, deep in the poll loop.
		login, err := newTokenValidator(token).ValidateToken(cmd.Context())
		if err != nil {
			return fmt.Errorf("token rejected by GitHub, not storing it: %w", err)
		}

		if err := auth.SetToken(token); err != nil {
			return fmt.Errorf("store token in keyring: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "GitHub token stored successfully in OS keyring (authenticated as %s).\n", login)

		// GetToken() prefers GITHUB_TOKEN/GH_TOKEN over the keychain, so a
		// token stored here has no effect at all until those are unset.
		if _, envVar := auth.EnvToken(); envVar != "" {
			fmt.Fprintf(cmd.OutOrStdout(),
				"Note: %s is set in this environment and takes precedence over the keychain -\n"+
					"the daemon will keep using that token, not the one just stored, until it's unset.\n", envVar)
		}

		// The running daemon read its token once at startup and holds it for
		// its lifetime; it will not pick up this change without a restart.
		if running, pid := daemon.IsRunning(daemon.DefaultStateDir()); running {
			fmt.Fprintf(cmd.OutOrStdout(),
				"The daemon (PID %d) is already running and won't see this change until it's restarted.\n", pid)
		}

		return nil
	},
}
