package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

// The ratified Codex adapter log begins with a namespaced structured invocation envelope
// so ParseResult can recover the model for honest cost estimation.
//
// Pinned envelope schema (version 1):
//
//	{
//	  "pr_triage_codex": {
//	    "version": 1,
//	    "kind": "invocation",
//	    "model": "gpt-5.6-sol"
//	  }
//	}
const envelopeSchemaVersion = 1

type invocationEnvelopePayload struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Model   string `json:"model"`
}

type invocationEnvelope struct {
	PrTriageCodex invocationEnvelopePayload `json:"pr_triage_codex"`
}

func invocationEnvelopeJSON(model string) string {
	env := invocationEnvelope{
		PrTriageCodex: invocationEnvelopePayload{
			Version: envelopeSchemaVersion,
			Kind:    "invocation",
			Model:   model,
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		panic(fmt.Sprintf("marshal invocation envelope: %v", err))
	}
	return string(b)
}

// Pricing verified 2026-08-29 at https://developers.openai.com/api/docs/models/compare:
// Model: gpt-5.6-sol
//
//	Uncached input: USD 4.00 per 1M tokens
//	Cached input:   USD 0.40 per 1M tokens
//	Output:         USD 20.00 per 1M tokens
//
// Captured usage from codex 0.151.0 fixture:
//
//	input_tokens: 16426
//	cached_input_tokens: 6912
//	output_tokens: 7
//
// Expected cost computation:
//
//	uncached_input = 16426 - 6912 = 9514
//	uncached_cost  = 9514 * 4.00 / 1e6   = 0.038056
//	cached_cost    = 6912 * 0.40 / 1e6   = 0.0027648
//	output_cost    = 7    * 20.00 / 1e6  = 0.000140
//	total_cost     = 0.038056 + 0.0027648 + 0.000140 = 0.0409608 USD
const (
	knownPricedModel        = "gpt-5.6-sol"
	knownPricedExpectedCost = 0.0409608
	costAssertionTolerance  = 1e-9

	// Fixture governance:
	// The two JSONL files (codex-0.151.0-success.jsonl and codex-0.151.0-failed.jsonl)
	// preserve the empirically captured Codex 0.151.0 event shapes and token counts;
	// only stable thread/item IDs and safe error wording were normalized. They are
	// ratified golden contracts once human gate 2 approves them.
	fixtureSuccess = "codex-0.151.0-success.jsonl"
	fixtureFailed  = "codex-0.151.0-failed.jsonl"
)

// mustGetCodex obtains the Codex runtime adapter from the shared registry.
// Under RED-first behavioral testing before implementation exists, this lookup
// fails and produces the intended runtime-unregistered failure across every test.
func mustGetCodex(t *testing.T) runtime.AgentRuntime {
	t.Helper()
	rt, err := runtime.Get(runtime.NameCodex)
	if err != nil {
		t.Fatalf("codex runtime is not registered: %v (adapter implementation or registration absent)", err)
	}
	return rt
}

func loadFixture(t *testing.T, filename string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("read fixture %s: %v", filename, err)
	}
	return string(raw)
}

func adapterLog(t *testing.T, model, fixtureFilename string) string {
	t.Helper()
	fixtureData := loadFixture(t, fixtureFilename)
	return invocationEnvelopeJSON(model) + "\n" + fixtureData
}

type fakeRunResult struct {
	record   string
	log      string
	exitCode int
	err      error
}

func runWithFakeCodex(t *testing.T, inv runtime.Invocation, childOutput string, childExitCode int) fakeRunResult {
	t.Helper()
	rt := mustGetCodex(t)

	binDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "record.txt")
	childOutPath := filepath.Join(t.TempDir(), "child-output.txt")

	if err := os.WriteFile(childOutPath, []byte(childOutput), 0o644); err != nil {
		t.Fatalf("write fake child output file: %v", err)
	}

	script := "#!/bin/sh\n" +
		"{ printf 'cwd=%s\\n' \"$(pwd)\"; for a in \"$@\"; do printf 'arg=%s\\n' \"$a\"; done; } > '" + recordPath + "'\n" +
		"cat '" + childOutPath + "'\n" +
		"exit " + strconv.Itoa(childExitCode) + "\n"

	fakeBin := filepath.Join(binDir, "codex")
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex executable: %v", err)
	}

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var logBuf bytes.Buffer
	exitCode, err := rt.Run(context.Background(), inv, &logBuf)

	recordBytes, _ := os.ReadFile(recordPath)
	return fakeRunResult{
		record:   string(recordBytes),
		log:      logBuf.String(),
		exitCode: exitCode,
		err:      err,
	}
}

func parseRecordedRun(record string) (cwd string, args []string) {
	for _, line := range strings.Split(record, "\n") {
		switch {
		case strings.HasPrefix(line, "cwd="):
			cwd = strings.TrimPrefix(line, "cwd=")
		case strings.HasPrefix(line, "arg="):
			args = append(args, strings.TrimPrefix(line, "arg="))
		}
	}
	return cwd, args
}
