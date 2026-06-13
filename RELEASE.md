# DaemonHound Release & Distribution Guide

This document describes how DaemonHound is built, released, and distributed across all supported platforms.

---

## Table of Contents

- [Release Philosophy](#release-philosophy)
- [Distribution Channels](#distribution-channels)
- [Installation Methods](#installation-methods)
- [Release Process](#release-process)
- [Platform Support Matrix](#platform-support-matrix)
- [Repository Requirements](#repository-requirements)
- [Troubleshooting](#troubleshooting)

---

## Release Philosophy

DaemonHound follows these principles for releases:

- **Automated**: Every release is fully automated via GitHub Actions + GoReleaser
- **Reproducible**: Same commit always produces identical binaries
- **Multi-platform**: Native binaries for all major platforms, no emulation required
- **Signed**: Checksums provided for all artifacts
- **Versioned**: Semantic versioning with clear changelogs

---

## Distribution Channels

### 1. GitHub Releases (Primary)

All binaries, packages, and checksums are published as GitHub Release assets.

**URL**: `https://github.com/0xdps/daemon-hound/releases`

**Assets per release**:
- 10 platform-specific binaries (`.tar.gz` or `.zip`)
- `.deb` package (Debian/Ubuntu)
- `.rpm` package (Fedora/RHEL/openSUSE)
- `.apk` package (Alpine Linux)
- Arch Linux package
- `checksums.txt` (SHA-256)

### 2. Homebrew (macOS & Linux)

**Tap**: `0xdps/packages`

```bash
# First-time setup
brew tap 0xdps/packages

# Install
brew install daemon-hound

# Upgrade
brew upgrade daemon-hound

# Uninstall
brew uninstall daemon-hound
```

**Repository**: `https://github.com/0xdps/homebrew-packages`

### 3. Scoop (Windows)

**Bucket**: `0xdps/scoop-bucket`

```powershell
# First-time setup
scoop bucket add daemon-hound https://github.com/0xdps/scoop-bucket

# Install
scoop install daemon-hound

# Upgrade
scoop update daemon-hound

# Uninstall
scoop uninstall daemon-hound
```

**Repository**: `https://github.com/0xdps/scoop-bucket`

### 4. Arch User Repository (AUR)

**Package**: `daemon-hound-bin`

```bash
# Using yay
yay -S daemon-hound-bin

# Using paru
paru -S daemon-hound-bin

# Manual
 git clone https://aur.archlinux.org/daemon-hound-bin.git
cd daemon-hound-bin
makepkg -si
```

### 5. Docker / GitHub Container Registry

**Registry**: `ghcr.io/0xdps/daemon-hound`

```bash
# Pull latest
docker pull ghcr.io/0xdps/daemon-hound:latest

# Pull specific version
docker pull ghcr.io/0xdps/daemon-hound:v0.1.0

# Run
docker run --rm -v ~/.dh:/home/dhuser/.dh -v $(pwd):/work ghcr.io/0xdps/daemon-hound:latest dh --help

# With shell alias
alias dh='docker run --rm -v ~/.dh:/home/dhuser/.dh -v $(pwd):/work ghcr.io/0xdps/daemon-hound:latest'
dh status
```

**Tags available**:
- `latest` — always points to the latest stable release
- `v{major}` — e.g. `v0` — latest in major version
- `v{major}.{minor}` — e.g. `v0.1` — latest in minor version
- `v{major}.{minor}.{patch}` — e.g. `v0.1.0` — exact version

### 6. Direct Download (Install Script)

```bash
# Default install to /usr/local/bin
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/main/install.sh | sh

# Custom install directory
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/main/install.sh | INSTALL_DIR=$HOME/.local/bin sh
```

The script auto-detects your OS and architecture, downloads the correct binary, and installs it.

### 7. Go Install

```bash
go install github.com/0xdps/daemon-hound/cmd/dh@latest
```

Requires Go 1.26+ installed.

### 8. Package Managers (Linux)

#### Debian / Ubuntu (.deb)

```bash
# Download from GitHub Releases
curl -LO https://github.com/0xdps/daemon-hound/releases/download/v0.1.0/daemon-hound_v0.1.0_Linux_x86_64.deb
sudo dpkg -i daemon-hound_v0.1.0_Linux_x86_64.deb
```

#### Fedora / RHEL / openSUSE (.rpm)

```bash
# Download from GitHub Releases
curl -LO https://github.com/0xdps/daemon-hound/releases/download/v0.1.0/daemon-hound_v0.1.0_Linux_x86_64.rpm
sudo rpm -i daemon-hound_v0.1.0_Linux_x86_64.rpm
```

#### Alpine Linux (.apk)

```bash
# Download from GitHub Releases
curl -LO https://github.com/0xdps/daemon-hound/releases/download/v0.1.0/daemon-hound_v0.1.0_Linux_x86_64.apk
sudo apk add --allow-untrusted daemon-hound_v0.1.0_Linux_x86_64.apk
```

---

## Installation Methods

### Quick Start by Platform

#### macOS (Apple Silicon / Intel)

```bash
# Homebrew (recommended)
brew tap 0xdps/packages
brew install daemon-hound

# Or install script
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/main/install.sh | sh
```

#### Linux

```bash
# Homebrew (works on Linux too)
brew tap 0xdps/packages
brew install daemon-hound

# Or install script
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/main/install.sh | sh

# Or download .deb/.rpm/.apk from releases
```

#### Windows

```powershell
# Scoop (recommended)
scoop bucket add daemon-hound https://github.com/0xdps/scoop-bucket
scoop install daemon-hound

# Or download .zip from releases and extract to PATH
```

#### FreeBSD

```bash
# Download from releases
curl -LO https://github.com/0xdps/daemon-hound/releases/download/v0.1.0/daemon-hound_v0.1.0_FreeBSD_x86_64.tar.gz
tar -xzf daemon-hound_v0.1.0_FreeBSD_x86_64.tar.gz
sudo mv dh /usr/local/bin/
```

---

## Release Process

### Prerequisites

1. **GoReleaser** installed locally (for testing):
   ```bash
   brew install goreleaser
   ```

2. **Required repositories** (see [Repository Requirements](#repository-requirements))

3. **GitHub secrets** configured:
   - `GITHUB_TOKEN` — auto-provided by GitHub Actions
   - `AUR_KEY` — SSH private key for AUR (optional)

### Creating a Release

#### Method 1: Push a Git Tag (Recommended)

```bash
# Ensure you're on trunk and everything is clean
git checkout trunk
git pull origin trunk

# Run tests
make test

# Tag the release
git tag v0.1.0

# Push the tag — this triggers the release workflow
git push origin v0.1.0
```

The GitHub Actions workflow will:
1. Build all binaries
2. Create a GitHub Release with changelog
3. Publish to Homebrew tap
4. Publish to Scoop bucket
5. Publish to AUR
6. Build and push Docker images
7. Attach `.deb`, `.rpm`, `.apk`, and Arch packages

#### Method 2: Local Snapshot (Testing)

```bash
# Build without publishing (for testing)
make snapshot

# Output will be in ./dist/
```

#### Method 3: Manual Release (Not Recommended)

```bash
# Only if you need to bypass CI
export GITHUB_TOKEN=ghp_xxx
make release
```

### Version Numbering

DaemonHound uses [Semantic Versioning](https://semver.org/):

- **MAJOR** — incompatible changes (breaking CLI changes, vault format changes)
- **MINOR** — new features, backwards compatible
- **PATCH** — bug fixes, backwards compatible

Examples:
- `v0.1.0` — first stable release
- `v0.2.0` — new features added
- `v0.2.1` — bug fix
- `v1.0.0` — stable API, breaking changes from v0.x

### Pre-releases

For beta/alpha releases, append a pre-release identifier:

```bash
git tag v0.2.0-beta.1
git push origin v0.2.0-beta.1
```

GoReleaser will mark these as pre-releases on GitHub.

---

## Platform Support Matrix

| Platform | amd64 | arm64 | armv7 | i386 | Package Managers |
|----------|:-----:|:-----:|:-----:|:----:|------------------|
| macOS 12+ | ✅ | ✅ | ❌ | ❌ | Homebrew |
| Linux | ✅ | ✅ | ❌ | ❌ | Homebrew, deb, rpm, apk, AUR |
| Windows 10+ | ✅ | ❌ | ❌ | ❌ | Scoop |
| FreeBSD 13+ | ✅ | ❌ | ❌ | ❌ | — |

**Minimum Requirements**:
- macOS 12 (Monterey) or later
- Linux kernel 3.10+ (glibc 2.17+)
- Windows 10 or later
- FreeBSD 13 or later

---

## Repository Requirements

The following repositories must exist for full distribution:

| Repository | Purpose | Visibility |
|------------|---------|------------|
| `0xdps/daemon-hound` | Main project | Public |
| `0xdps/homebrew-packages` | Homebrew tap | Public |
| `0xdps/scoop-bucket` | Scoop bucket | Public |

### Creating Required Repositories

```bash
# Homebrew tap
gh repo create 0xdps/homebrew-packages --public \
  --description "Homebrew tap for DaemonHound" \
  --homepage "https://github.com/0xdps/daemon-hound"

# Scoop bucket
gh repo create 0xdps/scoop-bucket --public \
  --description "Scoop bucket for DaemonHound" \
  --homepage "https://github.com/0xdps/daemon-hound"
```

### AUR Setup (Optional)

1. Create an AUR account: https://aur.archlinux.org/
2. Add your SSH public key to your AUR profile
3. Add your SSH private key as `AUR_KEY` in GitHub repository secrets

---

## Troubleshooting

### "command not found: dh" after installation

The binary directory is not in your PATH. Add it:

```bash
# For Homebrew (should be automatic)
export PATH="/opt/homebrew/bin:$PATH"  # Apple Silicon
export PATH="/usr/local/bin:$PATH"     # Intel

# For install.sh default
export PATH="/usr/local/bin:$PATH"

# For custom install dir
export PATH="$HOME/.local/bin:$PATH"
```

### "wrong password" when decrypting identity

The identity file was encrypted with a different password. If you've forgotten the password:
1. Delete `~/.dh/identity.age`
2. Delete `~/.dh/config.toml`
3. Run `dh init` again
4. Re-clone your vault

**Warning**: You will lose access to previously encrypted vault data unless you have the identity file backed up.

### Docker: "permission denied" when accessing files

The Docker image runs as a non-root user (`dhuser`). Ensure file permissions:

```bash
# Fix ownership of .dh directory
sudo chown -R $(id -u):$(id -g) ~/.dh

# Or run with current user
docker run --rm -u $(id -u):$(id -g) -v ~/.dh:/home/dhuser/.dh ...
```

### Homebrew: "Formula not found"

```bash
# Re-tap the repository
brew untap 0xdps/packages
brew tap 0xdps/packages
brew install daemon-hound
```

### Scoop: "bucket not found"

```powershell
# Re-add the bucket
scoop bucket rm daemon-hound
scoop bucket add daemon-hound https://github.com/0xdps/scoop-bucket
scoop install daemon-hound
```

### Release workflow failed

Check the GitHub Actions logs for the specific failure. Common issues:

1. **Missing repository**: Ensure `homebrew-packages` and `scoop-bucket` repos exist
2. **AUR key missing**: If AUR publish fails, either add `AUR_KEY` secret or remove the `aurs` section from `.goreleaser.yaml`
3. **Docker login failed**: Ensure `packages: write` permission is set in the workflow
4. **Tag already exists**: Delete and recreate the tag, or use a new version number

---

## Verification

After installation, verify DaemonHound is working:

```bash
# Check version
dh --version
# Expected: dh version v0.1.0 (commit: abc1234, built: 2026-06-12)

# Check help
dh --help

# Initialize (first time)
dh init --remote git@github.com:you/vault.git
```

---

## Security

### Checksum Verification

Always verify checksums when downloading manually:

```bash
# Download binary and checksums
curl -LO https://github.com/0xdps/daemon-hound/releases/download/v0.1.0/daemon-hound_v0.1.0_Darwin_arm64.tar.gz
curl -LO https://github.com/0xdps/daemon-hound/releases/download/v0.1.0/checksums.txt

# Verify
shasum -a 256 -c checksums.txt
# Expected: daemon-hound_v0.1.0_Darwin_arm64.tar.gz: OK
```

### GPG Signing (Future)

Future releases may include GPG-signed checksums. Enable by adding to `.goreleaser.yaml`:

```yaml
signs:
  - artifacts: checksum
    args: ["--batch", "-u", "${GPG_FINGERPRINT}", "--output", "${signature}", "--detach-sign", "${artifact}"]
```

---

## Contributing to Distribution

To add a new distribution channel:

1. Update `.goreleaser.yaml` with the new publisher configuration
2. Add installation instructions to this document
3. Update the release workflow if new secrets are needed
4. Test with `make snapshot` before merging

---

## See Also

- [CHANGELOG.md](./CHANGELOG.md) — Release history
- [CONTRIBUTING.md](./CONTRIBUTING.md) — Development guidelines
- [USERFLOW.md](./USERFLOW.md) — User workflows and commands
