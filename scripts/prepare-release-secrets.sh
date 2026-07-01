#!/bin/bash
set -euo pipefail

usage() {
    cat <<'EOF'
Usage:
  scripts/prepare-release-secrets.sh macos-cert <Certificates.p12>
  scripts/prepare-release-secrets.sh notary-key <AuthKey_ABC123XYZ.p8>
  scripts/prepare-release-secrets.sh gpg-key <KEY_ID>
  scripts/prepare-release-secrets.sh apt-key <KEY_ID>
  scripts/prepare-release-secrets.sh identity
  scripts/prepare-release-secrets.sh keychain-password

Outputs the value to paste into GitHub Actions secrets.
EOF
}

require_file() {
    local path="$1"
    if [[ ! -f "$path" ]]; then
        echo "File not found: $path" >&2
        exit 1
    fi
}

cmd="${1:-}"
arg="${2:-}"

case "$cmd" in
    macos-cert)
        require_file "$arg"
        base64 -i "$arg"
        ;;
    notary-key)
        require_file "$arg"
        base64 -i "$arg"
        ;;
    gpg-key)
        if [[ -z "$arg" ]]; then
            echo "Missing GPG key ID" >&2
            exit 1
        fi
        gpg --armor --export-secret-keys "$arg"
        ;;
    apt-key)
        if [[ -z "$arg" ]]; then
            echo "Missing APT key ID" >&2
            exit 1
        fi
        gpg --armor --export-secret-keys "$arg"
        ;;
    identity)
        security find-identity -v -p codesigning
        ;;
    keychain-password)
        openssl rand -base64 24
        ;;
    *)
        usage
        exit 1
        ;;
esac
