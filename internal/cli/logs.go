package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/daemon"
	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/dustinmays/pr-triage/internal/logs"
)

var (
	flagLogsFollow bool
	flagLogsLines  int
	flagLogsRunID  int64
)

func init() {
	logsCmd.Flags().BoolVarP(&flagLogsFollow, "follow", "f", false, "Follow log output in real time")
	logsCmd.Flags().IntVarP(&flagLogsLines, "lines", "n", 50, "Number of initial lines to show")
	logsCmd.Flags().Int64Var(&flagLogsRunID, "run", 0, "Show log for specific run ID")
}

// logsCmd shows daemon/agent logs.
var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show pr-triage daemon and agent logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		var targetLogPath string

		if flagLogsRunID > 0 {
			// Find log path for specified run
			database, err := db.Open(db.DefaultDBPath())
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer func() { _ = database.Close() }()

			var run db.Run
			if err := database.Get(&run, "SELECT id, log_path FROM runs WHERE id = ?", flagLogsRunID); err != nil {
				return fmt.Errorf("run #%d not found: %w", flagLogsRunID, err)
			}
			if run.LogPath == "" {
				return fmt.Errorf("run #%d has no recorded log path", flagLogsRunID)
			}
			targetLogPath = run.LogPath
		} else {
			// Check daemon stdout log
			daemonLog := filepath.Join(daemon.DefaultStateDir(), "daemon.stdout.log")
			if _, err := os.Stat(daemonLog); err == nil {
				targetLogPath = daemonLog
			} else {
				// Fallback to newest run log if daemon log doesn't exist
				if database, err := db.Open(db.DefaultDBPath()); err == nil {
					defer func() { _ = database.Close() }()
					store := db.NewStore(database)
					runs, _ := store.ListRuns(1)
					if len(runs) > 0 && runs[0].LogPath != "" {
						targetLogPath = runs[0].LogPath
					}
				}
			}
		}

		if targetLogPath == "" {
			fmt.Fprintln(out, "No active or recent logs found.")
			return nil
		}

		// Perform log rotation check if log file is oversized
		_ = logs.RotateIfNeeded(targetLogPath, logs.DefaultMaxLogSize, logs.DefaultMaxBackups)

		file, err := os.Open(targetLogPath)
		if err != nil {
			return fmt.Errorf("open log file %s: %w", targetLogPath, err)
		}
		defer func() { _ = file.Close() }()

		// Print initial lines
		scanner := bufio.NewScanner(file)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		startIdx := 0
		if flagLogsLines > 0 && len(lines) > flagLogsLines {
			startIdx = len(lines) - flagLogsLines
		}

		for _, l := range lines[startIdx:] {
			fmt.Fprintln(out, l)
		}

		// If follow mode enabled, tail file
		if flagLogsFollow {
			ctx := cmd.Context()
			reader := bufio.NewReader(file)
			for {
				if ctx != nil && ctx.Err() != nil {
					return nil
				}
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						time.Sleep(200 * time.Millisecond)
						continue
					}
					return err
				}
				fmt.Fprint(out, line)
			}
		}

		return nil
	},
}
