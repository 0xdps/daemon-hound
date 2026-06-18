# What Happens When You Release (Step-by-Step)

## Your Action: `make tag TAG=v1.1.0`

### What This Command Does

```bash
$ make tag TAG=v1.1.0

# Behind the scenes (from Makefile):
git tag -a v1.1.0 -m "Release v1.1.0"
git push origin v1.1.0

# Output:
✓ Tag v1.1.0 created and pushed
```

---

## GitHub Actions Automatic Process (8-10 minutes)

### Minute 0: GitHub Detects Tag

```
Event:   Push tag v1.1.0 to repository
Repo:    0xdps/daemon-hound
Trigger: .github/workflows/release.yml
Status:  Starting...
```

**What GitHub does**:
1. Detects push to refs/tags/v1.1.0
2. Checks .github/workflows/release.yml triggers
3. Matches: `on: push: tags: - "v*"`
4. ✅ Match! Workflow triggered

---

### Minute 0-1: GitHub Actions Setup

```
Status: Setting up runner
└─ Runner: ubuntu-latest (Ubuntu 22.04 LTS)
   ├─ CPUs: 4
   ├─ RAM: 16 GB
   ├─ Disk: 30 GB
   └─ Pre-installed: Go, Docker, Git, etc.

Workflow: Starting jobs
```

**Steps executing**:

```yaml
step 1: Checkout
  → git clone https://github.com/0xdps/daemon-hound
  → cd daemon-hound
  → git fetch-depth: 0  (full history)
  → ✓ Complete

step 2: Set up Go
  → Read go.mod → version 1.26.4
  → Install Go 1.26.4
  → ✓ Ready

step 3: Set up QEMU
  → Enable emulation for multiple architectures
  → ✓ arm64 support enabled
  → ✓ amd64 support enabled

step 4: Set up Docker Buildx
  → Install docker/setup-buildx
  → Configure multi-architecture builder
  → ✓ Ready for docker buildx build

step 5: Login to GitHub Container Registry
  → registry: ghcr.io
  → username: 0xdps (from github.actor)
  → password: ${GITHUB_TOKEN} (built-in)
  → ✓ Authenticated
```

**Duration**: 60 seconds

---

### Minute 1-2: Goreleaser Builds All Binaries

```
Status: Running goreleaser release --clean

goreleaser reads: .goreleaser.yaml
builds:
  - goos: [darwin, linux, windows, freebsd]
    goarch: [amd64, arm64]
```

**Building phase** (30 seconds):

```
Building darwin/amd64
  GOOS=darwin GOARCH=amd64
  go build -ldflags "-X version=v1.1.0 -X commit=abc1234 ..."
  → dist/daemon-hound_1.1.0_darwin_amd64/dhd
  ✓ Complete

Building darwin/arm64 (M1/M2 Macs)
  GOOS=darwin GOARCH=arm64
  → dist/daemon-hound_1.1.0_darwin_arm64/dhd
  ✓ Complete

Building linux/amd64
  GOOS=linux GOARCH=amd64
  → dist/daemon-hound_1.1.0_linux_amd64/dhd
  ✓ Complete

Building linux/arm64 (Raspberry Pi, ARM servers)
  GOOS=linux GOARCH=arm64
  → dist/daemon-hound_1.1.0_linux_arm64/dhd
  ✓ Complete

Building windows/amd64
  GOOS=windows GOARCH=amd64
  → dist/daemon-hound_1.1.0_windows_amd64/dhd.exe
  ✓ Complete

Building freebsd/amd64
  GOOS=freebsd GOARCH=amd64
  → dist/daemon-hound_1.1.0_freebsd_amd64/dhd
  ✓ Complete

Summary: 6 binaries built ✓
Time: 30 seconds
```

---

### Minute 2-5: Goreleaser Creates Packages

```
Status: Packaging phase

From binary files → Create OS-specific packages
```

#### macOS Packaging

```
Input: dist/daemon-hound_1.1.0_darwin_amd64/dhd
  dist/daemon-hound_1.1.0_darwin_arm64/dhd

Processing for .dmg creation:
  1. Create DaemonHound.app/ bundle structure
  2. Copy dhd binary to: Contents/MacOS/dhd
  3. Copy Info.plist to: Contents/Info.plist
  4. Copy AppIcon.icns to: Contents/Resources/AppIcon.icns
  5. Include: README.md, LICENSE, CHANGELOG.md
  6. Create universal binary (x86_64 + arm64)
  7. Sign the .dmg

Output: 
  daemon-hound_1.1.0_darwin_x86_64.dmg  (50 MB)
  daemon-hound_1.1.0_darwin_arm64.dmg   (48 MB)
  ✓ Complete
```

#### Linux Packaging

```
From: dist/daemon-hound_1.1.0_linux_*

nfpms (not-fake package management system):
  
  Creating .tar.gz archives:
    daemon-hound_1.1.0_linux_x86_64.tar.gz
    daemon-hound_1.1.0_linux_arm64.tar.gz
  
  Creating .deb (Debian/Ubuntu):
    daemon-hound_1.1.0_amd64.deb
    daemon-hound_1.1.0_arm64.deb
    (Dependencies: git)
    (Installs to: /usr/bin/dhd)
  
  Creating .rpm (RedHat/CentOS/Fedora):
    daemon-hound-1.1.0-1.x86_64.rpm
    daemon-hound-1.1.0-1.aarch64.rpm
  
  Creating .apk (Alpine):
    daemon-hound-1.1.0-1.apk
  
  Creating .pkg.tar.zst (Arch Linux):
    daemon-hound-1.1.0-1-x86_64.pkg.tar.zst

Output: 8 packages ✓
```

#### Windows Packaging

```
From: dist/daemon-hound_1.1.0_windows_amd64/dhd.exe

Creating portable ZIP:
  daemon-hound_1.1.0_windows_x86_64.zip
  └─ dhd.exe
  └─ README.md
  └─ LICENSE

Output: daemon-hound_1.1.0_Windows_x86_64.zip ✓
```

#### Docker Images

```
From: Dockerfile

Building multi-architecture Docker images:
  
  docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t ghcr.io/0xdps/daemon-hound:v1.1.0 \
    -t ghcr.io/0xdps/daemon-hound:v1 \
    -t ghcr.io/0xdps/daemon-hound:latest

Output:
  ghcr.io/0xdps/daemon-hound:v1.1.0 ✓
  ghcr.io/0xdps/daemon-hound:v1 ✓
  ghcr.io/0xdps/daemon-hound:latest ✓
```

**Time for packaging**: 3 minutes

---

### Minute 5-8: Goreleaser Distributes

```
Status: Distribution phase

GitHub Release
├─ Creating release page for v1.1.0
├─ Title: Release v1.1.0
├─ Body: Changelog from CHANGELOG.md
├─ Uploading all artifacts:
│  ├─ daemon-hound_1.1.0_macOS_x86_64.dmg
│  ├─ daemon-hound_1.1.0_macOS_arm64.dmg
│  ├─ daemon-hound_1.1.0_linux_x86_64.tar.gz
│  ├─ daemon-hound_1.1.0_linux_arm64.tar.gz
│  ├─ daemon-hound-1.1.0-1.x86_64.rpm
│  ├─ daemon-hound-1.1.0-1.aarch64.rpm
│  ├─ daemon-hound_1.1.0_amd64.deb
│  ├─ daemon-hound_1.1.0_arm64.deb
│  ├─ daemon-hound-1.1.0-1.apk
│  ├─ daemon-hound-1.1.0-1-x86_64.pkg.tar.zst
│  └─ daemon-hound_1.1.0_Windows_x86_64.zip
│
├─ Generating SHA256 checksums
│  └─ checksums.txt (uploaded)
│
└─ ✓ GitHub Release created
   URL: github.com/0xdps/daemon-hound/releases/tag/v1.1.0
```

#### Push to Homebrew (if GH_PAT set)

```
Homebrew repository: 0xdps/homebrew-packages

1. Generate formula file:
   Formula/daemon-hound.rb
   └─ Version: v1.1.0
   └─ URL: GitHub Release download link
   └─ SHA256: checksum
   └─ Install: bin.install "dhd"

2. Push to repository:
   git clone https://github.com/0xdps/homebrew-packages
   cp Formula/daemon-hound.rb homebrew-packages/
   git add Formula/daemon-hound.rb
   git commit -m "Brew formula update for daemon-hound version v1.1.0"
   git push origin trunk

✓ Homebrew updated
  Users can now: brew install daemon-hound
```

#### Push to Scoop (if GH_PAT set)

```
Scoop bucket: 0xdps/scoop-bucket

1. Generate manifest:
   bucket/daemon-hound.json
   └─ Version: 1.1.0
   └─ URL: GitHub Release download link
   └─ SHA256: checksum

2. Push to bucket:
   git clone https://github.com/0xdps/scoop-bucket
   cp daemon-hound.json scoop-bucket/bucket/
   git add bucket/daemon-hound.json
   git commit -m "Update daemon-hound to v1.1.0"
   git push origin trunk

✓ Scoop updated
  Users can now: scoop install daemon-hound
```

#### Push to Docker Registry

```
Docker images already built, now pushing:

docker push ghcr.io/0xdps/daemon-hound:v1.1.0
docker push ghcr.io/0xdps/daemon-hound:v1
docker push ghcr.io/0xdps/daemon-hound:latest

✓ Docker images published to GitHub Container Registry
  Users can now: docker pull ghcr.io/0xdps/daemon-hound:v1.1.0
```

**Time for distribution**: 3 minutes

---

### Minute 8-10: Final Verification

```
Status: Wrapping up

✓ All artifacts uploaded to GitHub Release
✓ All checksums generated
✓ Homebrew formula updated (if GH_PAT set)
✓ Scoop bucket updated (if GH_PAT set)
✓ Docker images pushed (if Docker login successful)
✓ Workflow logs available

Workflow Status: ✅ COMPLETED SUCCESSFULLY
Duration: 10 minutes (total)
```

---

## After Release (Minute 10+)

### GitHub Release Page

```
URL: github.com/0xdps/daemon-hound/releases/tag/v1.1.0

What users see:
┌─────────────────────────────────────────────────────────┐
│ Release daemon-hound v1.1.0                             │
│ Released on 15 Jun 2026                                 │
│                                                          │
│ 📝 Release Notes                                        │
│ Version 1.1.0 of daemon-hound includes:                │
│ - Smart merge system                                   │
│ - Multi-platform support                               │
│ ...                                                      │
│                                                          │
│ 📥 Downloads                                            │
│ • daemon-hound_1.1.0_macOS_x86_64.dmg (50 MB)         │
│ • daemon-hound_1.1.0_macOS_arm64.dmg (48 MB)          │
│ • daemon-hound_1.1.0_linux_x86_64.tar.gz (15 MB)      │
│ • daemon-hound_1.1.0_linux_arm64.tar.gz (14 MB)       │
│ • daemon-hound-1.1.0-1.x86_64.rpm (5 MB)              │
│ • daemon-hound-1.1.0-1.aarch64.rpm (5 MB)             │
│ • daemon-hound_1.1.0_amd64.deb (5 MB)                 │
│ • daemon-hound_1.1.0_arm64.deb (5 MB)                 │
│ • daemon-hound-1.1.0-1.apk (4 MB)                     │
│ • daemon-hound-1.1.0-1-x86_64.pkg.tar.zst (5 MB)     │
│ • daemon-hound_1.1.0_Windows_x86_64.zip (12 MB)       │
│                                                          │
│ 🔒 Checksums                                            │
│ checksums.txt: SHA256 for all above                    │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Users Can Now Install

**macOS**:
```bash
$ brew install daemon-hound
✓ Installed daemon-hound v1.1.0

$ dhd --version
Daemon Hound v1.1.0
```

**Linux (Debian/Ubuntu)**:
```bash
$ sudo apt install daemon-hound_1.1.0_amd64.deb
✓ Setting up daemon-hound

$ dhd --version
Daemon Hound v1.1.0
```

**Windows**:
```powershell
> scoop install daemon-hound
Installing 'daemon-hound' (1.1.0) [100%]

> dhd --version
Daemon Hound v1.1.0
```

**Docker**:
```bash
$ docker pull ghcr.io/0xdps/daemon-hound:v1.1.0
v1.1.0: Pulling from 0xdps/daemon-hound
```

---

## Summary

**Your involvement**:
```bash
make tag TAG=v1.1.0    # 1 command
# Wait 10 minutes...   # GitHub Actions does everything
```

**GitHub Actions does**:
- ✅ Builds 6 platform binaries
- ✅ Creates 8 Linux packages
- ✅ Creates 2 macOS DMGs
- ✅ Creates 1 Windows ZIP
- ✅ Builds 2 Docker images
- ✅ Creates GitHub Release
- ✅ Updates Homebrew
- ✅ Updates Scoop
- ✅ Generates checksums
- ✅ All automated!

**Users get**:
- ✅ Easy installation on any platform
- ✅ Multiple installation methods
- ✅ Auto-updates (if using package manager)
- ✅ Professional branding (app icon, name)
- ✅ Version consistency everywhere

**Time investment**: 1 command + 10 minutes wait
**Result**: Professional, multi-platform distribution to thousands of users
