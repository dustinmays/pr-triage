// Command agent-sync renders the neutral agent sources under agents/*.agent.yaml
// into the tool-specific formats consumed by Claude Code and OpenCode.
//
// By default it writes the generated files. With -check it only reports
// drift (files missing or different from what would be generated) and exits
// non-zero, which is what CI uses to gate on stale generated files.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dustinmays/pr-triage/internal/agentsync"
)

func main() {
	check := flag.Bool("check", false, "check for drift instead of writing files")
	agentsDir := flag.String("agents-dir", "agents", "directory containing *.agent.yaml sources")
	root := flag.String("root", ".", "repo root under which generated files are written/checked")
	flag.Parse()

	if *check {
		drifted, err := agentsync.Check(*agentsDir, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if len(drifted) > 0 {
			fmt.Println("agent definitions out of sync (run `make agents-sync`):")
			for _, path := range drifted {
				fmt.Println(path)
			}
			os.Exit(1)
		}
		fmt.Println("agent definitions in sync")
		return
	}

	written, err := agentsync.Sync(*agentsDir, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, path := range written {
		fmt.Println(path)
	}
}
