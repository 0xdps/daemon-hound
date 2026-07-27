# DaemonHound Web UI Guide

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Launching the Web UI](#launching-the-web-ui)
4. [Authentication](#authentication)
5. [Pages & Features](#pages--features)
   - [Dashboard](#dashboard)
   - [Live Status](#live-status)
   - [Conflicts](#conflicts)
   - [Tracked Files](#tracked-files)
   - [Secrets](#secrets)
   - [Settings](#settings)
6. [Data Flow](#data-flow)
7. [HTMX SPA Behavior](#htmx-spa-behavior)
8. [Security](#security)
9. [Keyboard Shortcuts](#keyboard-shortcuts)
10. [Technical Details](#technical-details)

---

## Overview

The DaemonHound Web UI is a **server-rendered, HTMX-driven single-page application** that provides a visual interface for managing your encrypted vault. It runs entirely in Go with no client-side JavaScript framework — all interactivity comes from HTMX for partial page updates and vanilla JavaScript for small enhancements.

### Key Characteristics

| Aspect | Detail |
|--------|--------|
| **Rendering** | Server-side Go templates (`html/template`) |
| **Interactivity** | HTMX 1.9.12 for AJAX partial swaps |
| **Styling** | Tailwind CSS + Basecoat CSS (Nova dark theme) |
| **Icons** | Lucide icons |
| **Real-time** | Server-Sent Events (SSE) for log streaming |
| **Auth** | HMAC-signed session cookies, 12-hour TTL |
| **Port** | Auto-detected in range 7734–7800 |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Browser                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Dashboard  │  │  HTMX swaps │  │  SSE Log Stream     │  │
│  │  (overview) │  │  (SPA nav)  │  │  (real-time logs)   │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
└─────────┼────────────────┼────────────────────┼─────────────┘
          │                │                    │
          ▼                ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│                    Go HTTP Server (internal/web)            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Handlers  │  │   Sessions  │  │   Template Engine   │  │
│  │  (9 pages)  │  │  (HMAC+map) │  │  (embed.FS + funcs) │  │
│  └──────┬──────┘  └─────────────┘  └─────────────────────┘  │
└─────────┼─────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│                      Vault & System Layer                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   storage   │  │   conflicts │  │      daemon         │  │
│  │   .Vault    │  │    Store    │  │   (halt/status)     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │     git     │  │    merge    │  │     keychain        │  │
│  │   Client    │  │  Registry   │  │  (password cache)   │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### File Layout

```
internal/web/
├── server.go              # Server struct, routing, middleware, auth
├── handler_auth.go        # Login/logout
├── handler_dashboard.go   # Dashboard overview
├── handler_conflicts.go   # Conflict list, diff, resolution
├── handler_files.go       # File browser and preview
├── handler_secrets.go     # Secret list, view, rotation
├── handler_settings.go    # Settings and untrack
├── handler_status.go      # Status page and SSE log streaming
├── util.go                # File read helper
├── static/
│   └── logo.png           # Brand logo / favicon
└── templates/
    ├── layout.html        # Base layout (sidebar, nav, CSS, JS)
    ├── login.html         # Standalone login page
    ├── dashboard.html     # Dashboard content
    ├── status.html        # Status + log stream
    ├── settings.html      # Settings content
    ├── conflicts/
    │   ├── list.html      # Conflict queue
    │   └── diff.html      # 3-way diff resolver
    ├── files/
    │   ├── list.html      # Namespace file browser
    │   └── view.html      # File content preview
    └── secrets/
        ├── list.html      # Secret vault list
        └── view.html      # Secret inspector + rotation
```

---

## Launching the Web UI

### From the CLI

```bash
# Launch the web UI server
dhd ui

# The server starts on the first available port in range 7734–7800
# Output: http://localhost:7734 (or next available)
```

### What Happens on Launch

1. The `ui` command creates a `web.Server` backed by the vault
2. A random 32-byte HMAC key is generated for session signing
3. The server scans ports 7734–7800 and binds to the first free one
4. Your default browser is automatically opened
5. If not authenticated, you are redirected to `/login`

---

## Authentication

### Login Flow

```
┌─────────┐     GET /login      ┌─────────┐
│ Browser │ ──────────────────► │ Server  │
│         │ ◄────────────────── │         │
│         │   Login form HTML   │         │
└────┬────┘                     └─────────┘
     │
     │ POST /login (password)
     ▼
┌─────────┐  1. Load config → get identity salt
│ Server  │  2. Read encrypted identity file
│         │  3. Attempt decryption with password
│         │  4. If success → store in keychain + issue session
└────┬────┘
     │
     │ Set-Cookie: dhd_session=<hmac-token>
     ▼
┌─────────┐
│ Browser │ ──► Redirect to /
└─────────┘
```

### Password Validation

The web UI does not store a separate password hash. Instead, it validates your password by **attempting to decrypt the identity file** (`~/.dh/identity.age`) using the provided password. This is the same password you use for CLI operations.

### Session Management

| Property | Value |
|----------|-------|
| **Mechanism** | HMAC-signed random token in cookie |
| **Cookie name** | `dhd_session` |
| **Cookie flags** | `HttpOnly`, `SameSite=Strict` |
| **Session key** | 32-byte random per process lifetime |
| **TTL** | 12 hours |
| **Storage** | In-memory map (lost on restart) |
| **HTMX handling** | Unauthenticated requests get `HX-Redirect: /login` |

### Logout

Clicking **Logout** in the sidebar:
1. Deletes the session from the in-memory store
2. Clears the cookie with `MaxAge: -1`
3. Redirects to `/login`

---

## Pages & Features

### Dashboard

**Endpoint:** `GET /`

The Dashboard is the landing page after login. It provides an at-a-glance health overview of your vault.

**Data displayed:**
- **Daemon status** — Running or halted (with reason if halted)
- **Pending conflicts** — Count of unresolved conflicts with quick link
- **Secret count** — Total number of secrets in vault
- **File count** — Total number of tracked files
- **Quick navigation cards** — Links to Conflicts, Files, Secrets, Status

**Behavior:**
- If the daemon is halted, a prominent alert banner appears with the halt reason
- If pending conflicts exist, the count appears as a badge on the sidebar nav
- All metric cards are clickable and navigate to their respective pages

---

### Live Status

**Endpoints:**
- `GET /status` — Status page
- `GET /status/stream` — SSE log stream

The Status page shows the current daemon health and a real-time log tail.

**Data displayed:**
- **Daemon state** — Running or halted with reason
- **Log file path** — Where daemon logs are stored
- **Recent logs** — Last 50 lines from the log file
- **Live stream** — Real-time log lines via SSE

**Real-time Log Streaming (SSE):**
```
Browser ──► GET /status/stream
Server  ──► event: log\ndata: [2024-01-15 10:23:45] Sync completed\n\n
Server  ──► : heartbeat\n\n (every 500ms)
```

The server opens the log file, seeks to the end, and streams new lines as they are written. A heartbeat comment keeps the connection alive. The browser uses HTMX's SSE extension to append lines to the log output.

**Controls:**
- **Autoscroll toggle** — Pause/resume automatic scrolling as new lines arrive
- The log output uses a monospace font with subtle timestamps

---

### Conflicts

**Endpoints:**
- `GET /conflicts` — Conflict queue
- `GET /conflicts/diff?file=<path>` — 3-way diff view
- `POST /conflicts/resolve` — Resolve a conflict

The Conflicts page is where you review and resolve merge conflicts that the daemon could not auto-resolve.

#### Conflict List

**Data displayed:**
- **Pending conflicts** — Files with unresolved conflicts
- **Resolved conflicts** — Files that have been resolved (with timestamp)
- **Halt indicator** — Whether the daemon is paused waiting for resolution

Each conflict shows:
- File path
- Conflict detection timestamp
- Resolution status
- Link to diff viewer

#### Diff Viewer

The diff viewer shows a **3-way comparison** of a conflicted file:

```
┌─────────────┬─────────────┬─────────────┐
│    Base     │    Local    │   Remote    │
│  (ancestor) │  (our side) │ (their side)│
└─────────────┴─────────────┴─────────────┘
```

**For encrypted files:** The server automatically decrypts all three versions before displaying them.

**For `.env` / `state.toml` files:** A **key-level diff** is computed showing per-key status:

| Status | Meaning |
|--------|---------|
| `conflict` | Both sides changed the key differently |
| `local-only` | Only local changed (or remote matches base) |
| `remote-only` | Only remote changed (or local matches base) |
| `same` | Both sides agree |

#### Resolution Strategies

When resolving, you choose one of three strategies:

| Strategy | Action |
|----------|--------|
| **Local** | Keep your local version (`git checkout --ours`) |
| **Remote** | Accept the remote version (`git checkout --theirs`) |
| **Merged** | Re-run smart merge, re-encrypt result, stage file |

After resolution:
1. The conflict is marked resolved in the store
2. The file is staged and committed
3. If no pending conflicts remain, the daemon halt is cleared
4. A success toast appears and the conflict list updates

---

### Tracked Files

**Endpoints:**
- `GET /files` — File browser
- `GET /files/view?key=<namespace:relPath>` — File preview

The Files page lets you browse all tracked files organized by namespace.

#### File List

**Data displayed:**
- Files grouped by **namespace** (e.g., `default`, `production`, `team`)
- Within each namespace, files sorted alphabetically by path
- **Search** — Filter by namespace, path, or mode
- **Counts** — Total files vs. filtered results

**Search behavior:**
- Debounced 300ms after keystroke
- Searches namespace, relative path, and mode
- HTMX partial swap updates the list without full page reload

#### File View

Clicking a file opens the preview:

**Data displayed:**
- File metadata (namespace, path, mode)
- **Decrypted content** in a monospace `<pre>` block
- **Copy button** — Copies content to clipboard

The server decrypts the file on-the-fly using the vault's age key (cached in keychain after login).

---

### Secrets

**Endpoints:**
- `GET /secrets` — Secret list
- `GET /secrets/view?name=<name>` — Secret detail
- `POST /secrets/rotate` — Rotate secret value

The Secrets page manages named secrets stored in the vault.

#### Secret List

**Data displayed:**
- Secret names sorted alphabetically
- Latest version tag (e.g., `v3`)
- Last update timestamp
- **Search** — Filter by secret name

#### Secret View

**Data displayed:**
- Secret name
- **Current value** — Blurred by default, revealed on hover/focus
- **Version history** — Newest first with:
  - Version tag (`v1`, `v2`, ...)
  - Creation timestamp
  - Change reason
  - "Current" indicator

**Actions:**
- **Rotate** — Enter a new value and optional reason
- **Password visibility toggle** — Eye icon to show/hide input
- **Copy name** — Copy secret name to clipboard

**Rotation flow:**
```
1. User enters new value + reason
2. Server encrypts value with age key
3. Auto-increments version tag (v1 → v2 → v3)
4. Saves updated SecretFile
5. Returns toast + updated view
```

---

### Settings

**Endpoints:**
- `GET /settings` — Settings page
- `POST /settings/untrack` — Remove tracked file

The Settings page shows all tracked files and allows untracking.

**Data displayed:**
- All tracked files sorted by namespace → path
- For each file: namespace, relative path, mode (sync/backup)

**Actions:**
- **Untrack** — Removes a file from the vault with confirmation dialog

**Untrack flow:**
```
1. User clicks "Untrack" → confirmation dialog appears
2. On confirm: server calls vault.RemoveFile()
   - Deletes encrypted file from disk
   - Removes entry from state
   - Saves updated state
3. Returns toast + updated table
```

---

## Data Flow

### Request Lifecycle

```
┌─────────┐
│ Request │
└────┬────┘
     │
     ▼
┌─────────────────────────────────────────┐
│ 1. Router (mux) matches path + method   │
└────┬────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────────┐
│ 2. requireAuth middleware               │
│    - Check session cookie               │
│    - Validate HMAC signature            │
│    - Check expiry                       │
│    - HTMX: HX-Redirect on failure       │
└────┬────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────────┐
│ 3. Handler executes                     │
│    - Load data from vault/conflicts/git │
│    - Build page-specific data struct    │
│    - Call s.pageData(active)            │
└────┬────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────────┐
│ 4. Template rendering                   │
│    - Parse layout.html + content template│
│    - Execute "base" template            │
│    - HTMX: renderPartial (content only) │
└────┬────────────────────────────────────┘
     │
     ▼
┌─────────┐
│ Response│
└─────────┘
```

### Handler → Template Data Flow

```go
// Example: Dashboard handler
func (s *Server) handleDashboard(w, r) {
    // 1. Load data from subsystems
    halted := daemon.IsHalted()
    pending := conflictsStore.Pending()
    state := vault.LoadState()

    // 2. Build page-specific struct
    data := dashboardData{
        Halted:       halted,
        HaltReason:   daemon.ReadHaltReason(),
        PendingCount: len(pending),
        SecretCount:  len(state.Secrets),
        FileCount:    len(state.Files),
    }

    // 3. Wrap in pageData (sidebar nav state)
    page := s.pageData("dashboard") // sets Active, PendingCount, DaemonRunning

    // 4. Render
    if isHTMX(r) {
        s.renderPartialWithTitle(w, "dashboard.html", data, "Dashboard")
    } else {
        s.render(w, "dashboard", "dashboard.html", data)
    }
}
```

### Template Execution Flow

```
layout.html (defines "base" template)
  ├── <head> (CSS, JS, meta)
  ├── <aside> Sidebar
  │     ├── Brand header
  │     ├── Navigation links (with active state)
  │     ├── Conflict badge
  │     ├── Daemon status
  │     └── Logout button
  ├── <main id="app-content">
  │     └── {{template "content" .Data}}
  │           └── dashboard.html (or other content template)
  ├── Toast container
  └── Confirm dialog
```

---

## HTMX SPA Behavior

The Web UI behaves like a single-page application without a client-side framework.

### Navigation

All internal links use HTMX attributes:

```html
<a hx-get="/conflicts"
   hx-target="#app-content"
   hx-swap="innerHTML"
   hx-push-url="true">
   Conflicts
</a>
```

**What happens:**
1. User clicks link
2. HTMX intercepts the click
3. Sends `GET /conflicts` with `HX-Request: true` header
4. Server detects HTMX and returns **only** the content template (no layout)
5. HTMX swaps the content into `#app-content`
6. Browser URL updates via `hx-push-url`
7. `syncNav()` updates the active sidebar state

### Full Page vs. Partial Render

| Request Type | Render Function | What Returns |
|--------------|-----------------|--------------|
| Full page load | `render()` | layout.html + content + `<title>` |
| HTMX navigation | `renderPartialWithTitle()` | content only + `HX-Title` header |
| HTMX form submit | `renderPartial()` | content only |
| Login page | `renderLogin()` | login.html (no sidebar) |

### Search & Forms

**Live search** (Files, Secrets):
```html
<input hx-get="/files"
       hx-target="#file-list"
       hx-trigger="keyup changed delay:300ms"
       hx-vals='{"q": this.value}'>
```

**Form submissions** (Resolve, Rotate, Untrack):
```html
<form hx-post="/conflicts/resolve"
      hx-target="#conflict-list"
      hx-swap="outerHTML">
```

### Toast Notifications

Server sends toast triggers via the `HX-Trigger` header:

```go
setHXToast(w, "success", "Resolved file.env using local")
```

Client-side `showToast()` creates a temporary DOM element that auto-dismisses after 4 seconds.

---

## Security

### Authentication & Session Security

1. **Password validation via decryption** — No separate password hash; the identity file itself is the proof
2. **HMAC-signed session tokens** — Cryptographically signed, not just random strings
3. **`HttpOnly`, `SameSite=Strict` cookies** — Protects against XSS and CSRF
4. **12-hour session TTL** — Sessions expire automatically
5. **In-memory session store** — Sessions are lost on server restart

### Secret Protection

1. **Secret blur-reveal** — Secret values are rendered with `.secret-reveal` class that blurs until hovered/focused
2. **No inline secrets in page source** — All secret values are decrypted server-side and rendered in blurred containers
3. **Password manager disabling** — `meta` tag and `data-*` attributes prevent password managers from auto-filling

### Action Confirmation

Destructive actions require explicit confirmation:
- **Untrack** — Custom modal dialog before removing a file
- **Conflict resolution** — Strategy selection prevents accidental resolution

### Input Validation

All handlers validate:
- Required parameters (file path, strategy, secret name)
- Strategy values against allowed set (`local`, `remote`, `merged`)
- Key format (`namespace:relPath` with colon separator)

---

## Keyboard Shortcuts

Press `?` anywhere to show the shortcut reference sheet.

| Shortcut | Action |
|----------|--------|
| `?` | Toggle keyboard shortcut help |
| `G` `D` | Go to Dashboard |
| `G` `C` | Go to Conflicts |
| `G` `L` | Go to Live Status |
| `G` `F` | Go to Tracked Files |
| `G` `S` | Go to Secrets |
| `G` `T` | Go to Settings |

---

## Technical Details

### Template Functions

Available in all templates:

| Function | Signature | Purpose |
|----------|-----------|---------|
| `truncate` | `string, int → string` | Truncate with ellipsis |
| `formatTime` | `time.Time → string` | Format as "2006-01-02 15:04:05" |
| `safeHTML` | `string → template.HTML` | Render pre-escaped HTML |
| `lines` | `string → []string` | Split by newline |

### External Dependencies (CDN)

| Resource | URL | Purpose |
|----------|-----|---------|
| Tailwind CSS | `cdn.tailwindcss.com` | Utility CSS framework |
| Basecoat CSS | `cdn.jsdelivr.net/npm/basecoat-css@1.0.2` | Nova dark theme + components |
| HTMX | `unpkg.com/htmx.org@1.9.12` | AJAX partial page updates |
| HTMX SSE | `unpkg.com/htmx.org/dist/ext/sse.js` | Server-Sent Events extension |
| Basecoat JS | `cdn.jsdelivr.net/npm/basecoat-css@1.0.2/dist/js/all.min.js` | Sidebar, toast, tabs behavior |
| Lucide | `unpkg.com/lucide@latest` | Icon library |

### Route Reference

| Method | Path | Handler | Auth | Description |
|--------|------|---------|------|-------------|
| GET | `/static/*` | FileServer | No | Static assets |
| GET | `/favicon.ico` | Redirect | No | Favicon |
| GET | `/login` | `handleLoginPage` | No | Login form |
| POST | `/login` | `handleLoginSubmit` | No | Password validation |
| POST | `/logout` | `handleLogout` | Yes | Clear session |
| GET | `/` | `handleDashboard` | Yes | Dashboard |
| GET | `/conflicts` | `handleConflictList` | Yes | Conflict queue |
| GET | `/conflicts/diff` | `handleConflictDiff` | Yes | 3-way diff view |
| POST | `/conflicts/resolve` | `handleConflictResolve` | Yes | Resolve conflict |
| GET | `/files` | `handleFileList` | Yes | File browser |
| GET | `/files/view` | `handleFileView` | Yes | File preview |
| GET | `/secrets` | `handleSecretList` | Yes | Secret list |
| GET | `/secrets/view` | `handleSecretView` | Yes | Secret detail |
| POST | `/secrets/rotate` | `handleSecretRotate` | Yes | Rotate secret |
| GET | `/status` | `handleStatus` | Yes | Status page |
| GET | `/status/stream` | `handleStatusStream` | Yes | SSE log stream |
| GET | `/settings` | `handleSettings` | Yes | Settings |
| POST | `/settings/untrack` | `handleUntrack` | Yes | Remove tracked file |

---

## Feature Summary

| Feature | Endpoint | Capabilities |
|---------|----------|--------------|
| **Dashboard** | `/` | Health overview, conflict/file/secret counts, halt alerts, quick navigation |
| **Live Status** | `/status`, `/status/stream` | Real-time SSE log tailing, daemon health, autoscroll toggle |
| **Conflicts** | `/conflicts`, `/conflicts/diff`, `/conflicts/resolve` | Queue view, 3-way diff, key-level diff for `.env` files, local/remote/merged resolution |
| **Tracked Files** | `/files`, `/files/view` | Namespace-grouped browser, search, decrypted content preview, copy |
| **Secrets** | `/secrets`, `/secrets/view`, `/secrets/rotate` | Versioned secret list, value reveal, version history audit trail, rotation with reason |
| **Settings** | `/settings`, `/settings/untrack` | Full tracked file list, destructive untrack with confirmation |
| **Auth** | `/login`, `/logout` | Password unlock, 12-hour sessions, keychain integration |
