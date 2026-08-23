package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dustinmays/pr-triage/internal/auth"
	"github.com/zalando/go-keyring"
)

func init() {
	keyring.MockInit()
}

func TestSetupCommandWithFlag(t *testing.T) {
	t.Setenv(auth.EnvGitHubToken, "")
	t.Setenv(auth.EnvGitHubTokenAlt, "")
	_ = auth.DeleteToken()
	setupTokenFlag = ""

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
	t.Setenv(auth.EnvGitHubToken, "")
	t.Setenv(auth.EnvGitHubTokenAlt, "")
	_ = auth.DeleteToken()
	setupTokenFlag = ""

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
