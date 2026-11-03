#!/bin/sh
# DaemonHound installer script
# Usage: curl -fsSL https://daemonhound.dev/install.sh | sh

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

# ─── macOS: prefer Homebrew Cask for signed app bundle ───
if [ "$OS" = "Darwin" ] && command -v brew >/dev/null 2>&1; then
    echo "Homebrew detected. Installing DaemonHound via Cask (recommended)..."
    brew tap 0xdps/packages 2>/dev/null || true
    brew install --cask daemon-hound
    echo ""
    echo "✓ DaemonHound installed successfully via Homebrew Cask!"
    dhd --version 2>/dev/null || dhd --help | head -n 1
    echo ""
    echo "Run 'dhd init' to get started."
    exit 0
fi

# Windows uses .zip, everything else uses .tar.gz
if [ "$OS" = "Windows" ]; then
    EXT="zip"
else
    EXT="tar.gz"
fi

# Determine release version
if [ -n "$VERSION" ]; then
    RELEASE_TAG="v${VERSION#v}"
elif [ -n "$RELEASE_TAG" ]; then
    VERSION="${RELEASE_TAG#v}"
else
    echo "Fetching latest release..."
    # 1. Resolve from GitHub release redirect without API rate-limits
    RELEASE_TAG=$(curl -sIL -o /dev/null -w "%{url_effective}" "https://github.com/${REPO}/releases/latest" 2>/dev/null | sed -E 's|.*/tag/||' | grep -E '^v[0-9]')

    # 2. Fallback to GitHub API (with optional GITHUB_TOKEN)
    if [ -z "$RELEASE_TAG" ]; then
        AUTH_HEADER=""
        [ -n "$GITHUB_TOKEN" ] && AUTH_HEADER="Authorization: Bearer $GITHUB_TOKEN"
        RELEASE_TAG=$(curl -fsSL ${AUTH_HEADER:+-H "$AUTH_HEADER"} "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi

    # 3. Known stable fallback if unauthenticated API calls are blocked (e.g. CI runner rate limits)
    if [ -z "$RELEASE_TAG" ]; then
        RELEASE_TAG="v1.1.5"
    fi
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

# ─── macOS: also download the signed DMG for the notarized app bundle ───
if [ "$OS" = "Darwin" ]; then
    DMG_ARCH="$ARCH"
    case "$ARCH" in
        x86_64) DMG_ARCH="x86_64" ;;
        arm64)  DMG_ARCH="arm64" ;;
    esac
    DMG_NAME="daemon-hound_${VERSION}_macOS_${DMG_ARCH}.dmg"
    DMG_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${DMG_NAME}"
    echo "Downloading signed & notarized DMG: ${DMG_NAME}..."
    curl -fsSL "$DMG_URL" -o "${TMP_DIR}/${DMG_NAME}"

    echo "Mounting DMG..."
    DMG_MOUNT=$(hdiutil attach "${TMP_DIR}/${DMG_NAME}" -nobrowse -readonly -mountrandom /tmp 2>&1 | tail -1 | awk '{print $NF}')
    if [ -z "$DMG_MOUNT" ]; then
        echo "Warning: could not mount DMG, falling back to raw binary"
    else
        trap 'hdiutil detach "$DMG_MOUNT" 2>/dev/null; rm -rf "$TMP_DIR"' EXIT
        echo "Installing signed app bundle to ~/Applications/DaemonHound.app..."
        rm -rf "${HOME}/Applications/DaemonHound.app"
        cp -R "${DMG_MOUNT}/DaemonHound.app" "${HOME}/Applications/DaemonHound.app"
        hdiutil detach "$DMG_MOUNT" 2>/dev/null
        echo "  ✓ Notarized app bundle installed (Verified Developer)"
    fi
fi

# Extract tarball for the CLI binary
echo "Extracting..."
cd "$TMP_DIR"
if [ "$EXT" = "zip" ]; then
    unzip -q "$ASSET_NAME"
else
    tar -xzf "$ASSET_NAME"
fi

# Install binary (symlink into signed bundle on macOS if available)
echo "Installing to ${INSTALL_DIR}..."
BUNDLE_BIN="${HOME}/Applications/DaemonHound.app/Contents/MacOS/dhd"
if [ "$OS" = "Darwin" ] && [ -f "$BUNDLE_BIN" ]; then
    chmod +x "$BUNDLE_BIN"
    rm -f "${INSTALL_DIR}/${BINARY}"
    rm -f "${INSTALL_DIR}/daemon-hound"
    if [ -w "$INSTALL_DIR" ]; then
        ln -sf "$BUNDLE_BIN" "${INSTALL_DIR}/${BINARY}"
        ln -sf "$BUNDLE_BIN" "${INSTALL_DIR}/daemon-hound"
    else
        sudo ln -sf "$BUNDLE_BIN" "${INSTALL_DIR}/${BINARY}"
        sudo ln -sf "$BUNDLE_BIN" "${INSTALL_DIR}/daemon-hound"
    fi
    echo "  ✓ Symlinked /usr/local/bin/dhd → signed app bundle"
    echo "  ✓ Symlinked /usr/local/bin/daemon-hound → signed app bundle"
else
    if [ -w "$INSTALL_DIR" ]; then
        mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
        ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/daemon-hound"
    else
        sudo mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
        sudo ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/daemon-hound"
    fi
    echo "  ✓ Installed dhd (alias: daemon-hound)"
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
