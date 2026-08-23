package auth

import (
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	ServiceName       = "pr-triage"
	AccountName       = "github-token"
	EnvGitHubToken    = "GITHUB_TOKEN"
	EnvGitHubTokenAlt = "GH_TOKEN"
)

// EnvToken returns a GitHub token from the environment and the variable name
// that supplied it.
// Precedence: GITHUB_TOKEN > GH_TOKEN.
func EnvToken() (string, string) {
	if tok := os.Getenv(EnvGitHubToken); tok != "" {
		return tok, EnvGitHubToken
	}
	if tok := os.Getenv(EnvGitHubTokenAlt); tok != "" {
		return tok, EnvGitHubTokenAlt
	}
	return "", ""
}

// GetToken returns the GitHub token from the environment or keyring.
// Precedence: GITHUB_TOKEN env var > GH_TOKEN env var > OS keyring.
func GetToken() (string, error) {
	if tok, _ := EnvToken(); tok != "" {
		return tok, nil
	}

	tok, err := keyring.Get(ServiceName, AccountName)
	if err != nil {
		return "", fmt.Errorf("no GITHUB_TOKEN or GH_TOKEN in env or keyring: %w", err)
	}
	return tok, nil
}

// SetToken stores the GitHub token in the OS keyring.
func SetToken(token string) error {
	return keyring.Set(ServiceName, AccountName, token)
}

// DeleteToken removes the GitHub token from the OS keyring.
func DeleteToken() error {
	return keyring.Delete(ServiceName, AccountName)
}

// HasKeyringToken returns true if a token is stored in the OS keyring.
func HasKeyringToken() bool {
	_, err := keyring.Get(ServiceName, AccountName)
	return err == nil
}
