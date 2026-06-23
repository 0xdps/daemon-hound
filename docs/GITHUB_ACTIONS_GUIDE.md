# GitHub Actions Workflow Guide

## Overview

Your repository has **two workflows** that automate building, testing, and releasing:

1. **build.yml** - Runs on every push/PR (continuous integration)
2. **release.yml** - Runs only on git tags (continuous deployment)

---

## Workflow 1: Build (Continuous Integration)

**File**: `.github/workflows/build.yml`

**Triggers**: 
- Every push to `trunk` or `main`
- Every pull request to `trunk` or `main`

### What It Does

```yaml
on:
  push:
    branches: [trunk, main]
  pull_request:
    branches: [trunk, main]
```

### Jobs

#### Job 1: Test
```bash
runs-on: ubuntu-latest

1. Checkout code
2. Set up Go environment
3. Run: go test -v -race ./...    # Test with race detection
4. Run: go vet ./...              # Static analysis
5. Run: gofmt check               # Code formatting validation
```

**Ensures**: Code compiles, all tests pass, no formatting issues

#### Job 2: Build Matrix (Tests all platforms)
```bash
runs-on: ubuntu-latest
strategy:
  matrix:
    - darwin/amd64, darwin/arm64   # macOS
    - linux/amd64, linux/arm64     # Linux
    - windows/amd64                # Windows
    - freebsd/amd64                # FreeBSD
```

**For each platform**:
```bash
GOOS=darwin GOARCH=amd64 go build \
  -ldflags "-X version=..., -X commit=..., -X date=..." \
  -o dist/dhd-darwin-amd64 \
  ./cmd/dhd
```

**Result**: 6 binaries compiled (one per platform/arch combination)

**Uploads**: Each binary as artifact (accessible for 7 days)

#### Job 3: Docker Build
```bash
Set up QEMU (multi-arch support)
Set up Docker Buildx (multi-platform builder)
Build Docker image for all architectures
```

---

## Workflow 2: Release (Continuous Deployment)

**File**: `.github/workflows/release.yml`

**Triggers Only**: When you push a git tag matching `v*`

```yaml
on:
  push:
    tags:
      - "v*"  # v1.0.0, v1.1.0, v2.0.0, etc.
```

### What It Does

When you run: `make tag TAG=v1.1.0`

```
1. git push origin v1.1.0
         ↓
2. GitHub detects tag
         ↓
3. GitHub Actions workflow triggers
         ↓
4. Runs goreleaser
         ↓
5. Publishes to:
   - GitHub Release
   - Homebrew
   - Scoop
   - Docker Registry
   - Package repositories
```

### Permissions

```yaml
permissions:
  contents: write      # Create GitHub releases
  packages: write      # Push Docker images to GitHub Container Registry
```

### Setup Steps

1. **Checkout** - Clone repo with full history (`fetch-depth: 0`)
   ```yaml
   - uses: actions/checkout@v4
     with:
       fetch-depth: 0  # Full git history for versioning
   ```

2. **Go Setup** - Install Go from go.mod version
   ```yaml
   - uses: actions/setup-go@v5
     with:
       go-version-file: go.mod
   ```

3. **QEMU** - Enable multi-architecture Docker builds
   ```yaml
   - uses: docker/setup-qemu-action@v3
   ```

4. **Docker Buildx** - Multi-platform builder
   ```yaml
   - uses: docker/setup-buildx-action@v3
   ```

5. **Docker Login** - Authenticate to GitHub Container Registry
   ```yaml
   - uses: docker/login-action@v3
     with:
       registry: ghcr.io
       username: ${{ github.actor }}
       password: ${{ secrets.GITHUB_TOKEN }}
   ```

6. **Run Goreleaser** - The magic happens here
   ```yaml
   - uses: goreleaser/goreleaser-action@v6
     with:
       distribution: goreleaser
       version: "~> v2"
       args: release --clean
     env:
       GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
       GH_PAT: ${{ secrets.GH_PAT }}
   ```

---

## Complete Flow Diagram

```
Developer commits code
         ↓
Push to trunk/main
         ↓
     ┌──────────────────────────────────┐
     │  build.yml triggers              │
     │  (tests + matrix builds)         │
     ├──────────────────────────────────┤
     │ ✓ Run tests (all platforms)      │
     │ ✓ Run vet (static analysis)      │
     │ ✓ Check formatting               │
     │ ✓ Build 6 binaries               │
     │ ✓ Build Docker image             │
     └──────────────────────────────────┘
         ↓
      ✓ All pass?
         ↓ YES
    PR can be merged / merge to main
         ↓
Developer creates tag:
  make tag TAG=v1.1.0
         ↓
  git push origin v1.1.0
         ↓
     ┌──────────────────────────────────┐
     │  release.yml triggers            │
     │  (goreleaser automates)          │
     ├──────────────────────────────────┤
     │ 1. Setup Go                      │
     │ 2. Setup Docker/QEMU             │
     │ 3. Login to container registry   │
     │ 4. Run goreleaser:               │
     │    ├─ Build all platforms        │
     │    ├─ Create packages            │
     │    ├─ Create GitHub Release      │
     │    ├─ Update Homebrew            │
     │    ├─ Update Scoop               │
     │    └─ Push Docker images         │
     └──────────────────────────────────┘
         ↓
  GitHub Release page created
  with all downloads available
         ↓
     Users download from:
     ├─ GitHub Release (direct)
     ├─ Homebrew (macOS)
     ├─ Scoop (Windows)
     ├─ Package managers (Linux)
     └─ Docker Hub
```

---

## Step-by-Step: What Happens When You Run `make tag TAG=v1.1.0`

### Your Machine
```bash
$ make tag TAG=v1.1.0

# Behind scenes:
#   1. git tag -a v1.1.0 -m "Release v1.1.0"
#   2. git push origin v1.1.0

$ echo "✓ Tag pushed. GitHub Actions takes it from here..."
```

### GitHub Actions (Automatic)

**Minute 0-1**: GitHub detects new tag `v1.1.0`

**Step 1**: Workflow triggered
```
Event: Push tag v1.1.0
Workflow: release.yml
Status: Started
```

**Step 2**: Checkout + Setup
```
✓ Clone repository (full history)
✓ Install Go v1.26.4 (from go.mod)
✓ Setup QEMU for multi-arch Docker builds
✓ Setup Docker Buildx
✓ Login to ghcr.io (GitHub Container Registry)
```

**Step 3**: Run Goreleaser
```bash
goreleaser release --clean

# Goreleaser reads .goreleaser.yaml and:

1. BUILD PHASE (5-10 seconds each)
   ├─ darwin/amd64 binary
   ├─ darwin/arm64 binary
   ├─ linux/amd64 binary
   ├─ linux/arm64 binary
   ├─ windows/amd64 binary
   └─ freebsd/amd64 binary

2. PACKAGE PHASE (2-5 seconds)
   ├─ macOS DMG (with app bundle)
   ├─ Linux .deb packages
   ├─ Linux .rpm packages
   ├─ Linux .apk packages
   ├─ Linux .tar.gz archives
   └─ Windows .zip

3. DISTRIBUTION PHASE (10-30 seconds)
   ├─ Create GitHub Release
   ├─ Upload all artifacts
   ├─ Generate checksums (SHA256)
   ├─ Push to Homebrew repo
   ├─ Push to Scoop bucket
   └─ Build + push Docker images to ghcr.io
```

**Total time**: 5-10 minutes

**Status**: ✓ All complete

---

## What Goreleaser Actually Does (Called from release.yml)

### From `.goreleaser.yaml`

#### Build Section
```yaml
builds:
  - id: dhd
    main: ./cmd/dhd
    binary: dhd
    goos: [darwin, linux, windows, freebsd]
    goarch: [amd64, arm64]
    ldflags: [version, commit, date]
```

✓ Creates: 6 platform-specific binaries

#### Archives Section
```yaml
archives:
  - format_overrides:
      - goos: windows
        formats: ["zip"]  # Windows gets .zip
    format: tar.gz        # Others get .tar.gz
```

✓ Packages binaries into archives

#### Homebrew Section
```yaml
brews:
  - name: daemon-hound
    repository:
      owner: 0xdps
      name: homebrew-packages
      token: "{{ .Env.GH_PAT }}"
```

✓ Pushes formula to `0xdps/homebrew-packages` repo
✓ Users can then: `brew install daemon-hound`

#### Scoop Section
```yaml
scoops:
  - name: daemon-hound
    repository:
      owner: 0xdps
      name: scoop-bucket
      token: "{{ .Env.GH_PAT }}"
```

✓ Pushes manifest to `0xdps/scoop-bucket` repo
✓ Users can then: `scoop install daemon-hound`

#### Docker Section
```yaml
dockers:
  - image_templates:
      - "ghcr.io/0xdps/daemon-hound:{{ .Tag }}"
      - "ghcr.io/0xdps/daemon-hound:v{{ .Major }}"
      - "ghcr.io/0xdps/daemon-hound:latest"
```

✓ Builds Docker images
✓ Tags as `1.1.0`, `v1`, `latest`
✓ Pushes to GitHub Container Registry

#### GitHub Release
```
Automatically created with:
├─ All built artifacts
├─ SHA256 checksums
├─ Release notes (from CHANGELOG.md)
└─ Download links
```

---

## Required Secrets for Release.yml to Work

GitHub Actions needs these **secrets** (stored in Settings → Secrets):

1. **GITHUB_TOKEN** (automatic)
   - Built-in GitHub token
   - Used for: Creating releases, uploading artifacts
   - ✅ Already available

2. **GH_PAT** (Personal Access Token)
   - Needed for: Pushing to external Homebrew/Scoop repos
   - What you need to set up: Token for `0xdps/homebrew-packages` repo access
   - Status: **Needs to be configured**

---

## Current Status

### ✅ Already Working

- **build.yml**: Tests + builds on every PR/push
- **release.yml**: Triggers on tag push
- **Goreleaser**: Configured to build all platforms
- **Permissions**: Set correctly in workflow

### ⚠️ Needs Configuration

- **GH_PAT Secret**: Add personal access token for Homebrew/Scoop updates
  ```bash
  # On GitHub: Settings → Secrets and variables → Actions
  # Add: GH_PAT = your_github_personal_access_token
  ```

---

## Example Release Execution Timeline

```
16:30 - You run: make tag TAG=v1.1.0
16:31 - GitHub detects tag v1.1.0
16:31 - GitHub Actions workflow starts
16:31-16:32 - Setup Go, Docker, auth
16:32-16:37 - Goreleaser builds 6 binaries
16:37-16:38 - Package into .deb, .rpm, .dmg, .zip
16:38-16:40 - Create GitHub Release, upload all files
16:40-16:42 - Generate checksums
16:42-16:44 - Push to Homebrew repo
16:44-16:46 - Push to Scoop bucket
16:46-16:48 - Build + push Docker images
16:48 - ✅ COMPLETE
       GitHub Release created
       Homebrew available: brew install daemon-hound
       Scoop available: scoop install daemon-hound
       Docker available: docker pull ghcr.io/0xdps/daemon-hound:v1.1.0
```

---

## How to Test Locally

```bash
# Test release build locally (without pushing)
make snapshot

# This runs goreleaser in snapshot mode:
# ✓ Builds all platforms
# ✓ Creates packages
# ✓ Skips GitHub upload
# ✓ Output in dist/
```

---

## Troubleshooting Release Workflow

### Issue: Release workflow fails at "Run GoReleaser"

**Check**:
```bash
# 1. Verify tag format
git tag -l | grep v

# 2. Verify .goreleaser.yaml syntax
goreleaser check

# 3. Verify go.mod is valid
go mod tidy
```

### Issue: GitHub Release created but packages missing

**Likely**: Goreleaser syntax error in `.goreleaser.yaml`

**Fix**:
```bash
# Test locally
make snapshot

# Look for errors in output
# Usually: missing dependencies or invalid docker setup
```

### Issue: Homebrew/Scoop not updated

**Likely**: GH_PAT secret not configured or invalid

**Fix**:
1. Generate new GitHub Personal Access Token
2. Set permissions: repo (full), workflows
3. Add as secret: Settings → Secrets → GH_PAT

### Issue: Docker images not pushed

**Check**:
```bash
# Verify login credentials
echo $GITHUB_TOKEN | docker login ghcr.io -u ${{ github.actor }} --password-stdin

# Check permissions on ghcr.io
```

---

## Next Time: Making a Release

```bash
# 1. Verify everything passes
make build
make test
make snapshot    # Test release build

# 2. Tag the release
make tag TAG=v1.1.0

# 3. Wait for GitHub Actions (5-10 minutes)
# Open: https://github.com/0xdps/daemon-hound/actions

# 4. Once complete, users can:
brew install daemon-hound           # macOS
scoop install daemon-hound          # Windows
echo 'deb [trusted=yes] https://0xdps.github.io/daemon-hound/apt stable main' | sudo tee /etc/apt/sources.list.d/daemon-hound.list
sudo apt update && sudo apt install daemon-hound   # Linux (Debian/Ubuntu)
docker pull ghcr.io/...             # Docker
```

All **completely automated** by GitHub Actions + Goreleaser!
