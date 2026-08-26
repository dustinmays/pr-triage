package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/db"
)

var (
	flagOverrideSignals []string
	flagOverrideReason  string
	flagOverrideRepo    string
)

func init() {
	overrideCmd.Flags().StringArrayVar(&flagOverrideSignals, "signal", nil,
		"Signal ID to waive (repeatable). Omit to waive all escalate-tier signals present at the current head SHA.")
	overrideCmd.Flags().StringVar(&flagOverrideReason, "reason", "",
		"Optional note recorded with the override for the audit trail.")
	overrideCmd.Flags().StringVar(&flagOverrideRepo, "repo", "",
		"Disambiguate by repository as owner/name when the PR number is tracked in more than one repo.")
	rootCmd.AddCommand(overrideCmd)
}

// overrideCmd records a human override that lets the review agent run on an
// escalated PR by waiving specific escalate-tier signals, pinned to the PR's
// current head SHA (state-first; see docs/epic-80/design/escalation-override.md).
var overrideCmd = &cobra.Command{
	Use:   "override <pr-number>",
	Short: "Waive escalate signals on a PR so the review agent runs instead of escalating",
	Long: `override records an owner decision to let pr-triage proceed with the
probabilistic review on a PR that would otherwise escalate.

It does NOT merge or approve the PR — it waives the specific escalate-tier
signal(s) and lets the review agent run. The override is pinned to the PR's
current head SHA, so a new push invalidates it and the PR re-evaluates from
scratch. The running daemon consults the override on its next poll; no restart
is required.`,
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
			if flagOverrideRepo != "" && !strings.EqualFold(flagOverrideRepo, r.Owner+"/"+r.Name) {
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
		if m.pr.HeadSHA == "" {
			return fmt.Errorf("PR #%d has no recorded head SHA yet; wait for the daemon to poll it", prNumber)
		}

		waived := ""
		if len(flagOverrideSignals) > 0 {
			waived = strings.Join(flagOverrideSignals, ",")
		}

		ov, err := store.RecordOverride(&db.Override{
			RepoID:        m.repo.ID,
			PRNumber:      prNumber,
			HeadSHA:       m.pr.HeadSHA,
			WaivedSignals: waived,
			Reason:        flagOverrideReason,
		})
		if err != nil {
			return fmt.Errorf("record override: %w", err)
		}

		shaShort := m.pr.HeadSHA
		if len(shaShort) > 12 {
			shaShort = shaShort[:12]
		}
		scope := "all escalate-tier signals present at this SHA"
		if waived != "" {
			scope = "signal(s) " + waived
		}

		fmt.Fprintf(out, "✅ Override #%d recorded for %s/%s PR #%d\n",
			ov.ID, m.repo.Owner, m.repo.Name, prNumber)
		fmt.Fprintf(out, "   head SHA : %s\n", shaShort)
		fmt.Fprintf(out, "   waiving  : %s\n", scope)
		if flagOverrideReason != "" {
			fmt.Fprintf(out, "   reason   : %s\n", flagOverrideReason)
		}
		if m.pr.State != "escalated" {
			fmt.Fprintf(out, "\n⚠️  PR #%d is currently in state %q, not \"escalated\". The override is\n"+
				"   recorded and will apply if/when this head SHA reaches the escalation decision.\n", prNumber, m.pr.State)
			return nil
		}

		// The PR is escalated, which is a terminal state — the poller will not
		// re-emit report_ready for it on its own. Reset it to ci_passed so the
		// daemon re-runs the report pipeline on its next poll and consults this
		// override before deciding. The head SHA is unchanged and its report
		// check still passed, so ci_passed is accurate. LastRunID (the report
		// check run) is preserved so re-evaluation fetches the right report.
		if _, err := store.UpsertPRState(m.repo.ID, prNumber, m.pr.HeadSHA, m.pr.LastRunID, "ci_passed"); err != nil {
			return fmt.Errorf("re-arm PR for re-evaluation: %w", err)
		}
		fmt.Fprintf(out, "\nThe daemon will re-evaluate PR #%d on its next poll and run the review agent.\n"+
			"A new push clears this override.\n", prNumber)
		return nil
	},
}
