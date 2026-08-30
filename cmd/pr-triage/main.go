// Command pr-triage is the entrypoint for the pr-triage CLI.
package main

import (
	"os"

	"github.com/dustinmays/pr-triage/internal/cli"

	// Register runtime adapters for their side-effecting init() so the
	// orchestrator can resolve them by name (e.g. "claude-code"). Without this
	// blank import the registry is empty and every run fails with
	// "unknown runtime". Add future adapters (codex, opencode) here too.
	_ "github.com/dustinmays/pr-triage/internal/runtime/claudecode"
	_ "github.com/dustinmays/pr-triage/internal/runtime/codex"
	_ "github.com/dustinmays/pr-triage/internal/runtime/opencode"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
