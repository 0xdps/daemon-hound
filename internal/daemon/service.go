package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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

// GetDaemonPath returns the path to the dhd executable.
func GetDaemonPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return exe, nil
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
