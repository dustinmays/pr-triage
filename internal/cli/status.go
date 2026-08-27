package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/daemon"
	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/events"
)

var (
	flagStatusJSON    bool
	flagStatusVerbose bool
)

func init() {
	statusCmd.Flags().BoolVar(&flagStatusJSON, "json", false, "Output structured JSON")
	statusCmd.Flags().BoolVarP(&flagStatusVerbose, "verbose", "v", false, "Show detailed metrics and run logs")
}

// statusCmd reports current PR/agent state.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current PR and agent status",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		// Read daemon status
		running, pid := daemon.IsRunning(daemon.DefaultStateDir())

		// Read status.json if present
		statusDoc, _ := events.ReadStatus(events.DefaultStatusPath())

		// Connect to database if available
		var runs []db.Run
		var repos []db.Repo
		if database, err := db.Open(db.DefaultDBPath()); err == nil {
			defer func() { _ = database.Close() }()
			store := db.NewStore(database)
			runs, _ = store.ListRuns(15)
			repos, _ = store.ListRepos()
		}

		if flagStatusJSON {
			data := map[string]any{
				"daemon_running": running,
				"daemon_pid":     pid,
				"status_doc":     statusDoc,
				"recent_runs":    runs,
				"repos":          repos,
			}
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(data)
		}

		// Text output
		if running {
			fmt.Fprintf(out, "Daemon: \033[32mRUNNING\033[0m (PID %d)\n", pid)
		} else {
			fmt.Fprintln(out, "Daemon: \033[90mSTOPPED\033[0m")
		}

		if statusDoc != nil && len(statusDoc.ActiveRuns) > 0 {
			fmt.Fprintf(out, "\nActive Invocations (%d):\n", len(statusDoc.ActiveRuns))
			for _, ar := range statusDoc.ActiveRuns {
				fmt.Fprintf(out, "  🔄 %s/%s#%d [%s/%s] started %s\n",
					ar.RepoOwner, ar.RepoName, ar.PRNumber, ar.Runtime, ar.Model, ar.StartedAt.Format("15:04:05"))
			}
		}

		if len(runs) > 0 {
			fmt.Fprintf(out, "\nRecent Runs (%d):\n", len(runs))
			for _, r := range runs {
				var statusColor string
				switch r.Status {
				case "failed", "timeout":
					statusColor = "\033[31m" // red
				case "agent_running":
					statusColor = "\033[33m" // yellow
				case "escalated":
					statusColor = "\033[35m" // magenta
				default:
					statusColor = "\033[32m" // green for done
				}

				fmt.Fprintf(out, "  #%-4d PR #%-4d %s%-12s\033[0m %-8s %-18s cost=$%.4f (%s)",
					r.ID, r.PRNumber, statusColor, r.Status, r.RiskTier, r.Model, r.CostUSD, r.CostBasis)

				if flagStatusVerbose {
					if r.StopReason != "" {
						fmt.Fprintf(out, " reason=%q", r.StopReason)
					}
					if r.LogPath != "" {
						fmt.Fprintf(out, " log=%s", r.LogPath)
					}
				}
				fmt.Fprintln(out)
			}
		} else {
			fmt.Fprintln(out, "\nNo recent runs recorded.")
		}

		if len(repos) > 0 {
			fmt.Fprintf(out, "\nTracked Repositories (%d):\n", len(repos))
			for _, repo := range repos {
				fmt.Fprintf(out, "  • %s/%s (base: %s, poll: %s)\n", repo.Owner, repo.Name, repo.BaseRef, repo.PollInterval)
			}
		}

		return nil
	},
}
