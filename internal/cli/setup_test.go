package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/auth"
	"github.com/zalando/go-keyring"
)

func init() {
	keyring.MockInit()
}

// stubTokenValidator lets setup tests avoid a real network call to GitHub.
type stubTokenValidator struct {
	login string
	err   error
}

func (s stubTokenValidator) ValidateToken(context.Context) (string, error) {
	return s.login, s.err
}

// withStubTokenValidator swaps in a stub validator for the duration of a test.
func withStubTokenValidator(t *testing.T, login string, err error) {
	t.Helper()
	original := newTokenValidator
	newTokenValidator = func(string) tokenValidator {
		return stubTokenValidator{login: login, err: err}
	}
	t.Cleanup(func() { newTokenValidator = original })
}

func TestSetupCommandWithFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // daemon.IsRunning resolves its pid file under ~/.pr-triage
	t.Setenv(auth.EnvGitHubToken, "")
	t.Setenv(auth.EnvGitHubTokenAlt, "")
	_ = auth.DeleteToken()
	setupTokenFlag = ""
	withStubTokenValidator(t, "octocat", nil)

	buf := new(bytes.Buffer)
	rootCmd.SetIn(nil)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"setup", "--token", "flag-token-12345"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("rootCmd.Execute() error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "flag-token-12345") {
		t.Fatalf("setup command output leaked token: %q", out)
	}

	tok, err := auth.GetToken()
	if err != nil {
		t.Fatalf("auth.GetToken() error: %v", err)
	}
	if tok != "flag-token-12345" {
		t.Fatalf("stored token = %q, want %q", tok, "flag-token-12345")
	}
}

func TestSetupCommandWithStdin(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // daemon.IsRunning resolves its pid file under ~/.pr-triage
	t.Setenv(auth.EnvGitHubToken, "")
	t.Setenv(auth.EnvGitHubTokenAlt, "")
	_ = auth.DeleteToken()
	setupTokenFlag = ""
	withStubTokenValidator(t, "octocat", nil)

	inBuf := bytes.NewBufferString("piped-token-67890\n")
	outBuf := new(bytes.Buffer)

	rootCmd.SetIn(inBuf)
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(outBuf)
	rootCmd.SetArgs([]string{"setup"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("rootCmd.Execute() error: %v", err)
	}

	out := outBuf.String()
	if strings.Contains(out, "piped-token-67890") {
		t.Fatalf("setup command output leaked token: %q", out)
	}

	tok, err := auth.GetToken()
	if err != nil {
		t.Fatalf("auth.GetToken() error: %v", err)
	}
	if tok != "piped-token-67890" {
		t.Fatalf("stored token = %q, want %q", tok, "piped-token-67890")
	}
}

func TestSetupCommandEmptyFails(t *testing.T) {
	t.Setenv(auth.EnvGitHubToken, "")
	t.Setenv(auth.EnvGitHubTokenAlt, "")
	_ = auth.DeleteToken()
	setupTokenFlag = ""

	inBuf := bytes.NewBufferString("\n")
	outBuf := new(bytes.Buffer)

	rootCmd.SetIn(inBuf)
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(outBuf)
	rootCmd.SetArgs([]string{"setup"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("expected error on empty token input, got nil")
	}
}

func TestSetupCommandRejectsInvalidToken(t *testing.T) {
	t.Setenv(auth.EnvGitHubToken, "")
	t.Setenv(auth.EnvGitHubTokenAlt, "")
	_ = auth.DeleteToken()
	setupTokenFlag = ""
	withStubTokenValidator(t, "", errors.New("401 Bad credentials"))

	buf := new(bytes.Buffer)
	rootCmd.SetIn(nil)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"setup", "--token", "bad-token"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for a token GitHub rejects, got nil")
	}

	if _, err := auth.GetToken(); err == nil {
		t.Fatalf("rejected token should not have been stored in the keyring")
	}
}
