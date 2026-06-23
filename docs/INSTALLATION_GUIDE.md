# Cross-Platform Installation Guide

> For current user-facing install, update, daemon restart, and uninstall instructions, see [../INSTALL.md](../INSTALL.md). This document keeps the broader packaging and distribution overview.

## Overview

daemon-hound supports **macOS, Linux, and Windows** through multiple installation methods. Here's how it all works together:

```
Development                Distribution                        Installation
┌─────────────┐    ┌──────────────────┐    ┌────────────────────────────────┐
│             │    │                  │    │                                │
│ make build  │───▶│ Local Testing    │    │ For Dev/Testing Locally        │
│             │    │ (Single Binary)  │    │                                │
└─────────────┘    └──────────────────┘    └────────────────────────────────┘
                                                        │
                                                        │
            ┌───────────────────────────────────────────┼─────────────────────────┐
            │                                           │                         │
            ▼                                           ▼                         ▼
    ┌────────────────┐                        ┌────────────────┐      ┌──────────────────┐
    │ goreleaser     │                        │ make snapshot  │      │ make release     │
    │ (CI/CD Pipeline)│                       │ (Test Build)   │      │ (GitHub Release) │
    └────────────────┘                        └────────────────┘      └──────────────────┘
            │                                           │                         │
    ┌───────┴───────────────────────────────────────────┼─────────────────────────┴──────────┐
    │                                                   │                                     │
    ▼                                                   ▼                                     ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│                         RELEASE ARTIFACTS (Published to GitHub)                                 │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  macOS (darwin):                                                                               │
│  ├─ daemon-hound_1.1.0_macOS_x86_64.dmg      ← App bundle + icon ready-to-install           │
│  ├─ daemon-hound_1.1.0_macOS_arm64.dmg       ← M1/M2 version                               │
│  └─ dhd (binary in archives/)                                                                │
│                                                                                                  │
│  Linux (linux):                                                                                │
│  ├─ daemon-hound_1.1.0_linux_x86_64.tar.gz   ← Raw binary + docs                           │
│  ├─ daemon-hound_1.1.0_linux_arm64.tar.gz    ← ARM64 (Raspberry Pi, etc)                  │
│  ├─ daemon-hound-1.1.0-1.x86_64.rpm          ← RedHat/CentOS/Fedora                       │
│  ├─ daemon-hound-1.1.0-1.aarch64.rpm         ← ARM64 RPM                                  │
│  ├─ daemon-hound_1.1.0_amd64.deb             ← Debian/Ubuntu x86_64                       │
│  ├─ daemon-hound_1.1.0_arm64.deb             ← Debian/Ubuntu ARM64                        │
│  ├─ daemon-hound-1.1.0-1.apk                 ← Alpine Linux                               │
│  └─ daemon-hound-1.1.0-1-x86_64.pkg.tar.zst  ← Arch Linux                                │
│                                                                                                  │
│  Windows (windows):                                                                            │
│  ├─ daemon-hound_1.1.0_Windows_x86_64.zip    ← Portable .exe + docs                       │
│  ├─ daemon-hound.exe                         ← Raw executable                              │
│  └─ Via Scoop/Chocolatey registries                                                        │
│                                                                                                  │
│  Docker:                                                                                       │
│  ├─ ghcr.io/0xdps/daemon-hound:1.1.0                                                      │
│  ├─ ghcr.io/0xdps/daemon-hound:v1                                                         │
│  └─ ghcr.io/0xdps/daemon-hound:latest                                                     │
│                                                                                                  │
│  Checksums:                                                                                    │
│  └─ checksums.txt (SHA256 for all artifacts)                                                 │
│                                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Installation Methods by Platform

### macOS

#### **Method 1: Homebrew (Recommended)**
```bash
brew tap 0xdps/packages
brew install daemon-hound
# Starts with: dhd daemon
```
✅ Auto-updates via Homebrew  
✅ App shows properly in Activity Monitor  
✅ Binary in PATH

#### **Method 2: DMG Installer**
```bash
# Download daemon-hound_1.1.0_macOS_x86_64.dmg from releases
# Double-click → drag app to Applications folder
# Then: /Applications/DaemonHound.app/Contents/MacOS/dhd daemon
```
✅ Graphical install  
✅ App icon visible in Applications  
✅ Desktop integration

#### **Method 3: Direct Binary Download**
```bash
# Download daemon-hound_1.1.0_macOS_x86_64.tar.gz
tar xzf daemon-hound_1.1.0_macOS_x86_64.tar.gz
./dhd daemon
```
⚠️ No auto-updates  
✅ Simple, no dependencies

#### **Method 4: Build Locally**
```bash
git clone https://github.com/0xdps/daemon-hound
cd daemon-hound
make install-macos
~/Applications/DaemonHound.app/Contents/MacOS/dhd daemon
```

---

### Linux

#### **Method 1: APT Repository (Debian/Ubuntu - Recommended)**

```bash
# Add the DaemonHound APT repository
echo 'deb [trusted=yes] https://0xdps.github.io/daemon-hound/apt stable main' | sudo tee /etc/apt/sources.list.d/daemon-hound.list

# Update and install
sudo apt update
sudo apt install daemon-hound
```

✅ Auto-updates via `apt upgrade`
✅ No manual download needed

#### **Method 2: Manual Package Install**

**Debian/Ubuntu**:
```bash
# From release .deb file
sudo dpkg -i daemon-hound_1.1.0_amd64.deb
dhd daemon
```

**RedHat/CentOS/Fedora**:
```bash
# From release .rpm file
sudo rpm -i daemon-hound-1.1.0-1.x86_64.rpm
dhd daemon
```

**Alpine**:
```bash
# From release .apk file
sudo apk add --allow-untrusted daemon-hound-1.1.0-1.apk
dhd daemon
```

**Arch Linux**:
```bash
# From release .pkg.tar.zst file
sudo pacman -U daemon-hound-1.1.0-1-x86_64.pkg.tar.zst
dhd daemon
```

✅ Auto-updates via package manager  
✅ Managed dependencies  
✅ Binary in PATH automatically

#### **Method 2: Direct Binary Download**
```bash
# Download daemon-hound_1.1.0_linux_x86_64.tar.gz
tar xzf daemon-hound_1.1.0_linux_x86_64.tar.gz
sudo mv dhd /usr/local/bin/
dhd daemon
```

#### **Method 3: Build Locally**
```bash
git clone https://github.com/0xdps/daemon-hound
cd daemon-hound
make build
./bin/dhd daemon
```

---

### Windows

#### **Method 1: Scoop (Recommended)**
```powershell
scoop bucket add 0xdps https://github.com/0xdps/scoop-bucket
scoop install daemon-hound
dhd daemon
```
✅ Auto-updates  
✅ PATH management  
✅ Easy uninstall

#### **Method 2: Portable ZIP**
```powershell
# Download daemon-hound_1.1.0_Windows_x86_64.zip
Expand-Archive daemon-hound_1.1.0_Windows_x86_64.zip -DestinationPath C:\daemon-hound
C:\daemon-hound\dhd.exe daemon
```

#### **Method 3: Build Locally**
```bash
# From Windows terminal (or WSL)
git clone https://github.com/0xdps/daemon-hound
cd daemon-hound
make build
.\bin\dhd.exe daemon
```

---

## How It Works Internally

### Development Workflow (Local)

```bash
# 1. Regular build (all platforms work the same)
make build
# Creates: bin/dhd (or bin/dhd.exe on Windows)

# 2. macOS app bundle (macOS only)
make build-macos
# Creates: build/DaemonHound.app/Contents/...

# 3. Install app to ~/Applications (macOS only)
make install-macos
```

### Release Workflow (CI/CD - goreleaser)

**File: `.goreleaser.yaml`**

```yaml
builds:
  # Build for all platforms
  goos: [darwin, linux, windows, freebsd]
  goarch: [amd64, arm64]

archives:
  # Package binaries into archives
  format: tar.gz (Linux/macOS) or zip (Windows)
  include: [README.md, LICENSE, CHANGELOG.md]

brews:
  # Generate Homebrew formula
  repository: 0xdps/homebrew-packages

scoops:
  # Generate Scoop manifest
  repository: 0xdps/scoop-bucket

nfpms:
  # Generate Linux packages
  formats: [deb, rpm, apk, archlinux]
  dependencies: [git]

dockers:
  # Build container images
  images: [ghcr.io/0xdps/daemon-hound:VERSION]
```

**When you run** `make release`:
1. ✅ Builds binaries for all platforms
2. ✅ Creates archives (.tar.gz, .zip)
3. ✅ Generates Linux packages (.deb, .rpm, .apk)
4. ✅ Updates Homebrew formula
5. ✅ Updates Scoop manifest
6. ✅ Builds Docker images
7. ✅ Creates GitHub release
8. ✅ Generates checksums

---

## Choosing Installation Method

| User Type | Platform | Recommended Method | Why |
|-----------|----------|-------------------|-----|
| **Developer** | Any | Build locally: `make build` | Latest code, full control |
| **macOS User** | macOS | Homebrew: `brew install` | Auto-updates, simple |
| **macOS User** | macOS | DMG installer | Graphical, no Terminal needed |
| **Linux User** | Linux | Package manager | Auto-updates, managed |
| **Linux User** | Linux | .tar.gz + manual PATH | Simple, no sudo needed |
| **Windows User** | Windows | Scoop: `scoop install` | Auto-updates, PowerShell native |
| **Windows User** | Windows | Portable ZIP | No installation needed |
| **Docker User** | Any | `docker pull ghcr.io/0xdps/daemon-hound` | Containerized, consistent |

---

## Architecture: Single Codebase, Multiple Outputs

```
daemon-hound (Go source)
        │
        └─────────────────────────────────────┐
        │                                     │
   (LOCAL)                                (CI/CD)
        │                                     │
   ┌────┴────┐                         ┌─────┴──────┐
   │          │                        │             │
make build   make build-macos    make release  make snapshot
   │          │                        │             │
   ▼          ▼                        ▼             ▼
bin/dhd   DaemonHound.app         All Platforms  Test Build
   │          │                        │             │
   └──────────┬────────────────────────┤─────────────┘
              │                        │
        User can choose        Multiple outputs
        which to use           for distribution
```

---

## Multi-Platform Development Example

If you're contributing to daemon-hound, you can test on all platforms:

```bash
# Test on macOS
make build-macos && make install-macos
~/Applications/DaemonHound.app/Contents/MacOS/dhd --version

# Test on Linux
make build && ./bin/dhd --version

# Test on Windows (from WSL or Windows terminal)
make build && .\bin\dhd.exe --version

# Test Docker
make snapshot  # Creates docker images
docker run ghcr.io/0xdps/daemon-hound:dev dhd --version
```

---

## Troubleshooting Multi-Platform Builds

### Issue: "make build" doesn't work for other platforms

**Why**: `make build` is platform-aware (uses native Go toolchain)

**Solution**: Use goreleaser to build all platforms:
```bash
make snapshot  # Test release build for all platforms
make release   # Full release (requires git tag)
```

### Issue: DMG installer not showing app properly

**Solution**: Ensure macOS app bundle was created:
```bash
make build-macos
# Verify: build/DaemonHound.app/Contents/Info.plist exists
```

### Issue: Linux package has wrong binary path

**Solution**: goreleaser controls this via `.goreleaser.yaml`:
```yaml
nfpms:
  bindir: /usr/bin  # Where binary gets installed
```

---

## Next Steps

1. **Local Development**: Use `make build` or `make build-macos`
2. **Testing**: Use `make snapshot` to build all platforms
3. **Release**: Create a git tag, then `make release`
4. **Distribution**: Users choose their preferred method
5. **Updates**: Handled by Homebrew/Scoop/Package Manager

Each platform gets the same core binary, but packaged appropriately for that ecosystem.
