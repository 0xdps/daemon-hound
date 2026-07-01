# Release Secrets Setup

This repository's release workflow requires GitHub Actions secrets for:

- macOS Developer ID signing
- macOS notarization
- release checksum signing
- Homebrew/Scoop publishing
- APT repository signing

## Required secrets

| Secret | Required for | Notes |
|---|---|---|
| `APPLE_DEVELOPER_IDENTITY` | macOS signing | Example: `Developer ID Application: Your Name (TEAMID)` |
| `MACOS_CERT_P12_BASE64` | macOS signing | Base64 of exported `.p12` certificate |
| `MACOS_CERT_PASSWORD` | macOS signing | Password used when exporting the `.p12` |
| `MACOS_KEYCHAIN_PASSWORD` | macOS signing | Temporary password for CI keychain |
| `MACOS_NOTARY_KEY_ID` | notarization | App Store Connect API key ID |
| `MACOS_NOTARY_ISSUER_ID` | notarization | App Store Connect issuer UUID |
| `MACOS_NOTARY_KEY_BASE64` | notarization | Base64 of `AuthKey_XXXXXX.p8` |
| `GPG_PRIVATE_KEY` | release signing | ASCII-armored secret key |
| `GPG_PASSPHRASE` | release signing | Passphrase for the GPG key |
| `GH_PAT` | publishing | PAT with repo access to external formula/bucket repos |
| `APT_GPG_KEY` | APT repo signing | ASCII-armored private key |

## macOS signing setup

### 1. Export Developer ID certificate

In `Keychain Access`:

1. find your `Developer ID Application` certificate
2. export it as `.p12`
3. choose an export password

Then encode it:

```bash
base64 -i Certificates.p12 | pbcopy
```

Save the result as `MACOS_CERT_P12_BASE64`.

Save the export password as `MACOS_CERT_PASSWORD`.

### 2. Set the signing identity name

Use the exact certificate identity string:

```bash
security find-identity -v -p codesigning
```

Example value:

```text
Developer ID Application: Your Name (TEAMID)
```

Save that as `APPLE_DEVELOPER_IDENTITY`.

### 3. Create an App Store Connect API key

In App Store Connect:

1. go to `Users and Access`
2. open `Integrations` → `App Store Connect API`
3. create a key
4. download `AuthKey_<KEY_ID>.p8`

Encode it:

```bash
base64 -i AuthKey_ABC123XYZ.p8 | pbcopy
```

Save:

- `MACOS_NOTARY_KEY_BASE64`
- `MACOS_NOTARY_KEY_ID`
- `MACOS_NOTARY_ISSUER_ID`

### 4. Temporary keychain password

Generate any strong random string:

```bash
openssl rand -base64 24 | pbcopy
```

Save as `MACOS_KEYCHAIN_PASSWORD`.

## GPG checksum signing setup

Export your secret key in ASCII-armored format:

```bash
gpg --armor --export-secret-keys YOUR_KEY_ID | pbcopy
```

Save as `GPG_PRIVATE_KEY`.

If your key has a passphrase, save it as `GPG_PASSPHRASE`.

To find the fingerprint:

```bash
gpg --list-secret-keys --keyid-format LONG
```

The workflow imports the key and automatically uses the imported fingerprint.

## APT repository signing

Export the ASCII-armored private key used for Debian repository metadata signing:

```bash
gpg --armor --export-secret-keys YOUR_APT_KEY_ID | pbcopy
```

Save as `APT_GPG_KEY`.

## GitHub PAT

Create a personal access token with access to:

- `0xdps/homebrew-packages`
- `0xdps/scoop-bucket`

Save it as `GH_PAT`.

## Add secrets to GitHub

In GitHub:

1. open repository `Settings`
2. open `Secrets and variables` → `Actions`
3. add each secret above

## Verify before tagging

Before creating a release tag, confirm:

- Developer ID certificate is valid
- App Store Connect key is active
- `GH_PAT` can push to formula/bucket repos
- GPG key imports successfully locally
- APT key matches the published public key
