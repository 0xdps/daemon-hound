# GitHub Actions Workflows at a Glance

## Your Workflows Are Already Set Up

### 1️⃣ Build Workflow (Continuous Integration)

**File**: `.github/workflows/build.yml`

**When it runs**:
- Every push to `trunk` or `main`
- Every pull request

**What it does**:
```
Checkout Code
    ↓
Set up Go v1.26.4
    ↓
TEST JOB:
├─ go test -v -race ./...        (tests with race detection)
├─ go vet ./...                  (static analysis)
└─ gofmt check                   (format validation)
    ↓
BUILD JOBS (6 parallel):
├─ darwin/amd64    → dhd-darwin-amd64
├─ darwin/arm64    → dhd-darwin-arm64
├─ linux/amd64     → dhd-linux-amd64
├─ linux/arm64     → dhd-linux-arm64
├─ windows/amd64   → dhd-windows-amd64.exe
└─ freebsd/amd64   → dhd-freebsd-amd64
    ↓
DOCKER BUILD JOB:
└─ Build Docker image
    ↓
Upload Artifacts (keep for 7 days)
    ↓
✅ COMPLETE
```

**Time**: ~5-7 minutes

**Status**: ✅ **Already working** (no changes needed)

**Example Result**:
```
Pull Request or Push → GitHub Actions starts
                     ↓
                  build.yml
                     ↓
        5 min: ✓ All tests pass
               ✓ All binaries compile
               ✓ Docker builds
                     ↓
        PR can be merged OR
        Push appears on trunk
```

---

### 2️⃣ Release Workflow (Continuous Deployment)

**File**: `.github/workflows/release.yml`

**When it runs**: 
Only when you push a git tag matching `v*`

```bash
git push origin v1.1.0    # Triggers release.yml
```

**What it does**:
```
GitHub detects tag v1.1.0
        ↓
Release Workflow Starts
        ↓
Setup Phase:
├─ Checkout (full history)
├─ Set up Go v1.26.4
├─ Set up Docker multi-arch support (QEMU)
├─ Set up Docker Buildx
├─ Login to ghcr.io (GitHub Container Registry)
    ↓
Run Goreleaser:
    ├─ BUILD (30 seconds)
    │  ├─ darwin/amd64 binary with version info
    │  ├─ darwin/arm64 binary with version info
    │  ├─ linux/amd64 binary with version info
    │  ├─ linux/arm64 binary with version info
    │  ├─ windows/amd64 binary with version info
    │  └─ freebsd/amd64 binary with version info
    │
    ├─ PACKAGE (3 minutes)
    │  ├─ macOS: Create .dmg with app bundle
    │  ├─ Linux: Create .deb, .rpm, .apk, .tar.gz
    │  └─ Windows: Create .zip
    │
    ├─ DISTRIBUTE (5 minutes)
    │  ├─ Create GitHub Release
    │  ├─ Upload all artifacts
    │  ├─ Generate SHA256 checksums
    │  ├─ Push to 0xdps/homebrew-packages (if GH_PAT set)
    │  ├─ Push to 0xdps/scoop-bucket (if GH_PAT set)
    │  └─ Build + push Docker images to ghcr.io
        ↓
✅ COMPLETE (8-10 minutes total)
        ↓
Results available:
├─ GitHub Release: github.com/0xdps/daemon-hound/releases/tag/v1.1.0
├─ Homebrew: brew install daemon-hound
├─ Scoop: scoop install daemon-hound
├─ Docker: docker pull ghcr.io/0xdps/daemon-hound:v1.1.0
└─ Package managers: apt install, rpm -i, etc.
```

**Time**: ~8-10 minutes

**Status**: ✅ **Already configured** (needs optional GH_PAT for Homebrew/Scoop)

**Example Result**:
```
You: make tag TAG=v1.1.0
     ↓
     (pushes v1.1.0 tag)
     ↓
GitHub: Detects tag
         ↓
     release.yml triggers
         ↓
     8 min later...
         ↓
macOS user: brew install daemon-hound ✓
Linux user: sudo apt install daemon-hound ✓
Windows user: scoop install daemon-hound ✓
Docker user: docker pull ghcr.io/... ✓
GitHub user: Downloads from Release page ✓
```

---

## What's Different from Regular Binary

### Before (Just Binary)

```
dhd (executable)
└─ No metadata
   └─ macOS shows: "exec" icon + generic name
   └─ No version info embedded
```

### After (With App Bundle)

```
DaemonHound.app/
├─ Contents/Info.plist
│  ├─ CFBundleName: "Daemon Hound"
│  └─ CFBundleVersion: "1.1.0"
├─ Contents/MacOS/dhd
│  └─ Binary with embedded version
└─ Contents/Resources/AppIcon.icns
   └─ Your logo as app icon
```

macOS reads Info.plist → Shows "Daemon Hound" with your logo ✓

---

## The Build Happens Here

### GitHub Actions runs on their servers:

```
Ubuntu Runner (docker-based):
├─ CPUs: 4 cores
├─ RAM: 16 GB
├─ Disk: 30 GB
├─ OS: Ubuntu 22.04 LTS
├─ Pre-installed: Docker, Git, Go, etc.
│
└─ Runs your workflow in isolated container
   ├─ Tests your code
   ├─ Compiles for all platforms
   ├─ Builds Docker images
   └─ Uploads artifacts
```

**Why Ubuntu**:
- Go's cross-compilation is perfect on Linux
- Can build binaries for macOS, Linux, Windows, FreeBSD from one machine
- Docker support for multi-arch images
- Fastest and most reliable

---

## The Goreleaser Magic

**What's Goreleaser?**: Tool that automates build → package → distribute

**What it reads**: `.goreleaser.yaml`

**What it does**:

```yaml
version: 2
project_name: daemon-hound

builds:
  - goos: [darwin, linux, windows, freebsd]  # Platforms
    goarch: [amd64, arm64]                   # Architectures
    ldflags: [version, commit, date]         # Embed version info

archives:
  - formats: [tar.gz, zip]                   # Package format
    files: [README, LICENSE, CHANGELOG]      # What to include

brews:
  - repository: 0xdps/homebrew-packages      # Homebrew repo

scoops:
  - repository: 0xdps/scoop-bucket           # Scoop repo

nfpms:
  - formats: [deb, rpm, apk, archlinux]      # Linux packages

dockers:
  - image_templates:                         # Docker tags
      - ghcr.io/0xdps/daemon-hound:{{ .Tag }}
      - ghcr.io/0xdps/daemon-hound:latest
```

**Result**: One command → All platforms packaged + distributed

---

## Security & Automation

### How GitHub Actions is Secure

✅ **Isolated containers** - Each run gets a fresh Ubuntu container
✅ **Secrets encrypted** - GH_PAT never shown in logs
✅ **No network by default** - Whitelist needed for external access
✅ **Temporary credentials** - GITHUB_TOKEN expires after workflow
✅ **Audit trail** - All actions logged and traceable

### What Secrets Are Used

1. **GITHUB_TOKEN** (automatic, built-in)
   - Created per workflow run
   - Expires after workflow completes
   - Permissions: limited to repository

2. **GH_PAT** (optional, you create)
   - Personal Access Token you generate
   - Used for: Pushing to homebrew-packages and scoop-bucket repos
   - ⚠️ Never shown in logs

---

## Current Status Summary

### ✅ What's Ready Now

| Component | Status | Notes |
|-----------|--------|-------|
| build.yml | ✅ Ready | Runs every push/PR |
| release.yml | ✅ Ready | Runs on git tags |
| .goreleaser.yaml | ✅ Ready | Full config |
| Go Setup | ✅ Ready | v1.26.4 from go.mod |
| Docker Setup | ✅ Ready | Multi-arch builds |
| GitHub Token | ✅ Auto | Built-in |
| GH_PAT Token | ⚠️ Optional | For Homebrew/Scoop |

### 🚀 Ready to Use Now

Just run: `make tag TAG=v1.1.0`

Everything else is automatic!

---

## Timeline: v1.1.0 Release

```
14:00 - You: git push origin trunk
14:00 - GitHub: Triggers build.yml
14:05 - GitHub: ✓ All tests pass, ✓ All binaries build
        You can merge PRs now

14:30 - You: make tag TAG=v1.1.0
14:30 - GitHub: Detects tag
14:30 - GitHub: Triggers release.yml
14:31 - Goreleaser: Starts building
14:32 - Goreleaser: Builds all binaries (30 sec)
14:33 - Goreleaser: Creates packages (3 min)
14:36 - Goreleaser: Publishes to GitHub (1 min)
14:37 - Goreleaser: Updates Homebrew (1 min)
14:38 - Goreleaser: Updates Scoop (1 min)
14:39 - Goreleaser: Builds Docker images (1 min)
14:40 - GitHub: Release complete ✓

14:41 - macOS user: brew install daemon-hound  ✓
14:41 - Linux user: sudo apt install           ✓
14:41 - Windows user: scoop install            ✓
14:41 - Docker user: docker pull               ✓
14:41 - GitHub user: Downloads from release    ✓
```

**Total automation time**: ~10 minutes
**Your involvement**: 1 command (`make tag`)

---

## Next Release Instructions

### Ready for v1.1.0?

```bash
# 1. Update changelog (manual)
vim CHANGELOG.md
# Add v1.1.0 section

# 2. Commit
git add CHANGELOG.md
git commit -m "chore: release v1.1.0"
git push origin staging

# 3. Create release tag (this is IT!)
make tag TAG=v1.1.0

# 4. Wait 10 minutes
# Watch: https://github.com/0xdps/daemon-hound/actions

# 5. Done! Users can now install from anywhere
```

**That's the entire release process!**
