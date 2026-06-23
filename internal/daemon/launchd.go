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
	plistPath := filepath.Join(home, "Library/LaunchAgents/com.0xdps.daemon-hound.plist")
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

	// Ensure the app bundle exists so Activity Monitor shows "Daemon Hound"
	// with the proper icon instead of just the raw binary name.
	bundlePath, err := ensureAppBundle(exePath)
	if err != nil {
		// Non-fatal: fall back to the raw binary path.
		fmt.Fprintf(os.Stderr, "Warning: could not create app bundle: %v\n", err)
	} else if bundlePath != "" {
		exePath = bundlePath
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
	cmd := exec.Command("launchctl", "list", "com.0xdps.daemon-hound")
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
	<string>com.0xdps.daemon-hound</string>
	
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

// ensureAppBundle creates a minimal macOS app bundle at
// ~/Applications/DaemonHound.app if it does not already exist.
// It symlinks the real dhd binary into the bundle and writes an Info.plist
// so that Activity Monitor shows "Daemon Hound" with the proper icon.
// Returns the path to the bundle's executable (MacOS/dhd).
func ensureAppBundle(exePath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	appDir := filepath.Join(home, "Applications", "DaemonHound.app")
	contentsDir := filepath.Join(appDir, "Contents")
	macOSDir := filepath.Join(contentsDir, "MacOS")
	resourcesDir := filepath.Join(contentsDir, "Resources")
	bundleExe := filepath.Join(macOSDir, "dhd")
	plistPath := filepath.Join(contentsDir, "Info.plist")

	// Already exists and looks valid — nothing to do.
	if info, err := os.Stat(bundleExe); err == nil && !info.IsDir() {
		if _, err := os.Stat(plistPath); err == nil {
			return bundleExe, nil
		}
	}

	// Create bundle directories.
	if err := os.MkdirAll(macOSDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create MacOS dir: %w", err)
	}
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create Resources dir: %w", err)
	}

	// Remove any stale symlink / file at the bundle executable path.
	_ = os.Remove(bundleExe)

	// Symlink the real binary into the bundle.
	if err := os.Symlink(exePath, bundleExe); err != nil {
		return "", fmt.Errorf("failed to symlink binary into bundle: %w", err)
	}

	// Write Info.plist.
	infoPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>dhd</string>
    <key>CFBundleIdentifier</key>
    <string>com.0xdps.daemon-hound</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>Daemon Hound</string>
    <key>CFBundleDisplayName</key>
    <string>Daemon Hound</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.1.0</string>
    <key>CFBundleVersion</key>
    <string>1.1.0</string>
    <key>LSBackgroundOnly</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSRequiresAquaSystemAppearance</key>
    <false/>
</dict>
</plist>
`
	if err := os.WriteFile(plistPath, []byte(infoPlist), 0644); err != nil {
		return "", fmt.Errorf("failed to write Info.plist: %w", err)
	}

	return bundleExe, nil
}
