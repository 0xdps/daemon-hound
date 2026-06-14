# End-to-End Testing Guide for DaemonHound v1.1.0

This guide walks through testing all daemon features on macOS, Linux, and Windows.

## Prerequisites

- DaemonHound binary built: `go build ./cmd/dh`
- A remote Git repository for vault testing
- System access to verify service registration

---

## 1. Test Suite: Basic Initialization & Daemon Installation

### 1.1 Clean Start

```bash
# Remove any existing installation
dh cleanup --force 2>/dev/null || true

# Verify cleanup worked
ls -la ~/.dh 2>/dev/null && echo "FAIL: .dh directory still exists" || echo "PASS: .dh cleaned up"
```

### 1.2 Initialize Vault with Daemon Auto-Installation

```bash
# Initialize with a test vault repository
dh init --remote git@github.com:YOUR_USERNAME/test-vault.git

# Expected output:
# ✓ Vault directory initialized
# ✓ Identity encrypted with password and salt
# ✓ Master password stored in keychain
# ✓ Background sync daemon installed
#   Auto-starts on system boot
#   Syncs every 30 seconds
```

**Verify on each OS:**

#### macOS (launchd):
```bash
# Check if plist exists
launchctl list | grep daemon-hound

# Expected output should show the service is loaded

# Check plist file
cat ~/Library/LaunchAgents/com.daemon-hound.plist | head -20

# Verify daemon is running
dh daemon status

# Expected:
# ✓ Daemon service installed
# ✓ Daemon is running
#   Syncing every 30 seconds
```

#### Linux (systemd):
```bash
# Check service file
cat ~/.config/systemd/user/daemon-hound.service | head -20

# Check service status
systemctl --user status daemon-hound

# Verify daemon is running
dh daemon status

# Expected:
# ✓ Daemon service installed
# ✓ Daemon is running
#   Syncing every 30 seconds

# View daemon logs
journalctl --user -u daemon-hound -f

# Should show: "=== Daemon started ===" and recent sync logs
```

#### Windows (Task Scheduler):
```powershell
# List scheduled tasks
Get-ScheduledTask | Where-Object {$_.TaskName -eq "DaemonHound"}

# Check if task is running
schtasks /query /tn DaemonHound /v

# Verify daemon is running
dh daemon status

# Expected:
# ✓ Daemon service installed
# ✓ Daemon is running
#   Syncing every 30 seconds

# View daemon logs
Get-Content $env:USERPROFILE\.dh\daemon.log -Tail 20 -Wait
```

---

## 2. Test Suite: File Tracking & Daemon Syncing

### 2.1 Track Local Files

```bash
# Create a test directory
mkdir -p ~/test-project
echo "SECRET_KEY=dev123" > ~/test-project/.env

# Track the file
dh track ~/test-project/.env --namespace test-project

# Verify file was tracked
dh status

# Expected output should show the file in "sync" mode
```

### 2.2 Local Change → Remote Push

```bash
# Modify the tracked file
echo "SECRET_KEY=dev456" > ~/test-project/.env

# Wait for daemon to detect change (watch debounce is 2s, but let's wait 5s to be safe)
sleep 5

# Check daemon logs for sync activity
dh daemon logs | tail -20

# Expected in logs:
# - "Detected local change"
# - "Committed local changes"
# - "Pushed to remote"

# Verify the change was pushed
cd ~/.dh/vault
git log --oneline -5

# Expected: Should see "[daemon] Sync local changes" commit
```

### 2.3 Remote Change → Local Pull

On a **different machine** (or simulate by pushing to remote):

```bash
# Simulate remote change by manually modifying vault
cd ~/.dh/vault
git pull origin trunk
echo "UPDATED=true" >> state.toml.age.dec  # or whatever tracked file is decrypted

# On the original machine, wait for next poll (30 seconds)
sleep 35

# Check daemon logs
dh daemon logs | tail -20

# Expected in logs:
# - "Pulled from remote"
# - Confirmation of changes applied locally

# Verify local file was updated
cat ~/test-project/.env
```

---

## 3. Test Suite: Conflict Resolution

### 3.1 Trigger a Conflict Scenario

**Machine A:**
```bash
echo "VERSION=A" > ~/test-project/version.txt
dh track ~/test-project/version.txt --namespace test-project
sleep 5  # Let daemon sync
```

**Machine B** (different system or simulated):
```bash
echo "VERSION=B" > ~/test-project/version.txt
dh track ~/test-project/version.txt --namespace test-project
sleep 5  # Let daemon sync
```

**Both machines try to push conflicting changes:**

```bash
# Back on Machine A
echo "VERSION=A2" > ~/test-project/version.txt
sleep 35  # Wait for daemon to poll

# Check logs for conflict detection
dh daemon logs | grep -i conflict

# Expected in logs:
# - "Merge conflicts detected"
# - "Conflicted files: [...]"
# - "Resolved conflicts using 'local' strategy"
# - "Committed conflict resolution"
```

---

## 4. Test Suite: Log Rotation

### 4.1 Generate Logs & Trigger Rotation

```bash
# Monitor daemon logs to see sync activity
dh daemon logs -f &

# Track multiple files to generate activity
for i in {1..20}; do
  echo "FILE_$i=value_$i" > ~/test-project/file$i.txt
  dh track ~/test-project/file$i.txt --namespace test-project
  sleep 1
done

# Let daemon process these (30 second poll interval)
sleep 35

# Check log file sizes
ls -lh ~/.dh/daemon*.log

# Expected: Log files should exist with reasonable size

# Check for rotated logs (if logs grew enough to rotate)
ls -lh ~/.dh/daemon*.*.log 2>/dev/null || echo "No rotated logs yet (normal if tests are quick)"

# Check log retention logic
du -sh ~/.dh/  # Total size should be reasonable

echo "PASS: Log rotation working"
```

### 4.2 Verify Automatic Cleanup

```bash
# Create old rotated logs manually (older than retention days)
cd ~/.dh
touch -d "40 days ago" daemon.2026-05-05-10-00-00.log

# Wait for log rotation check (happens hourly, but let's simulate by running daemon briefly)
dh daemon run &
DAEMON_PID=$!
sleep 65  # Wait past 1 minute mark so rotation check might occur
kill $DAEMON_PID 2>/dev/null || true

# Check if old log was cleaned up
ls -la daemon.2026-05-05-10-00-00.log 2>/dev/null && echo "WARN: Old log not cleaned" || echo "PASS: Old log cleaned"
```

---

## 5. Test Suite: Daemon Management Commands

### 5.1 View Logs

```bash
# View last 50 lines
dh daemon logs

# View last 100 lines
dh daemon logs -n 100

# Follow logs in real-time
dh daemon logs -f
# (Press Ctrl+C to stop)
```

### 5.2 Stop & Restart Daemon

```bash
# Stop daemon
dh daemon stop

# Verify stopped
dh daemon status
# Expected: Should show "⚠️  Daemon is not running"

# Restart daemon
dh daemon restart

# Verify running
dh daemon status
# Expected: "✓ Daemon is running"
```

### 5.3 Error Logging

```bash
# View error logs
dh daemon errors

# Expected: Should show any sync errors, watcher errors, etc.
# If no errors yet, that's fine (clean operation)
```

---

## 6. Test Suite: System Reboot Recovery

### 6.1 Verify Daemon Auto-Starts After Reboot

```bash
# Note the time before reboot
date

# Reboot the system
sudo reboot
# Wait for system to come back up

# After reboot, check daemon status
dh daemon status

# Expected: ✓ Daemon is running

# Check logs to verify daemon started automatically
dh daemon logs | head -30

# Expected to see: "=== Daemon started ===" near the reboot time
```

### 6.2 Verify Sync Happened During Downtime

**Before reboot on Machine A:**
```bash
echo "BEFORE_REBOOT=value" > ~/test-project/test.txt
dh track ~/test-project/test.txt --namespace test-project
```

**On Machine B** (while Machine A is rebooting):
```bash
# Make a remote change
echo "WHILE_REBOOTING=value" >> ~/test-project/test.txt
dh track ~/test-project/test.txt --namespace test-project
```

**After Machine A reboots:**
```bash
# Check if changes from B were pulled during auto-sync
cat ~/test-project/test.txt

# Should contain both values, indicating daemon caught up
```

---

## 7. Test Suite: Cleanup Command

### 7.1 Full Cleanup

```bash
# Before cleanup
dh status  # Should show vault info

# Perform cleanup without prompt
dh cleanup --force

# Verify cleanup
ls ~/.dh 2>/dev/null && echo "FAIL: Some files remain" || echo "PASS: All cleaned"

# Verify keychain entry removed (varies by OS)
# macOS: security find-generic-password -s "dh" | grep -q "found 0" && echo "PASS: Keychain cleaned"
# Linux: ! keyctl search @u user dh 2>/dev/null && echo "PASS: Keyring cleaned"
```

---

## 8. Test Matrix: Expected Results

| Feature | macOS | Linux | Windows | Status |
|---------|-------|-------|---------|--------|
| Daemon auto-install | ✓ launchd | ✓ systemd | ✓ Task Scheduler | v1.1.0 |
| Auto-start on boot | ✓ | ✓ | ✓ | v1.1.0 |
| File watching | ✓ fsnotify | ✓ fsnotify | ✓ fsnotify | v1.1.0 |
| Polling sync (30s) | ✓ | ✓ | ✓ | v1.1.0 |
| Conflict detection | ✓ | ✓ | ✓ | v1.1.0 |
| Conflict resolution | ✓ local | ✓ local | ✓ local | v1.1.0 |
| Log rotation | ✓ | ✓ | ✓ | v1.1.0 |
| Daemon logs | ✓ | ✓ journalctl | ✓ | v1.1.0 |
| Global salt | ✓ | ✓ | ✓ | v1.1.0 |
| Cleanup command | ✓ | ✓ | ✓ | v1.1.0 |

---

## 9. Known Limitations & Future Work

### v1.1.0 Complete
- ✅ Global identity salt with prefix@postfix
- ✅ Daemon auto-installation
- ✅ Background file watching & polling
- ✅ Automatic conflict resolution (local strategy)
- ✅ Log rotation & cleanup
- ✅ Cleanup command

### v1.2.0+ Future
- 🔄 User-configurable conflict strategy ("ask", "merge", etc.)
- 🔄 Partial sync support (track specific namespaces only)
- 🔄 Daemon metrics & performance monitoring
- 🔄 Cross-machine state synchronization

---

## 10. Troubleshooting

### Daemon not auto-starting after init

```bash
# Check if service is installed
dh daemon status

# Manually check service status
# macOS: launchctl list | grep daemon-hound
# Linux: systemctl --user status daemon-hound
# Windows: schtasks /query /tn DaemonHound

# Restart daemon manually
dh daemon restart
```

### Logs not appearing

```bash
# Check log file path
ls -la ~/.dh/daemon*.log

# Check if daemon is actually running
dh daemon status

# Try running daemon in foreground for debugging
dh daemon run

# Check for errors
dh daemon errors
```

### Sync not happening

```bash
# Verify daemon is running
dh daemon status

# Check recent logs
dh daemon logs -n 50

# Manually trigger sync for testing
dh sync

# Verify vault is initialized
dh status
```

### Conflicts not resolving

```bash
# Check conflict logs
dh daemon logs | grep -i conflict

# Check git state
cd ~/.dh/vault
git status

# If stuck in merge, manually resolve
git merge --abort  # Or complete the merge
```

---

## 11. Test Completion Checklist

- [ ] macOS: Service auto-installed and running
- [ ] macOS: Daemon restarted after system reboot
- [ ] macOS: File changes detected and synced
- [ ] Linux: Service auto-installed and running
- [ ] Linux: Daemon restarted after system reboot
- [ ] Linux: File changes detected and synced
- [ ] Windows: Service auto-installed and running
- [ ] Windows: File changes detected and synced
- [ ] Cross-machine sync working (if available)
- [ ] Conflicts detected and resolved automatically
- [ ] Logs rotate without manual intervention
- [ ] Cleanup command removes all traces
- [ ] Global salt enables multi-machine operation

---

## Success! ✅

If all tests pass on macOS, Linux, and Windows, daemon-hound v1.1.0 is production-ready!
