package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

// fakeRuntime is a minimal in-memory AgentRuntime for exercising the doctor
// command without launching a real subprocess.
type fakeRuntime struct {
	name    string
	log     string // written to logFile by Run
	runErr  error  // returned by Run (simulates a launch failure)
	outcome runtime.Outcome
	caps    *runtime.Capabilities
}

func (f *fakeRuntime) Name() string { return f.name }

func (f *fakeRuntime) Run(_ context.Context, _ runtime.Invocation, logFile io.Writer) (int, error) {
	if f.runErr != nil {
		return -1, f.runErr
	}
	if logFile != nil {
		_, _ = io.WriteString(logFile, f.log)
	}
	return 0, nil
}

func (f *fakeRuntime) ParseResult(log io.Reader) (*runtime.Result, error) {
	b, _ := io.ReadAll(log)
	return &runtime.Result{
		CostBasis: runtime.CostBasisExact,
		Summary:   string(b),
	}, nil
}

func (f *fakeRuntime) ClassifyOutcome(_ *runtime.Result, _ int) runtime.Outcome {
	if f.outcome == "" {
		return runtime.OutcomeSuccess
	}
	return f.outcome
}

func (f *fakeRuntime) Capabilities() runtime.Capabilities {
	if f.caps == nil {
		return runtime.Capabilities{CostBasis: runtime.CostBasisExact, EnforcesTimeout: true, ModelForm: runtime.ModelFormPlain}
	}
	return *f.caps
}

func runDoctor(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"runtime", "check"}, args...))
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		runtimeCheckModel = ""
	})
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestRuntimeCheck_Success(t *testing.T) {
	runtime.Register(&fakeRuntime{name: "fake-ok", log: "OK"})

	out, err := runDoctor(t, "fake-ok")
	if err != nil {
		t.Fatalf("expected success, got error: %v\noutput:\n%s", err, out)
	}
	for _, want := range []string{"registered", "process launched", "parseable result", "classified success", "is usable"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestRuntimeCheck_UnknownRuntimeFails(t *testing.T) {
	out, err := runDoctor(t, "definitely-not-registered")
	if err == nil {
		t.Fatalf("expected error for unknown runtime\n%s", out)
	}
	if !strings.Contains(out, "not registered") {
		t.Errorf("expected 'not registered' in output:\n%s", out)
	}
}

func TestRuntimeCheck_BadOutcomeFails(t *testing.T) {
	runtime.Register(&fakeRuntime{name: "fake-bad", log: "auth error: no key", outcome: runtime.OutcomeFailed})

	out, err := runDoctor(t, "fake-bad")
	if err == nil {
		t.Fatalf("expected error when runtime classifies failure\n%s", out)
	}
	if !strings.Contains(out, "auth error: no key") {
		t.Errorf("expected failing log tail to be printed:\n%s", out)
	}
}

func TestRuntimeCheck_ProviderSlashModelPreflight(t *testing.T) {
	caps := runtime.Capabilities{CostBasis: runtime.CostBasisExact, ModelForm: runtime.ModelFormProviderSlashModel}
	runtime.Register(&fakeRuntime{name: "fake-slash", log: "OK", caps: &caps})

	out, err := runDoctor(t, "fake-slash", "--model", "no-slash-here")
	if err == nil {
		t.Fatalf("expected pre-flight rejection of slash-less model\n%s", out)
	}
	if !strings.Contains(out, "provider/model form") {
		t.Errorf("expected model-form failure message:\n%s", out)
	}
}
