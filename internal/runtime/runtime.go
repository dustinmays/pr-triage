package runtime

import (
	"context"
	"io"
)

// AgentRuntime is the abstraction implemented by each coding-agent adapter
// (claude-code, codex, opencode, ...). Adapters self-register an
// AgentRuntime implementation into the package registry via Register,
// typically from an init() function.
type AgentRuntime interface {
	// Name returns the runtime's registry name (e.g. "claude-code").
	Name() string

	// Run executes a single agent invocation, writing the runtime's raw log
	// output to logFile as it is produced. It returns the process exit
	// code and any error encountered launching or waiting on the process.
	// A non-nil error means the run could not be completed at all; a
	// non-zero exitCode with a nil error means the process ran and exited
	// non-zero.
	Run(ctx context.Context, inv Invocation, logFile io.Writer) (exitCode int, err error)

	// ParseResult parses a previously written log (as produced by Run) into
	// a normalized Result. It must populate Result.CostBasis from the
	// runtime's own structured output, never by scraping log text for
	// substrings that only one runtime happens to emit.
	ParseResult(log io.Reader) (*Result, error)

	// ClassifyOutcome normalizes a parsed Result and the process exit code
	// into an Outcome.
	ClassifyOutcome(res *Result, exitCode int) Outcome
}
