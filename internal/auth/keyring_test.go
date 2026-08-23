package auth

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func init() {
	keyring.MockInit()
}

func TestEnvTokenPrefersGitHubToken(t *testing.T) {
	t.Setenv(EnvGitHubToken, "github-token")
	t.Setenv(EnvGitHubTokenAlt, "gh-token")

	tok, name := EnvToken()
	if tok != "github-token" || name != EnvGitHubToken {
		t.Fatalf("EnvToken() = (%q, %q), want (%q, %q)", tok, name, "github-token", EnvGitHubToken)
	}
}

func TestEnvTokenFallsBackToGHToken(t *testing.T) {
	t.Setenv(EnvGitHubToken, "")
	t.Setenv(EnvGitHubTokenAlt, "gh-token")

	tok, name := EnvToken()
	if tok != "gh-token" || name != EnvGitHubTokenAlt {
		t.Fatalf("EnvToken() = (%q, %q), want (%q, %q)", tok, name, "gh-token", EnvGitHubTokenAlt)
	}
}

func TestGetTokenPrefersEnvOverKeyring(t *testing.T) {
	_ = keyring.Set(ServiceName, AccountName, "keyring-val")
	t.Setenv(EnvGitHubToken, "env-val")

	tok, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken error: %v", err)
	}
	if tok != "env-val" {
		t.Fatalf("GetToken() = %q, want %q", tok, "env-val")
	}
}

func TestGetTokenKeyringFallback(t *testing.T) {
	t.Setenv(EnvGitHubToken, "")
	t.Setenv(EnvGitHubTokenAlt, "")
	_ = keyring.Set(ServiceName, AccountName, "keyring-val")

	tok, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken error: %v", err)
	}
	if tok != "keyring-val" {
		t.Fatalf("GetToken() = %q, want %q", tok, "keyring-val")
	}
}

func TestSetTokenAndGetToken(t *testing.T) {
	t.Setenv(EnvGitHubToken, "")
	t.Setenv(EnvGitHubTokenAlt, "")

	if err := SetToken("secret-123"); err != nil {
		t.Fatalf("SetToken error: %v", err)
	}

	if !HasKeyringToken() {
		t.Fatalf("HasKeyringToken() = false, want true")
	}

	tok, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken error: %v", err)
	}
	if tok != "secret-123" {
		t.Fatalf("GetToken() = %q, want %q", tok, "secret-123")
	}

	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken error: %v", err)
	}

	if HasKeyringToken() {
		t.Fatalf("HasKeyringToken() = true after delete, want false")
	}
}

func TestGetTokenErrorWhenEmpty(t *testing.T) {
	t.Setenv(EnvGitHubToken, "")
	t.Setenv(EnvGitHubTokenAlt, "")
	_ = keyring.Delete(ServiceName, AccountName)

	tok, err := GetToken()
	if err == nil {
		t.Fatalf("expected error from GetToken when empty, got tok=%q", tok)
	}
}
