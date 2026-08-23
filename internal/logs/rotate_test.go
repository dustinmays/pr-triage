package logs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dustinmays/pr-triage/internal/logs"
)

func TestRotateIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "daemon.log")

	// 1. Small file should not rotate
	_ = os.WriteFile(logPath, []byte("short log line\n"), 0644)
	if err := logs.RotateIfNeeded(logPath, 100, 3); err != nil {
		t.Fatalf("RotateIfNeeded failed: %v", err)
	}

	if _, err := os.Stat(logPath + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected no rotation for small file")
	}

	// 2. Large file exceeding limit should rotate
	largeData := make([]byte, 150)
	for i := range largeData {
		largeData[i] = 'A'
	}
	_ = os.WriteFile(logPath, largeData, 0644)

	if err := logs.RotateIfNeeded(logPath, 100, 3); err != nil {
		t.Fatalf("RotateIfNeeded failed: %v", err)
	}

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("expected log.1 to exist after rotation: %v", err)
	}

	// 3. Second rotation moves .1 to .2
	_ = os.WriteFile(logPath, largeData, 0644)
	if err := logs.RotateIfNeeded(logPath, 100, 3); err != nil {
		t.Fatalf("second RotateIfNeeded failed: %v", err)
	}

	if _, err := os.Stat(logPath + ".2"); err != nil {
		t.Fatalf("expected log.2 to exist after second rotation: %v", err)
	}

	// 4. Test non-existent file doesn't error
	if err := logs.RotateIfNeeded(filepath.Join(tmpDir, "nonexistent.log"), 100, 3); err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}
}

func TestLogDir(t *testing.T) {
	got := logs.LogDir("/tmp/test")
	want := fmt.Sprintf("%s%clogs", "/tmp/test", filepath.Separator)
	if got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}
