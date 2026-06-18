# Security Policy

## Supported Versions

Only the latest release is supported with security fixes.

| Version | Supported |
| ------- | --------- |
| Latest  | ✅        |
| Older   | ❌        |

## Reporting a Vulnerability

**Please do not disclose security vulnerabilities publicly.**

Open a [private security report](../../security/advisories/new) on GitHub and include:

- Description of the vulnerability
- Potential impact
- Steps to reproduce
- Suggested mitigation (if any)

Reports will be acknowledged as quickly as possible. Please allow reasonable time for a fix before any public disclosure.

## Security Philosophy

- **Encryption by default** — all tracked files and secrets are encrypted at rest using [age](https://age-encryption.org/)
- **User-controlled keys** — the age identity key lives at `~/.dh/identity.age` and never leaves your machine
- **Minimal hosted attack surface** — no hosted service, dashboard, telemetry, or SaaS control plane
- **User-scoped daemon** — the optional background sync daemon runs as the current user and only syncs with your configured Git vault remote
- **Secrets stay local** — DaemonHound does not exfiltrate data to any third party
- **No telemetry** — no usage data is collected

## Key Material

The age identity key (`~/.dh/identity.age`) is the root of all encryption. It is:

- Generated once at `dh init`
- Never stored in the vault
- Never transmitted anywhere

If this key is lost, encrypted vault data cannot be recovered. You are responsible for backing it up.

To set up another machine against the same vault, export the identity from an existing machine with `dh export-identity` and provide it to `dh init --age-key <key>` on the new machine. Treat the exported key as plaintext secret material.

## Background Daemon

`dh init` attempts to install a user-level background daemon on supported platforms:

- macOS: launchd user agent
- Linux: systemd user service
- Windows: Task Scheduler task

The daemon watches tracked file locations and the local vault clone, polls the configured Git remote, and writes logs under `~/.dh/`. It does not run with elevated privileges and does not contact any network service other than the Git remote configured by the user.

Use `dh daemon status`, `dh daemon logs`, and `dh daemon errors` to inspect daemon state. Use `dh cleanup` to stop/uninstall the daemon and remove local DaemonHound data from a machine.
