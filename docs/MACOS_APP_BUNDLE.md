# macOS App Bundle Setup

## Overview

Daemon Hound is now properly configured to display as a native macOS application with:
- ✅ Custom app icon (AppIcon.icns)
- ✅ Proper app metadata (Info.plist)
- ✅ Display name: "Daemon Hound"
- ✅ Bundle identifier: com.0xdps.daemon-hound
- ✅ Configured to run in background

## Build Options

### Build Regular Binary
```bash
make build
# Creates: bin/dh
```

### Build macOS App Bundle
```bash
make build-macos
# Creates: build/DaemonHound.app
```

### Install to ~/Applications
```bash
make install-macos
# Installs to: ~/Applications/DaemonHound.app
```

### Launch Daemon
```bash
make launch-daemon
# Installs and launches the daemon
```

## File Structure

```
DaemonHound.app/
├── Contents/
│   ├── Info.plist              # App metadata (name, icon, etc.)
│   ├── MacOS/
│   │   └── dh                  # Executable binary
│   └── Resources/
│       └── AppIcon.icns        # App icon (1024x1024 + retina)
```

## Info.plist Configuration

The `build/Info.plist` includes:

| Key | Value | Purpose |
|-----|-------|---------|
| CFBundleName | Daemon Hound | Display name in Activity Monitor |
| CFBundleDisplayName | Daemon Hound | Display name in menu bar |
| CFBundleIdentifier | com.0xdps.daemon-hound | Unique app identifier |
| CFBundleExecutable | dh | Binary name within MacOS/ |
| CFBundleIconFile | AppIcon | Icon file name (without .icns) |
| LSBackgroundOnly | true | Run without dock icon |
| NSHighResolutionCapable | true | Support retina displays |

## App Icon

The icon is automatically generated from `images/logo.png` and includes:
- 16×16 (standard)
- 32×32 (standard)
- 64×64 (for compatibility)
- 128×128 (standard)
- 256×256 (standard)
- 512×512 (standard)
- 1024×1024 (retina)

All sizes are created and bundled into `AppIcon.icns`.

## Background Daemon Display

When running `dh daemon` from the bundled app:

**Before (v1.0.0)**:
```
exec
Running in background
```

**After (v1.1.0+)**:
```
Daemon Hound
Running in background
```

## Usage

### Start daemon from app bundle:
```bash
~/Applications/DaemonHound.app/Contents/MacOS/dh daemon
```

### Or create an alias:
```bash
alias dh-daemon="~/Applications/DaemonHound.app/Contents/MacOS/dh daemon"
dh-daemon
```

### Keep the binary in PATH:
```bash
# Regular install still works
make install  # or make build -> bin/dh
dh daemon
```

## Rebuilding After Changes

If you update the logo:
```bash
# Regenerate icon
make build-macos-icon

# Rebuild app bundle
make build-macos

# Reinstall
make install-macos
```

## Clean Up

```bash
# Remove all build artifacts
make clean

# Or specific:
rm -rf build/DaemonHound.app
rm -f build/AppIcon.icns
```

## Troubleshooting

### Icon not showing
- Rebuild with `make build-macos-icon`
- Check that `build/AppIcon.icns` exists
- Restart daemon

### App name still shows as "exec"
- Ensure Info.plist was copied: `cat build/DaemonHound.app/Contents/Info.plist`
- Rebuild app bundle: `make build-macos`
- Restart daemon

### Binary not executable
- Run: `chmod +x ~/Applications/DaemonHound.app/Contents/MacOS/dh`
- Or use: `make install-macos`
