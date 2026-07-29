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
	"runtime"
	"strings"

	"github.com/0xdps/daemon-hound/internal/config"
)

// ServiceManager handles daemon registration with the OS.
type ServiceManager interface {
	Install() error
	Uninstall() error
	IsInstalled() (bool, error)
	IsRunning() (bool, error)
}

// NewServiceManager returns the appropriate ServiceManager for the current OS.
func NewServiceManager() ServiceManager {
	switch runtime.GOOS {
	case "darwin":
		return NewLaunchdManager()
	case "linux":
		return NewSystemdManager()
	case "windows":
		return &TaskSchedulerManager{}
	default:
		return &NoOpManager{} // Unsupported OS
	}
}

// NoOpManager is a placeholder for unsupported OSes.
type NoOpManager struct{}

func (n *NoOpManager) Install() error {
	return fmt.Errorf("daemon auto-installation not supported on %s. Manual setup required", runtime.GOOS)
}

func (n *NoOpManager) Uninstall() error {
	return fmt.Errorf("daemon auto-uninstall not supported on %s", runtime.GOOS)
}

func (n *NoOpManager) IsInstalled() (bool, error) {
	return false, fmt.Errorf("daemon check not supported on %s", runtime.GOOS)
}

func (n *NoOpManager) IsRunning() (bool, error) {
	return false, fmt.Errorf("daemon check not supported on %s", runtime.GOOS)
}

// GetDaemonPath returns the path to the dhd executable (the real binary
// that is currently running, not the app-bundle copy).
func GetDaemonPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	// If running from inside an app bundle, resolve to the real binary for
	// proper path resolution and comparison when checking for updates.
	if strings.Contains(exe, ".app/Contents/MacOS/") {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			return resolved, nil
		}
	}
	return exe, nil
}

// GetAppBundlePath returns the path to the dhd executable inside the macOS app
// bundle, or an empty string if the bundle is not installed.
// It checks the following locations in order:
//  1. ~/Applications/DaemonHound.app (user-installed DMG or manual)
//  2. /opt/homebrew/opt/daemon-hound/DaemonHound.app (Homebrew on Apple Silicon)
//  3. /usr/local/opt/daemon-hound/DaemonHound.app (Homebrew on Intel)
func GetAppBundlePath() string {
	if runtime.GOOS != "darwin" {
		return ""
	}

	candidates := []string{}

	// 1. User Applications folder
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Applications", "DaemonHound.app", "Contents", "MacOS", "dhd"))
	}

	// 2. Homebrew on Apple Silicon
	candidates = append(candidates, "/opt/homebrew/opt/daemon-hound/DaemonHound.app/Contents/MacOS/dhd")

	// 3. Homebrew on Intel
	candidates = append(candidates, "/usr/local/opt/daemon-hound/DaemonHound.app/Contents/MacOS/dhd")

	for _, bundleExe := range candidates {
		if _, err := os.Stat(bundleExe); err == nil {
			return bundleExe
		}
	}
	return ""
}

// GetLogPath returns the path to the daemon log file.
func GetLogPath() string {
	return filepath.Join(config.AppDir(), "daemon.log")
}

// GetErrorLogPath returns the path to the daemon error log file.
func GetErrorLogPath() string {
	return filepath.Join(config.AppDir(), "daemon.error.log")
}

// GetPIDPath returns the path to the daemon PID file.
func GetPIDPath() string {
	return filepath.Join(config.AppDir(), "daemon.pid")
}

// GetHaltPath returns the path to the daemon halt sentinel file.
// When this file exists, the daemon is suspended waiting for the user to
// resolve conflicts before syncing resumes.
func GetHaltPath() string {
	return filepath.Join(config.AppDir(), "daemon.halt")
}

// IsHalted reports whether the daemon halt sentinel file exists.
func IsHalted() bool {
	_, err := os.Stat(GetHaltPath())
	return err == nil
}

// WriteHalt creates the halt sentinel file with the given reason.
func WriteHalt(reason string) error {
	return os.WriteFile(GetHaltPath(), []byte(reason), 0644)
}

// ClearHalt removes the halt sentinel file, allowing sync to resume.
func ClearHalt() error {
	err := os.Remove(GetHaltPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadHaltReason returns the reason stored in the halt sentinel file.
func ReadHaltReason() string {
	data, err := os.ReadFile(GetHaltPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
