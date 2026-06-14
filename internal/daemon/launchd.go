package daemon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// LaunchdManager handles daemon registration with macOS launchd.
type LaunchdManager struct {
	plistPath string
}

// NewLaunchdManager creates a new LaunchdManager.
func NewLaunchdManager() *LaunchdManager {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library/LaunchAgents/com.daemon-hound.plist")
	return &LaunchdManager{plistPath: plistPath}
}

// Install registers the daemon with launchd.
func (l *LaunchdManager) Install() error {
	// Create LaunchAgents directory if it doesn't exist
	dir := filepath.Dir(l.plistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	// Get the daemon executable path
	exePath, err := GetDaemonPath()
	if err != nil {
		return err
	}

	// Create the plist content
	plistContent, err := l.generatePlist(exePath)
	if err != nil {
		return err
	}

	// Write the plist file
	if err := os.WriteFile(l.plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	// Load the plist with launchctl
	cmd := exec.Command("launchctl", "load", l.plistPath)
	if err := cmd.Run(); err != nil {
		// Clean up if load fails
		_ = os.Remove(l.plistPath)
		return fmt.Errorf("failed to load plist with launchctl: %w", err)
	}

	return nil
}

// Uninstall unregisters the daemon from launchd.
func (l *LaunchdManager) Uninstall() error {
	// Unload the plist with launchctl
	cmd := exec.Command("launchctl", "unload", l.plistPath)
	_ = cmd.Run() // Ignore errors if not loaded

	// Remove the plist file
	if err := os.Remove(l.plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	return nil
}

// IsInstalled checks if the daemon is registered with launchd.
func (l *LaunchdManager) IsInstalled() (bool, error) {
	_, err := os.Stat(l.plistPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check plist file: %w", err)
	}
	return true, nil
}

// IsRunning checks if the daemon is currently running.
func (l *LaunchdManager) IsRunning() (bool, error) {
	cmd := exec.Command("launchctl", "list", "com.daemon-hound")
	err := cmd.Run()
	return err == nil, nil
}

// generatePlist generates the launchd plist content.
func (l *LaunchdManager) generatePlist(exePath string) (string, error) {
	logPath := GetLogPath()
	errLogPath := GetErrorLogPath()

	// Ensure log directories exist
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}

	plistTemplate := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.daemon-hound</string>
	
	<key>ProgramArguments</key>
	<array>
		<string>{{.ExePath}}</string>
		<string>daemon</string>
		<string>run</string>
	</array>
	
	<key>RunAtLoad</key>
	<true/>
	
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	
	<key>StandardOutPath</key>
	<string>{{.LogPath}}</string>
	
	<key>StandardErrorPath</key>
	<string>{{.ErrLogPath}}</string>
</dict>
</plist>`

	data := struct {
		ExePath    string
		LogPath    string
		ErrLogPath string
	}{
		ExePath:    exePath,
		LogPath:    logPath,
		ErrLogPath: errLogPath,
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse plist template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute plist template: %w", err)
	}

	return buf.String(), nil
}
