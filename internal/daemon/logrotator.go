// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogRotator manages daemon log rotation based on size and retention.
type LogRotator struct {
	logPath         string
	errorLogPath    string
	maxLogSize      int64 // in bytes
	maxErrorLogSize int64
	retentionDays   int
}

// NewLogRotator creates a log rotator with the given limits.
func NewLogRotator(maxLogSizeMB, maxErrorLogSizeMB, retentionDays int) *LogRotator {
	dhPath := filepath.Join(os.Getenv("HOME"), ".daemon-hound")
	return &LogRotator{
		logPath:         filepath.Join(dhPath, "daemon.log"),
		errorLogPath:    filepath.Join(dhPath, "daemon.error.log"),
		maxLogSize:      int64(maxLogSizeMB) * 1024 * 1024,
		maxErrorLogSize: int64(maxErrorLogSizeMB) * 1024 * 1024,
		retentionDays:   retentionDays,
	}
}

// Rotate checks and rotates logs if they exceed size or retention limits.
func (lr *LogRotator) Rotate() error {
	if err := lr.rotateFile(lr.logPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to rotate daemon.log: %v\n", err)
	}

	if err := lr.rotateFile(lr.errorLogPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to rotate daemon.error.log: %v\n", err)
	}

	// Clean up old rotated logs
	if err := lr.cleanupOldLogs(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to cleanup old logs: %v\n", err)
	}

	return nil
}

// rotateFile rotates a single log file if it exceeds max size.
func (lr *LogRotator) rotateFile(logPath string) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet
		}
		return err
	}

	// Determine which size limit applies
	maxSize := lr.maxLogSize
	if logPath == lr.errorLogPath {
		maxSize = lr.maxErrorLogSize
	}

	// If file size exceeds limit, rotate it
	if info.Size() > maxSize {
		timestamp := time.Now().Format("2006-01-02-15-04-05")
		ext := filepath.Ext(logPath)
		base := logPath[:len(logPath)-len(ext)]
		rotatedPath := fmt.Sprintf("%s.%s%s", base, timestamp, ext)

		if err := os.Rename(logPath, rotatedPath); err != nil {
			return fmt.Errorf("failed to rotate log file: %w", err)
		}
	}

	return nil
}

// cleanupOldLogs removes rotated logs older than retention period.
func (lr *LogRotator) cleanupOldLogs() error {
	dhPath := filepath.Join(os.Getenv("HOME"), ".daemon-hound")

	entries, err := os.ReadDir(dhPath)
	if err != nil {
		return err
	}

	cutoffTime := time.Now().AddDate(0, 0, -lr.retentionDays)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Match rotated log files: daemon.YYYY-MM-DD-HH-MM-SS.log
		if !((name == "daemon.log" || name == "daemon.error.log") ||
			(strings.HasPrefix(name, "daemon.") && (strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".error.log")))) {
			continue
		}

		// Only delete rotated files (those with timestamps)
		if name == "daemon.log" || name == "daemon.error.log" {
			continue
		}

		path := filepath.Join(dhPath, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			_ = os.Remove(path)
		}
	}

	return nil
}
