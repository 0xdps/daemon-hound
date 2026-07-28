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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	launchdLabel       = "com.0xdps.daemon-hound"
	legacyLaunchdLabel = "com.daemon-hound"
	appBundleName      = "Daemon Hound"
	appBundleID        = launchdLabel
)

// LaunchdManager handles daemon registration with macOS launchd.
type LaunchdManager struct {
	plistPath string
}

// NewLaunchdManager creates a new LaunchdManager.
func NewLaunchdManager() *LaunchdManager {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library/LaunchAgents", launchdLabel+".plist")
	return &LaunchdManager{plistPath: plistPath}
}

// Install registers the daemon with launchd.
func (l *LaunchdManager) Install() error {
	// Create LaunchAgents directory if it doesn't exist
	dir := filepath.Dir(l.plistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	// Remove any previously registered current or legacy agents so repeated
	// installs do not accumulate duplicate Background Items entries.
	if err := l.removeKnownAgents(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fully clean previous launch agents: %v\n", err)
	}

	// Get the daemon executable path
	exePath, err := GetDaemonPath()
	if err != nil {
		return err
	}

	// Check for the signed app bundle (from GitHub Releases DMG) first.
	// When installed via install.sh, the DMG is downloaded and the
	// app bundle extracted to ~/Applications/DaemonHound.app. This bundle
	// is Developer ID-signed and notarized by CI, so macOS shows "Verified
	// Developer" in System Settings with no TCC prompts for background use.
	//
	// If the signed bundle doesn't exist, use the raw binary directly.
	// This avoids creating locally-generated unsigned app bundles that
	// trigger "Unverified Developer" warnings and TCC privacy prompts.
	if bundlePath := GetAppBundlePath(); bundlePath != "" {
		exePath = bundlePath
		fmt.Printf("Using signed app bundle: %s\n", bundlePath)
	} else {
		fmt.Printf("Using raw binary (no signed app bundle found): %s\n", exePath)
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

	// Load the plist into the current user's GUI domain.
	if err := launchctlRun("bootstrap", userLaunchDomain(), l.plistPath); err != nil {
		// Clean up if load fails
		_ = os.Remove(l.plistPath)
		return fmt.Errorf("failed to bootstrap plist with launchctl: %w", err)
	}

	// Force a fresh start to ensure the daemon uses the correct executable.
	_ = launchctlRun("kickstart", "-k", userLaunchService(launchdLabel))

	return nil
}

// Uninstall unregisters the daemon from launchd.
func (l *LaunchdManager) Uninstall() error {
	if err := l.removeKnownAgents(); err != nil {
		return err
	}

	return nil
}

// IsInstalled checks if the daemon is registered with launchd.
func (l *LaunchdManager) IsInstalled() (bool, error) {
	for _, plistPath := range knownLaunchAgentPaths() {
		_, err := os.Stat(plistPath)
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to check plist file %s: %w", plistPath, err)
		}
	}
	return false, nil
}

// IsRunning checks if the daemon is currently running.
func (l *LaunchdManager) IsRunning() (bool, error) {
	for _, label := range knownLaunchAgentLabels() {
		if err := launchctlRun("print", userLaunchService(label)); err == nil {
			return true, nil
		}
	}
	return false, nil
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
	<string>` + launchdLabel + `</string>
	
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
	
	<key>ProcessType</key>
	<string>Background</string>
	
	<key>AbandonProcessGroup</key>
	<true/>
	
	<key>StandardOutPath</key>
	<string>{{.LogPath}}</string>
	
	<key>StandardErrorPath</key>
	<string>{{.ErrLogPath}}</string>
	
	<key>ThrottleInterval</key>
	<integer>10</integer>
	
	<key>EnableTransactions</key>
	<true/>
	
	<key>ProcessName</key>
	<string>daemon-hound</string>
	
	<key>MachServices</key>
	<dict>
		<key>` + launchdLabel + `</key>
		<true/>
	</dict>
	
	<key>CFBundleDisplayName</key>
	<string>DaemonHound</string>
	
	<key>CFBundleName</key>
	<string>DaemonHound</string>
	
	<key>CFBundleIdentifier</key>
	<string>` + launchdLabel + `</string>
	
	<key>Description</key>
	<string>DaemonHound background sync service</string>
	
	<key>NSHumanReadableCopyright</key>
	<string>DaemonHound Contributors</string>
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

func (l *LaunchdManager) removeKnownAgents() error {
	var errs []string

	for _, label := range knownLaunchAgentLabels() {
		_ = launchctlRun("bootout", userLaunchService(label))
		_ = launchctlRun("bootout", userLaunchDomain(), label)
		_ = launchctlRun("remove", label)
	}

	for _, plistPath := range knownLaunchAgentPaths() {
		_ = launchctlRun("bootout", userLaunchDomain(), plistPath)
		_ = launchctlRun("unload", plistPath)
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", plistPath, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

func knownLaunchAgentLabels() []string {
	return []string{launchdLabel, legacyLaunchdLabel}
}

func knownLaunchAgentPaths() []string {
	home, _ := os.UserHomeDir()
	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	return []string{
		filepath.Join(launchAgentsDir, launchdLabel+".plist"),
		filepath.Join(launchAgentsDir, legacyLaunchdLabel+".plist"),
	}
}

// RemoveAppBundle deletes the generated per-user app bundle used for launchd.
func RemoveAppBundle() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	appDir := filepath.Join(home, "Applications", "DaemonHound.app")
	if err := os.RemoveAll(appDir); err != nil {
		return err
	}

	return nil
}

func userLaunchDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func userLaunchService(label string) string {
	return fmt.Sprintf("%s/%s", userLaunchDomain(), label)
}

func launchctlRun(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return fmt.Errorf("launchctl %s: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("launchctl %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

// ExplainInstallationMethod returns a user-friendly explanation of how
// the daemon was installed (signed app bundle vs raw binary).
func ExplainInstallationMethod() string {
	if GetAppBundlePath() != "" {
		return "Using signed app bundle from GitHub releases (no security warnings)"
	}
	return "Using raw binary (no unsigned app bundle created to avoid security warnings)"
}
