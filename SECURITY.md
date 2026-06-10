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
- **Minimal attack surface** — no hosted service, no background daemon, no outbound network calls except to your own Git remote
- **Secrets stay local** — DaemonHound does not exfiltrate data to any third party
- **No telemetry** — no usage data is collected

## Key Material

The age identity key (`~/.dh/identity.age`) is the root of all encryption. It is:

- Generated once at `dh init`
- Never stored in the vault
- Never transmitted anywhere

If this key is lost, encrypted vault data cannot be recovered. You are responsible for backing it up.
