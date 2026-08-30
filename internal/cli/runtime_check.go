package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dustinmays/pr-triage/internal/git"
	"github.com/dustinmays/pr-triage/internal/runtime"
)

var (
	runtimeCheckModel   string
	runtimeCheckTimeout time.Duration
)

func init() {
	runtimeCheckCmd.Flags().StringVar(&runtimeCheckModel, "model", "", "Model to test the runtime with (form depends on the runtime; see its capabilities)")
	runtimeCheckCmd.Flags().DurationVar(&runtimeCheckTimeout, "timeout", 90*time.Second, "Timeout for the canned probe run")
	runtimeCmd.AddCommand(runtimeCheckCmd)
	runtimeCmd.AddCommand(runtimeListCmd)
	rootCmd.AddCommand(runtimeCmd)
}

// runtimeCmd groups runtime inspection/vetting subcommands.
var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Inspect and vet agent runtimes",
}

// runtimeListCmd lists the registered runtimes and their declared capabilities.
var runtimeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered runtimes and their declared capabilities",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		names := runtime.KnownNames()
		if len(names) == 0 {
			return fmt.Errorf("no runtimes registered (are the adapter blank-imports present in cmd/pr-triage/main.go?)")
		}
		for _, name := range names {
			rt, err := runtime.Get(name)
			if err != nil {
				continue
			}
			fmt.Fprintf(out, "%s\n", name)
			if caps, ok := runtime.CapabilitiesOf(rt); ok {
				fmt.Fprintf(out, "    cost basis:   %s\n", caps.CostBasis)
				fmt.Fprintf(out, "    model form:   %s\n", modelFormLabel(caps.ModelForm))
				fmt.Fprintf(out, "    enforces:     %s\n", enforcedLimits(caps))
				if caps.AuthModel != "" {
					fmt.Fprintf(out, "    auth:         %s\n", caps.AuthModel)
				}
			} else {
				fmt.Fprintf(out, "    (capabilities not declared)\n")
			}
		}
		return nil
	},
}

// runtimeCheckCmd runs a trivial canned prompt through a real adapter and
// reports whether it is usable *from this process's environment* — the gap
// between "works in my shell" and "works in the daemon".
var runtimeCheckCmd = &cobra.Command{
	Use:   "check <runtime>",
	Short: "Vet a runtime end-to-end with a trivial canned run",
	Long: "Runs a one-line prompt through the named runtime's real adapter in a temporary " +
		"working directory and reports, in order: the runtime is registered, the model matches " +
		"the runtime's required form, the process launched, it produced a parseable Result, and " +
		"how it classifies. Run it in the SAME environment the daemon runs in (its env is frozen " +
		"at start), so an auth or PATH problem surfaces here instead of when a PR silently escalates.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		out := cmd.OutOrStdout()

		pass := func(format string, a ...any) { fmt.Fprintf(out, "  ✓ "+format+"\n", a...) }
		fail := func(format string, a ...any) { fmt.Fprintf(out, "  ✗ "+format+"\n", a...) }

		fmt.Fprintf(out, "Checking runtime %q\n", name)

		// 1. Registered?
		adapter, err := runtime.Get(name)
		if err != nil {
			fail("not registered: %v", err)
			return fmt.Errorf("runtime %q is not usable", name)
		}
		pass("registered")

		// Declared capabilities (advisory).
		caps, hasCaps := runtime.CapabilitiesOf(adapter)
		if hasCaps {
			fmt.Fprintf(out, "    cost basis %s; model form %s; enforces %s\n",
				caps.CostBasis, modelFormLabel(caps.ModelForm), enforcedLimits(caps))
		}

		// 2. Model form matches the runtime's requirement (cheap pre-flight).
		if hasCaps && caps.ModelForm == runtime.ModelFormProviderSlashModel &&
			runtimeCheckModel != "" && !strings.Contains(runtimeCheckModel, "/") {
			fail("model %q is not in provider/model form required by %s", runtimeCheckModel, name)
			return fmt.Errorf("runtime %q is not usable with model %q", name, runtimeCheckModel)
		}
		if runtimeCheckModel != "" {
			pass("model %q accepted for %s form", runtimeCheckModel, modelFormLabel(capsModelForm(caps, hasCaps)))
		}

		// 3-5. Run the canned probe through the real adapter.
		tmp, err := os.MkdirTemp("", "pr-triage-runtime-check-")
		if err != nil {
			fail("could not create probe workdir: %v", err)
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()

		if _, err := git.Run(cmd.Context(), tmp, "init", "-q"); err != nil {
			fail("could not initialize probe git repository: %v", err)
			return err
		}

		inv := runtime.Invocation{
			Model:   runtimeCheckModel,
			Prompt:  "Reply with exactly the word OK and nothing else. Do not use any tools.",
			Workdir: tmp,
			Limits:  runtime.Limits{Timeout: runtimeCheckTimeout},
		}

		var logBuf bytes.Buffer
		exitCode, runErr := adapter.Run(cmd.Context(), inv, &logBuf)
		if runErr != nil {
			// Could not execute at all: binary missing, or a pre-launch rejection.
			fail("could not launch: %v", runErr)
			printLogTail(out, &logBuf)
			return fmt.Errorf("runtime %q could not be executed", name)
		}
		pass("process launched and exited (code %d)", exitCode)

		res, parseErr := adapter.ParseResult(bytes.NewReader(logBuf.Bytes()))
		if parseErr != nil || res == nil {
			fail("output was not parseable: %v", parseErr)
			printLogTail(out, &logBuf)
			return fmt.Errorf("runtime %q produced no parseable result", name)
		}
		if validateErr := res.Validate(); validateErr != nil {
			fail("result invalid: %v", validateErr)
			return fmt.Errorf("runtime %q produced an invalid result", name)
		}
		pass("produced a parseable result (cost basis: %s)", res.CostBasis)

		outcome := adapter.ClassifyOutcome(res, exitCode)
		switch outcome {
		case runtime.OutcomeSuccess:
			pass("classified success")
			fmt.Fprintf(out, "\nRuntime %q is usable from this environment.\n", name)
			return nil
		case runtime.OutcomeTimeout:
			fail("timed out after %s (raise --timeout, or the model/auth may be wrong)", runtimeCheckTimeout)
		default:
			fail("classified %s (likely an auth or model problem — see log tail)", outcome)
		}
		printLogTail(out, &logBuf)
		return fmt.Errorf("runtime %q ran but did not succeed (%s)", name, outcome)
	},
}

func capsModelForm(caps runtime.Capabilities, ok bool) runtime.ModelForm {
	if !ok {
		return runtime.ModelFormUnknown
	}
	return caps.ModelForm
}

func modelFormLabel(f runtime.ModelForm) string {
	if f == runtime.ModelFormUnknown {
		return "unspecified"
	}
	return string(f)
}

func enforcedLimits(caps runtime.Capabilities) string {
	var parts []string
	if caps.EnforcesTimeout {
		parts = append(parts, "timeout")
	}
	if caps.EnforcesTurns {
		parts = append(parts, "turns")
	}
	if caps.EnforcesBudget {
		parts = append(parts, "budget")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func printLogTail(out io.Writer, buf *bytes.Buffer) {
	const maxLines = 20
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		fmt.Fprintf(out, "    (no output captured)\n")
		return
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	fmt.Fprintf(out, "    --- last %d log line(s) ---\n", len(lines))
	for _, l := range lines {
		fmt.Fprintf(out, "    | %s\n", l)
	}
}
