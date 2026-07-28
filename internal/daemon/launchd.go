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
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Load the plist into the current user's GUI domain.
	if err := launchctlRun("bootstrap", userLaunchDomain(), l.plistPath); err != nil {
		// Clean up if load fails
		_ = os.Remove(l.plistPath)
		return fmt.Errorf("failed to bootstrap plist with launchctl: %w", err)
	}

	// Force a fresh start so the daemon uses the just-written app bundle.
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
	
	<key>AssociatedBundleIdentifiers</key>
	<array>
		<string>` + appBundleID + `</string>
	</array>
	
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

// ensureAppBundle creates a minimal macOS app bundle at
// ~/Applications/DaemonHound.app if it does not already exist, or refreshes
// the binary inside it if the source has changed.
// It COPIES (not symlinks) the real dhd binary into the bundle so that
// macOS TCC associates permissions with the bundle identifier
// ("Daemon Hound" in Activity Monitor) instead of the raw binary.
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
	iconPath := filepath.Join(resourcesDir, "AppIcon.icns")

	// Compare source and destination to decide if a refresh is needed.
	srcInfo, srcErr := os.Stat(exePath)
	if srcErr != nil {
		return "", fmt.Errorf("source binary not accessible: %w", srcErr)
	}
	dstInfo, dstErr := os.Stat(bundleExe)

	needsRefresh := true
	if dstErr == nil && !dstInfo.IsDir() {
		if dstInfo.Size() == srcInfo.Size() && dstInfo.ModTime().Equal(srcInfo.ModTime()) {
			if _, err := os.Stat(plistPath); err == nil {
				if _, err := os.Stat(iconPath); err == nil {
					needsRefresh = false
				}
			}
		}
	}

	if !needsRefresh {
		return bundleExe, nil
	}

	// Create bundle directories.
	if err := os.MkdirAll(macOSDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create MacOS dir: %w", err)
	}
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create Resources dir: %w", err)
	}

	// Remove previous binary (symlink or copy) and write a fresh copy.
	// A copy (not a symlink) is required so macOS TCC treats the process
	// as running from within the app bundle.
	_ = os.Remove(bundleExe)

	srcFile, err := os.Open(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to open source binary: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(bundleExe, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create bundle binary: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = os.Remove(bundleExe)
		return "", fmt.Errorf("failed to copy binary into bundle: %w", err)
	}

	// Preserve the same modification time so future comparisons work.
	_ = os.Chtimes(bundleExe, srcInfo.ModTime(), srcInfo.ModTime())

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
	<string>` + appBundleID + `</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
	<string>` + appBundleName + `</string>
    <key>CFBundleDisplayName</key>
	<string>` + appBundleName + `</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
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

	if err := ensureBundleIcon(resourcesDir); err != nil {
		return "", fmt.Errorf("failed to create app icon: %w", err)
	}

	if err := signAppBundleIfConfigured(appDir); err != nil {
		return "", err
	}

	// Register the app bundle with Launch Services so macOS System Settings
	// and Activity Monitor show "Daemon Hound" with the proper name. Without
	// this, the launchd agent appears as a generic process with no metadata.
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	if _, statErr := os.Stat(lsregister); statErr == nil {
		// -f forces re-registration even if already registered.
		_ = exec.Command(lsregister, "-f", appDir).Run()
	}

	return bundleExe, nil
}

func ensureBundleIcon(resourcesDir string) error {
	iconPath := filepath.Join(resourcesDir, "AppIcon.icns")
	if err := copyBundleIcon(iconPath); err == nil {
		return nil
	}

	logoPath, err := findLogoPNG()
	if err != nil {
		return writeFallbackICNS(iconPath)
	}

	return generateICNSFromPNG(logoPath, iconPath)
}

func copyBundleIcon(iconPath string) error {
	for _, candidate := range iconCandidates() {
		if candidate == "" {
			continue
		}
		if err := copyFile(candidate, iconPath, 0644); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no existing AppIcon.icns asset found")
}

func iconCandidates() []string {
	var candidates []string
	if explicit := strings.TrimSpace(os.Getenv("DHD_APP_ICON_PATH")); explicit != "" {
		candidates = append(candidates, explicit)
	}

	if exe, err := os.Executable(); err == nil {
		exe = resolveExecutablePath(exe)
		binDir := filepath.Dir(exe)
		prefixDir := filepath.Dir(binDir)
		candidates = append(candidates,
			filepath.Join(prefixDir, "share", "daemon-hound", "AppIcon.icns"),
			filepath.Join(prefixDir, "share", "daemon-hound", "resources", "AppIcon.icns"),
			filepath.Join(prefixDir, "share", "daemon-hound", "build", "AppIcon.icns"),
			filepath.Join(binDir, "AppIcon.icns"),
		)
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		candidates = append(candidates,
			filepath.Join(repoRoot, "build", "AppIcon.icns"),
		)
	}

	return candidates
}

func findLogoPNG() (string, error) {
	var candidates []string
	if explicit := strings.TrimSpace(os.Getenv("DHD_APP_LOGO_PATH")); explicit != "" {
		candidates = append(candidates, explicit)
	}

	if exe, err := os.Executable(); err == nil {
		exe = resolveExecutablePath(exe)
		binDir := filepath.Dir(exe)
		prefixDir := filepath.Dir(binDir)
		candidates = append(candidates,
			filepath.Join(prefixDir, "share", "daemon-hound", "logo-trans.png"),
			filepath.Join(prefixDir, "share", "daemon-hound", "images", "logo-trans.png"),
			filepath.Join(binDir, "logo-trans.png"),
		)
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		candidates = append(candidates,
			filepath.Join(repoRoot, "images", "logo-trans.png"),
		)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no logo PNG asset found")
}

func resolveExecutablePath(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func generateICNSFromPNG(srcPNG, dstICNS string) error {
	iconutilPath, err := exec.LookPath("iconutil")
	if err != nil {
		return fmt.Errorf("iconutil unavailable: %w", err)
	}
	sipsPath, err := exec.LookPath("sips")
	if err != nil {
		return fmt.Errorf("sips unavailable: %w", err)
	}

	iconsetDir, err := os.MkdirTemp("", "dhd-iconset-*.iconset")
	if err != nil {
		return fmt.Errorf("failed to create temp iconset dir: %w", err)
	}
	defer os.RemoveAll(iconsetDir)

	sizes := []struct {
		name string
		size int
	}{
		{name: "icon_16x16.png", size: 16},
		{name: "icon_16x16@2x.png", size: 32},
		{name: "icon_32x32.png", size: 32},
		{name: "icon_32x32@2x.png", size: 64},
		{name: "icon_128x128.png", size: 128},
		{name: "icon_128x128@2x.png", size: 256},
		{name: "icon_256x256.png", size: 256},
		{name: "icon_256x256@2x.png", size: 512},
		{name: "icon_512x512.png", size: 512},
		{name: "icon_512x512@2x.png", size: 1024},
	}

	for _, item := range sizes {
		target := filepath.Join(iconsetDir, item.name)
		cmd := exec.Command(sipsPath, "-z", fmt.Sprintf("%d", item.size), fmt.Sprintf("%d", item.size), srcPNG, "--out", target)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to generate %s: %s", item.name, strings.TrimSpace(string(output)))
		}
	}

	cmd := exec.Command(iconutilPath, "-c", "icns", iconsetDir, "-o", dstICNS)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build AppIcon.icns: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

func writeFallbackICNS(dstICNS string) error {
	tempPNG, err := os.CreateTemp("", "dhd-fallback-*.png")
	if err != nil {
		return fmt.Errorf("failed to create fallback icon temp file: %w", err)
	}
	tempPath := tempPNG.Name()
	defer os.Remove(tempPath)

	img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	bg := color.RGBA{R: 24, G: 85, B: 180, A: 255}
	inner := color.RGBA{R: 255, G: 184, B: 76, A: 255}
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			img.Set(x, y, bg)
		}
	}
	for y := 180; y < 844; y++ {
		for x := 180; x < 844; x++ {
			if x < 300 || x > 724 || y < 300 || y > 724 {
				img.Set(x, y, inner)
			}
		}
	}
	if err := png.Encode(tempPNG, img); err != nil {
		tempPNG.Close()
		return fmt.Errorf("failed to encode fallback icon: %w", err)
	}
	if err := tempPNG.Close(); err != nil {
		return fmt.Errorf("failed to finalize fallback icon: %w", err)
	}

	return generateICNSFromPNG(tempPath, dstICNS)
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = os.Remove(dstPath)
		return err
	}

	return nil
}

func signAppBundleIfConfigured(appDir string) error {
	identity := strings.TrimSpace(os.Getenv("APPLE_DEVELOPER_IDENTITY"))
	if identity == "" {
		return nil
	}

	cmd := exec.Command("codesign", "--force", "--deep", "--options", "runtime", "--sign", identity, appDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to sign app bundle: %s", strings.TrimSpace(string(output)))
	}

	return nil
}
