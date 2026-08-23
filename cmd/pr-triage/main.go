// Command pr-triage is the entrypoint for the pr-triage CLI.
package main

import (
	"os"

	"github.com/dustinmays/pr-triage/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
