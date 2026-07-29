# DaemonHound User Flows

This document describes the primary user journeys in DaemonHound.

---

## Flow 1: First-Time Setup (Machine 1)

You have a new machine or are using DaemonHound for the first time.

```bash
dhd init --remote git@github.com:you/my-vault.git
# Enter master password: ••••••••
# Confirm master password: ••••••••
```

DaemonHound will:
1. Generate a stable machine UUID → stored in `~/.daemon-hound/config.toml`
2. Generate an age identity key → encrypted with your password → `~/.daemon-hound/identity.age`
3. Clone (or create) the vault repo locally at `~/.daemon-hound/vault/`
4. Save the vault remote URL so `--force` re-init can pre-fill it
5. Store the master password in the OS keychain when available
6. Install the background sync daemon on supported platforms

> ⚠️ Back up `~/.daemon-hound/identity.age` immediately. Without it, encrypted vault data cannot be recovered.

### Re-initializing an existing machine

```bash
dhd init --force
# Vault repository URL [git@github.com:you/my-vault.git]:   ← pre-filled
```

---

## Flow 2: Track Files on Machine 1

Inside a project that has a Git remote:

```bash
cd ~/projects/pingpong-api

dhd track .env.local        # sync mode (default) — shared across machines
dhd track .env.test         # sync mode

dhd track ~/.zshrc --mode backup   # backup mode — this machine only
```

DaemonHound:
1. Reads `origin` remote → derives namespace `github.com/you/pingpong-api`
2. Warns if the file is already tracked in Git (secrets shouldn't be committed)
3. Encrypts the file with the age identity key
4. Stores it in the vault under `sync/github.com/you/pingpong-api/.env.local.age`
5. Records the local binding: namespace → absolute root path
6. Commits and pushes the vault immediately
7. Writes an audit log entry to `~/.daemon-hound/audit.log`

Re-running `dhd track` on an already-tracked file is safe — it reports "already tracked" and exits.

### Stop tracking files

Remove a file from the shared vault entirely:

```bash
dhd untrack .env.local
```

Remove a file from this machine only, keeping the vault entry available for other machines:

```bash
dhd untrack --local .env.local
dhd untrack --local github.com/you/pingpong-api:.env.local
```

After deleting local project folders, remove every missing tracked file from this machine only:

```bash
dhd untrack --missing
```

Local-only untracking records an ignore in `~/.daemon-hound/config.toml`; it does not delete encrypted vault contents or shared tracking metadata.

---

## Flow 3: Check Sync Status

```bash
dhd status
# github.com/you/pingpong-api
#   .env.local    clean
#   .env.test     dirty  (modified locally, not yet pushed)
#
# global
#   .zshrc        clean  (backup, this machine only)
#
# ⚠ Pending push: local vault commits not yet pushed to remote

dhd status --namespace github.com/you/pingpong-api   # filter to one namespace

dhd status --output json   # machine-readable output
```

Status values:
- **clean** — local file matches vault
- **dirty** — local file has been modified since last sync
- **new** — tracked but never synced (no checksum yet)
- **missing** — file no longer exists on disk

---

## Flow 4: Push and Pull Changes

```bash
dhd sync
# Pulls any remote changes first, then pushes local dirty files.
# → github.com/you/pingpong-api  .env.test    pushed
# → Done

dhd sync --dry-run           # preview what would change without writing anything

dhd sync --namespace github.com/you/pingpong-api   # only this namespace
```

### Offline behavior

If the vault remote is unreachable, `dhd sync` skips the pull and pushes local dirty files to the local git repo. A **pending push** flag is set. The next successful `dhd sync` retries the remote push automatically. `dhd status` shows the pending push notice.

---

## Flow 5: New Machine Setup (Machine 2)

You have a second machine where the project lives at a different path.

### Step 1 — Initialize

On Machine 1, export the age identity:

```bash
dhd export-identity
# Master password: ••••••••
# AGE-SECRET-KEY-...
```

Keep the printed key secret. Anyone with it can decrypt the vault.

On Machine 2, initialize with the exported key:

```bash
dhd init --remote git@github.com:you/my-vault.git --age-key AGE-SECRET-KEY-...
# Enter and confirm the master password for this machine's encrypted identity file
```

The master password protects `~/.daemon-hound/identity.age` locally. The shared age identity is what allows multiple machines to decrypt the same vault contents.

### Step 2a — Sync a single project

```bash
cd ~/work/pingpong-api

dhd sync
# → github.com/you/pingpong-api: 2 tracked files found in vault
# →   .env.local   ✓ restored
# →   .env.test    ✓ restored
```

No need to run `dhd track` again. The namespace is already registered from Machine 1.

### Step 2b — Discover and sync all projects at once

```bash
dhd discover ~/work
# Scanning ~/work...
#
# github.com/you/pingpong-api   2 tracked files  ✓ synced
# github.com/you/portfolio      1 tracked file   ✓ synced
# github.com/you/side-project   not in vault     – skipped
#
# 2 namespaces restored, 1 skipped

dhd discover ~/work --output json   # machine-readable output
```

---

## Flow 5b: Lightweight Vault Access (Clone and Read)

You need to access vault contents on a machine without full DaemonHound initialization — no `~/.daemon-hound/` directory, no daemon, no tracked file bindings. This is useful for CI pipelines, temporary environments, or quick secret retrieval.

### Clone the vault (empty, index-only)

```bash
dhd clone git@github.com:you/my-vault.git
# → Cloned to ./my-vault/
# → Working tree contains only state.toml.age (the vault index)
```

The clone uses Git partial clone with `--filter=blob:none` so only the index downloads initially. No file contents are fetched yet.

Options:
- `--directory <dir>` — clone into a specific directory instead of deriving from the repo name
- `--identity <path>` — use an existing age identity file instead of generating one
- `--namespace <ns>` — pre-fetch a namespace's tracked files (optional)
- `--secret <name>` — pre-fetch a specific secret file (optional)

### Read files on demand

```bash
cd my-vault

# Read a tracked file — pulled automatically on first access
dhd read github.com/you/pingpong-api:.env.local
# → Pulling sync/github.com/you/pingpong-api/.env.local.age...
# → (decrypted content printed to stdout)

# Read a secret — pulled automatically on first access
dhd read secret:openai-key
# → Pulling secrets/openai-key.toml.age...
# → sk-proj-xxxxxxxxxxxx
```

Files and secrets are fetched lazily via `git sparse-checkout add`, which triggers a network fetch only for the requested path. Subsequent reads use the cached local copy.

### Read a specific secret version

```bash
dhd read secret:openai-key@v2
# → (version 2 value printed)
```

### How it differs from `dhd init` + `dhd sync`

| Aspect | `dhd init` + `dhd sync` | `dhd clone` + `dhd read` |
|--------|--------------------------|--------------------------|
| Config directory | Creates `~/.daemon-hound/` | None |
| Daemon | Installed and running | None |
| File bindings | Restores files to project directories | Prints to stdout |
| Machine identity | Generates machine UUID | No machine UUID |
| Backup mode | Supported | Not supported |
| Use case | Daily development machine | CI, temporary access, quick reads |

---

## Flow 6: Secret Management

### Store a secret

```bash
dhd secret set openai-key
# Enter value: ••••••••••••••
# → Stored: openai-key
```

The secret is stored encrypted in the vault. No file is touched yet.

### Map the secret to a file (one-time, per repo)

Each repo can use a different environment variable name for the same logical secret:

```bash
cd ~/projects/pingpong-api
dhd secret ref openai-key .env.local OPENAI_API_KEY
# → Mapped: openai-key → github.com/you/pingpong-api:.env.local:OPENAI_API_KEY

cd ~/projects/portfolio
dhd secret ref openai-key .env.local OPENAI_KEY
# → Mapped: openai-key → github.com/you/portfolio:.env.local:OPENAI_KEY
```

DaemonHound writes the secret value into each file at the mapped key and marks those files dirty. Run `dhd sync` to push.

### List mappings

```bash
dhd secret list
# openai-key
#   github.com/you/pingpong-api   .env.local   OPENAI_API_KEY
#   github.com/you/portfolio      .env.local   OPENAI_KEY

dhd secret list openai-key   # list mappings for one secret
```

### Retrieve the raw value

```bash
dhd secret get openai-key
# → sk-proj-xxxxxxxxxxxx
```

### Rotate the secret

```bash
dhd secret set openai-key NEW_VALUE
# → Updated .env.local in github.com/you/pingpong-api  (OPENAI_API_KEY)
# → Updated .env.local in github.com/you/portfolio     (OPENAI_KEY)
# → 2 files marked dirty — run `dhd sync` to push
```

All mapped files get the new value written to their respective keys automatically.

### Rename a secret

```bash
dhd secret rename openai-key openai-prod-key
# → Renamed secret: openai-key → openai-prod-key
```

The value and all mappings are preserved.

### Remove a mapping

```bash
dhd secret unref openai-key .env.local
# → Removed mapping: openai-key from .env.local in github.com/you/pingpong-api
```

### Delete a secret entirely

```bash
dhd secret delete openai-key
# → Deleted secret: openai-key
# → Note: any files that contained this secret value were NOT modified.
```

---

## Flow 7: Health Check and Repair

```bash
dhd doctor
# ✓ Config loaded
# ✓ Identity file exists
# ✓ Vault directory exists
# ✓ Vault is a git repo
# ✓ Vault remote configured
# ⚠ Vault remote not reachable (offline?)
# ✓ 2 binding(s) configured
# ✓ No pending push

dhd doctor --fix
# Attempts to auto-repair: creates vault directory, initializes git repo,
# adds the configured remote if missing.
```

---

## Flow 8: Multi-Machine Backup Files

```bash
dhd machines
# Machine ID                                Note
# ----------                                ----
# a1b2c3d4-...                              (this machine)
# e5f6g7h8-...

dhd machines --output json
```

Each machine that has used `--mode backup` appears here. Backup files from decommissioned machines remain in the vault but are no longer updated.

---

## Flow 9: Change Master Password

```bash
dhd rekey
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
dhd export
# ⚠ WARNING: This writes plaintext secrets to disk. Delete the export when done.
# Exporting to dhd-export-20260614-120000/
# → files/github.com/you/pingpong-api/.env.local
# → files/github.com/you/portfolio/.env.local
# → secrets.json
# Done.

dhd export --dir /tmp/my-export   # custom output directory
```

The export directory contains:
- `files/<namespace>/<relPath>` — decrypted tracked files
- `secrets.json` — all secret names and values in plaintext

> ⚠️ Delete the export directory when you're done with it.

---

## Flow 11: Machine Decommission

When retiring a machine:

1. Run `dhd sync` to ensure all local changes are pushed
2. Run `dhd cleanup` to stop/uninstall the daemon, remove the cached keychain password, and delete local data under `~/.daemon-hound/`
3. Backup mode files for that machine UUID remain in the vault but will no longer be updated

```bash
dhd cleanup
# or, for automation:
dhd cleanup --force
```

Sync mode files are unaffected — other machines continue to use them normally.

---

## Flow 12: Background Daemon Operations

`dhd init` installs the daemon automatically when the platform supports user-level services. Manual `dhd sync` remains available, but the daemon handles routine sync cycles.

```bash
dhd daemon status
# ✓ Daemon service installed
# ✓ Daemon is running

dhd daemon logs
dhd daemon logs -f
dhd daemon logs -n 100

dhd daemon errors

dhd daemon restart
dhd daemon stop
```

For debugging or manual service setup, run the daemon in the foreground:

```bash
dhd daemon run
```

The daemon watches tracked file directories, watches the local vault clone, polls the remote every 30 seconds, rotates logs, and uses the same user permissions as the CLI.

---

## Flow 13: Conflict Review

Most encrypted vault conflicts are resolved automatically by the sync engine. When the daemon records a conflict for review:

```bash
dhd conflicts list
dhd conflicts show .env.local

dhd conflicts resolve .env.local --strategy local
# or:
dhd conflicts resolve .env.local --strategy remote

dhd sync
dhd conflicts clear
```

`local` keeps this machine's version; `remote` accepts the remote version. `dhd conflicts clear` only removes resolved conflict records.

---

## Audit Log

Every mutating command (`track`, `untrack`, `sync`, `secret set/ref/delete/rename/unref`, `discover`, `export`) is recorded in `~/.daemon-hound/audit.log`:

```
2026-06-14T12:00:00Z  track        github.com/you/pingpong-api:.env.local
2026-06-14T12:01:00Z  sync         push: github.com/you/pingpong-api:.env.local
2026-06-14T12:02:00Z  secret set   openai-key refs=2
```

---

## Summary

| Scenario                          | Command                                    |
|-----------------------------------|--------------------------------------------|
| First-time setup                  | `dhd init --remote <url>`                   |
| Re-initialize machine             | `dhd init --force`                          |
| Track a project file              | `dhd track .env.local`                      |
| Track a machine-only file         | `dhd track ~/.zshrc --mode backup`          |
| Remove from vault                 | `dhd untrack <file>`                        |
| Remove from this machine only     | `dhd untrack --local <file>`                |
| Remove missing local references   | `dhd untrack --missing`                     |
| Push/pull changes                 | `dhd sync`                                  |
| Preview changes                   | `dhd sync --dry-run`                        |
| Sync one namespace only           | `dhd sync --namespace <ns>`                 |
| Set up a new machine (one repo)   | `dhd init` then `dhd sync` in repo dir       |
| Set up a new machine (all repos)  | `dhd init` then `dhd discover ~/`            |
| Lightweight vault clone           | `dhd clone <git-url>`                      |
| Read a file without init          | `dhd read <namespace>:<relPath>`           |
| Read a secret without init        | `dhd read secret:<name>`                    |
| Check sync status                 | `dhd status`                                |
| JSON status (scripting)           | `dhd status --output json`                  |
| Store a secret                    | `dhd secret set <key>`                      |
| Retrieve a secret                 | `dhd secret get <key>`                      |
| List secrets                      | `dhd secret list [key]`                     |
| Rotate a secret                   | `dhd secret set <key>` (new value)          |
| Rename a secret                   | `dhd secret rename <old> <new>`             |
| Map secret to a file              | `dhd secret ref <key> <file> <ENV_VAR>`     |
| Remove a secret mapping           | `dhd secret unref <key> <file>`             |
| Delete a secret                   | `dhd secret delete <key>`                   |
| Health check                      | `dhd doctor`                                |
| Auto-repair common issues         | `dhd doctor --fix`                          |
| List backup machines              | `dhd machines`                              |
| Change master password            | `dhd rekey`                                 |
| Export everything to plaintext    | `dhd export`                                |
| Export identity for new machine   | `dhd export-identity`                       |
| Check daemon                      | `dhd daemon status`                         |
| View daemon logs                  | `dhd daemon logs`                           |
| Resolve conflicts                 | `dhd conflicts resolve <file>`              |
| Remove cached password            | `dhd logout`                                |
| Remove local installation         | `dhd cleanup`                               |
