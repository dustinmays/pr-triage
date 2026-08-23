// Package logs provides log rotation and viewing utilities.
package logs

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultMaxLogSize is the default threshold before rotating a log file (10MB).
const DefaultMaxLogSize int64 = 10 * 1024 * 1024

// DefaultMaxBackups is the default number of rotated log files to retain.
const DefaultMaxBackups = 5

// RotateIfNeeded checks if logPath exceeds maxSize and rotates it if necessary.
func RotateIfNeeded(logPath string, maxSize int64, maxBackups int) error {
	if maxSize <= 0 {
		maxSize = DefaultMaxLogSize
	}
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}

	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat log file: %w", err)
	}

	if info.Size() < maxSize {
		return nil
	}

	// Rotate backups starting from oldest
	for i := maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", logPath, i)
		newPath := fmt.Sprintf("%s.%d", logPath, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			if i+1 > maxBackups {
				_ = os.Remove(oldPath)
			} else {
				_ = os.Rename(oldPath, newPath)
			}
		}
	}

	// Rename current log to .1
	firstBackup := fmt.Sprintf("%s.1", logPath)
	if err := os.Rename(logPath, firstBackup); err != nil {
		return fmt.Errorf("rotate active log: %w", err)
	}

	return nil
}

// LogDir returns ~/.pr-triage/logs.
func LogDir(baseDir string) string {
	return filepath.Join(baseDir, "logs")
}
