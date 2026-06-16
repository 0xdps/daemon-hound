# Release Workflow: From Code to Download

## End-to-End Release Process

### Step 1: Local Verification ✓

```bash
# Your machine
make build          # Compiles locally
make test           # Runs all tests
make snapshot       # Builds for ALL platforms locally (test run)

# Verify snapshot artifacts
ls -lh dist/
# You'll see: macOS, Linux, Windows artifacts
```

**Output**: 
- `dist/daemon-hound_VERSION_Darwin_x86_64/dh` (macOS binary)
- `dist/daemon-hound_VERSION_Linux_x86_64/dh` (Linux binary)
- `dist/daemon-hound_VERSION_Windows_x86_64/dh.exe` (Windows binary)
- Docker images built locally

---

### Step 2: Create Release Tag

```bash
# Your machine
make tag TAG=v1.1.0

# Behind the scenes:
#   git tag -a v1.1.0 -m "Release v1.1.0"
#   git push origin v1.1.0
```

---

### Step 3: GitHub Actions Triggers Automatically

**What happens** (automated in `.github/workflows/release.yml`):

1. ✅ GitHub detects new tag
2. ✅ Spins up CI runner
3. ✅ Runs goreleaser

```yaml
# .github/workflows/release.yml
on:
  push:
    tags:
      - 'v*'  # v1.1.0, v1.2.0, etc.
```

---

### Step 4: Goreleaser Builds All Platforms

**Single github.com/0xdps/daemon-hound repository → goreleaser creates:**

#### **macOS**
```
.goreleaser.yaml builds:
  goos: darwin
  goarch: [amd64, arm64]
  
Result:
  ✓ daemon-hound_1.1.0_macOS_x86_64.dmg
  ✓ daemon-hound_1.1.0_macOS_arm64.dmg
  
Each includes:
  ├─ DaemonHound.app/
  │  ├─ Contents/MacOS/dh          (binary)
  │  ├─ Contents/Info.plist        (metadata)
  │  └─ Contents/Resources/AppIcon.icns (your logo as app icon)
  ├─ README.md
  └─ LICENSE
```

#### **Linux**
```
.goreleaser.yaml builds:
  goos: linux
  goarch: [amd64, arm64]
  
.goreleaser.yaml nfpms:
  formats: [deb, rpm, apk, archlinux]
  
Result:
  ✓ daemon-hound_1.1.0_linux_x86_64.tar.gz
  ✓ daemon-hound_1.1.0_linux_arm64.tar.gz
  ✓ daemon-hound-1.1.0-1.x86_64.rpm
  ✓ daemon-hound-1.1.0-1.aarch64.rpm
  ✓ daemon-hound_1.1.0_amd64.deb
  ✓ daemon-hound_1.1.0_arm64.deb
  ✓ daemon-hound-1.1.0-1.apk
  ✓ daemon-hound-1.1.0-1-x86_64.pkg.tar.zst
```

#### **Windows**
```
.goreleaser.yaml builds:
  goos: windows
  goarch: amd64
  
Result:
  ✓ daemon-hound_1.1.0_Windows_x86_64.zip
    └─ dh.exe (portable executable)
    
.goreleaser.yaml scoops:
  Updates → 0xdps/scoop-bucket
  
Result:
  ✓ Scoop installation ready
```

#### **Docker**
```
.goreleaser.yaml dockers:
  images:
    - ghcr.io/0xdps/daemon-hound:1.1.0
    - ghcr.io/0xdps/daemon-hound:v1
    - ghcr.io/0xdps/daemon-hound:latest
```

---

### Step 5: Package Managers Updated

**Automatically by goreleaser:**

#### **Homebrew**
```bash
# Pushes to: 0xdps/homebrew-packages
# Creates formula at: Formula/daemon-hound.rb
# Users can: brew install daemon-hound
```

#### **Scoop (Windows)**
```bash
# Pushes to: 0xdps/scoop-bucket
# Creates manifest at: bucket/daemon-hound.json
# Users can: scoop install daemon-hound
```

---

### Step 6: GitHub Release Created

**Automatic release page with:**
```
Release v1.1.0 - June 15, 2026

📥 Downloads:
├─ macOS
│  ├─ daemon-hound_1.1.0_macOS_x86_64.dmg (50 MB)
│  └─ daemon-hound_1.1.0_macOS_arm64.dmg (48 MB)
│
├─ Linux
│  ├─ daemon-hound_1.1.0_linux_x86_64.tar.gz (15 MB)
│  ├─ daemon-hound_1.1.0_linux_arm64.tar.gz (14 MB)
│  ├─ daemon-hound-1.1.0-1.x86_64.rpm (5 MB)
│  ├─ daemon-hound_1.1.0_amd64.deb (5 MB)
│  └─ ... (more packages)
│
└─ Windows
   └─ daemon-hound_1.1.0_Windows_x86_64.zip (12 MB)

🔒 Checksums:
   SHA256: 8a4f3b2c1e9d5f7a2b4c6e8d9f1a3b5c...
   ... (all artifacts checksummed)
```

---

## Distribution Paths

```
GitHub Release (hub.github.com/0xdps/daemon-hound/releases)
        │
        ├─────────────────────────────────┬─────────────────┐
        │                                 │                 │
        ▼                                 ▼                 ▼
    
   macOS User                        Linux User        Windows User
   
   Homebrew (easiest)         Package Manager         Scoop (easiest)
   ├─ brew install            ├─ sudo apt install     ├─ scoop install
   └─ Auto-updates            ├─ sudo rpm -i          └─ Auto-updates
                              └─ Package updates
                              
   OR DMG Download            OR .tar.gz Download     OR ZIP Download
   ├─ Double-click            └─ tar xzf && install   └─ Extract & run
   └─ Drag to Applications
   
   OR Binary Download         OR Binary Download      OR Binary Download
   └─ Extract & run           └─ Extract & run        └─ Extract & run
```

---

## What the User Sees

### macOS User (Homebrew path):
```bash
$ brew install daemon-hound
==> Downloading daemon-hound_1.1.0_macOS_x86_64.dmg
######################################################################## 100.0%
==> Installing daemon-hound
✓ Installed

$ dh --version
Daemon Hound v1.1.0
```

Then in Activity Monitor:
```
Daemon Hound
Running in background
```

✅ Icon shows correctly  
✅ Name is "Daemon Hound"  
✅ Version 1.1.0

---

### Linux User (apt path):
```bash
$ sudo apt install daemon-hound_1.1.0_amd64.deb
Setting up daemon-hound (1.1.0)...
Processing triggers for man-db (2.10.2-1)...

$ dh --version
Daemon Hound v1.1.0

$ dh daemon
```

✅ Binary in `/usr/bin/dh`  
✅ Version 1.1.0

---

### Windows User (Scoop path):
```powershell
> scoop install daemon-hound
Installing 'daemon-hound' (1.1.0) [100%]

> dh --version
Daemon Hound v1.1.0

> dh daemon
```

✅ Available in PowerShell  
✅ Version 1.1.0

---

## Version Numbers Flow Through

All builds inherit version from:

1. **Git tag** (v1.1.0) → goreleaser
2. **Goreleaser** → ldflags in Makefile
3. **Build** → `go build -ldflags "-X github.com/0xdps/daemon-hound/internal/cmd.version=1.1.0"`
4. **Binary** → `dh --version` shows v1.1.0
5. **Package** → .deb, .rpm, .dmg all show 1.1.0
6. **Distribution** → Homebrew, Scoop, package managers see 1.1.0

Result: Consistent version everywhere.

---

## Complete Release Checklist

```bash
# In your local repo
☐ Update CHANGELOG.md
☐ Commit: git commit -m "chore: release v1.1.0"
☐ Run tests: make test ✓
☐ Build locally: make build ✓
☐ Tag: make tag TAG=v1.1.0
☐ Monitor GitHub Actions (wait 5-10 minutes)
☐ Verify release page created
☐ Test Homebrew: brew install daemon-hound ✓
☐ Test Linux: apt install or direct download ✓
☐ Test Windows: scoop install ✓
☐ Announce release
```

---

## Why App Icon Now Shows Correctly

**Before**: `dh` binary alone has no metadata → generic "exec" icon

**Now**: `DaemonHound.app` bundle includes:
```
DaemonHound.app/Contents/
├─ Info.plist          ← CFBundleName = "Daemon Hound"
│                      ← CFBundleIconFile = "AppIcon"
└─ Resources/AppIcon.icns  ← Your logo as icon
```

macOS reads Info.plist and shows your icon + proper name in Activity Monitor.

---

## Next Release

Same process:
```bash
# v1.2.0 when ready
make tag TAG=v1.2.0
# → Automatically builds + publishes everything
```
