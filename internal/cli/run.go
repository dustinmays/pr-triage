package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/auth"
	"github.com/dustinmays/pr-triage/internal/config"
	"github.com/dustinmays/pr-triage/internal/daemon"
	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/escalate"
	"github.com/dustinmays/pr-triage/internal/github"
	"github.com/dustinmays/pr-triage/internal/orchestrator"
	"github.com/dustinmays/pr-triage/internal/poller"
)

// runCmd starts the daemon's poll loop and orchestrator.
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the pr-triage daemon poll loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		lock, err := daemon.AcquireLock(daemon.DefaultStateDir())
		if err != nil {
			return err
		}
		defer lock.Release()

		dbPath := db.DefaultDBPath()
		database, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer func() { _ = database.Close() }()

		store := db.NewStore(database)

		token, tokenSource, err := auth.GetTokenWithSource()
		if err != nil {
			return fmt.Errorf("auth: %w", err)
		}

		ghClient := github.NewClient(token)
		escalator := escalate.New(store, ghClient)

		out := cmd.OutOrStdout()
		printStartupBanner(ctx, out, store, ghClient, dbPath, tokenSource)

		p := poller.New(store, ghClient)
		orch := orchestrator.New(store, ghClient, escalator)

		errCh := make(chan error, 2)

		go func() {
			errCh <- p.Start(ctx)
		}()

		go func() {
			errCh <- orch.Start(ctx, p.ReportReadyEvents())
		}()

		fmt.Fprintln(out, "pr-triage daemon running... (press Ctrl+C to stop)")

		select {
		case <-ctx.Done():
			p.Stop()
			orch.Stop()
			return nil
		case err := <-errCh:
			p.Stop()
			orch.Stop()
			if err != nil && err != context.Canceled {
				return err
			}
			return nil
		}
	},
}

// printStartupBanner prints a one-time diagnostic summary before the poll
// loop starts: token source/identity, build revision, watched repos and
// their resolved config, any PRs already tracked, and rate limit headroom.
// None of this affects control flow - it exists purely so a broken token,
// wrong repo/branch, or stale tracked state is visible immediately instead
// of only showing up later as "nothing seems to be happening."
func printStartupBanner(ctx context.Context, out io.Writer, store *db.Store, ghClient *github.Client, dbPath, tokenSource string) {
	fmt.Fprintln(out, "pr-triage starting")
	fmt.Fprintf(out, "  pid          : %d\n", os.Getpid())
	fmt.Fprintf(out, "  build        : %s\n", buildInfoLine())
	fmt.Fprintf(out, "  db           : %s\n", dbPath)

	login, err := ghClient.ValidateToken(ctx)
	if err != nil {
		fmt.Fprintf(out, "  auth         : token from %s - REJECTED by GitHub: %v\n", tokenSource, err)
	} else {
		fmt.Fprintf(out, "  auth         : token from %s, authenticated as %s\n", tokenSource, login)
	}

	if rate, err := ghClient.CoreRateLimit(ctx); err == nil && rate != nil {
		fmt.Fprintf(out, "  rate limit   : %d/%d remaining, resets %s\n",
			rate.Remaining, rate.Limit, rate.Reset.Format("15:04:05 MST"))
	}

	repos, err := store.ListRepos()
	if err != nil {
		fmt.Fprintf(out, "  watched repos: error listing repos: %v\n", err)
		return
	}

	fmt.Fprintf(out, "  watched repos (%d):\n", len(repos))
	for _, repo := range repos {
		fmt.Fprintf(out, "    - %s/%s  base=%q  poll=%s\n", repo.Owner, repo.Name, repo.BaseRef, repo.PollInterval)

		cfg := config.DefaultConfig()
		if repo.ConfigPath != "" {
			if loaded, cfgErr := config.Load(repo.ConfigPath); cfgErr == nil && loaded != nil {
				cfg = loaded
				fmt.Fprintf(out, "        config    : %s (loaded)\n", repo.ConfigPath)
			} else {
				fmt.Fprintf(out, "        config    : %s (not found/unreadable, using defaults: %v)\n", repo.ConfigPath, cfgErr)
			}
		} else {
			fmt.Fprintln(out, "        config    : none recorded, using defaults")
		}
		if r, ok := cfg.Routing["routine"]; ok {
			fmt.Fprintf(out, "        routing   : routine -> runtime=%s model=%s\n", r.Runtime, r.Model)
		}

		prs, err := store.ListPRsForRepo(repo.ID)
		if err != nil {
			fmt.Fprintf(out, "        tracked PRs: error: %v\n", err)
			continue
		}
		if len(prs) == 0 {
			fmt.Fprintln(out, "        tracked PRs: none")
			continue
		}
		fmt.Fprintf(out, "        tracked PRs (%d):\n", len(prs))
		for _, pr := range prs {
			sha := pr.HeadSHA
			if len(sha) > 8 {
				sha = sha[:8]
			}
			fmt.Fprintf(out, "          #%-4d %-14s sha=%-8s updated=%s\n", pr.Number, pr.State, sha, pr.UpdatedAt)
		}
	}
}

// buildInfoLine summarizes the running binary's VCS revision so it's obvious
// which branch/commit produced it, without reaching for `go version -m`.
func buildInfoLine() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown (no build info embedded)"
	}

	var revision, dirty, buildTime string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value
		case "vcs.time":
			buildTime = s.Value
		}
	}
	if revision == "" {
		return "unknown (no vcs info embedded, e.g. built outside a git checkout)"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	suffix := ""
	if dirty == "true" {
		suffix = "+dirty"
	}
	return fmt.Sprintf("%s%s (built %s)", revision, suffix, buildTime)
}
