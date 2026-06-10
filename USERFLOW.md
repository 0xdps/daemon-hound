# DaemonHound User Flows

This document describes the primary user journeys in DaemonHound.

---

## Flow 1: First-Time Setup (Machine 1)

You have a new machine or are using DaemonHound for the first time.

```
dh init --remote git@github.com:you/my-vault.git
```

DaemonHound will:
1. Generate a stable machine UUID → stored in `~/.dh/config.toml`
2. Generate an age identity key → stored in `~/.dh/identity.age`
3. Clone the vault repo locally
4. Prompt for a master password used to encrypt the identity key at rest

> ⚠️ Back up `~/.dh/identity.age` immediately. Without it, encrypted vault data cannot be recovered.

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
2. Encrypts the file using the age identity key
3. Stores it in the vault under `sync/github.com/you/pingpong-api/.env.local.age`
4. Records the local binding: namespace → absolute root path

Push to the vault:

```bash
dh sync
# → Encrypting and pushing 2 files...
# → github.com/you/pingpong-api  .env.local   pushed
# → github.com/you/pingpong-api  .env.test    pushed
# → Done
```

---

## Flow 3: New Machine Setup (Machine 2)

You have a second machine where the project lives at a different path.

### Step 1 — Initialize

```bash
dh init --remote git@github.com:you/my-vault.git
# Enter the same master password used on Machine 1
```

### Step 2a — Sync a single project

```bash
cd ~/work/pingpong-api

dh sync
# → github.com/you/pingpong-api: 2 tracked files found in vault
# →   .env.local   ✓ restored
# →   .env.test    ✓ restored
```

No need to run `dh track` again. The namespace is already registered in the vault from Machine 1.

### Step 2b — Discover and sync all projects at once

If you have many repositories, use `dh discover` instead of syncing each one manually:

```bash
dh discover ~/work
# Scanning ~/work (depth 4)...
#
# github.com/you/pingpong-api   2 tracked files  ✓ synced
# github.com/you/portfolio      1 tracked file   ✓ synced
# github.com/you/side-project   not in vault     – skipped
#
# 2 namespaces restored, 1 skipped
```

---

## Flow 4: Day-to-Day Usage

### Check what's tracked and what's changed

```bash
dh status
# github.com/you/pingpong-api
#   .env.local    clean
#   .env.test     dirty  (modified locally, not yet pushed)
#
# global
#   zshrc         clean  (backup, this machine only)
```

### Push and pull changes

```bash
dh sync
# Pulls any remote changes first, then pushes local dirty files.
```

### Offline behavior

If the vault remote is unreachable, `dh sync` queues changes locally and exits cleanly. The next successful sync pushes them. `dh status` shows pending pushes.

---

## Flow 5: Secret Management

### Step 1 — Store a secret

```bash
dh secret set openai-key
# Enter value: ••••••••••••••
# → Stored: openai-key
```

The secret is stored encrypted in the vault. No file is touched yet.

---

### Step 2 — Map the secret to each repo (one-time, per repo)

Each repo can use a different environment variable name for the same logical secret. You map them independently, once per repo:

```bash
# This repo calls it OPENAI_API_KEY
cd ~/projects/pingpong-api
dh secret ref openai-key .env.local OPENAI_API_KEY
# → Mapped: openai-key → github.com/you/pingpong-api:.env.local:OPENAI_API_KEY

# This repo calls it OPENAI_KEY
cd ~/projects/portfolio
dh secret ref openai-key .env.local OPENAI_KEY
# → Mapped: openai-key → github.com/you/portfolio:.env.local:OPENAI_KEY

# This repo calls it AI_SECRET_KEY
cd ~/projects/worker
dh secret ref openai-key .env.local AI_SECRET_KEY
# → Mapped: openai-key → github.com/you/worker:.env.local:AI_SECRET_KEY
```

DaemonHound writes the secret value into each file at the mapped key and marks those files dirty.

```bash
dh sync
# → Pushing 3 updated files...
```

You never need to re-map these. The mappings are stored in the vault and apply on every machine.

---

### Step 3 — List mappings for a secret

```bash
dh secret list openai-key
# openai-key
#   github.com/you/pingpong-api   .env.local   OPENAI_API_KEY
#   github.com/you/portfolio      .env.local   OPENAI_KEY
#   github.com/you/worker         .env.local   AI_SECRET_KEY
```

---

### Step 4 — Retrieve the raw value

```bash
dh secret get openai-key
# → sk-proj-xxxxxxxxxxxx
```

---

### Step 5 — Rotate the secret

When the key changes, update it once:

```bash
dh secret set openai-key NEW_VALUE
# → Updated .env.local in github.com/you/pingpong-api  (OPENAI_API_KEY)
# → Updated .env.local in github.com/you/portfolio     (OPENAI_KEY)
# → Updated .env.local in github.com/you/worker        (AI_SECRET_KEY)
# → 3 files marked dirty — run `dh sync` to push
```

Each file gets the new value written to its own key name. The mappings handle the differences automatically. Run `dh sync` to push the updated encrypted files to the vault.

---

## Flow 6: Machine Decommission

When retiring a machine:

1. Run `dh sync` to ensure all local changes are pushed
2. Delete `~/.dh/` from the machine
3. Backup mode files for that machine UUID remain in the vault but will no longer be updated

Sync mode files are unaffected — other machines continue to use them normally.

---

## Summary

| Scenario                        | Command                            |
|---------------------------------|------------------------------------|
| First-time setup                | `dh init --remote <url>`           |
| Track a project file            | `dh track .env.local`              |
| Track a machine-only file       | `dh track ~/.zshrc --mode backup`  |
| Push/pull changes               | `dh sync`                          |
| Set up a new machine (one repo) | `dh init` then `dh sync` in repo   |
| Set up a new machine (all repos)| `dh init` then `dh discover ~/`    |
| Check sync status               | `dh status`                        |
| Store a secret                  | `dh secret set <key>`              |
| Rotate a secret                 | `dh secret set <key> NEW_VALUE`    |
