package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/auth"
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

		database, err := db.Open(db.DefaultDBPath())
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer func() { _ = database.Close() }()

		store := db.NewStore(database)

		token, err := auth.GetToken()
		if err != nil {
			return fmt.Errorf("auth: %w", err)
		}

		ghClient := github.NewClient(token)
		escalator := escalate.New(store, ghClient)

		p := poller.New(store, ghClient)
		orch := orchestrator.New(store, ghClient, escalator)

		errCh := make(chan error, 2)

		go func() {
			errCh <- p.Start(ctx)
		}()

		go func() {
			errCh <- orch.Start(ctx, p.ReportReadyEvents())
		}()

		fmt.Fprintln(cmd.OutOrStdout(), "pr-triage daemon running... (press Ctrl+C to stop)")

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
