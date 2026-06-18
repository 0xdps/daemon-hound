# DaemonHound User Flows

This document describes the primary user journeys in DaemonHound.

---

## Flow 1: First-Time Setup (Machine 1)

You have a new machine or are using DaemonHound for the first time.

```bash
dh init --remote git@github.com:you/my-vault.git
# Enter master password: ••••••••
# Confirm master password: ••••••••
```

DaemonHound will:
1. Generate a stable machine UUID → stored in `~/.dh/config.toml`
2. Generate an age identity key → encrypted with your password → `~/.dh/identity.age`
3. Clone (or create) the vault repo locally at `~/.dh/vault/`
4. Save the vault remote URL so `--force` re-init can pre-fill it
5. Store the master password in the OS keychain when available
6. Install the background sync daemon on supported platforms

> ⚠️ Back up `~/.dh/identity.age` immediately. Without it, encrypted vault data cannot be recovered.

### Re-initializing an existing machine

```bash
dh init --force
# Vault repository URL [git@github.com:you/my-vault.git]:   ← pre-filled
```

---

## Flow 2: Track Files on Machine 1

Inside a project that has a Git remote:

```bash
cd ~/projects/pingpong-api

dh track .env.local        # sync mode (default) — shared across machines
dh track .env.test         # sync mode

dh track ~/.zshrc --mode backup   # backup mode — this machine only
```

DaemonHound:
1. Reads `origin` remote → derives namespace `github.com/you/pingpong-api`
2. Warns if the file is already tracked in Git (secrets shouldn't be committed)
3. Encrypts the file with the age identity key
4. Stores it in the vault under `sync/github.com/you/pingpong-api/.env.local.age`
5. Records the local binding: namespace → absolute root path
6. Commits and pushes the vault immediately
7. Writes an audit log entry to `~/.dh/audit.log`

Re-running `dh track` on an already-tracked file is safe — it reports "already tracked" and exits.

---

## Flow 3: Check Sync Status

```bash
dh status
# github.com/you/pingpong-api
#   .env.local    clean
#   .env.test     dirty  (modified locally, not yet pushed)
#
# global
#   .zshrc        clean  (backup, this machine only)
#
# ⚠ Pending push: local vault commits not yet pushed to remote

dh status --namespace github.com/you/pingpong-api   # filter to one namespace

dh status --output json   # machine-readable output
```

Status values:
- **clean** — local file matches vault
- **dirty** — local file has been modified since last sync
- **new** — tracked but never synced (no checksum yet)
- **missing** — file no longer exists on disk

---

## Flow 4: Push and Pull Changes

```bash
dh sync
# Pulls any remote changes first, then pushes local dirty files.
# → github.com/you/pingpong-api  .env.test    pushed
# → Done

dh sync --dry-run           # preview what would change without writing anything

dh sync --namespace github.com/you/pingpong-api   # only this namespace
```

### Offline behavior

If the vault remote is unreachable, `dh sync` skips the pull and pushes local dirty files to the local git repo. A **pending push** flag is set. The next successful `dh sync` retries the remote push automatically. `dh status` shows the pending push notice.

---

## Flow 5: New Machine Setup (Machine 2)

You have a second machine where the project lives at a different path.

### Step 1 — Initialize

On Machine 1, export the age identity:

```bash
dh export-identity
# Master password: ••••••••
# AGE-SECRET-KEY-...
```

Keep the printed key secret. Anyone with it can decrypt the vault.

On Machine 2, initialize with the exported key:

```bash
dh init --remote git@github.com:you/my-vault.git --age-key AGE-SECRET-KEY-...
# Enter and confirm the master password for this machine's encrypted identity file
```

The master password protects `~/.dh/identity.age` locally. The shared age identity is what allows multiple machines to decrypt the same vault contents.

### Step 2a — Sync a single project

```bash
cd ~/work/pingpong-api

dh sync
# → github.com/you/pingpong-api: 2 tracked files found in vault
# →   .env.local   ✓ restored
# →   .env.test    ✓ restored
```

No need to run `dh track` again. The namespace is already registered from Machine 1.

### Step 2b — Discover and sync all projects at once

```bash
dh discover ~/work
# Scanning ~/work...
#
# github.com/you/pingpong-api   2 tracked files  ✓ synced
# github.com/you/portfolio      1 tracked file   ✓ synced
# github.com/you/side-project   not in vault     – skipped
#
# 2 namespaces restored, 1 skipped

dh discover ~/work --output json   # machine-readable output
```

---

## Flow 6: Secret Management

### Store a secret

```bash
dh secret set openai-key
# Enter value: ••••••••••••••
# → Stored: openai-key
```

The secret is stored encrypted in the vault. No file is touched yet.

### Map the secret to a file (one-time, per repo)

Each repo can use a different environment variable name for the same logical secret:

```bash
cd ~/projects/pingpong-api
dh secret ref openai-key .env.local OPENAI_API_KEY
# → Mapped: openai-key → github.com/you/pingpong-api:.env.local:OPENAI_API_KEY

cd ~/projects/portfolio
dh secret ref openai-key .env.local OPENAI_KEY
# → Mapped: openai-key → github.com/you/portfolio:.env.local:OPENAI_KEY
```

DaemonHound writes the secret value into each file at the mapped key and marks those files dirty. Run `dh sync` to push.

### List mappings

```bash
dh secret list
# openai-key
#   github.com/you/pingpong-api   .env.local   OPENAI_API_KEY
#   github.com/you/portfolio      .env.local   OPENAI_KEY

dh secret list openai-key   # list mappings for one secret
```

### Retrieve the raw value

```bash
dh secret get openai-key
# → sk-proj-xxxxxxxxxxxx
```

### Rotate the secret

```bash
dh secret set openai-key NEW_VALUE
# → Updated .env.local in github.com/you/pingpong-api  (OPENAI_API_KEY)
# → Updated .env.local in github.com/you/portfolio     (OPENAI_KEY)
# → 2 files marked dirty — run `dh sync` to push
```

All mapped files get the new value written to their respective keys automatically.

### Rename a secret

```bash
dh secret rename openai-key openai-prod-key
# → Renamed secret: openai-key → openai-prod-key
```

The value and all mappings are preserved.

### Remove a mapping

```bash
dh secret unref openai-key .env.local
# → Removed mapping: openai-key from .env.local in github.com/you/pingpong-api
```

### Delete a secret entirely

```bash
dh secret delete openai-key
# → Deleted secret: openai-key
# → Note: any files that contained this secret value were NOT modified.
```

---

## Flow 7: Health Check and Repair

```bash
dh doctor
# ✓ Config loaded
# ✓ Identity file exists
# ✓ Vault directory exists
# ✓ Vault is a git repo
# ✓ Vault remote configured
# ⚠ Vault remote not reachable (offline?)
# ✓ 2 binding(s) configured
# ✓ No pending push

dh doctor --fix
# Attempts to auto-repair: creates vault directory, initializes git repo,
# adds the configured remote if missing.
```

---

## Flow 8: Multi-Machine Backup Files

```bash
dh machines
# Machine ID                                Note
# ----------                                ----
# a1b2c3d4-...                              (this machine)
# e5f6g7h8-...

dh machines --output json
```

Each machine that has used `--mode backup` appears here. Backup files from decommissioned machines remain in the vault but are no longer updated.

---

## Flow 9: Change Master Password

```bash
dh rekey
# Enter current password: ••••••••
# Enter new password: ••••••••••••
# Confirm new password: ••••••••••••
# → Identity re-encrypted with new password.
```

The age identity is decrypted with the old password and re-encrypted with the new one atomically. The vault contents are unaffected.

---

## Flow 10: Export Vault Contents

To migrate away from DaemonHound or create a plaintext backup:

```bash
dh export
# ⚠ WARNING: This writes plaintext secrets to disk. Delete the export when done.
# Exporting to dh-export-20260614-120000/
# → files/github.com/you/pingpong-api/.env.local
# → files/github.com/you/portfolio/.env.local
# → secrets.json
# Done.

dh export --dir /tmp/my-export   # custom output directory
```

The export directory contains:
- `files/<namespace>/<relPath>` — decrypted tracked files
- `secrets.json` — all secret names and values in plaintext

> ⚠️ Delete the export directory when you're done with it.

---

## Flow 11: Machine Decommission

When retiring a machine:

1. Run `dh sync` to ensure all local changes are pushed
2. Run `dh cleanup` to stop/uninstall the daemon, remove the cached keychain password, and delete local data under `~/.dh/`
3. Backup mode files for that machine UUID remain in the vault but will no longer be updated

```bash
dh cleanup
# or, for automation:
dh cleanup --force
```

Sync mode files are unaffected — other machines continue to use them normally.

---

## Flow 12: Background Daemon Operations

`dh init` installs the daemon automatically when the platform supports user-level services. Manual `dh sync` remains available, but the daemon handles routine sync cycles.

```bash
dh daemon status
# ✓ Daemon service installed
# ✓ Daemon is running

dh daemon logs
dh daemon logs -f
dh daemon logs -n 100

dh daemon errors

dh daemon restart
dh daemon stop
```

For debugging or manual service setup, run the daemon in the foreground:

```bash
dh daemon run
```

The daemon watches tracked file directories, watches the local vault clone, polls the remote every 30 seconds, rotates logs, and uses the same user permissions as the CLI.

---

## Flow 13: Conflict Review

Most encrypted vault conflicts are resolved automatically by the sync engine. When the daemon records a conflict for review:

```bash
dh conflicts list
dh conflicts show .env.local

dh conflicts resolve .env.local --strategy local
# or:
dh conflicts resolve .env.local --strategy remote

dh sync
dh conflicts clear
```

`local` keeps this machine's version; `remote` accepts the remote version. `dh conflicts clear` only removes resolved conflict records.

---

## Audit Log

Every mutating command (`track`, `untrack`, `sync`, `secret set/ref/delete/rename/unref`, `discover`, `export`) is recorded in `~/.dh/audit.log`:

```
2026-06-14T12:00:00Z  track        github.com/you/pingpong-api:.env.local
2026-06-14T12:01:00Z  sync         push: github.com/you/pingpong-api:.env.local
2026-06-14T12:02:00Z  secret set   openai-key refs=2
```

---

## Summary

| Scenario                          | Command                                    |
|-----------------------------------|--------------------------------------------|
| First-time setup                  | `dh init --remote <url>`                   |
| Re-initialize machine             | `dh init --force`                          |
| Track a project file              | `dh track .env.local`                      |
| Track a machine-only file         | `dh track ~/.zshrc --mode backup`          |
| Push/pull changes                 | `dh sync`                                  |
| Preview changes                   | `dh sync --dry-run`                        |
| Sync one namespace only           | `dh sync --namespace <ns>`                 |
| Set up a new machine (one repo)   | `dh init` then `dh sync` in repo dir       |
| Set up a new machine (all repos)  | `dh init` then `dh discover ~/`            |
| Check sync status                 | `dh status`                                |
| JSON status (scripting)           | `dh status --output json`                  |
| Store a secret                    | `dh secret set <key>`                      |
| Retrieve a secret                 | `dh secret get <key>`                      |
| List secrets                      | `dh secret list [key]`                     |
| Rotate a secret                   | `dh secret set <key>` (new value)          |
| Rename a secret                   | `dh secret rename <old> <new>`             |
| Map secret to a file              | `dh secret ref <key> <file> <ENV_VAR>`     |
| Remove a secret mapping           | `dh secret unref <key> <file>`             |
| Delete a secret                   | `dh secret delete <key>`                   |
| Health check                      | `dh doctor`                                |
| Auto-repair common issues         | `dh doctor --fix`                          |
| List backup machines              | `dh machines`                              |
| Change master password            | `dh rekey`                                 |
| Export everything to plaintext    | `dh export`                                |
| Export identity for new machine   | `dh export-identity`                       |
| Check daemon                      | `dh daemon status`                         |
| View daemon logs                  | `dh daemon logs`                           |
| Resolve conflicts                 | `dh conflicts resolve <file>`              |
| Remove cached password            | `dh logout`                                |
| Remove local installation         | `dh cleanup`                               |
