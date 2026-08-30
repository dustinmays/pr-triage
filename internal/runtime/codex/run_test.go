package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

// Given a review invocation, when the adapter launches Codex, it executes `codex exec --json --ephemeral --sandbox workspace-write`.
// This pins machine-readable output, non-persistent session state, and workspace-write sandbox access.
func TestRunInvokesCodexExecWithRatifiedBaseFlags(t *testing.T) {
	workdir := t.TempDir()
	inv := runtime.Invocation{
		AgentName: "review-agent",
		Model:     knownPricedModel,
		Prompt:    "Verify review contract",
		Workdir:   workdir,
	}

	result := runWithFakeCodex(t, inv, `{"type":"turn.started"}`+"\n", 0)
	if result.err != nil {
		t.Fatalf("Run failed: %v", result.err)
	}

	_, args := parseRecordedRun(result.record)
	wantBase := []string{"exec", "--json", "--ephemeral", "--sandbox", "workspace-write"}
	if len(args) < len(wantBase) || !reflect.DeepEqual(args[:len(wantBase)], wantBase) {
		t.Fatalf("Run argv must start with ratified base flags %v, got: %v", wantBase, args)
	}
}

// Given an invocation specifying a plain model name, when building CLI arguments, the adapter passes `-m <model>` verbatim.
// This ensures Codex model names are not mistakenly rejected by provider/model slash validation.
func TestRunPassesModelExactlyAsMinusMFlagWithoutSlashValidation(t *testing.T) {
	workdir := t.TempDir()
	// Plain Codex model name without slash (must NOT be rejected like OpenCode)
	const plainModel = "gpt-5.6-sol"
	inv := runtime.Invocation{
		Model:   plainModel,
		Prompt:  "Review diff",
		Workdir: workdir,
	}

	result := runWithFakeCodex(t, inv, `{"type":"turn.started"}`+"\n", 0)
	if result.err != nil {
		t.Fatalf("Run failed for plain model without slash: %v (slash validation must not be applied)", result.err)
	}

	_, args := parseRecordedRun(result.record)
	foundModelFlag := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-m" && args[i+1] == plainModel {
			foundModelFlag = true
			break
		}
	}
	if !foundModelFlag {
		t.Fatalf("argv does not contain exact model passthrough ['-m', %q]; got: %v", plainModel, args)
	}
}

// Given an invocation with a rendered review-agent prompt, when launching Codex, the prompt is passed inline as the final argument.
// This supplies triage instructions directly without requiring pre-configured named agent files.
func TestRunPassesReviewPromptInlineAsFinalPositionalArgument(t *testing.T) {
	workdir := t.TempDir()
	const inlinePrompt = "You are the automated review agent. Check schema conformance."
	inv := runtime.Invocation{
		Model:   knownPricedModel,
		Prompt:  inlinePrompt,
		Workdir: workdir,
	}

	result := runWithFakeCodex(t, inv, `{"type":"turn.started"}`+"\n", 0)
	if result.err != nil {
		t.Fatalf("Run failed: %v", result.err)
	}

	_, args := parseRecordedRun(result.record)
	if len(args) == 0 {
		t.Fatal("recorded argv is empty")
	}
	lastArg := args[len(args)-1]
	if lastArg != inlinePrompt {
		t.Fatalf("last argv argument = %q, want inline prompt %q", lastArg, inlinePrompt)
	}
}

// Given an invocation specifying a target workdir, when the child process is spawned, its working directory matches Invocation.Workdir.
// This ensures Codex operations take place within the intended repository worktree.
func TestRunExecutesInTheInvocationWorkdir(t *testing.T) {
	expectedWorkdir := t.TempDir()
	inv := runtime.Invocation{
		Model:   knownPricedModel,
		Prompt:  "Cwd check",
		Workdir: expectedWorkdir,
	}

	result := runWithFakeCodex(t, inv, `{"type":"turn.started"}`+"\n", 0)
	if result.err != nil {
		t.Fatalf("Run failed: %v", result.err)
	}

	cwd, _ := parseRecordedRun(result.record)
	if cwd != expectedWorkdir {
		t.Fatalf("process executed in cwd %q, want Invocation.Workdir %q", cwd, expectedWorkdir)
	}
}

// Given a standard triage invocation, when assembling arguments, `--skip-git-repo-check` is omitted.
// This guarantees production runs strictly enforce execution inside genuine Git repositories.
func TestRunOmitsSkipGitRepoCheckFlagInProduction(t *testing.T) {
	workdir := t.TempDir()
	inv := runtime.Invocation{
		Model:   knownPricedModel,
		Prompt:  "Production flag check",
		Workdir: workdir,
	}

	result := runWithFakeCodex(t, inv, `{"type":"turn.started"}`+"\n", 0)
	if result.err != nil {
		t.Fatalf("Run failed: %v", result.err)
	}

	_, args := parseRecordedRun(result.record)
	for _, arg := range args {
		if arg == "--skip-git-repo-check" || strings.HasPrefix(arg, "--skip-git-repo-check") {
			t.Fatalf("production arguments must NOT include --skip-git-repo-check; found %q in argv %v", arg, args)
		}
	}
}

// Given a Codex run, when output logging begins, the adapter emits a namespaced invocation envelope containing the model before child JSONL.
// This preserves model identity in the log stream so downstream parsing can perform honest cost estimation.
func TestRunWritesInvocationEnvelopeBeforeChildJSONL(t *testing.T) {
	workdir := t.TempDir()
	inv := runtime.Invocation{
		Model:   knownPricedModel,
		Prompt:  "Envelope order check",
		Workdir: workdir,
	}

	const childJSONL = `{"type":"thread.started","thread_id":"test-thread-id"}` + "\n"
	result := runWithFakeCodex(t, inv, childJSONL, 0)
	if result.err != nil {
		t.Fatalf("Run failed: %v", result.err)
	}

	lines := strings.Split(strings.TrimRight(result.log, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("log must contain at least envelope and child JSONL, got %d lines:\n%s", len(lines), result.log)
	}

	// Line 1 must parse into the namespaced invocation envelope schema
	var rawEnvelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &rawEnvelope); err != nil {
		t.Fatalf("first log line is not valid JSON: %v (line=%q)", err, lines[0])
	}
	if len(rawEnvelope) != 1 || rawEnvelope["pr_triage_codex"] == nil {
		t.Fatalf("first log line must have single namespaced key 'pr_triage_codex', got: %s", lines[0])
	}

	var parsedEnvelope invocationEnvelope
	if err := json.Unmarshal([]byte(lines[0]), &parsedEnvelope); err != nil {
		t.Fatalf("failed unmarshaling envelope schema: %v", err)
	}
	if parsedEnvelope.PrTriageCodex.Version != envelopeSchemaVersion {
		t.Fatalf("envelope version = %d, want %d", parsedEnvelope.PrTriageCodex.Version, envelopeSchemaVersion)
	}
	if parsedEnvelope.PrTriageCodex.Kind != "invocation" {
		t.Fatalf("envelope kind = %q, want 'invocation'", parsedEnvelope.PrTriageCodex.Kind)
	}
	if parsedEnvelope.PrTriageCodex.Model != knownPricedModel {
		t.Fatalf("envelope model = %q, want %q", parsedEnvelope.PrTriageCodex.Model, knownPricedModel)
	}

	// Child JSONL must appear after the envelope in the log
	envelopeIndex := strings.Index(result.log, lines[0])
	childIndex := strings.Index(result.log, `{"type":"thread.started"`)
	if childIndex == -1 {
		t.Fatalf("child JSONL not found in adapter log:\n%s", result.log)
	}
	if childIndex <= envelopeIndex {
		t.Fatalf("child JSONL at index %d must appear strictly after envelope at index %d", childIndex, envelopeIndex)
	}
}

// Given a log writer that rejects the invocation envelope, when Run is called, it fails before launching Codex.
// This prevents a run whose captured log cannot preserve the model identity required for honest cost reporting.
func TestRunInvocationEnvelopeWriteFailureAbortsBeforeLaunch(t *testing.T) {
	sentinel := errors.New("log is unavailable")
	a := New()
	a.Binary = "/binary/must/not/be/launched"

	exitCode, err := a.Run(context.Background(), runtime.Invocation{Model: knownPricedModel}, failingWriter{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want invocation-envelope write error wrapping %v", err, sentinel)
	}
	if exitCode != -1 {
		t.Fatalf("exitCode = %d, want -1 when the invocation envelope cannot be written", exitCode)
	}
}

// Given a log writer that silently short-writes the invocation envelope, when Run is called, it fails before launching Codex.
// This prevents malformed metadata from being treated as a usable execution log.
func TestRunInvocationEnvelopeShortWriteAbortsBeforeLaunch(t *testing.T) {
	a := New()
	a.Binary = "/binary/must/not/be/launched"

	exitCode, err := a.Run(context.Background(), runtime.Invocation{Model: knownPricedModel}, shortWriter{})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Run error = %v, want %v", err, io.ErrShortWrite)
	}
	if exitCode != -1 {
		t.Fatalf("exitCode = %d, want -1 when the invocation envelope is short-written", exitCode)
	}
}

// Given a PATH environment missing the codex executable, when Run is called, it returns exit code -1 and a non-nil error.
// This clearly distinguishes process launch failures from normal non-zero child command exits.
func TestRunMissingCodexBinaryReturnsLaunchError(t *testing.T) {
	rt := mustGetCodex(t)
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	inv := runtime.Invocation{
		Model:  knownPricedModel,
		Prompt: "Should fail to launch",
	}

	exitCode, err := rt.Run(context.Background(), inv, nil)
	if err == nil {
		t.Fatalf("Run succeeded with empty PATH, want launch error")
	}
	if exitCode != -1 {
		t.Fatalf("exitCode = %d, want -1 on process launch failure", exitCode)
	}
}

// Given a Codex process that exits with a non-zero exit code, when Run finishes, it returns the child exit code with a nil error.
// This allows outcome classifiers to inspect normal agent failures without treating them as runtime launch errors.
func TestRunNonZeroChildExitReportsExitCodeWithoutLaunchError(t *testing.T) {
	workdir := t.TempDir()
	inv := runtime.Invocation{
		Model:   knownPricedModel,
		Prompt:  "Exit non-zero check",
		Workdir: workdir,
	}

	const childExit = 3
	result := runWithFakeCodex(t, inv, `{"type":"turn.failed"}`+"\n", childExit)
	if result.err != nil {
		t.Fatalf("Run returned launch error %v, want nil error for normal non-zero child exit", result.err)
	}
	if result.exitCode != childExit {
		t.Fatalf("exitCode = %d, want %d from child exit", result.exitCode, childExit)
	}
}
