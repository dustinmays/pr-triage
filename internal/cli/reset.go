package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/db"
)

var flagResetRepo string

func init() {
	resetCmd.Flags().StringVar(&flagResetRepo, "repo", "",
		"Disambiguate by repository as owner/name when the PR number is tracked in more than one repo.")
	rootCmd.AddCommand(resetCmd)
}

// resetCmd clears a PR's tracked state so the daemon treats it as a fresh,
// never-seen PR on its next poll.
var resetCmd = &cobra.Command{
	Use:   "reset <pr-number>",
	Short: "Clear a PR's tracked state so the daemon re-evaluates it from scratch",
	Long: `reset deletes the local tracking record for a PR (its state-machine state
and any recorded runs). It does not touch GitHub - CI results, checks, and the
PR itself are untouched.

Use this to unstick a PR whose local state no longer reflects reality (e.g. it
was marked ci_failed because of a transient API/auth error, not a real CI
failure). The running daemon picks the PR back up as brand new on its next
poll; no restart is required.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		var prNumber int
		if _, err := fmt.Sscanf(args[0], "%d", &prNumber); err != nil || prNumber <= 0 {
			return fmt.Errorf("invalid PR number %q", args[0])
		}

		database, err := db.Open(db.DefaultDBPath())
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() { _ = database.Close() }()
		store := db.NewStore(database)

		repos, err := store.ListRepos()
		if err != nil {
			return fmt.Errorf("list repos: %w", err)
		}

		// Resolve which tracked repo this PR number belongs to.
		type match struct {
			repo db.Repo
			pr   *db.PR
		}
		var matches []match
		for _, r := range repos {
			if flagResetRepo != "" && !strings.EqualFold(flagResetRepo, r.Owner+"/"+r.Name) {
				continue
			}
			pr, err := store.GetPRState(r.ID, prNumber)
			if err == nil && pr != nil {
				matches = append(matches, match{repo: r, pr: pr})
			}
		}

		switch {
		case len(matches) == 0:
			return fmt.Errorf("no tracked PR #%d found (is the daemon watching its repo?)", prNumber)
		case len(matches) > 1:
			var names []string
			for _, m := range matches {
				names = append(names, m.repo.Owner+"/"+m.repo.Name)
			}
			return fmt.Errorf("PR #%d is tracked in multiple repos (%s); pass --repo owner/name to disambiguate",
				prNumber, strings.Join(names, ", "))
		}

		m := matches[0]
		if err := store.DeletePRState(m.repo.ID, prNumber); err != nil {
			return fmt.Errorf("clear pr state: %w", err)
		}

		shaShort := m.pr.HeadSHA
		if len(shaShort) > 12 {
			shaShort = shaShort[:12]
		}

		fmt.Fprintf(out, "Cleared %s/%s PR #%d (was %q at %s).\n", m.repo.Owner, m.repo.Name, prNumber, m.pr.State, shaShort)
		fmt.Fprintln(out, "The daemon will re-poll it as a fresh PR on its next sweep.")
		return nil
	},
}
