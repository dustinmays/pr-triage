package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dustinmays/pr-triage/internal/config"
)

var configShowRepoDir string

func init() {
	configShowCmd.Flags().StringVar(&configShowRepoDir, "repo-dir", ".", "Repository directory whose .pr-triage/config.yaml to read")
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

// configCmd groups configuration inspection subcommands.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect pr-triage configuration",
}

// configShowCmd prints the effective merged configuration.
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the effective (merged) configuration",
	Long: "Print the effective configuration: the repo's .pr-triage/config.yaml layered " +
		"over built-in defaults, including the full signal_tiers and routing tables the " +
		"daemon will actually use. Read-only; writes nothing.",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		repoDir, err := filepath.Abs(configShowRepoDir)
		if err != nil {
			return fmt.Errorf("resolve repo dir: %w", err)
		}
		cfgPath := filepath.Join(repoDir, ".pr-triage", "config.yaml")

		var cfg *config.Config
		if _, statErr := os.Stat(cfgPath); statErr == nil {
			cfg, err = config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config %s: %w", cfgPath, err)
			}
		} else {
			// No config file -> show the pure built-in defaults.
			cfg = config.DefaultConfig()
			fmt.Fprintf(cmd.ErrOrStderr(), "# no %s found; showing built-in defaults\n", cfgPath)
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		fmt.Fprint(out, string(data))
		return nil
	},
}
