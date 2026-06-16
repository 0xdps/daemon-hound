# GitHub Actions Setup Checklist

## Current Status ✅

Your repository **already has** GitHub Actions workflows configured:

```
✅ .github/workflows/build.yml   - Runs on every PR/push
✅ .github/workflows/release.yml - Runs on git tags
✅ .goreleaser.yaml              - Distribution config
```

---

## What's Already Working

### Build Workflow (.github/workflows/build.yml)

**Status**: ✅ Ready to use

- Runs on every push to `trunk`/`main`
- Runs on every pull request
- Tests all code with `go test -v -race`
- Builds binaries for 6 platforms (darwin/linux/windows × amd64/arm64)
- Builds Docker images
- Uploads artifacts for 7 days

**Triggers**:
```yaml
on:
  push:
    branches: [trunk, main]
  pull_request:
    branches: [trunk, main]
```

**Latest Status**: Run any time by pushing to trunk or creating a PR

---

### Release Workflow (.github/workflows/release.yml)

**Status**: ⚠️ Partially ready (needs one secret)

- Runs **only** when you push a git tag matching `v*`
- Uses goreleaser to build all platforms
- Creates GitHub Release
- Publishes Docker images
- Updates package managers (Homebrew, Scoop)

**Triggers**:
```yaml
on:
  push:
    tags:
      - "v*"  # v1.1.0, v2.0.0, etc.
```

---

## What Needs to Be Done

### ✅ Already Configured

1. **GitHub Workflows**: Exist and are correct
2. **Goreleaser Config**: `.goreleaser.yaml` is complete
3. **Go Version**: go.mod specifies v1.26.4
4. **Permissions**: Set correctly in workflows
5. **GITHUB_TOKEN**: Automatic (no setup needed)

### ⚠️ Optional Configuration (for full automation)

#### 1. Personal Access Token (GH_PAT)

**Needed for**: Updating external Homebrew/Scoop repositories

**What to do**:
```
1. Go to: GitHub.com → Settings → Developer settings → Personal access tokens
2. Click: Tokens (classic)
3. Generate new token
4. Name: GH_PAT_DAEMON_HOUND
5. Expiration: No expiration (or 1 year, renew before expiring)
6. Select scopes:
   ✓ repo (full control of private repositories)
   ✓ workflow (update workflow files)
7. Click: Generate token
8. Copy the token (you won't see it again)
```

**Store in Repository**:
```
1. Go to: GitHub.com → 0xdps/daemon-hound → Settings → Secrets and variables → Actions
2. Click: New repository secret
3. Name: GH_PAT
4. Value: [paste your token]
5. Click: Add secret
```

**Without GH_PAT**:
- GitHub Release still created ✅
- Docker images still pushed ✅
- Homebrew/Scoop **not** updated ❌
- Users must download manually or wait for manual upload

**With GH_PAT**:
- Everything automated ✅
- `brew install daemon-hound` works immediately ✅
- `scoop install daemon-hound` works immediately ✅

---

## Test Before Real Release

### 1. Test Build Workflow
```bash
# Push any change to trunk/main
git add -A
git commit -m "test: verify build workflow"
git push origin trunk

# Go to: GitHub.com → Actions
# Verify build.yml runs successfully
```

### 2. Test Release Build Locally
```bash
# Build all platforms locally (doesn't push)
make snapshot

# Check dist/ folder for artifacts
ls -lh dist/
```

### 3. Test Release Workflow (Full)
```bash
# Create test tag
make tag TAG=v1.0.0-rc1

# Go to: GitHub.com → Actions
# Watch release.yml run (should complete in 5-10 minutes)

# After completion, check:
# - GitHub Release created
# - Docker images published
# - (If GH_PAT set: Homebrew/Scoop updated)
```

---

## Complete Workflow at a Glance

### Developer Perspective

```
┌─────────────────────────────────────────┐
│ You make changes                        │
│ git push origin trunk                   │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│ GitHub Actions triggers build.yml       │
│ ✓ Tests                                 │
│ ✓ Lints                                 │
│ ✓ Builds 6 platforms                    │
│ ✓ Builds Docker                         │
│ Duration: ~5 minutes                    │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│ You create release tag                  │
│ make tag TAG=v1.1.0                     │
│ (or git push origin v1.1.0)             │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│ GitHub Actions triggers release.yml     │
│ ✓ Builds final artifacts                │
│ ✓ Creates GitHub Release                │
│ ✓ Publishes Docker images               │
│ ✓ Updates Homebrew (if GH_PAT set)      │
│ ✓ Updates Scoop (if GH_PAT set)         │
│ Duration: ~8 minutes                    │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│ Users can now install from:             │
│ • GitHub Release (direct download)      │
│ • brew install daemon-hound (macOS)     │
│ • scoop install daemon-hound (Windows)  │
│ • Package managers (Linux)              │
│ • docker pull ghcr.io/...               │
└─────────────────────────────────────────┘
```

---

## Release Checklist

### Before Tagging

```bash
☐ Update CHANGELOG.md with new version
☐ Update version in README.md if needed
☐ Verify all tests pass: go test ./...
☐ Verify no linting issues: golangci-lint run
☐ Commit changes: git commit -m "chore: release v1.1.0"
☐ Push to trunk: git push origin trunk
```

### Create Release Tag

```bash
☐ Create tag: make tag TAG=v1.1.0
  (This runs: git tag -a v1.1.0 -m "Release v1.1.0" && git push origin v1.1.0)
```

### Monitor Release

```bash
☐ Watch GitHub Actions: https://github.com/0xdps/daemon-hound/actions
☐ Verify release.yml completes (5-10 minutes)
☐ Check GitHub Release page created
☐ Verify all artifacts present
```

### Post-Release

```bash
☐ Test macOS: brew install daemon-hound (if GH_PAT configured)
☐ Test Linux: download .deb or .rpm
☐ Test Windows: scoop install daemon-hound (if GH_PAT configured)
☐ Test Docker: docker pull ghcr.io/0xdps/daemon-hound:v1.1.0
☐ Announce release
```

---

## Key Files Reference

| File | Purpose | Status |
|------|---------|--------|
| `.github/workflows/build.yml` | Test + build on push | ✅ Ready |
| `.github/workflows/release.yml` | Release on tag | ✅ Ready |
| `.goreleaser.yaml` | Build + packaging config | ✅ Ready |
| `Makefile` | Local build targets | ✅ Ready |
| `Repository Secret: GH_PAT` | Homebrew/Scoop updates | ⚠️ Optional |

---

## Troubleshooting

### Build workflow fails

**Check**: 
```bash
# Locally verify
go test ./...
go vet ./...
go fmt ./... # Format all files
golangci-lint run
```

**Common causes**:
- Tests failing
- Format issues
- Linting errors

### Release workflow fails

**Check GitHub Actions logs**:
1. Go to: `https://github.com/0xdps/daemon-hound/actions`
2. Find failing workflow
3. Click to expand failed step
4. Read error message

**Common causes**:
- Invalid `.goreleaser.yaml` syntax
- Missing GH_PAT for Homebrew/Scoop (non-fatal, others still work)
- Docker build issue

**Fix**:
```bash
# Test locally
goreleaser check
make snapshot
```

### Docker images not pushed

**Check**: 
```bash
# Verify login in workflow
echo $GITHUB_TOKEN | docker login ghcr.io -u ${{ github.actor }} --password-stdin
```

**Likely**: Already working, images at `ghcr.io/0xdps/daemon-hound`

### Homebrew/Scoop not updating

**Check**: Is GH_PAT secret configured?
```bash
# Go to: GitHub Settings → Secrets and variables
# Verify GH_PAT exists
```

**Without GH_PAT**: Manual workaround
```bash
# Push formula manually
git clone https://github.com/0xdps/homebrew-packages
# Update Formula/daemon-hound.rb
# Push and create PR
```

---

## Next: Your First Release

Once you're ready to release v1.1.0:

```bash
# 1. Prepare
git checkout staging
git merge trunk  # If needed
make test        # Ensure all pass

# 2. Update changelog
# Edit CHANGELOG.md - add v1.1.0 section

# 3. Commit and push
git add CHANGELOG.md
git commit -m "chore: release v1.1.0"
git push origin staging

# 4. Tag release
make tag TAG=v1.1.0

# 5. Watch GitHub Actions
# Go to: GitHub.com → Actions → Release workflow

# 6. Celebrate! 🎉
# Release is automatic from this point
```

That's it! Everything else is automated by GitHub Actions + Goreleaser.
