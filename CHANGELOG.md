# Changelog

All notable changes to DaemonHound will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
DaemonHound uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## v1.0.2

### Added
- `dh cleanup` — completely remove DaemonHound from the local machine
  - Removes `~/.dh/` directory (vault, config, identity, audit log, lock file)
  - Removes master password from OS keychain
  - `--force` flag to skip confirmation prompt
  - Does NOT affect remote vault repository (safe to re-initialize later)
- Global identity salt with `prefix@postfix` format for multi-machine encryption consistency
  - New `identity_salt` field in `config.toml` (shared across all machines in vault)
  - Salt format: `base64(16-byte-prefix)@base64(16-byte-postfix)` = 32-byte salt
  - Ensures same password + same salt = same encryption key across all machines
  - Enables proper identity synchronization in multi-machine setups
  - Generated once during `dh init`, preserved during `dh rekey`

### Changed
- Identity encryption now requires global salt (removed legacy random-salt mode)
  - Smaller encrypted identity file (no embedded salt, saves 32 bytes)
  - All machines must share the same `identity_salt` from config

---

## v1.0.1

### Changed
- `dh doctor` now displays the configured git remote URL
  - Updated health checks to explicitly show git remote configuration
  - Improved clarity by separating "git remote configured" from "vault remote reachable" checks

### Fixed
- **CRITICAL**: Global files were being restored on all machines instead of only on the machine that tracked them
  - This caused files like `.zshrc` to be overwritten with stale versions from the vault during `dh sync`
  - Global files are now always machine-specific with a `MachineID` set during tracking
  - Pull operation now checks `MachineID` for global files before restoring them
- Docker build failures in GitHub Actions
  - Created multi-stage `Dockerfile` that builds from source using `golang:1.23-alpine`
  - Created `Dockerfile.goreleaser` for releases (uses pre-built binaries from GoReleaser)
  - Added `GOTOOLCHAIN=auto` to enable automatic Go version downloading for Go 1.26.4
- Code formatting issues detected by `gofmt` in CI/CD pipeline

---

## v1.0.0

### Added
- `dh init` — initialize vault, generate age identity, clone or create vault repo
  - Password confirmation (type twice) to prevent typo-induced key loss
  - `--force` flag to re-initialize an already-configured machine
  - Pre-fills vault remote URL when using `--force`
- `dh track <file>` — encrypt and track a file
  - `--mode sync` (default) for files shared across machines
  - `--mode backup` for machine-specific files
  - Idempotency check: reports "already tracked" if file is already tracked
  - Gitignore warning: warns if file is tracked in Git (to prevent committing secrets)
  - Immediate commit and push to vault after tracking
- `dh untrack <file>` — stop tracking a file; removes encrypted copy from vault
  - Commits and pushes the removal to the vault
- `dh sync` — pull then push tracked files
  - `--dry-run` flag to preview changes without modifying anything
  - `--namespace` flag to sync only a specific namespace
  - Offline resilience: continues with local commit if remote push fails
  - Pending push tracking: retries failed pushes on next successful sync
  - Colour-coded output for visual clarity
- `dh status` — show per-file status (clean / dirty / new / missing)
  - Mode annotations: shows `(backup, this machine only)` for backup files
  - Pending push notice: warns when local vault commits haven't been pushed
  - `--output json` flag for machine-readable output
  - `--namespace` flag to filter to a specific namespace
  - Colour-coded status indicators
- `dh discover [path]` — scan directory tree and sync all known namespaces
  - Shows correct vault file counts per namespace
  - `--output json` flag for machine-readable output
  - Colour-coded output for visual clarity
- `dh secret set <name>` — store an encrypted secret
  - Per-file rotation output showing which files were updated
- `dh secret get <name>` — retrieve the raw value of a secret
- `dh secret list [name]` — list all secrets or mappings for a specific secret
- `dh secret ref <name> <file> <KEY>` — map a secret to a file and environment variable
  - Commits and pushes the mapping to the vault
- `dh secret delete <name>` — delete a secret and all its mappings
- `dh secret unref <name> <file>` — remove a secret mapping from a file
- `dh secret rename <old> <new>` — rename a secret, preserving its value and all mappings
- `dh logout` — remove cached master password from OS keychain
  - Gracefully handles "already logged out" state
- `dh doctor` — comprehensive health check
  - Checks: config, identity file, vault directory, git repo, remote, bindings, pending push
  - `--fix` flag to auto-repair common issues (creates dirs, inits git, adds remote)
  - Colour-coded output (✓ green, ⚠ yellow, ✗ red)
- `dh machines` — list machine UUIDs that have backup files in the vault
  - Highlights current machine
  - `--output json` flag for machine-readable output
- `dh rekey` — change the master password
  - Prompts for current password, then new password (with confirmation)
  - Re-encrypts the age identity atomically
  - Vault contents remain unchanged
- `dh export` — decrypt all files and secrets to a local directory
  - `--dir` flag to specify output directory (default: `dh-export-<timestamp>`)
  - Exports tracked files to `files/<namespace>/<relPath>`
  - Exports secrets to `secrets.json`
  - Displays warning about plaintext exposure
- `dh version` — print version string
- Shell completions via `dh completion [bash|zsh|fish|powershell]` (built-in via cobra)
- **Security enhancements:**
  - File locking: PID-based lock at `~/.dh/sync.lock` prevents concurrent mutations
  - Audit log: append-only log at `~/.dh/audit.log` records all mutating commands
  - Encrypted vault state: `state.toml.age` encrypted at rest (migrates from legacy `state.toml`)
- **UX improvements:**
  - Colour output: ANSI colour for status, sync actions, and doctor output (respects `NO_COLOR`)
  - Global file paths: preserve subdirectory structure relative to `$HOME` (not just basename)

### Fixed
- `dh untrack` was committing locally but never pushing to remote
- `dh track` silently overwrote state when tracking the same file twice
- `dh secret set` rotation output now shows per-file detail instead of a bare count
- `dh secret ref` was saving the mapping but never committing/pushing the vault state
- `updateFileWithSecret` no longer constructs its own `config.Config`; receives it as a parameter
- `dh logout` gracefully handles "already logged out" instead of returning an error
- `dh discover` now shows correct vault file counts instead of sync-result counts
- `dh status` was discarding config; now checks `PendingPush` and shows mode annotations
- `StatusNew` is now returned by `tracker.Status()` when the stored checksum is empty

### Changed
- `dh init` now prompts to confirm the master password (type twice)
- `dh init --force` pre-fills the vault remote URL prompt with existing configured remote

---

## Release History

Previous releases (v0.1.0, v0.1.1) have been removed in preparation for a proper v1.0.0 release with the complete feature set documented above.
- `dh init` persists the vault remote URL in `~/.dh/config.toml`
- `dh sync` continues to push local dirty files even when the remote pull fails (offline mode); commits locally and sets a pending-push flag instead of aborting
- `dh sync` retries a pending push on the next successful `dh sync`
- `dh secret list` shows mapping count per secret when listing all secrets

### Architecture
- `models.MachineConfig` gains `PendingPush bool` field to track offline-committed-but-not-pushed state
- `config.Config` gains `VaultRemote()`, `PendingPush()`, and `SetPendingPush()` methods
- `sync.ConfigReader` interface extended with `PendingPush()` / `SetPendingPush()` for offline tracking
- `tracker.Tracker` gains `ResolveKey(localPath)` — resolves `(namespace, relPath)` without any writes; used by `track`, `untrack`, and idempotency checks to eliminate duplicated namespace-resolution logic
- `sync.Syncer` gains `DryRun()` method — computes what would be pushed/pulled without touching disk or network
