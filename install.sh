#!/bin/sh
# DaemonHound installer script
# Usage: curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/trunk/install.sh | sh

set -e

REPO="0xdps/daemon-hound"
BINARY="dhd"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux) OS="Linux" ;;
    darwin) OS="Darwin" ;;
    mingw*|msys*|cygwin*) OS="Windows" ;;
    freebsd) OS="Freebsd" ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    armv7l|armhf) ARCH="armv7" ;;
    i386|i686) ARCH="i386" ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Windows uses .zip, everything else uses .tar.gz
if [ "$OS" = "Windows" ]; then
    EXT="zip"
else
    EXT="tar.gz"
fi

# Fetch latest release version
echo "Fetching latest release..."
RELEASE_TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$RELEASE_TAG" ]; then
    echo "Failed to fetch latest release version"
    exit 1
fi
VERSION=${RELEASE_TAG#v}

echo "Latest version: ${RELEASE_TAG}"

# Construct download URL
ASSET_NAME="daemon-hound_${VERSION}_${OS}_${ARCH}.${EXT}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${ASSET_NAME}"

echo "Downloading ${ASSET_NAME}..."
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ASSET_NAME}"

# Extract
echo "Extracting..."
cd "$TMP_DIR"
if [ "$EXT" = "zip" ]; then
    unzip -q "$ASSET_NAME"
else
    tar -xzf "$ASSET_NAME"
fi

# Install binary
echo "Installing to ${INSTALL_DIR}..."
if [ -w "$INSTALL_DIR" ]; then
    mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
    sudo mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

# Verify installation
if command -v "$BINARY" >/dev/null 2>&1; then
    echo ""
    echo "✓ DaemonHound installed successfully!"
    echo ""
    "${INSTALL_DIR}/${BINARY}" --version 2>/dev/null || "${INSTALL_DIR}/${BINARY}" --help | head -n 1
    echo ""
    echo "Run '${BINARY} init' to get started."
else
    echo "Installation complete, but ${BINARY} is not in your PATH."
    echo "You may need to add ${INSTALL_DIR} to your PATH or restart your shell."
fi
