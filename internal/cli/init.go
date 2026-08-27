package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustinmays/pr-triage/internal/config"
	"github.com/dustinmays/pr-triage/internal/db"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	initBaseRef        string
	initPollInterval   string
	initTimeout        string
	initGitHubUser     string
	initRuntime        string
	initModel          string
	initNonInteractive bool
	initDBPath         string
	initRepoDir        string
	initOwner          string
	initName           string
)

func init() {
	initCmd.Flags().StringVar(&initBaseRef, "base-ref", "main", "Base branch or glob pattern to watch")
	initCmd.Flags().StringVar(&initPollInterval, "poll-interval", "5m", "Polling interval for checking PRs and CI")
	initCmd.Flags().StringVar(&initTimeout, "timeout", "10m", "Timeout cap for agent execution")
	initCmd.Flags().StringVar(&initGitHubUser, "github-user", "", "GitHub username to notify/tag on escalation")
	initCmd.Flags().StringVar(&initRuntime, "runtime", "", "Agent runtime (e.g. claude-code, codex, opencode)")
	initCmd.Flags().StringVar(&initModel, "model", "", "Model identifier for review agent")
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false, "Disable interactive prompts and use flags/defaults")
	initCmd.Flags().StringVar(&initDBPath, "db-path", "", "Path to SQLite database (defaults to ~/.pr-triage/pr-triage.db)")
	initCmd.Flags().StringVar(&initRepoDir, "repo-dir", ".", "Repository directory to register")
	initCmd.Flags().StringVar(&initOwner, "owner", "", "GitHub repository owner (overrides git remote detection)")
	initCmd.Flags().StringVar(&initName, "name", "", "GitHub repository name (overrides git remote detection)")
}

// promptDefault reads a single line of input from r, returning defaultValue if empty.
func promptDefault(scanner *bufio.Scanner, prompt, defaultValue string, out *os.File) string {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", prompt)
	}
	if scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			return text
		}
	}
	return defaultValue
}

// initCmd registers the current repo into the shared local store and writes its config.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Register the current repo with pr-triage",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoDir, err := filepath.Abs(initRepoDir)
		if err != nil {
			return fmt.Errorf("resolve repo dir: %w", err)
		}

		owner := initOwner
		name := initName
		if owner == "" || name == "" {
			detOwner, detName, detErr := config.DetectRepoFromGit(repoDir)
			if detErr != nil && (owner == "" || name == "") {
				return fmt.Errorf("detect git repository: %w (use --owner and --name to set manually)", detErr)
			}
			if owner == "" {
				owner = detOwner
			}
			if name == "" {
				name = detName
			}
		}

		baseRef := initBaseRef
		pollInterval := initPollInterval
		timeout := initTimeout
		githubUser := initGitHubUser
		runtimeName := initRuntime
		modelName := initModel

		isInteractive := !initNonInteractive && term.IsTerminal(int(os.Stdin.Fd()))
		if isInteractive {
			scanner := bufio.NewScanner(os.Stdin)
			baseRef = promptDefault(scanner, "Base branch or glob to watch", baseRef, os.Stdout)
			pollInterval = promptDefault(scanner, "Poll interval", pollInterval, os.Stdout)
			timeout = promptDefault(scanner, "Agent execution timeout", timeout, os.Stdout)
			githubUser = promptDefault(scanner, "GitHub username for escalation", githubUser, os.Stdout)
		}

		// Write config file to .pr-triage/config.yaml
		cfg := &config.Config{
			BaseRef:      baseRef,
			PollInterval: pollInterval,
			Timeout:      timeout,
			GitHubUser:   githubUser,
			Runtime:      runtimeName,
			Model:        modelName,
		}

		// Make --model concrete: pin it into routing.routine.model so the agent
		// actually runs on it. The top-level Model field alone is NOT consulted
		// at agent invocation (the orchestrator routes on routing.<tier>.model),
		// so without this the flag is a silent no-op. Only touch routing when a
		// model was explicitly provided; otherwise leave routing unset so the
		// daemon's DefaultConfig routing applies unchanged.
		if modelName != "" {
			routing := config.DefaultConfig().Routing
			r := routing["routine"]
			r.Model = modelName
			routing["routine"] = r
			cfg.Routing = routing
		}

		configRelPath := filepath.Join(".pr-triage", "config.yaml")
		configAbsPath := filepath.Join(repoDir, configRelPath)
		if err := config.Save(configAbsPath, cfg); err != nil {
			return fmt.Errorf("write config file: %w", err)
		}

		// Register in SQLite DB
		dbPath := initDBPath
		if dbPath == "" {
			dbPath = db.DefaultDBPath()
		}

		dbConn, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database %s: %w", dbPath, err)
		}
		defer func() { _ = dbConn.Close() }()

		store := db.NewStore(dbConn)
		repoRecord := &db.Repo{
			Owner:        owner,
			Name:         name,
			BaseRef:      baseRef,
			PollInterval: pollInterval,
			ConfigPath:   configRelPath,
		}

		savedRepo, err := store.UpsertRepo(repoRecord)
		if err != nil {
			return fmt.Errorf("upsert repo in database: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Registered repository %s/%s (id=%d) in store %s\nConfig written to %s\n",
			savedRepo.Owner, savedRepo.Name, savedRepo.ID, dbPath, configAbsPath)
		return nil
	},
}
