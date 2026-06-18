# Workflow Files Deep Dive

## Your Existing Workflows

### File 1: `.github/workflows/build.yml`

```yaml
name: Build

on:
  push:
    branches: [trunk, main]    # Runs on pushes to trunk/main
  pull_request:
    branches: [trunk, main]    # Runs on pull requests to trunk/main
```

**When it triggers**:
- Any push to `trunk` or `main` branch
- Any PR targeting `trunk` or `main`

---

#### Job 1: Test

```yaml
test:
  name: Test
  runs-on: ubuntu-latest       # Run on Ubuntu 22.04

  steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version-file: go.mod  # Read version from go.mod

    - name: Run tests
      run: go test -v -race ./...
      # -v: verbose
      # -race: detect data races

    - name: Run vet
      run: go vet ./...
      # Static analysis (catch common mistakes)

    - name: Check formatting
      run: |
        fmt=$(gofmt -l .)
        if [ -n "$fmt" ]; then
          echo "Unformatted files:"
          echo "$fmt"
          exit 1
        fi
```

**What happens**:
1. ✓ All tests pass?
2. ✓ No race conditions?
3. ✓ Code is properly formatted?

**If fails**: PR blocked until fixed

---

#### Job 2: Build Matrix

```yaml
build:
  name: Build (${{ matrix.os }}/${{ matrix.arch }})
  runs-on: ubuntu-latest
  strategy:
    fail-fast: false           # Don't stop if one fails
    matrix:
      include:
        - os: darwin            # macOS
          arch: amd64
        - os: darwin
          arch: arm64           # M1/M2 Macs
        - os: linux
          arch: amd64
        - os: linux
          arch: arm64           # ARM Servers (Raspberry Pi)
        - os: windows
          arch: amd64
        - os: freebsd
          arch: amd64
```

**How matrix works**:
- Creates 6 parallel jobs (one per OS/arch combo)
- All run simultaneously
- If one fails, others continue (`fail-fast: false`)

**Build step**:
```yaml
- name: Build binary
  env:
    GOOS: ${{ matrix.os }}      # macOS, Linux, etc.
    GOARCH: ${{ matrix.arch }}  # amd64, arm64, etc.
    CGO_ENABLED: 0              # No C dependencies
  run: |
    VERSION=${{ github.ref_name }}
    COMMIT=${{ github.sha }}
    DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    go build \
      -ldflags "-s -w \
        -X github.com/0xdps/daemon-hound/internal/cmd.version=${VERSION} \
        -X github.com/0xdps/daemon-hound/internal/cmd.commit=${COMMIT:0:7} \
        -X github.com/0xdps/daemon-hound/internal/cmd.date=${DATE}" \
      -o dist/dhd-${{ matrix.os }}-${{ matrix.arch }}${{ matrix.os == 'windows' && '.exe' || '' }} \
      ./cmd/dhd
```

**ldflags explained**:
```
-s -w                          # Strip symbols & debug info (smaller binary)
-X version=...                 # Embed version string
-X commit=...                  # Embed git commit hash
-X date=...                    # Embed build date/time
```

**Result**: Binaries like:
```
dist/dhd-darwin-amd64
dist/dhd-darwin-arm64
dist/dhd-linux-amd64
dist/dhd-linux-arm64
dist/dhd-windows-amd64.exe
dist/dhd-freebsd-amd64
```

**Upload artifacts**:
```yaml
- name: Upload artifact
  uses: actions/upload-artifact@v4
  with:
    name: dhd-${{ matrix.os }}-${{ matrix.arch }}
    path: dist/dhd-${{ matrix.os }}-${{ matrix.arch }}${{ matrix.os == 'windows' && '.exe' || '' }}
    retention-days: 7          # Keep for 7 days
```

Available for download from GitHub Actions run details

---

#### Job 3: Docker Build

```yaml
docker:
  name: Docker Build
  runs-on: ubuntu-latest

  steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up QEMU
      uses: docker/setup-qemu-action@v3
      # Enables building for multiple architectures

    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v3
      # Modern Docker build system with multi-arch support

    - name: Build Docker image
      uses: docker/build-push-action@v5
      with:
        context: .
        file: ./Dockerfile
        push: false               # Don't push (it's a test build)
        tags: daemon-hound:test
```

**What it does**:
- Builds Docker image from `./Dockerfile`
- Tests that it builds without errors
- Doesn't push anywhere (that happens in release.yml)

---

### File 2: `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - "v*"  # Matches v1.0.0, v1.1.0, v2.0.0, etc.
```

**When it triggers**: Only on git tags starting with `v`

---

#### Permissions

```yaml
permissions:
  contents: write      # Can create releases, push tags
  packages: write      # Can push to GitHub Container Registry
```

**Why needed**:
- `contents: write` → Create GitHub Release
- `packages: write` → Push Docker images to ghcr.io

---

#### Setup Steps

```yaml
jobs:
  release:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0    # Full git history (needed for version detection)

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}    # Your GitHub username
          password: ${{ secrets.GITHUB_TOKEN }}
```

**What each does**:
1. Checkout → Clone repo
2. Go setup → Install Go compiler
3. QEMU setup → Enable multi-arch Docker builds
4. Docker Buildx → Modern multi-platform builder
5. Docker login → Authenticate to push images

---

#### Run Goreleaser (The Magic)

```yaml
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"          # Use goreleaser v2.x
          args: release --clean     # Build + release
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}  # Built-in secret
          GH_PAT: ${{ secrets.GH_PAT }}              # Your token (if set)
```

**What goreleaser does**:
1. Reads `.goreleaser.yaml`
2. Builds all binaries for all platforms
3. Creates archives (.tar.gz, .zip, .dmg)
4. Creates Linux packages (.deb, .rpm, .apk)
5. Creates GitHub Release
6. Pushes to Homebrew (if GH_PAT set)
7. Pushes to Scoop (if GH_PAT set)
8. Builds + pushes Docker images

---

## What's Already Configured ✅

### Build Workflow
```
✅ Triggers on push/PR
✅ Tests all code
✅ Builds 6 platform binaries
✅ Builds Docker image (test)
✅ Uploads artifacts
```

### Release Workflow
```
✅ Triggers on git tag (v*)
✅ Checkouts full history
✅ Sets up Go
✅ Sets up Docker
✅ Authenticates to GitHub Container Registry
✅ Runs goreleaser
```

---

## What Might Need Configuration

### 1. GH_PAT Secret (Optional)

**Currently**: Not set (Homebrew/Scoop updates will fail silently)

**To fix**:
1. Generate Personal Access Token on GitHub.com
2. Add to repository secrets as `GH_PAT`
3. Next release will update Homebrew/Scoop

**Without it**: GitHub Release still works ✓

---

### 2. Goreleaser Configuration

**Already configured** in `.goreleaser.yaml`:
```yaml
builds:
  goos: [darwin, linux, windows, freebsd]
  goarch: [amd64, arm64]

archives:
  formats: [tar.gz, zip, deb, rpm, apk]

brews:
  repository: 0xdps/homebrew-packages

scoops:
  repository: 0xdps/scoop-bucket

dockers:
  image_templates:
    - ghcr.io/0xdps/daemon-hound:{{ .Tag }}
    - ghcr.io/0xdps/daemon-hound:latest
```

**Status**: ✅ Complete

---

## Example Workflow Execution

### When you push to trunk:

```
16:00 - git push origin trunk
16:00 - GitHub: build.yml triggers
        ├─ test job starts
        ├─ build job matrix starts (6 parallel)
        └─ docker job starts
16:03 - All jobs complete
        ├─ ✓ Tests passed
        ├─ ✓ 6 binaries compiled
        ├─ ✓ Docker image built
        └─ Artifacts available for download
16:04 - You can merge PRs now
```

### When you push a tag:

```
16:30 - make tag TAG=v1.1.0
16:30 - git push origin v1.1.0
16:31 - GitHub: release.yml triggers
16:31 - Goreleaser setup (1 min)
16:32 - Build phase (30 sec)
        ├─ darwin/amd64 binary
        ├─ darwin/arm64 binary
        ├─ linux/amd64 binary
        ├─ linux/arm64 binary
        ├─ windows/amd64 binary
        └─ freebsd/amd64 binary
16:32 - Package phase (3 min)
        ├─ Create .dmg
        ├─ Create .deb/.rpm/.apk
        ├─ Create .tar.gz/.zip
        └─ Create Docker images
16:35 - Distribution phase (5 min)
        ├─ Create GitHub Release
        ├─ Upload artifacts
        ├─ Generate checksums
        ├─ Push to Homebrew (if GH_PAT set)
        ├─ Push to Scoop (if GH_PAT set)
        └─ Push Docker images
16:40 - ✅ COMPLETE
        └─ Users can install from anywhere
```

---

## How to Trigger Workflows Manually

### Trigger Build Workflow
```bash
git add -A
git commit -m "test: trigger build"
git push origin trunk
# Watch: https://github.com/0xdps/daemon-hound/actions
```

### Trigger Release Workflow
```bash
make tag TAG=v1.1.0
# Watch: https://github.com/0xdps/daemon-hound/actions/workflows/release.yml
```

---

## Monitoring Workflows

### Check Status
```
Go to: https://github.com/0xdps/daemon-hound/actions
```

### View Details
- Click the workflow run
- See step-by-step execution
- View logs for each step
- Download artifacts

### If Something Fails
- Click the failed step
- Read the error message
- Common issues:
  - Tests failing → Fix code
  - Format issues → Run `go fmt ./...`
  - Goreleaser error → Check `.goreleaser.yaml`

---

## Summary

| Component | Status | Triggers | Action |
|-----------|--------|----------|--------|
| build.yml | ✅ Ready | Push/PR | Test + compile |
| release.yml | ✅ Ready | Git tag | Full release |
| Secrets (auto) | ✅ Ready | N/A | GitHub auth |
| Secrets (GH_PAT) | ⚠️ Optional | N/A | Homebrew/Scoop |

**You're ready to release!** Just run:
```bash
make tag TAG=v1.1.0
```

Everything else is automatic.
