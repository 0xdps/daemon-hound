# Changelog

All notable changes to DaemonHound will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
DaemonHound uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## v1.1.0

### Added

#### Web UI
- **Browser-based dashboard** — full-featured web interface for vault management
  - `dhd ui` — starts the web server and opens the browser
  - Login protected by master password with 12-hour HMAC session
  - **Dashboard** — vault health overview with conflict/secret/file counts
  - **Conflict Resolver** — 3-way diff view (base | local | remote) with smart key-value parsing for `.env` files
  - **Tracked Files Browser** — namespace-grouped file list with search, content preview, and syntax highlighting
  - **Secrets Manager** — version-controlled secret viewer with rotation, history, and diff comparison
  - **Live Status** — real-time SSE log streaming with daemon health monitoring and log level filtering
  - **Settings** — bulk untrack operations with sortable table
  - **Help** — environment info, clipboard debug dump, and CLI reference
  - Dark theme with green accent, Lucide icons, HTMX for SPA-like navigation

#### Smart Merge
- **Semantic merge drivers** for encrypted vault files — resolves conflicts at the content level
  - `.env` files — key-value aware merge (adds, removes, conflicts by key)
  - JSON files — structural merge with conflict markers
  - CSV files — row-level merge
  - TOML/plaintext — line-based merge
  - Secret files — version-aware merge for per-secret TOML files
- **Git integration** — `dhd git setup` configures merge driver, diff driver, clean/smudge filters
- **Git hooks** — `dhd git hook` installs pre-commit, post-merge, post-checkout, pre-push hooks
- **Daemon auto-recovery** — daemon attempts smart merge on stuck in-progress merges before falling back to remote

#### Lightweight Vault Access
- **`dhd clone`** — clone a vault without full machine initialization (no config, no daemon, no keychain)
  - Partial clone support: `--namespace` and `--secret` sparse checkout
- **`dhd read`** — decrypt and read tracked files or secrets from any cloned vault

#### Background Daemon
- **Automatic sync** — `dhd daemon run` for foreground, `dhd daemon start` for background
  - 30-second polling: fetches remote, restores files, pushes local changes
  - Halt-on-conflict: pauses sync when true conflicts are detected, resumes via `dhd daemon resume`
- **Daemon management** — `status`, `stop`, `restart`, `logs`, `errors` subcommands
- **Automatic installation** — daemon registered during `dhd init`
  - macOS: launchd (`~/Library/LaunchAgents/com.daemon-hound.plist`)
  - Linux: systemd (`~/.config/systemd/user/daemon-hound.service`)
  - Windows: Task Scheduler
- **Log rotation** — hourly checks; rotates at 10MB (daemon.log) / 5MB (daemon.error.log); 30-day retention

#### Expanded Secret Management
- **11 secret subcommands** — `set`, `get`, `list`, `rotate`, `delete`, `rename`, `history`, `rollback`, `ref`, `unref`, `diff`
- **Per-secret files** — secrets stored as individual encrypted TOML files (migrated from legacy inline format)
- **Version history** — full audit trail with timestamps, reasons, and version comparison

#### macOS Release
- **Signed & notarized DMG** — Apple Developer ID signing, notarization via notarytool
- **App bundle** — `.app` bundle in `~/Applications` for launchd
- **GPG-signed checksums** — release artifacts verified with GPG
- **APT repository** — signed with GPG keyring for Debian/Ubuntu

#### Other
- `dhd doctor` — expanded to verify git merge driver, diff driver, filters, hooks, .gitattributes, .gitignore, rerere
- `dhd discover` — scan directories for git repos and auto-track known namespaces

### Changed
- `dhd init` now automatically installs the daemon service
- `dhd export` — now exports both files and secrets in a mirror directory structure
- `dhd cleanup` — now removes legacy launch agents and macOS app bundle
- Service registration uses factory pattern (`daemon.ServiceManager` interface)

### Security
- Daemon runs with same user permissions as CLI (no escalation)
- Log files stored in `~/.daemon-hound/` (user-private, mode 0700)
- HMAC-signed session tokens for web UI (per-process random key, 12-hour TTL)

### Performance
- **Sync latency**: ~30 seconds (polling interval) + ~2 seconds (pull + apply + push)
- **Resource usage**: ~20-30MB memory, minimal CPU when idle

### Known Limitations
- No file-watching (polling-only at 30-second intervals)
- Conflict strategy is always smart-merge where possible; true conflicts halt the daemon
- Full vault sync only (partial sync by namespace planned for v1.2.0)
- Conflict resolution runs automatically without user prompts for non-conflicting merges
- Web UI uses CDN for Tailwind, Lucide, Highlight.js (offline-first planned for v1.2.0)

---

## v1.0.2

### Added
- `dhd cleanup` — completely remove DaemonHound from the local machine
  - Removes `~/.daemon-hound/` directory (vault, config, identity, audit log, lock file)
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
  - File locking: PID-based lock at `~/.daemon-hound/sync.lock` prevents concurrent mutations
  - Audit log: append-only log at `~/.daemon-hound/audit.log` records all mutating commands
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
- `dhd init` persists the vault remote URL in `~/.daemon-hound/config.toml`
- `dhd sync` continues to push local dirty files even when the remote pull fails (offline mode); commits locally and sets a pending-push flag instead of aborting
- `dhd sync` retries a pending push on the next successful `dhd sync`
- `dhd secret list` shows mapping count per secret when listing all secrets

### Architecture
- `models.MachineConfig` gains `PendingPush bool` field to track offline-committed-but-not-pushed state
- `config.Config` gains `VaultRemote()`, `PendingPush()`, and `SetPendingPush()` methods
- `sync.ConfigReader` interface extended with `PendingPush()` / `SetPendingPush()` for offline tracking
- `tracker.Tracker` gains `ResolveKey(localPath)` — resolves `(namespace, relPath)` without any writes; used by `track`, `untrack`, and idempotency checks to eliminate duplicated namespace-resolution logic
- `sync.Syncer` gains `DryRun()` method — computes what would be pushed/pulled without touching disk or network
