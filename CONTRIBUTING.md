# Contributing to DaemonHound

Thanks for your interest in contributing.

## Development Principles

- Keep the tool simple
- Prefer opinionated defaults over configuration
- Avoid feature creep
- Local-first over cloud-first
- Security over convenience

## Stack

- **Language:** Go
- **Encryption:** [age](https://age-encryption.org/) via `filippo.io/age`
- **CLI framework:** `github.com/spf13/cobra`
- **Config format:** TOML

## Getting Started

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/your-feature`)
3. Make your changes
4. Add tests where appropriate
5. Open a pull request against `trunk`

## Before Opening a PR

- `go build ./...` succeeds
- `go test ./...` passes
- Documentation is updated if behavior changed
- Keep changes focused and small — one concern per PR

## Commit Style

Use conventional commits:

```
feat: add sync conflict resolution
fix: correct encryption key derivation
docs: update namespace architecture docs
chore: bump dependencies
```

## Key Concepts

- **Namespace** — logical project identifier derived from the `origin` Git remote (e.g. `github.com/dps/pingpong-api`)
- **Vault** — the user's private Git repository where encrypted files are stored
- **Machine identity** — a stable UUID generated at `dhd init`, stored in `~/.daemon-hound/config.toml`
- **Age identity key** — the encryption key at `~/.daemon-hound/identity.age`, never committed to the vault

## Feature Requests

Before proposing a new feature, please explain:

- The problem being solved
- The proposed workflow from a user perspective
- Alternatives you considered

Feature requests that expand DaemonHound into enterprise secret management, team collaboration, or SaaS territory will be declined — these are explicit non-goals.

## Reporting Bugs

Use the bug report issue template. Include reproduction steps, your OS, and DaemonHound version.
