# Build & Release Reference

## Quick Reference

### Local Development (macOS)
```bash
make build              # Build binary: bin/dhd
make build-macos        # Build app bundle: build/DaemonHound.app
make install-macos      # Install to ~/Applications
make test               # Run tests
```

### Local Development (Linux/Windows)
```bash
make build              # Build native binary
make test               # Run tests
make snapshot           # Build all platforms locally
```

### Release Process

```bash
# 1. Tag the release
make tag TAG=v1.1.0

# 2. Goreleaser builds everything automatically (via GitHub Actions)
# It creates:
# - macOS DMG with app bundle
# - Linux packages (.deb, .rpm, .apk)
# - Windows ZIP + updates Scoop
# - Docker images
# - GitHub Release page with all downloads
```

---

## Build Targets Explained

### Development Targets

| Target | What it does | Output | Use when |
|--------|-------------|--------|----------|
| `make build` | Build single binary for current OS | `bin/dhd` | Testing locally |
| `make build-macos` | Build app bundle with icon | `build/DaemonHound.app/` | macOS desktop distribution |
| `make build-macos-icon` | Regenerate app icon | `build/AppIcon.icns` | Logo changed |
| `make install-macos` | Install app to ~/Applications | Installed app | Ready to use |
| `make launch-daemon` | Install + launch | Running daemon | Testing daemon |
| `make test` | Run all tests | Test output | Before commit |
| `make tidy` | Clean up Go modules | Updated go.mod | Before release |
| `make fmt` | Format code | Formatted code | Before commit |
| `make lint` | Run linter | Lint report | Before commit |

### Release Targets

| Target | What it does | Requires | Output |
|--------|-------------|----------|--------|
| `make snapshot` | Build all platforms (test release) | goreleaser | `dist/` folder with all artifacts |
| `make release` | Full release to GitHub | git tag + goreleaser | GitHub Release + all platforms |
| `make tag TAG=v1.1.0` | Create and push git tag | Clean repo | Git tag pushed |

### Cleanup

| Target | What it does |
|--------|-------------|
| `make clean` | Remove `bin/`, `build/`, `dist/` |

---

## Goreleaser Automation

When you push a **git tag** (v1.1.0), GitHub Actions automatically runs goreleaser which:

### 1. Builds Binaries
```
For each platform (darwin/linux/windows) × architecture (amd64/arm64):
  ✓ Compiles binary with version info
```

### 2. Creates macOS Artifacts
```
✓ daemon-hound_1.1.0_macOS_x86_64.dmg  (with app bundle)
✓ daemon-hound_1.1.0_macOS_arm64.dmg   (M1/M2 native)
```

### 3. Creates Linux Artifacts
```
✓ daemon-hound_1.1.0_linux_x86_64.tar.gz
✓ daemon-hound_1.1.0_linux_arm64.tar.gz
✓ daemon-hound-1.1.0-1.x86_64.rpm      (RedHat/CentOS/Fedora)
✓ daemon-hound-1.1.0-1.aarch64.rpm
✓ daemon-hound_1.1.0_amd64.deb         (Debian/Ubuntu)
✓ daemon-hound_1.1.0_arm64.deb
✓ daemon-hound-1.1.0-1.apk             (Alpine)
✓ daemon-hound-1.1.0-1-x86_64.pkg.tar.zst (Arch)
```

### 4. Creates Windows Artifacts
```
✓ daemon-hound_1.1.0_Windows_x86_64.zip
✓ dhd.exe (portable executable)
```

### 5. Updates Package Managers
```
✓ Homebrew formula → 0xdps/homebrew-packages
✓ Scoop manifest → 0xdps/scoop-bucket
```

### 6. Publishes Docker Images
```
✓ ghcr.io/0xdps/daemon-hound:1.1.0
✓ ghcr.io/0xdps/daemon-hound:v1
✓ ghcr.io/0xdps/daemon-hound:latest
```

### 7. Creates GitHub Release
```
✓ Release page with all downloads
✓ Checksums for verification (SHA256)
✓ Release notes from CHANGELOG
```

---

## Understanding the Architecture

### Single Binary, Multiple Packages

```
daemon-hound (Go source code)
    │
    ├─── macOS ──────┐
    │                ├─→ DMG installer
    │                ├─→ Homebrew formula
    │                └─→ App bundle (with icon, metadata)
    │
    ├─── Linux ──────┐
    │                ├─→ .tar.gz archive
    │                ├─→ .deb package (Debian/Ubuntu)
    │                ├─→ .rpm package (RedHat/CentOS)
    │                ├─→ .apk package (Alpine)
    │                └─→ .pkg.tar.zst (Arch Linux)
    │
    ├─── Windows ────┐
    │                ├─→ .zip portable
    │                ├─→ .exe executable
    │                └─→ Scoop manifest
    │
    └─── Docker ─────→ Container images (amd64, arm64)
```

**Key Point**: Same Go code compiles to different executables for each OS/architecture. Goreleaser packages them appropriately for each platform's ecosystem.

---

## Complete Release Checklist

### Pre-Release
```bash
☐ Update CHANGELOG.md
☐ Update version numbers in code if needed
☐ Run tests: make test
☐ Run linter: make lint
☐ Create local snapshot: make snapshot
☐ Test snapshot builds
☐ Commit all changes
```

### Release
```bash
☐ Tag release: make tag TAG=v1.1.0
  (This triggers GitHub Actions → goreleaser)
```

### Post-Release (Monitor)
```bash
☐ Watch GitHub Actions workflow
☐ Verify all artifacts appear in Release
☐ Test installation on macOS (Homebrew)
☐ Test installation on Linux (APT repo + manual .deb)
☐ Test installation on Windows (Scoop)
☐ Verify Docker images published
```

### User Communication
```bash
☐ Update README with download links
☐ Post release announcement
☐ Update documentation if needed
```

---

## How Users Install

### macOS User
```bash
# Easiest: Homebrew
brew tap 0xdps/packages
brew install daemon-hound

# Or: Download DMG from GitHub Release
# Double-click and drag to Applications
```

### Linux User
```bash
# Easiest: Package manager
sudo dpkg -i ./daemon-hound_1.1.0_amd64.deb

# Or: Untar binary
tar xzf daemon-hound_1.1.0_linux_x86_64.tar.gz
sudo mv dhd /usr/local/bin/
```

### Windows User
```powershell
# Easiest: Scoop
scoop install daemon-hound

# Or: Download ZIP
Expand-Archive daemon-hound_1.1.0_Windows_x86_64.zip
```

### Docker User
```bash
docker pull ghcr.io/0xdps/daemon-hound:latest
docker run ghcr.io/0xdps/daemon-hound dhd --version
```

---

## Configuration Files

### `.goreleaser.yaml`
Controls how all artifacts are built and published:
- Which platforms/architectures to build
- How to package each (DMG, deb, rpm, etc.)
- Where to publish (Homebrew, Scoop, GitHub)
- How to sign/notarize

### `Makefile`
Local development and testing targets

### `.github/workflows/release.yml`
GitHub Actions workflow that runs goreleaser on tag

## macOS Signing & Notarization

Release-time macOS artifacts are now built on `macos-latest` and require these GitHub Actions secrets:

| Secret | Purpose |
|---|---|
| `APPLE_DEVELOPER_IDENTITY` | Full Developer ID Application identity, e.g. `Developer ID Application: Your Name (TEAMID)` |
| `MACOS_CERT_P12_BASE64` | Base64-encoded exported Developer ID `.p12` certificate |
| `MACOS_CERT_PASSWORD` | Password for the `.p12` certificate |
| `MACOS_KEYCHAIN_PASSWORD` | Temporary CI keychain password |
| `MACOS_NOTARY_KEY_ID` | App Store Connect API key ID |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect issuer UUID |
| `MACOS_NOTARY_KEY_BASE64` | Base64-encoded `AuthKey_XXXXXX.p8` contents |
| `GPG_PRIVATE_KEY` | ASCII-armored GPG private key for release checksum signing |
| `GPG_PASSPHRASE` | Passphrase for the GPG key |

The release workflow now:

1. imports your Developer ID certificate into a temporary CI keychain
2. builds arch-specific `DaemonHound.app` bundles
3. signs the app bundle and resulting `.dmg`
4. notarizes the `.dmg` with `xcrun notarytool`
5. staples the notarization ticket
6. uploads the notarized DMGs into the GitHub release
7. signs `checksums.txt` as `checksums.txt.asc`

### Prepare secrets locally

Export your certificate and notary key once:

```bash
# Developer ID certificate exported from Keychain Access
base64 -i Certificates.p12 | pbcopy

# App Store Connect API key
base64 -i AuthKey_ABC123XYZ.p8 | pbcopy

# Optional: ASCII-armored GPG secret key for checksums
gpg --armor --export-secret-keys YOUR_KEY_ID | pbcopy
```

There is also a helper script for preparing secret values:

```bash
scripts/prepare-release-secrets.sh macos-cert Certificates.p12
scripts/prepare-release-secrets.sh notary-key AuthKey_ABC123XYZ.p8
scripts/prepare-release-secrets.sh gpg-key YOUR_KEY_ID
scripts/prepare-release-secrets.sh identity
scripts/prepare-release-secrets.sh keychain-password
```

See `.github/SECRETS.md` for the full checklist.

### Local macOS release smoke test

```bash
export APPLE_DEVELOPER_IDENTITY="Developer ID Application: Your Name (TEAMID)"
export KEYCHAIN_PATH="$HOME/Library/Keychains/login.keychain-db"
export MACOS_NOTARY_KEY_PATH="$HOME/private_keys/AuthKey_ABC123XYZ.p8"
export MACOS_NOTARY_KEY_ID="ABC123XYZ"
export MACOS_NOTARY_ISSUER_ID="00000000-0000-0000-0000-000000000000"

scripts/build-macos-release.sh arm64 v1.2.0
```

This produces a signed, notarized DMG in `dist/`.

---

## Troubleshooting

### "make snapshot" failed
```bash
# Ensure goreleaser is installed
brew install goreleaser

# Clean and retry
make clean
make snapshot
```

### DMG installer not created
```bash
# Verify build/AppIcon.icns exists
make build-macos-icon

# Rebuild
make build-macos
```

### GitHub Actions release failed
```bash
# Check workflow logs in GitHub
# Most common: missing GH_PAT token for Homebrew/Scoop updates
# Or git tag not pushed correctly
git push origin v1.1.0
```

### Package not showing in Homebrew
```bash
# May take 5-10 minutes for Homebrew cache to update
# Force update: brew tap --force-auto-update 0xdps/packages
```

---

## Next Release Version

Example for v1.2.0:

```bash
# Update version in files if needed
# Update CHANGELOG.md
git add -A
git commit -m "chore: prepare v1.2.0 release"

# Tag and release
make tag TAG=v1.2.0

# GitHub Actions handles the rest automatically!
```
