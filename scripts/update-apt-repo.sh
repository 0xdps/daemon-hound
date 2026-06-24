#!/bin/bash
# update-apt-repo.sh
# Builds the APT repository into a local directory.
# Output goes to the first argument (default: _site/).
#
# Usage (local):
#   GITHUB_TOKEN=xxx ./scripts/update-apt-repo.sh v1.1.0 _site/
#
# Usage (CI):
#   ./scripts/update-apt-repo.sh "${TAG}"

set -euo pipefail

TAG="${1:-}"
OUT_DIR="${2:-_site}"

if [ -z "$TAG" ]; then
    echo "Usage: $0 <tag> [output-dir]"
    echo "Example: $0 v1.1.0 _site/"
    exit 1
fi

REPO="0xdps/daemon-hound"
APT_DIR="apt"
ARCHITECTURES="amd64 arm64"
COMPONENT="main"
DIST="stable"

GITHUB_TOKEN="${GITHUB_TOKEN:-${GH_PAT:-}}"
if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GITHUB_TOKEN or GH_PAT must be set"
    exit 1
fi

echo "=== Building APT repo for ${TAG} ==="
echo "    Output directory: ${OUT_DIR}"

rm -rf "$OUT_DIR"
mkdir -p "${OUT_DIR}/${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-amd64"
mkdir -p "${OUT_DIR}/${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-arm64"
mkdir -p "${OUT_DIR}/${APT_DIR}/pool/${COMPONENT}"

# ---- html pages ----
cat > "${OUT_DIR}/index.html" <<'PAGE_EOF'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>DaemonHound</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 700px; margin: 60px auto; padding: 0 20px; color: #333; }
h1 { font-size: 2em; margin-bottom: 0.2em; }
.subtitle { color: #666; margin-bottom: 2em; }
a { color: #0366d6; }
</style>
</head>
<body>
<h1>DaemonHound</h1>
<p class="subtitle">Opinionated local config and secret management for developers</p>
<ul>
<li><a href="https://github.com/0xdps/daemon-hound">GitHub Repository</a></li>
<li><a href="apt/">APT Repository</a> &mdash; for Debian/Ubuntu users</li>
</ul>
</body>
</html>
PAGE_EOF

cat > "${OUT_DIR}/${APT_DIR}/index.html" <<'PAGE_EOF'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>DaemonHound APT Repository</title>
<style>
body { font-family: monospace; max-width: 700px; margin: 60px auto; padding: 0 20px; color: #333; }
h2 { border-bottom: 2px solid #eee; padding-bottom: 6px; }
a { color: #0366d6; text-decoration: none; }
a:hover { text-decoration: underline; }
</style>
</head>
<body>
<h2>DaemonHound APT Repository</h2>
<p>Add this repository to your system:</p>
<pre>echo 'deb [trusted=yes] https://0xdps.github.io/daemon-hound/apt stable main' | sudo tee /etc/apt/sources.list.d/daemon-hound.list
sudo apt update
sudo apt install daemon-hound</pre>
<p><a href="pool/">Package Pool</a></p>
<p><a href="dists/">Distributions</a></p>
<p><a href="..">&larr; Back to DaemonHound</a></p>
</body>
</html>
PAGE_EOF

# ---- Download .deb files from the release ----
echo "Downloading .deb files from release ${TAG}..."
RELEASE_JSON=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" \
    "https://api.github.com/repos/${REPO}/releases/tags/${TAG}")

DEB_URLS=$(echo "$RELEASE_JSON" | grep -o '"browser_download_url": "[^"]*\.deb"' | sed 's/.*"\(https:\/\/[^"]*\)".*/\1/')

if [ -z "$DEB_URLS" ]; then
    echo "Warning: No .deb files found in release ${TAG} -- APT repo will be empty"
fi

TMP_DOWNLOAD=$(mktemp -d)
trap 'rm -rf "$TMP_DOWNLOAD"' EXIT

for url in $DEB_URLS; do
    filename=$(basename "$url")
    echo "  Downloading ${filename}..."
    curl -fsSL -L -H "Authorization: token ${GITHUB_TOKEN}" "$url" -o "${TMP_DOWNLOAD}/${filename}"
    cp "${TMP_DOWNLOAD}/${filename}" "${OUT_DIR}/${APT_DIR}/pool/${COMPONENT}/"
done

# ---- Generate Packages files ----
echo "Generating Packages files..."
for arch in $ARCHITECTURES; do
    PKG_DIR="${OUT_DIR}/${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
    mkdir -p "$PKG_DIR"
    > "${PKG_DIR}/Packages"

    for deb_path in "${OUT_DIR}/${APT_DIR}/pool/${COMPONENT}/"*.deb; do
        [ -f "$deb_path" ] || continue
        pkg_name=$(dpkg-deb -f "$deb_path" Package 2>/dev/null || true)
        [ -z "$pkg_name" ] && continue

        pkg_version=$(dpkg-deb -f "$deb_path" Version)
        pkg_arch=$(dpkg-deb -f "$deb_path" Architecture)
        pkg_maintainer=$(dpkg-deb -f "$deb_path" Maintainer)
        pkg_description=$(dpkg-deb -f "$deb_path" Description)
        pkg_depends=$(dpkg-deb -f "$deb_path" Depends 2>/dev/null || true)
        pkg_section=$(dpkg-deb -f "$deb_path" Section 2>/dev/null || echo "utils")
        pkg_priority=$(dpkg-deb -f "$deb_path" Priority 2>/dev/null || echo "optional")

        if [ "$pkg_arch" != "$arch" ] && [ "$pkg_arch" != "all" ]; then
            continue
        fi

        deb_size=$(stat -c%s "$deb_path")
        deb_md5=$(md5sum "$deb_path" | cut -d' ' -f1)
        deb_sha1=$(sha1sum "$deb_path" | cut -d' ' -f1)
        deb_sha256=$(sha256sum "$deb_path" | cut -d' ' -f1)
        deb_filename="pool/${COMPONENT}/$(basename "$deb_path")"

        {
            echo "Package: ${pkg_name}"
            echo "Version: ${pkg_version}"
            echo "Architecture: ${pkg_arch}"
            echo "Maintainer: ${pkg_maintainer}"
            echo "Filename: ${deb_filename}"
            echo "Size: ${deb_size}"
            echo "MD5sum: ${deb_md5}"
            echo "SHA1: ${deb_sha1}"
            echo "SHA256: ${deb_sha256}"
            echo "Section: ${pkg_section}"
            echo "Priority: ${pkg_priority}"
            [ -n "$pkg_depends" ] && echo "Depends: ${pkg_depends}"
            echo "Description: ${pkg_description}"
            echo ""
        } >> "${PKG_DIR}/Packages"
    done

    gzip -k -f "${PKG_DIR}/Packages" || true
    PKG_COUNT=$(grep -c '^Package:' "${PKG_DIR}/Packages" 2>/dev/null || echo 0)
    echo "  ${arch}: ${PKG_COUNT} packages"
done

# ---- Generate Release file ----
echo "Generating Release file..."
RELEASE_FILE="${OUT_DIR}/${APT_DIR}/dists/${DIST}/Release"

{
    echo "Origin: DaemonHound"
    echo "Label: DaemonHound APT Repository"
    echo "Suite: ${DIST}"
    echo "Codename: ${DIST}"
    echo "Version: 1.0"
    echo "Architectures: ${ARCHITECTURES}"
    echo "Components: ${COMPONENT}"
    echo "Description: DaemonHound APT repository for Debian/Ubuntu"
    echo "Date: $(date -Ru)"
} > "$RELEASE_FILE"

for section in MD5Sum SHA1 SHA256; do
    echo "" >> "$RELEASE_FILE"
    echo "${section}:" >> "$RELEASE_FILE"
    for arch in $ARCHITECTURES; do
        dir="${OUT_DIR}/${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
        for f in Packages Packages.gz; do
            fp="${dir}/${f}"
            [ -f "$fp" ] || continue
            sz=$(stat -c%s "$fp")
            case "$section" in
                MD5Sum) hsh=$(md5sum "$fp" | cut -d' ' -f1) ;;
                SHA1)   hsh=$(sha1sum "$fp" | cut -d' ' -f1) ;;
                SHA256) hsh=$(sha256sum "$fp" | cut -d' ' -f1) ;;
            esac
            echo " ${hsh} ${sz} ${COMPONENT}/binary-${arch}/${f}" >> "$RELEASE_FILE"
        done
    done
done

echo ""
echo "=== APT repository built in ${OUT_DIR}/ ==="
echo "    Root:  ${OUT_DIR}/index.html"
echo "    APT:   ${OUT_DIR}/${APT_DIR}/dists/${DIST}/Release"
echo ""
echo "Users can add it with:"
echo "  echo 'deb [trusted=yes] https://0xdps.github.io/daemon-hound/apt stable main' | sudo tee /etc/apt/sources.list.d/daemon-hound.list"
echo "  sudo apt update"
echo "  sudo apt install daemon-hound"
