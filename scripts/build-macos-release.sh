#!/bin/bash
set -euo pipefail

ARCH="${1:-${MACOS_ARCH:-}}"
TAG="${2:-${MACOS_RELEASE_TAG:-}}"

if [[ -z "$ARCH" || -z "$TAG" ]]; then
    echo "Usage: $0 <amd64|arm64> <tag>"
    exit 1
fi

if [[ -z "${APPLE_DEVELOPER_IDENTITY:-}" ]]; then
    echo "APPLE_DEVELOPER_IDENTITY is required"
    exit 1
fi
if [[ -z "${KEYCHAIN_PATH:-}" ]]; then
    echo "KEYCHAIN_PATH is required"
    exit 1
fi
if [[ -z "${MACOS_NOTARY_KEY_PATH:-}" || -z "${MACOS_NOTARY_KEY_ID:-}" || -z "${MACOS_NOTARY_ISSUER_ID:-}" ]]; then
    echo "Notarization credentials are required"
    exit 1
fi

case "$ARCH" in
    amd64) ARCH_LABEL="x86_64" ;;
    arm64) ARCH_LABEL="arm64" ;;
    *)
        echo "Unsupported arch: $ARCH"
        exit 1
        ;;
esac

VERSION="${TAG#v}"
COMMIT="$(git rev-parse --short HEAD)"
DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
ROOT_DIR="$(pwd)"
BUILD_DIR="$ROOT_DIR/dist/macos/$ARCH"
APP_DIR="$BUILD_DIR/DaemonHound.app"
BIN_DIR="$BUILD_DIR/bin"
BIN_PATH="$BIN_DIR/dhd"
DMG_PATH="$ROOT_DIR/dist/daemon-hound_${VERSION}_macOS_${ARCH_LABEL}.dmg"
STAGE_DIR="$BUILD_DIR/dmg-root"
PLIST_BUDDY="/usr/libexec/PlistBuddy"

mkdir -p "$BIN_DIR"
rm -rf "$APP_DIR" "$STAGE_DIR" "$DMG_PATH"

GOOS=darwin GOARCH="$ARCH" CGO_ENABLED=0 \
    go build \
    -ldflags "-s -w -X github.com/0xdps/daemon-hound/internal/cmd.version=$VERSION -X github.com/0xdps/daemon-hound/internal/cmd.commit=$COMMIT -X github.com/0xdps/daemon-hound/internal/cmd.date=$DATE" \
    -o "$BIN_PATH" ./cmd/dhd

mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources" "$STAGE_DIR"
cp "$BIN_PATH" "$APP_DIR/Contents/MacOS/dhd"
cp macos/Info.plist "$APP_DIR/Contents/Info.plist"
cp macos/AppIcon.icns "$APP_DIR/Contents/Resources/AppIcon.icns"
chmod +x "$APP_DIR/Contents/MacOS/dhd"

"$PLIST_BUDDY" -c "Set :CFBundleShortVersionString $VERSION" "$APP_DIR/Contents/Info.plist"
"$PLIST_BUDDY" -c "Set :CFBundleVersion $VERSION" "$APP_DIR/Contents/Info.plist"

codesign --force --deep --options runtime --timestamp --keychain "$KEYCHAIN_PATH" --sign "$APPLE_DEVELOPER_IDENTITY" "$APP_DIR"
codesign --verify --deep --strict --verbose=2 "$APP_DIR"

cp -R "$APP_DIR" "$STAGE_DIR/"
ln -s /Applications "$STAGE_DIR/Applications"

hdiutil create -volname "Daemon Hound" -srcfolder "$STAGE_DIR" -ov -format UDZO "$DMG_PATH"
codesign --force --timestamp --keychain "$KEYCHAIN_PATH" --sign "$APPLE_DEVELOPER_IDENTITY" "$DMG_PATH"

xcrun notarytool submit "$DMG_PATH" \
    --key "$MACOS_NOTARY_KEY_PATH" \
    --key-id "$MACOS_NOTARY_KEY_ID" \
    --issuer "$MACOS_NOTARY_ISSUER_ID" \
    --wait

xcrun stapler staple "$DMG_PATH"
spctl -a -vv -t open --context context:primary-signature "$DMG_PATH"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "dmg_path=$DMG_PATH" >> "$GITHUB_OUTPUT"
fi

echo "Created notarized DMG: $DMG_PATH"
