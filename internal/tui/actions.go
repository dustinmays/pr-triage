package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// BrowserOpener is a function type that opens a URL in the default browser.
type BrowserOpener func(url string) error

// DefaultOpenBrowser opens a URL using the OS default browser launcher.
func DefaultOpenBrowser(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	return cmd.Start()
}

// ReadLogFile reads the log file content for a run.
func ReadLogFile(logPath string) (string, error) {
	if logPath == "" {
		return "", fmt.Errorf("no log path recorded for this run")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("read log file %s: %w", logPath, err)
	}
	return string(data), nil
}
