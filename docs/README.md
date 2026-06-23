# daemon-hound Documentation

## Quick Navigation

### 🚀 Getting Started
- **[CROSS_PLATFORM_SETUP.md](CROSS_PLATFORM_SETUP.md)** ← **START HERE**
  - Overview of how everything works together
  - Multi-platform support explained
  - What's ready to use now

### 📦 Installation & Distribution
- **[../INSTALL.md](../INSTALL.md)** ← **USER INSTALL GUIDE**
  - Latest install, update, daemon restart, and uninstall instructions
  - Linux packages, install script, Homebrew, Scoop, and direct downloads

- **[INSTALLATION_GUIDE.md](INSTALLATION_GUIDE.md)**
  - Historical cross-platform distribution details
  - Packaging and artifact overview
  
- **[MACOS_APP_BUNDLE.md](MACOS_APP_BUNDLE.md)**
  - macOS app bundle with proper icon
  - Info.plist configuration
  - Build and install targets

### 🔄 GitHub Actions & Releases
- **[WORKFLOWS_AT_A_GLANCE.md](WORKFLOWS_AT_A_GLANCE.md)**
  - What's already configured ✅
  - Build workflow (CI)
  - Release workflow (CD)
  - Timeline from tag to users

- **[GITHUB_ACTIONS_GUIDE.md](GITHUB_ACTIONS_GUIDE.md)**
  - Deep dive into how GitHub Actions works
  - Goreleaser integration
  - Security and secrets
  - Troubleshooting

- **[GITHUB_ACTIONS_SETUP.md](GITHUB_ACTIONS_SETUP.md)**
  - Setup checklist (what's done, what needs to be done)
  - Release checklist
  - Testing workflows
  - One optional step: Add GH_PAT secret

- **[WORKFLOW_FILES_EXPLAINED.md](WORKFLOW_FILES_EXPLAINED.md)**
  - Technical breakdown of workflow YAML files
  - What each step does
  - How matrix builds work

### 🏗️ Build & Release Process
- **[BUILD_AND_RELEASE.md](BUILD_AND_RELEASE.md)**
  - Build targets reference
  - Release checklist
  - Makefile commands explained

- **[RELEASE_WORKFLOW.md](RELEASE_WORKFLOW.md)**
  - End-to-end release process
  - Step-by-step what happens
  - Complete timeline

### 🎯 Features & Architecture
- **[SMART_MERGE_ARCHITECTURE.md](SMART_MERGE_ARCHITECTURE.md)**
  - Smart merge system (v1.1.0 feature)
  - 5 merge drivers (State, Env, JSON, CSV, Text)
  - Conflict resolution workflow
  - CLI commands reference

- **[FEATURES_V1_1_0.md](FEATURES_V1_1_0.md)**
  - v1.1.0 features overview
  - What changed

- **[E2E_TESTING_GUIDE.md](E2E_TESTING_GUIDE.md)**
  - End-to-end testing scenarios
  - Multi-machine testing
  - Verification steps

---

## Current Status

### ✅ What's Ready

- [x] Smart merge system implemented (5 drivers)
- [x] macOS app bundle with icon
- [x] Multi-platform build system
- [x] GitHub Actions workflows (build + release)
- [x] Goreleaser configuration
- [x] Cross-platform documentation
- [x] Build targets in Makefile

### ⚠️ What Needs Configuration

- [ ] **Optional**: Add GH_PAT secret to GitHub (for Homebrew/Scoop auto-updates)
  - Without it: GitHub Release still works ✅
  - With it: Homebrew/Scoop update automatically ✅

### 📋 Release-Ready Checklist

```bash
✓ Code implemented
✓ Tests passing
✓ Documentation complete
✓ GitHub Actions configured
✓ App bundle created
✓ Ready for v1.1.0 release
```

---

## Quick Start

### Local Development

```bash
# Build regular binary
make build
./bin/dhd --version

# Build macOS app bundle
make build-macos
open build/DaemonHound.app

# Run tests
make test

# Test multi-platform build locally
make snapshot
```

### Release to GitHub

```bash
# Tag a release
make tag TAG=v1.1.0

# GitHub Actions automatically:
# ✓ Builds all platforms
# ✓ Creates packages (.deb, .rpm, .dmg, .zip)
# ✓ Creates GitHub Release
# ✓ Publishes Docker images
# ✓ Updates Homebrew/Scoop (if GH_PAT set)

# Users can then install from the release channels in ../INSTALL.md
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/trunk/install.sh | sh
brew install daemon-hound
scoop install daemon-hound
docker pull ghcr.io/0xdps/daemon-hound:latest
```

---

## Architecture Overview

```
Single Go Codebase (daemon-hound/)
        │
        ├─ Local Development (make build)
        │  └─ bin/dhd (single platform)
        │
        ├─ macOS App Bundle (make build-macos)
        │  └─ DaemonHound.app/ (with icon + metadata)
        │
        └─ Multi-Platform Release (make tag)
           └─ GitHub Actions triggers
              ├─ Builds for all platforms
              ├─ Creates packages
              ├─ Creates GitHub Release
              ├─ Updates package managers
              └─ Publishes Docker images
```

---

## For Users

### Installation Options

See [../INSTALL.md](../INSTALL.md) for the current user-facing install and update instructions.

**macOS**:
```bash
# Easiest
brew install daemon-hound

# Or download DMG
# Click and drag to Applications
```

**Linux**:
```bash
# Debian/Ubuntu (APT Repository - Recommended)
echo 'deb [trusted=yes] https://0xdps.github.io/daemon-hound/apt stable main' | sudo tee /etc/apt/sources.list.d/daemon-hound.list
sudo apt update
sudo apt install daemon-hound

# Or manually install .deb
sudo dpkg -i daemon-hound_1.1.0_amd64.deb

# Or any package manager (apt, rpm, apk, pacman)
```

**Windows**:
```powershell
# Easiest
scoop install daemon-hound

# Or download portable ZIP
```

**Docker**:
```bash
docker pull ghcr.io/0xdps/daemon-hound:latest
```

---

## For Contributors

### Development Workflow

```bash
# 1. Clone and setup
git clone https://github.com/0xdps/daemon-hound
cd daemon-hound
go mod download

# 2. Make changes
vim internal/cmd/root.go

# 3. Test locally
make test
make build
./bin/dhd --version

# 4. Push and create PR
git push origin feature/my-change

# 5. GitHub Actions tests automatically
# (watch at github.com/0xdps/daemon-hound/actions)
```

### Release Process

```bash
# 1. Prepare for release
vim CHANGELOG.md  # Document changes
git add CHANGELOG.md
git commit -m "chore: prepare v1.1.0"
git push origin trunk

# 2. Tag release
make tag TAG=v1.1.0

# 3. Monitor release (5-10 minutes)
# Go to: github.com/0xdps/daemon-hound/actions

# 4. Done! Users can install
```

---

## Documentation Structure

```
docs/
├─ README.md (this file)
│
├─ Getting Started
│  └─ CROSS_PLATFORM_SETUP.md
│
├─ Installation
│  ├─ INSTALLATION_GUIDE.md
│  └─ MACOS_APP_BUNDLE.md
│
├─ GitHub Actions & Release
│  ├─ WORKFLOWS_AT_A_GLANCE.md (START HERE for workflows)
│  ├─ GITHUB_ACTIONS_GUIDE.md
│  ├─ GITHUB_ACTIONS_SETUP.md
│  ├─ WORKFLOW_FILES_EXPLAINED.md
│  ├─ BUILD_AND_RELEASE.md
│  └─ RELEASE_WORKFLOW.md
│
└─ Features & Architecture
   ├─ SMART_MERGE_ARCHITECTURE.md
   ├─ FEATURES_V1_1_0.md
   └─ E2E_TESTING_GUIDE.md
```

---

## Key Files in Repository

### Source Code
- `cmd/dhd/main.go` - Entry point
- `internal/cmd/` - CLI commands
- `internal/merge/` - Smart merge drivers
- `internal/conflicts/` - Conflict management

### Configuration
- `.goreleaser.yaml` - Release & distribution config
- `Makefile` - Build targets
- `.github/workflows/build.yml` - CI workflow
- `.github/workflows/release.yml` - CD workflow
- `go.mod` - Go dependencies

### Build Artifacts
- `build/Info.plist` - macOS app metadata
- `build/AppIcon.icns` - App icon
- `build/DaemonHound.app/` - macOS app bundle
- `bin/dhd` - Regular binary
- `dist/` - Release artifacts (generated)

---

## Next Steps

### If You Want to Release v1.1.0 Now

1. ✅ All code is ready
2. ✅ Tests pass
3. ✅ Documentation complete
4. Just run: `make tag TAG=v1.1.0`
5. Wait 10 minutes for GitHub Actions

### If You Want to Configure Homebrew/Scoop Auto-Updates

1. Generate Personal Access Token on GitHub
2. Add as secret `GH_PAT` in repository settings
3. Next release will auto-update Homebrew/Scoop

### If You Want to Test Locally First

```bash
make snapshot       # Build everything locally
ls -lh dist/       # See all platforms
```

---

## Support & Questions

### GitHub Actions Not Running?

See: [GITHUB_ACTIONS_SETUP.md](GITHUB_ACTIONS_SETUP.md)

### App Bundle Icon Not Showing?

See: [MACOS_APP_BUNDLE.md](MACOS_APP_BUNDLE.md#troubleshooting)

### Release Workflow Failed?

See: [GITHUB_ACTIONS_GUIDE.md](GITHUB_ACTIONS_GUIDE.md#troubleshooting-release-workflow)

### Installation Questions?

See: [INSTALLATION_GUIDE.md](INSTALLATION_GUIDE.md)

---

## Summary

**daemon-hound v1.1.0** is fully set up for:
- ✅ Multi-platform development (macOS, Linux, Windows)
- ✅ Automated testing via GitHub Actions
- ✅ Automated releases via Goreleaser
- ✅ Professional app branding (icon + name on macOS)
- ✅ User-friendly installation (Homebrew, package managers, Docker)

**You're ready to release!**

```bash
make tag TAG=v1.1.0
```

Then wait 10 minutes, and your app is available to users everywhere.
