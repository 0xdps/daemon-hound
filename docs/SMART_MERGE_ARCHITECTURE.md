# Smart Merge Architecture - v1.1.0

## Table of Contents
1. [Overview](#overview)
2. [The Problem](#the-problem)
3. [The Solution](#the-solution)
4. [Architecture](#architecture)
5. [Merge Drivers](#merge-drivers)
6. [Integration with Daemon](#integration-with-daemon)
7. [User Workflow](#user-workflow)
8. [Examples](#examples)
9. [Technical Details](#technical-details)
10. [Future Enhancements](#future-enhancements)

---

## Overview

daemon-hound v1.1.0 introduces **intelligent conflict resolution** for encrypted vault files. When two machines concurrently edit tracked files, the daemon automatically merges them where possible, dramatically reducing manual intervention.

### Key Innovation

Instead of treating encrypted files as opaque binary blobs (which cannot be meaningfully merged), daemon-hound:
1. **Decrypts** both conflicted versions
2. **Applies smart, format-aware merging** (state TOML, environment variables, JSON, CSV, plain text)
3. **Re-encrypts** the merged result
4. **Records unresolvable conflicts** for user review

This transforms conflicts from **"impossible without manual resolution"** to **"auto-resolved when non-overlapping, requires user input only for true divergence"**.

---

## The Problem

### Traditional Git Merge (Broken for Encrypted Files)

```
Machine A: state.toml.age (encrypted blob A)
         ↓ (git merge)
Machine B: state.toml.age (encrypted blob B)
         ↓
git checkout --ours → Takes Machine A's encrypted blob
                    → Discards ALL changes from Machine B ✗
```

**Issue**: You can't meaningfully merge encrypted binary files. The merge driver has no semantic understanding of what changed, so it must choose one or the other entirely.

### Real-World Scenario (Before v1.1.0)

```
Machine A (Dev):
  - Adds file: src/config.prod.js
  - Commits to vault state
  
Machine B (Ops):
  - Adds secret: DB_PASSWORD
  - Commits to vault state

Next sync:
  git merge conflict on state.toml.age ✗
  
User must manually:
  1. Choose one version
  2. Manually re-add the other side's changes
  3. Re-encrypt
  4. Commit and push
```

This is **tedious, error-prone, and defeats the purpose of automated sync**.

---

## The Solution

### Decrypt → Semantic Merge → Re-encrypt

```
Conflicted File Detected
         ↓
git show :1:state.toml.age → Decrypt → Base State (ancestor)
git show :2:state.toml.age → Decrypt → Local State (our changes)
git show :3:state.toml.age → Decrypt → Remote State (their changes)
         ↓
merge.Registry.Resolve(filename, base, local, remote)
         ↓
    [Smart Driver Selection]
         ↓
    STATE DRIVER
    ├─ Files map: "namespace:relPath" → TrackedFile
    │  ├─ One side added file → Include both ✓
    │  ├─ Both sides added same file → Merge metadata ✓
    │  └─ Both sides changed same file → Conflict ✗
    │
    └─ Secrets map: "secret_name" → Secret
       ├─ One side added secret → Include both ✓
       └─ Both sides changed same secret value → Conflict ✗
         ↓
Result: Merged ✓
         ↓
vault.Encrypt(merged) → Re-encrypted blob
         ↓
git add state.toml.age → Automatically marked resolved
         ↓
Daemon continues sync without user intervention
```

---

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│ Daemon Runner (internal/daemon/runner.go)                   │
│                                                             │
│  Responsibilities:                                          │
│  - Detect merge conflicts via git                           │
│  - Load vault identity for decryption                       │
│  - Coordinate smart merge pipeline                          │
│  - Record unresolved conflicts for user                     │
│  - Log all merge attempts                                   │
└─────────────────────────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────────────────────────┐
│ Git Client (internal/git/git.go)                            │
│                                                             │
│  Methods:                                                   │
│  - GetConflictVersions(file) → (base, local, remote)        │
│  - StageFile(file, content) → marks resolved                │
│  - HasConflicts() → bool                                    │
│  - GetConflictedFiles() → []string                          │
└─────────────────────────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────────────────────────┐
│ Vault Storage (internal/storage/vault.go)                   │
│                                                             │
│  Methods:                                                   │
│  - Decrypt(ciphertext) → plaintext                          │
│  - Encrypt(plaintext) → ciphertext                          │
│  - Identity management via age                              │
└─────────────────────────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────────────────────────┐
│ Merge Registry (internal/merge/driver.go)                   │
│                                                             │
│  Responsibilities:                                          │
│  - Select appropriate driver by filename                    │
│  - Orchestrate 3-way merge                                  │
│  - Return Result: Merged | HasConflict | Unsupported        │
│                                                             │
│  Drivers:                                                   │
│  ├─ StateDriver    (state.toml.age)                         │
│  ├─ EnvDriver      (.env, .env.*, .envrc)                   │
│  ├─ JSONDriver     (*.json)                                 │
│  ├─ CSVDriver      (*.csv)                                  │
│  └─ TextDriver     (everything else)                        │
└─────────────────────────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────────────────────────┐
│ Conflict Store (internal/conflicts/conflicts.go)            │
│                                                             │
│  File: ~/.dh/conflicts.json                                 │
│  Methods:                                                   │
│  - Add(conflict) → persist to disk                          │
│  - List() → all conflicts                                   │
│  - Pending() → unresolved only                              │
│  - Resolve(file, strategy) → mark as resolved               │
│  - Delete(file) → clean up                                  │
└─────────────────────────────────────────────────────────────┘
            ↓
┌─────────────────────────────────────────────────────────────┐
│ CLI Commands (internal/cmd/conflicts.go)                    │
│                                                             │
│  - dh conflicts list                                        │
│  - dh conflicts show <file>                                 │
│  - dh conflicts resolve <file> --strategy local|remote      │
│  - dh conflicts clear                                       │
└─────────────────────────────────────────────────────────────┘
```

---

## Merge Drivers

Each driver understands the semantics of a specific file format and performs intelligent 3-way merging.

### 1. StateDriver (`internal/merge/state.go`)

**Handles**: `state.toml.age`, `state.toml`

**Purpose**: Merge vault metadata (tracked files + secrets)

**Algorithm**: Structural TOML merge at the key level

```toml
# Base (common ancestor)
[files]
"github.com/myorg/repo:src/main.go" = { checksum = "abc123", ... }

[secrets]
db_password = { value = <encrypted>, updated_at = "2026-06-14T10:00:00Z" }
```

**Merge Rules**:

| Scenario | Result |
|----------|--------|
| Machine A adds file `app:config.js`, Machine B adds secret `API_KEY` | ✅ Both merged - they touch different keys |
| Machine A adds file `app:config.js`, Machine B adds file `app:main.js` | ✅ Both merged - different files in same namespace |
| Machine A changes file `app:config.js` checksum, Machine B doesn't touch it | ✅ Take A's version |
| Machine A changes file `app:config.js` checksum, Machine B changes it differently | ❌ Conflict - both changed same file |
| Machine A deletes secret `db_password`, Machine B modifies it | ❌ Conflict - one deleted, one modified |
| Machine A and Machine B both add secret `API_KEY` with same value/timestamp | ✅ Merged - identical change |
| Machine A and Machine B both add secret `API_KEY` with different values | ❌ Conflict - diverged values |

**Implementation**:
- Decode TOML into `VaultState` struct
- Compare `Files` map keys individually
- Compare `Secrets` map keys individually  
- Use `checksum` field to detect changes (for files)
- Use `updated_at` timestamp to detect changes (for secrets)
- Merge logic: unchanged on both sides → keep; only one side changed → take that; both changed identically → keep; both changed differently → conflict

---

### 2. EnvDriver (`internal/merge/env.go`)

**Handles**: `.env`, `.env.local`, `.env.production`, `production.env`, `.envrc`

**Purpose**: Merge environment variable files

**Format**:
```bash
# Comments and blank lines are ignored
KEY=value
ANOTHER_KEY="quoted value"
DATABASE_URL=postgres://...
```

**Merge Rules**:

| Scenario | Result |
|----------|--------|
| Machine A adds `API_KEY=key123`, Machine B adds `LOG_LEVEL=debug` | ✅ Both merged - different keys |
| Machine A changes `DB_PASS=abc`, Machine B doesn't touch it | ✅ Take A's version |
| Machine A sets `DB_PASS=abc123`, Machine B sets `DB_PASS=xyz789` | ❌ Conflict - same key, different values |
| Machine A sets `DB_PASS=secret`, Machine B also sets `DB_PASS=secret` | ✅ Merged - both set same value |
| Machine A deletes `DB_PASS`, Machine B modifies it | ❌ Conflict - one deleted, one modified |

**Implementation**:
- Parse as ordered key=value pairs (preserve order)
- Skip comments (#) and blank lines
- Compare by key name
- Merge logic: new keys from both sides → include both; same key changed to same value → include once; same key changed differently → conflict

---

### 3. JSONDriver (`internal/merge/json.go`)

**Handles**: `*.json`

**Purpose**: Deep recursive merge of JSON objects

**Example**:

```json
{
  "version": "1.0",
  "database": {
    "host": "localhost",
    "port": 5432
  },
  "features": {
    "auth": true,
    "cache": false
  }
}
```

**Merge Rules**:

| Scenario | Result |
|----------|--------|
| Machine A adds `features.mfa`, Machine B adds `features.sso` | ✅ Both merged - different fields |
| Machine A modifies `database.host`, Machine B doesn't touch it | ✅ Take A's version |
| Machine A sets `database.host = "db.prod"`, Machine B sets it to `"db.staging"` | ❌ Conflict - same leaf changed |
| Machine A adds `cache: true`, Machine B adds `cache: true` | ✅ Merged - same value |
| Machine A adds `cache: true`, Machine B adds `cache: false` | ❌ Conflict - different values |

**Implementation**:
- Unmarshal JSON into `interface{}`
- Recursively merge object fields
- For leaf values, use 3-way pick: if both sides changed to same value → keep; if only one changed → take that; if both changed differently → conflict
- Arrays handled as scalar values (3-way pick, not element-by-element merge)

---

### 4. CSVDriver (`internal/merge/csv.go`)

**Handles**: `*.csv`

**Purpose**: Row and column-aware merge

**Example**:

```csv
user_id,name,email,active
1,Alice,alice@example.com,true
2,Bob,bob@example.com,true
```

**Merge Rules**:

| Scenario | Result |
|----------|--------|
| Machine A adds row `user_id=5`, Machine B adds row `user_id=6` | ✅ Both rows merged - different primary keys |
| Machine A adds column `department`, Machine B adds column `region` | ✅ Both columns merged - column union |
| Machine A changes cell (user_id=2, email), Machine B doesn't touch that row | ✅ Take A's version |
| Machine A changes cell (user_id=2, email) to `bob_new@example.com`, Machine B to `bob_old@example.com` | ❌ Conflict - same cell changed |
| Machine A deletes row `user_id=2`, Machine B adds a new column | ✅ Merged - they touch different things |
| Machine A deletes row `user_id=2`, Machine B modifies it | ❌ Conflict - one deleted, one modified |

**Implementation**:
- Parse CSV with standard library
- Identify rows by first-column value (primary key)
- Merge headers: union of all columns in stable order
- For each row: merge cells column-by-column
- Row order: base rows first, then new from local, then new from remote
- Cell-by-cell conflict detection

---

### 5. TextDriver (`internal/merge/text.go`)

**Handles**: `.txt`, `.md`, `.yaml`, `.sh`, and everything else

**Purpose**: Generic line-based diff3 merge

**Algorithm**: Longest Common Subsequence (LCS) based 3-way merge

```
Base:
  line 1: import os
  line 2: def main():
  line 3:     print("hello")

Local:
  line 1: import os
  line 2: import sys
  line 3: def main():
  line 4:     print("hello")
  line 5:     return 0

Remote:
  line 1: import os
  line 2: import json
  line 3: def main():
  line 4:     print("hello")
```

**Merge**:
```
import os
import sys    ← Local added (non-overlapping)
import json   ← Remote added (non-overlapping)
def main():
    print("hello")
    return 0  ← Local added (non-overlapping)
```

**Merge Rules**:

| Scenario | Result |
|----------|--------|
| Machine A adds lines 1-3 in section X, Machine B adds lines 1-3 in section Y | ✅ Both merged - non-overlapping |
| Machine A modifies lines 5-7, Machine B doesn't touch them | ✅ Take A's version |
| Machine A modifies lines 5-7, Machine B modifies the same lines differently | ❌ Conflict - overlapping edits |
| Machine A modifies lines 5-7, Machine B modifies lines 8-10 | ✅ Both merged - non-overlapping |

**Implementation**:
- Split content into lines
- Compute Longest Common Subsequence (LCS) between base and each side
- LCS anchors identify unchanged sections
- Hunks between LCS anchors are edits
- Merge hunks: unchanged on both → keep; only one side changed → take that; both changed identically → keep; both changed differently → conflict

---

## Integration with Daemon

### Daemon Sync Flow (Every 30 Seconds)

```
1. performSync() called
   ├─ git pull origin HEAD
   ├─ Check: HasConflicts()?
   │
   ├─ NO conflicts
   │  └─ Continue to push local changes
   │
   └─ HAS conflicts
      ├─ GetConflictedFiles() → ["state.toml.age", "vault/sync/app/.env.age"]
      │
      ├─ resolveConflicts() for each file
      │  │
      │  ├─ git show :1:state.toml.age → base bytes
      │  ├─ git show :2:state.toml.age → local bytes
      │  ├─ git show :3:state.toml.age → remote bytes
      │  │
      │  ├─ vault.Decrypt(base) → plaintext base
      │  ├─ vault.Decrypt(local) → plaintext local
      │  ├─ vault.Decrypt(remote) → plaintext remote
      │  │
      │  ├─ merger.Resolve("state.toml.age", base, local, remote)
      │  │
      │  ├─ StateDriver selected
      │  ├─ Merge: Files & Secrets maps merged independently
      │  │
      │  ├─ Result: Merged ✓
      │  │  ├─ vault.Encrypt(merged) → re-encrypted blob
      │  │  ├─ git.StageFile("state.toml.age", blob)
      │  │  ├─ Log: "Smart-merged state.toml.age successfully"
      │  │  └─ conflicts.Delete("state.toml.age")
      │  │
      │  └─ Result: HasConflict ✗
      │     ├─ recordConflict(filename, localHash, remoteHash)
      │     ├─ Log: "True conflict in state.toml.age — user action required"
      │     └─ [sync pauses, no push]
      │
      ├─ Check: still unresolved conflicts?
      ├─ NO
      │  ├─ git commit "[daemon] Merge conflicts auto-resolved"
      │  ├─ git push origin HEAD
      │  └─ Log: "Sync completed successfully"
      │
      └─ YES
         ├─ Log: "[WAITING] Some conflicts need manual resolution — dh conflicts list"
         └─ Return early (no push, no commit)

2. Next sync (30 seconds later)
   └─ User may have resolved conflicts via: dh conflicts resolve
      └─ Daemon applies user's decision on next poll
```

### Smart Merge Attempt Logging

```
[daemon] Detected local change: vault/sync/app/.env.age
[daemon] Starting sync (vault: git@github.com:org/vault)
[daemon] Pulled from remote
[daemon] Merge conflicts detected
[daemon] Conflicted files: [vault/sync/app/.env.age]
[daemon] [conflict] Attempting smart merge for vault/sync/app/.env.age
[daemon] Smart-merged vault/sync/app/.env.age successfully
[daemon] Committed conflict resolution
[daemon] Pushed to remote
[daemon] Sync completed in 2.34s
```

Or with true conflict:

```
[daemon] Detected local change: vault/sync/app/state.toml.age
[daemon] Starting sync (vault: git@github.com:org/vault)
[daemon] Pulled from remote
[daemon] Merge conflicts detected
[daemon] Conflicted files: [vault/sync/app/state.toml.age]
[daemon] [conflict] True conflict in vault/sync/app/state.toml.age — user action required
[daemon] [WAITING] Some conflicts need manual resolution — dh conflicts list
```

---

## User Workflow

### Scenario 1: Auto-Resolved (Transparent)

```bash
# Background daemon running...
# Machine A commits: adds file src/app.js to vault state
# Machine B commits: adds secret API_KEY to vault state
# Next daemon poll: files and secrets are independent → auto-merged ✓

$ dh daemon logs
[daemon] Smart-merged vault/sync/proj/state.toml.age successfully
```

**User action required**: None! Daemon handled it.

---

### Scenario 2: True Conflict (User Input Needed)

```bash
# Machine A: dh track .env; edits DB_PASS=machine_a_pass
# Machine B: dh track .env; edits DB_PASS=machine_b_pass
# Next daemon poll: same env var changed differently → conflict

# User reviews:
$ dh conflicts list
⚠️  PENDING Conflicts:
[1] vault/sync/myapp/.env.age
    Detected: 2026-06-15 13:30:45

$ dh conflicts show vault/sync/myapp/.env.age
Conflict: vault/sync/myapp/.env.age
Status: ⚠️  PENDING RESOLUTION

LOCAL VERSION (this machine):
Hash: a3b2c1d0
Content:
DATABASE_URL=postgres://localhost/db
DB_PASS=machine_a_password
LOG_LEVEL=info

REMOTE VERSION (other machine):
Hash: x9y8z7w6
Content:
DATABASE_URL=postgres://prod.db/db
DB_PASS=machine_b_password
LOG_LEVEL=debug

# User decides to use remote (they're the ops person with prod settings)
$ dh conflicts resolve vault/sync/myapp/.env.age --strategy remote
✓ Conflict resolved with 'remote' strategy

# Next daemon poll applies the decision
$ dh daemon logs
[daemon] Using user-selected strategy 'remote' for vault/sync/myapp/.env.age
[daemon] Merged: replaced with remote version
[daemon] Committed conflict resolution
[daemon] Pushed to remote
```

**User action required**: Review + choose a strategy

---

### Scenario 3: Complex JSON Merge (Auto-Resolved)

```bash
# config.json tracked in vault

# Machine A adds to config.json:
{
  "database": {
    "pool_size": 20
  }
}

# Machine B adds to config.json:
{
  "cache": {
    "ttl": 3600
  }
}

# Next daemon poll:
# JSONDriver deep merges → different top-level keys → ✓

# Result:
{
  "database": {
    "pool_size": 20
  },
  "cache": {
    "ttl": 3600
  }
}
```

**User action required**: None!

---

### Scenario 4: CSV Row Addition (Auto-Resolved)

```bash
# users.csv tracked in vault

# Machine A adds row:
user_id,name,email
3,Charlie,charlie@example.com

# Machine B adds row:
user_id,name,email
4,Diana,diana@example.com

# Next daemon poll:
# CSVDriver recognizes different primary keys → ✓

# Result:
user_id,name,email
3,Charlie,charlie@example.com
4,Diana,diana@example.com
```

**User action required**: None!

---

## Examples

### Example 1: Multi-File Smart Merge

```
Conflicted: state.toml.age, .env.age, config.json

Loop through each:
1. state.toml.age
   ├─ StateDriver selected
   ├─ Result: Merged ✓
   └─ Auto-committed
   
2. .env.age
   ├─ EnvDriver selected
   ├─ Result: Merged ✓
   └─ Auto-committed
   
3. config.json
   ├─ JSONDriver selected
   ├─ Result: Merged ✓
   └─ Auto-committed

All conflicts resolved → daemon commits everything + pushes
```

---

### Example 2: Partial Resolution (Some Auto, Some Manual)

```
Conflicted: state.toml.age, database.yml, backup.tar.gz

Loop through each:
1. state.toml.age
   ├─ StateDriver selected
   ├─ Result: Merged ✓
   
2. database.yml
   ├─ TextDriver selected (YAML is treated as text)
   ├─ Result: HasConflict ✗
   └─ Recorded in conflicts.json
   
3. backup.tar.gz
   ├─ No driver handles binary
   └─ Unsupported, recorded in conflicts.json

Daemon sync pauses (unresolved conflicts remain)

User:
$ dh conflicts show database.yml          # Decide on YAML
$ dh conflicts resolve database.yml --strategy remote
$ dh conflicts show backup.tar.gz         # Binary file, manual only
$ # (must manually resolve binary file outside daemon)
$ # Commit it manually: git add backup.tar.gz && git commit

Next daemon poll:
├─ database.yml now resolved
├─ Auto-merged
└─ Pushed successfully
```

---

## Technical Details

### 1. Conflict Detection

Git marks files as conflicted when all three versions exist in the index:

```bash
$ git ls-files -s
100644 <hash1> 1   state.toml.age  ← stage 1 (base)
100644 <hash2> 2   state.toml.age  ← stage 2 (local)
100644 <hash3> 3   state.toml.age  ← stage 3 (remote)

$ git status --porcelain
UU state.toml.age  ← U = both updated
```

---

### 2. Extracting Conflict Versions

```go
func (c *Client) GetConflictVersions(filename string) (base, local, remote []byte, err error) {
    // Git index stores three versions with stage numbers
    base, _ = exec.Command("git", "show", ":1:" + filename).Output()
    local, _ = exec.Command("git", "show", ":2:" + filename).Output()
    remote, _ = exec.Command("git", "show", ":3:" + filename).Output()
    return
}
```

---

### 3. Re-encryption & Staging

```go
func (r *Runner) resolveConflicts(...) {
    for each conflicted file:
        // 1. Get 3-way versions
        base, local, remote := gc.GetConflictVersions(file)
        
        // 2. Decrypt all three
        plainBase := vault.Decrypt(base)
        plainLocal := vault.Decrypt(local)
        plainRemote := vault.Decrypt(remote)
        
        // 3. Smart merge
        merged, result := merger.Resolve(file, plainBase, plainLocal, plainRemote)
        
        if result == Merged:
            // 4. Re-encrypt
            encrypted := vault.Encrypt(merged)
            
            // 5. Stage (marks conflict as resolved)
            gc.StageFile(file, encrypted)
            
            // 6. Record success
            conflicts.Delete(file)
        else:
            // Record conflict for user
            recordConflict(file, ...)
}
```

---

### 4. Vault Identity Loading (Optional)

The daemon loads the vault identity for smart merge capability, but this is **best-effort**:

```go
// In daemonRunCmd:
_, vault, _, err := loadContext()
if err != nil {
    fmt.Fprintf(os.Stderr, "Warning: could not load vault identity — smart merge disabled\n")
    vault = nil  // Graceful degradation
}
runner, _ := daemon.NewRunner(cfg, vault)
```

If identity cannot be loaded:
- Daemon still runs and syncs
- Conflicts are recorded without smart merge attempts
- User must manually review and resolve

---

### 5. Conflict Store Format

File: `~/.dh/conflicts.json`

```json
[
  {
    "file_path": "vault/sync/myapp/.env.age",
    "local_hash": "a3b2c1d0f5e8a2c7",
    "remote_hash": "x9y8z7w6v5u4t3s2",
    "detected_at": "2026-06-15T13:30:45Z",
    "resolved_at": null,
    "resolution_strategy": ""
  },
  {
    "file_path": "vault/sync/myapp/config.json",
    "local_hash": "b4c3d2e1f6g9h3d8",
    "remote_hash": "y0z1a2b3c4d5e6f7",
    "detected_at": "2026-06-15T13:30:50Z",
    "resolved_at": "2026-06-15T13:31:20Z",
    "resolution_strategy": "remote"
  }
]
```

---

## Future Enhancements

### v1.2.0 Planned

1. **Configurable Merge Strategies**
   ```bash
   dh config daemon.merge_strategy local  # default
   dh config daemon.merge_strategy remote
   dh config daemon.merge_strategy ask    # (interactive, not yet supported)
   ```

2. **Partial Vault Sync**
   ```bash
   dh daemon sync --namespace github.com/myorg/repo
   ```

3. **User Prompts for Conflicts** (not silent auto-record)
   ```bash
   [daemon] Conflict in config.json — press 'r' for remote, 'l' for local, 'a' to abort
   ```

4. **Conflict Retry Policy**
   - Record when conflict detected
   - Retry smart merge after grace period
   - Escalate to user if still unresolved after N attempts

5. **Merge Driver Plugins**
   - YAML-specific driver (structure-aware vs. line-based)
   - TOML-specific driver (schema-aware)
   - YAML frontmatter driver (e.g., Markdown with YAML headers)

6. **Statistics & Reporting**
   ```bash
   dh daemon stats
   Total syncs: 1,234
   Auto-resolved: 892 (72%)
   True conflicts: 42 (3.4%)
   Unsupported: 300 (24%)
   ```

### v2.0.0 Vision

1. **Real-time Collaboration**
   - Operational Transform (OT) or CRDT
   - Live conflict preview as other machines sync

2. **Conflict Preview UI**
   - Browser-based visual diff
   - Side-by-side comparison with merge suggestions

3. **Custom Merge Algorithms**
   - Plugin system for user-provided drivers
   - Lua/WASM for extensibility

4. **Multi-Version History**
   - Keep all three versions for manual review
   - Rollback to earlier conflict resolutions

---

## Conclusion

daemon-hound v1.1.0's smart merge system dramatically reduces the operational burden of managing encrypted multi-machine vaults. By understanding file semantics (TOML, env vars, JSON, CSV, text), it auto-resolves the vast majority of concurrent edits, requiring user intervention only for genuine conflicts.

The system is **transparent** (daemon runs silently for auto-resolved cases), **debuggable** (detailed logs and conflict records), and **flexible** (easy to add new drivers for custom formats).
