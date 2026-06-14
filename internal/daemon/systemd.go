package daemon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// SystemdManager handles daemon registration with systemd (Linux).
type SystemdManager struct {
	servicePath string
}

// NewSystemdManager creates a new SystemdManager.
func NewSystemdManager() *SystemdManager {
	home, _ := os.UserHomeDir()
	servicePath := filepath.Join(home, ".config/systemd/user/daemon-hound.service")
	return &SystemdManager{servicePath: servicePath}
}

// Install registers the daemon with systemd.
func (s *SystemdManager) Install() error {
	// Create systemd user directory if it doesn't exist
	dir := filepath.Dir(s.servicePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd user directory: %w", err)
	}

	// Get the daemon executable path
	exePath, err := GetDaemonPath()
	if err != nil {
		return err
	}

	// Create the service content
	serviceContent, err := s.generateService(exePath)
	if err != nil {
		return err
	}

	// Write the service file
	if err := os.WriteFile(s.servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd daemon
	cmd := exec.Command("systemctl", "--user", "daemon-reload")
	if err := cmd.Run(); err != nil {
		_ = os.Remove(s.servicePath)
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable the service
	cmd = exec.Command("systemctl", "--user", "enable", "daemon-hound.service")
	if err := cmd.Run(); err != nil {
		_ = os.Remove(s.servicePath)
		return fmt.Errorf("failed to enable service: %w", err)
	}

	// Start the service
	cmd = exec.Command("systemctl", "--user", "start", "daemon-hound.service")
	if err := cmd.Run(); err != nil {
		_ = exec.Command("systemctl", "--user", "disable", "daemon-hound.service").Run()
		_ = os.Remove(s.servicePath)
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

// Uninstall unregisters the daemon from systemd.
func (s *SystemdManager) Uninstall() error {
	// Stop the service
	cmd := exec.Command("systemctl", "--user", "stop", "daemon-hound.service")
	_ = cmd.Run() // Ignore errors if not running

	// Disable the service
	cmd = exec.Command("systemctl", "--user", "disable", "daemon-hound.service")
	_ = cmd.Run() // Ignore errors if not enabled

	// Reload systemd daemon
	cmd = exec.Command("systemctl", "--user", "daemon-reload")
	_ = cmd.Run() // Ignore errors

	// Remove the service file
	if err := os.Remove(s.servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	return nil
}

// IsInstalled checks if the daemon is registered with systemd.
func (s *SystemdManager) IsInstalled() (bool, error) {
	_, err := os.Stat(s.servicePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check service file: %w", err)
	}
	return true, nil
}

// IsRunning checks if the daemon is currently running.
func (s *SystemdManager) IsRunning() (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "daemon-hound.service")
	err := cmd.Run()
	return err == nil, nil
}

// generateService generates the systemd service file content.
func (s *SystemdManager) generateService(exePath string) (string, error) {
	logPath := GetLogPath()

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}

	serviceTemplate := `[Unit]
Description=DaemonHound Background Sync Service
After=network.target

[Service]
Type=simple
ExecStart={{.ExePath}} daemon run
Restart=always
RestartSec=10

StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target`

	data := struct {
		ExePath string
	}{
		ExePath: exePath,
	}

	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse service template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute service template: %w", err)
	}

	return buf.String(), nil
}
