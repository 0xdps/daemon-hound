# Changelog

All notable changes to DaemonHound will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
DaemonHound uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## v1.1.0

### Added
- **Background Daemon Service** — automatic, always-running synchronization
  - `dhd daemon run` — run daemon in foreground (for testing/debugging)
  - `dhd daemon status` — check if daemon is installed and running
  - `dhd daemon logs` — view recent sync activity logs
    - `dhd daemon logs -f` to follow logs in real-time
    - `dhd daemon logs -n 100` to view last N lines
  - `dhd daemon errors` — view daemon error logs for troubleshooting
  - `dhd daemon stop` — stop the daemon service
  - `dhd daemon restart` — restart the daemon service
- **Automatic Daemon Installation** — installed during `dhd init` with no manual setup required
  - macOS: Registers with launchd (`~/Library/LaunchAgents/com.daemon-hound.plist`)
  - Linux: Registers with systemd (`~/.config/systemd/user/daemon-hound.service`)
  - Windows: Registers with Task Scheduler (`DaemonHound` task)
  - Auto-starts on system boot
  - Auto-restarts if crashed
- **File Watching** — detects local vault changes in real-time
  - Uses `fsnotify` for efficient OS-level file monitoring
  - 2-second debounce to batch rapid changes
  - Ignores `.git` directory and temporary files (ending with `~`)
  - Automatically commits and pushes changes to remote
- **Remote Polling** — checks for new commits from remote every 30 seconds
  - Fetches from remote vault without blocking other operations
  - Pulls new commits and applies them locally
  - Detects and resolves merge conflicts automatically
- **Automatic Conflict Resolution** — handles concurrent edits gracefully
  - Detects merge conflicts from concurrent changes on multiple machines
  - Resolves using "local" strategy (keeps local changes as default)
  - Automatically commits conflict resolution with message `[daemon] Resolve merge conflicts`
  - Logs conflicted files for audit trail
  - Extensible to support "remote", "ask", and "merge" strategies in future releases
- **Log Rotation & Cleanup** — keeps log files manageable
  - Hourly log rotation checks
  - Rotates `~/.dh/daemon.log` when exceeding 10MB
  - Rotates `~/.dh/daemon.error.log` when exceeding 5MB
  - Rotated files named `daemon.YYYY-MM-DD-HH-MM-SS.log`
  - Automatically removes logs older than 30 days
  - Configurable size and retention limits (via config in v1.2.0+)
- **Comprehensive Logging** — detailed audit trail of all operations
  - Sync logs with timestamps: "Pulled from remote", "Detected conflicts", etc.
  - Error logs for troubleshooting: watcher errors, permission issues, etc.
  - Both logs viewable via `dhd daemon logs` and `dhd daemon errors` commands

### Changed
- `dhd init` now automatically installs the daemon service
  - No separate `dhd daemon install` command needed
  - Daemon starts on next system boot automatically
  - Manual `dhd sync` is now optional (daemon syncs continuously)
- Service registration abstracted to factory pattern
  - Single `daemon.ServiceManager` interface handles all OS-specific details
  - Easy to add support for additional service managers in future

### Security
- Daemon service runs with same user permissions as `dhd` CLI (no escalation)
- Log files stored in `~/.dh/` (user-private directory, mode 0700)
- Global salt remains plaintext in config (correct design for multi-machine consistency)

### Performance
- **Local change latency**: ~2-4 seconds (file detection + debounce + commit + push)
- **Remote change latency**: ~30 seconds (polling interval) + ~2 seconds (pull + apply)
- **Resource usage**: ~20-30MB memory, minimal CPU when idle, active only during sync operations

### Known Limitations
- Conflict strategy fixed to "local" in v1.1.0 (configurable in v1.2.0+)
- Daemon syncs entire vault (partial sync by namespace in v1.2.0+)
- No real-time collaboration conflict detection (30-second polling is best-effort)
- Conflict resolution runs silently; no user prompts yet (planned for v1.2.0+)

---

## v1.0.2

### Added
- `dhd cleanup` — completely remove DaemonHound from the local machine
  - Removes `~/.dh/` directory (vault, config, identity, audit log, lock file)
  - Removes master password from OS keychain
  - `--force` flag to skip confirmation prompt
  - Does NOT affect remote vault repository (safe to re-initialize later)
- Global identity salt with `prefix@postfix` format for multi-machine encryption consistency
  - New `identity_salt` field in `config.toml` (shared across all machines in vault)
  - Salt format: `base64(16-byte-prefix)@base64(16-byte-postfix)` = 32-byte salt
  - Ensures same password + same salt = same encryption key across all machines
  - Enables proper identity synchronization in multi-machine setups
  - Generated once during `dhd init`, preserved during `dhd rekey`

### Changed
- Identity encryption now requires global salt (removed legacy random-salt mode)
  - Smaller encrypted identity file (no embedded salt, saves 32 bytes)
  - All machines must share the same `identity_salt` from config

---

## v1.0.1

### Changed
- `dhd doctor` now displays the configured git remote URL
  - Updated health checks to explicitly show git remote configuration
  - Improved clarity by separating "git remote configured" from "vault remote reachable" checks

### Fixed
- **CRITICAL**: Global files were being restored on all machines instead of only on the machine that tracked them
  - This caused files like `.zshrc` to be overwritten with stale versions from the vault during `dhd sync`
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
- `dhd init` — initialize vault, generate age identity, clone or create vault repo
  - Password confirmation (type twice) to prevent typo-induced key loss
  - `--force` flag to re-initialize an already-configured machine
  - Pre-fills vault remote URL when using `--force`
- `dhd track <file>` — encrypt and track a file
  - `--mode sync` (default) for files shared across machines
  - `--mode backup` for machine-specific files
  - Idempotency check: reports "already tracked" if file is already tracked
  - Gitignore warning: warns if file is tracked in Git (to prevent committing secrets)
  - Immediate commit and push to vault after tracking
- `dhd untrack <file>` — stop tracking a file; removes encrypted copy from vault
  - Commits and pushes the removal to the vault
- `dhd sync` — pull then push tracked files
  - `--dry-run` flag to preview changes without modifying anything
  - `--namespace` flag to sync only a specific namespace
  - Offline resilience: continues with local commit if remote push fails
  - Pending push tracking: retries failed pushes on next successful sync
  - Colour-coded output for visual clarity
- `dhd status` — show per-file status (clean / dirty / new / missing)
  - Mode annotations: shows `(backup, this machine only)` for backup files
  - Pending push notice: warns when local vault commits haven't been pushed
  - `--output json` flag for machine-readable output
  - `--namespace` flag to filter to a specific namespace
  - Colour-coded status indicators
- `dhd discover [path]` — scan directory tree and sync all known namespaces
  - Shows correct vault file counts per namespace
  - `--output json` flag for machine-readable output
  - Colour-coded output for visual clarity
- `dhd secret set <name>` — store an encrypted secret
  - Per-file rotation output showing which files were updated
- `dhd secret get <name>` — retrieve the raw value of a secret
- `dhd secret list [name]` — list all secrets or mappings for a specific secret
- `dhd secret ref <name> <file> <KEY>` — map a secret to a file and environment variable
  - Commits and pushes the mapping to the vault
- `dhd secret delete <name>` — delete a secret and all its mappings
- `dhd secret unref <name> <file>` — remove a secret mapping from a file
- `dhd secret rename <old> <new>` — rename a secret, preserving its value and all mappings
- `dhd logout` — remove cached master password from OS keychain
  - Gracefully handles "already logged out" state
- `dhd doctor` — comprehensive health check
  - Checks: config, identity file, vault directory, git repo, remote, bindings, pending push
  - `--fix` flag to auto-repair common issues (creates dirs, inits git, adds remote)
  - Colour-coded output (✓ green, ⚠ yellow, ✗ red)
- `dhd machines` — list machine UUIDs that have backup files in the vault
  - Highlights current machine
  - `--output json` flag for machine-readable output
- `dhd rekey` — change the master password
  - Prompts for current password, then new password (with confirmation)
  - Re-encrypts the age identity atomically
  - Vault contents remain unchanged
- `dhd export` — decrypt all files and secrets to a local directory
  - `--dir` flag to specify output directory (default: `dhd-export-<timestamp>`)
  - Exports tracked files to `files/<namespace>/<relPath>`
  - Exports secrets to `secrets.json`
  - Displays warning about plaintext exposure
- `dhd version` — print version string
- Shell completions via `dhd completion [bash|zsh|fish|powershell]` (built-in via cobra)
- **Security enhancements:**
  - File locking: PID-based lock at `~/.dh/sync.lock` prevents concurrent mutations
  - Audit log: append-only log at `~/.dh/audit.log` records all mutating commands
  - Encrypted vault state: `state.toml.age` encrypted at rest (migrates from legacy `state.toml`)
- **UX improvements:**
  - Colour output: ANSI colour for status, sync actions, and doctor output (respects `NO_COLOR`)
  - Global file paths: preserve subdirectory structure relative to `$HOME` (not just basename)

### Fixed
- `dhd untrack` was committing locally but never pushing to remote
- `dhd track` silently overwrote state when tracking the same file twice
- `dhd secret set` rotation output now shows per-file detail instead of a bare count
- `dhd secret ref` was saving the mapping but never committing/pushing the vault state
- `updateFileWithSecret` no longer constructs its own `config.Config`; receives it as a parameter
- `dhd logout` gracefully handles "already logged out" instead of returning an error
- `dhd discover` now shows correct vault file counts instead of sync-result counts
- `dhd status` was discarding config; now checks `PendingPush` and shows mode annotations
- `StatusNew` is now returned by `tracker.Status()` when the stored checksum is empty

### Changed
- `dhd init` now prompts to confirm the master password (type twice)
- `dhd init --force` pre-fills the vault remote URL prompt with existing configured remote

---

## Release History

Previous releases (v0.1.0, v0.1.1) have been removed in preparation for a proper v1.0.0 release with the complete feature set documented above.
- `dhd init` persists the vault remote URL in `~/.dh/config.toml`
- `dhd sync` continues to push local dirty files even when the remote pull fails (offline mode); commits locally and sets a pending-push flag instead of aborting
- `dhd sync` retries a pending push on the next successful `dhd sync`
- `dhd secret list` shows mapping count per secret when listing all secrets

### Architecture
- `models.MachineConfig` gains `PendingPush bool` field to track offline-committed-but-not-pushed state
- `config.Config` gains `VaultRemote()`, `PendingPush()`, and `SetPendingPush()` methods
- `sync.ConfigReader` interface extended with `PendingPush()` / `SetPendingPush()` for offline tracking
- `tracker.Tracker` gains `ResolveKey(localPath)` — resolves `(namespace, relPath)` without any writes; used by `track`, `untrack`, and idempotency checks to eliminate duplicated namespace-resolution logic
- `sync.Syncer` gains `DryRun()` method — computes what would be pushed/pulled without touching disk or network
