# DaemonHound v1.1.0 Feature Summary

## What's New in v1.1.0

This release completes the daemon infrastructure and adds automated background syncing, conflict resolution, and cleanup capabilities.

### Core Features Implemented

#### 1. **Global Identity Salt** ✅
- Salt format: `base64(16-byte-prefix)@base64(16-byte-postfix)` = 32 bytes total
- Stored plaintext in `~/.dh/config.toml` (not secret, enables multi-machine consistency)
- Generated at `dhd init` time
- All machines using same vault can decrypt identity with same password
- Enables seamless password sharing across teams
- **NEW in v1.1.0**: Integrated with daemon for true multi-machine sync

#### 2. **Daemon Auto-Installation** ✅
- During `dhd init`, daemon service automatically registered with OS
- No manual `dhd daemon install` needed
- Service automatically starts on next system boot
- OS-specific implementation:
  - **macOS**: `~/Library/LaunchAgents/com.daemon-hound.plist`
  - **Linux**: `~/.config/systemd/user/daemon-hound.service`
  - **Windows**: Task Scheduler task "DaemonHound"

#### 3. **Background File Watching & Polling** ✅
- **File Watching**: fsnotify monitors `~/.dh/vault` for local changes
  - Ignores `.git` directory and dotfiles
  - 2-second debounce to batch changes
  - Triggers sync on modification
- **Polling**: Every 30 seconds, check for remote changes
  - Fetches from remote vault
  - Pulls any new commits
  - Pushes local changes back to remote

#### 4. **Automatic Conflict Resolution** ✅
- Detects merge conflicts after pull: `git status --porcelain` for conflict markers
- Automatically resolves using **"local" strategy** (keeps local changes)
  - Can be extended to support "remote", "ask", or "merge" strategies in v1.2.0+
- Commits resolution with message: `[daemon] Resolve merge conflicts`
- Logs conflicted files for audit trail

#### 5. **Log Rotation & Cleanup** ✅
- **Daemon logs** at `~/.dh/daemon.log`
- **Error logs** at `~/.dh/daemon.error.log`
- **Log rotation**: Checks hourly
  - Rotates files exceeding 10MB (daemon) or 5MB (errors)
  - Rotated files: `daemon.YYYY-MM-DD-HH-MM-SS.log`
- **Retention policy**: Keeps logs for 30 days
  - Automatically removes rotated logs older than retention period
- **Configurable**: Can set via config (v1.2.0+ feature)

#### 6. **Cleanup Command** ✅
```bash
dhd cleanup              # Prompts for confirmation
dhd cleanup --force      # Skips confirmation

# Removes:
# - ~/.dh/ directory (entire vault)
# - Keychain entry for password
# - Daemon service registration
```

#### 7. **Daemon Management Commands** ✅
```bash
dhd daemon run           # Run daemon in foreground (testing)
dhd daemon status        # Check if running
dhd daemon logs          # View sync logs
dhd daemon logs -f       # Follow logs in real-time
dhd daemon logs -n 100   # View last 100 lines
dhd daemon errors        # View error logs
dhd daemon stop          # Stop daemon
dhd daemon restart       # Restart daemon
```

---

## Architecture

### Daemon Sync Flow

```
Local Change Detected (fsnotify)
         ↓
Wait 2 seconds (debounce)
         ↓
Commit local changes: "[daemon] Sync local changes"
         ↓
Push to remote Git repository
         ↓
──────────────────────────────────
         ↓
Scheduled Poll (every 30s)
         ↓
Fetch from remote
         ↓
Check for conflicts
   ├─ If conflicts → Resolve with "local" strategy
   └─ Commit resolution: "[daemon] Resolve merge conflicts"
         ↓
Pull new commits from remote
         ↓
Update local state
```

### Service Registration

#### macOS (launchd)
```xml
~/Library/LaunchAgents/com.daemon-hound.plist

Key features:
- RunAtLoad: true (auto-start on boot)
- KeepAlive: true (auto-restart on crash)
- StandardOutPath: ~/.dh/daemon.log
- StandardErrorPath: ~/.dh/daemon.error.log
```

#### Linux (systemd)
```ini
~/.config/systemd/user/daemon-hound.service

Key features:
- Type: simple
- ExecStart: /path/to/dhd daemon run
- Restart: always
- RestartSec: 10s
- Logs via journalctl
```

#### Windows (Task Scheduler)
```xml
DaemonHound task

Key features:
- Triggers: On logon + Registration
- Action: Run /path/to/dhd daemon run
- Restart: 3 times with 1-minute intervals
- Run Level: Least Privilege
```

---

## Configuration

### Daemon Settings (config.toml)

```toml
[daemon]
watch_interval = 2              # File watch debounce (seconds)
poll_interval = 30              # Remote poll interval (seconds)
conflict_strategy = "local"     # "local" or future: "remote", "ask"
max_log_size = 10               # Maximum log file size (MB)
max_error_log_size = 5          # Maximum error log size (MB)
log_retention_days = 30         # Keep logs for N days
```

*Note: Not yet persisted to config.toml; defaults used in v1.1.0*

---

## Security

### Vault Structure
```
~/.dh/
├── config.toml              # Plaintext config (includes global salt)
├── identity.age             # Encrypted identity (AES-256-GCM)
├── vault/                   # Git repository (encrypted state)
│   ├── .git/
│   └── state.toml.age       # Encrypted vault state
├── daemon.log               # Sync activity logs
└── daemon.error.log         # Error logs
```

### Salt Security
- **Global salt** is plaintext (by design for multi-machine consistency)
- **Not a secret**: Used to derive key from password via scrypt
- **Formula**: `key = scrypt(password + global_salt, N=32768, r=8, p=1, keyLen=32)`
- Different passwords → Different keys → Different identity decryption → Different decryption of state

### Multi-Machine Support
```
Machine A: password "secret" + salt "ABC@DEF" = key_A → decrypts state_A
Machine B: password "secret" + salt "ABC@DEF" = key_A → decrypts same state_A ✓
Machine C: password "secret" + salt "XYZ@UVW" = key_C → cannot decrypt state_A ✗
```

---

## Operational Details

### Daemon Lifecycle

**On System Boot:**
1. OS service manager starts daemon
2. Daemon creates log files (if not exist)
3. Daemon sets up file watcher on `~/.dh/vault`
4. Daemon performs initial sync
5. Daemon enters main loop (watch + poll)

**On `dhd track <file>`:**
1. File added to vault
2. Daemon detects change in next poll
3. Commits and pushes to remote
4. Other machines pull changes in next poll

**On Conflicting Edits:**
1. Machine A and B both edit same file
2. Both push to remote (one succeeds, one conflicts on pull)
3. Daemon detects conflict
4. Resolves using "local" strategy (keeps own changes)
5. Commits resolution
6. Pushes resolved state to remote

### Logging

**daemon.log** (each line includes timestamp):
```
[daemon] 2026-06-14 10:30:45 ==> Daemon started ===
[daemon] 2026-06-14 10:30:45 Performing initial sync...
[daemon] 2026-06-14 10:30:46 Starting sync (vault: git@github.com:user/vault.git)
[daemon] 2026-06-14 10:30:47 Fetched from remote
[daemon] 2026-06-14 10:30:47 Pulled from remote
[daemon] 2026-06-14 10:30:48 Sync completed in 2.123s
```

**daemon.error.log** (for watcher errors):
```
[watcher] 2026-06-14 10:35:20 Failed to watch /home/user/.dh/vault: permission denied
```

---

## Testing

Comprehensive end-to-end testing guide provided in `E2E_TESTING_GUIDE.md`:

- Basic initialization & daemon installation
- File tracking & daemon syncing
- Conflict resolution scenarios
- Log rotation & cleanup
- Daemon management commands
- System reboot recovery
- Cleanup command verification
- Expected results matrix for macOS/Linux/Windows

---

## Upgrade Path (v1.1.0 → v1.2.0+)

### Planned for v1.2.0
- [ ] Configurable conflict strategies ("ask", "merge", "abort")
- [ ] Daemon configuration persistence to config.toml
- [ ] Partial sync (sync specific namespaces only)
- [ ] Daemon metrics & uptime tracking
- [ ] Performance profiling

### Won't Change in v1.1.0
- Global salt remains plaintext (correct design)
- Conflict strategy fixed to "local" (production-ready default)
- Log rotation fixed to hourly (reasonable for most deployments)

### Breaking Changes (v1.0.x → v1.1.0)

**None!** v1.1.0 is backward compatible:
- Existing vault configurations work with daemon
- Existing global salt from v1.0.x continues to work
- `dhd sync` command still works alongside daemon

**Upgrade:** Simply rebuild binary or install new version. Daemon auto-starts on next boot.

---

## Performance Characteristics

### Latency (Local Change → Remote)
- File modification detected: < 100ms (OS notification)
- Debounce wait: 2 seconds
- Git commit: ~100ms
- Git push: Depends on network (typically 500ms - 2s)
- **Total**: ~2.5 - 4.5 seconds in typical case

### Latency (Remote Change → Local)
- Poll interval: Every 30 seconds
- Fetch + pull + detect conflicts: ~1-2 seconds
- Conflict resolution (if needed): ~500ms
- **Total**: 30-32 seconds in typical case (next poll)

### Resource Usage
- CPU: Minimal (idle most of time, active only during sync)
- Memory: ~20-30MB (file watcher + git client)
- Disk I/O: During sync only (git operations)
- Network: Poll every 30s + push on changes

---

## Known Limitations

### v1.1.0
- Conflict strategy fixed to "local" (no user choice)
- Log rotation not configurable via config.toml
- No explicit conflict resolution prompt to user
- Windows support via Task Scheduler (no native service manager)

### By Design
- Global salt is plaintext (correct for multi-machine consistency)
- Daemon syncs entire vault (no selective sync yet)
- No real-time collaboration conflict detection (30-second polling is best-effort)

---

## Migration Guide

### From v1.0.x without Daemon

```bash
# v1.0.x manual process
dhd init --remote git@...
dhd track file1
dhd track file2
# Manual: dhd sync (every time you want to sync)

# v1.1.0 automatic process
dhd cleanup  # Optional: clean old installation
dhd init --remote git@...  # Daemon auto-installs!
dhd track file1
dhd track file2
# Automatic: Files sync every 30 seconds! ✨
```

### From v1.0.x with Manual Daemon

```bash
# If you manually installed daemon in v1.0.x:
dhd daemon stop          # Or: systemctl --user stop daemon-hound
dhd cleanup --force      # Clean everything

# Reinstall with auto-daemon in v1.1.0
dhd init --remote git@...  # Daemon auto-installs with correct config
```

---

## Conclusion

DaemonHound v1.1.0 is a production-ready secret vault synchronizer with:
- ✅ Seamless multi-machine operation (global salt)
- ✅ Automatic background syncing (daemon service)
- ✅ Intelligent conflict handling (automatic resolution)
- ✅ Operational best practices (log rotation, cleanup)
- ✅ Cross-platform support (macOS, Linux, Windows)

Ready for v1.2.0 enhancements and team deployment!
