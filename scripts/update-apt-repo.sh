#!/bin/bash
# update-apt-repo.sh
# Updates the APT repository on the gh-pages branch.
# Run this after a new release is published to GitHub.
#
# Usage (local):
#   GITHUB_TOKEN=xxx ./scripts/update-apt-repo.sh v1.1.0
#
# Usage (CI):
#   ./scripts/update-apt-repo.sh ${{ github.event.release.tag_name }}

set -euo pipefail

TAG="${1:-}"
if [ -z "$TAG" ]; then
    echo "Usage: $0 <tag>"
    echo "Example: $0 v1.1.0"
    exit 1
fi

REPO="0xdps/daemon-hound"
APT_DIR="apt"
ARCHITECTURES="amd64 arm64"
COMPONENT="main"
DIST="stable"

# GitHub token for API access and git push
GITHUB_TOKEN="${GITHUB_TOKEN:-${GH_PAT:-}}"
if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GITHUB_TOKEN or GH_PAT must be set"
    exit 1
fi

echo "=== Updating APT repo for ${TAG} ==="

# Create temp workspace
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

# Clone the gh-pages branch (or create if missing)
REPO_URL="https://x-access-token:${GITHUB_TOKEN}@github.com/${REPO}.git"
GIT_DIR="${WORK_DIR}/repo"

if git ls-remote --heads "$REPO_URL" gh-pages | grep -q gh-pages; then
    echo "Cloning existing gh-pages branch..."
    git clone --depth 1 --branch gh-pages "$REPO_URL" "$GIT_DIR"
else
    echo "Creating new gh-pages branch..."
    git clone --depth 1 "$REPO_URL" "$GIT_DIR"
    cd "$GIT_DIR"
    git checkout --orphan gh-pages
    git rm -rf . >/dev/null 2>&1 || true
    cd - >/dev/null
fi

mkdir -p "${GIT_DIR}/${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-amd64"
mkdir -p "${GIT_DIR}/${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-arm64"
mkdir -p "${GIT_DIR}/${APT_DIR}/pool/${COMPONENT}"

cd "$WORK_DIR"

# Download .deb files from the release
echo "Downloading .deb files from release ${TAG}..."
RELEASE_JSON=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" \
    "https://api.github.com/repos/${REPO}/releases/tags/${TAG}")

# Extract .deb download URLs
DEB_URLS=$(echo "$RELEASE_JSON" | grep -o '"browser_download_url": "[^"]*\.deb"' | sed 's/.*"\(https:\/\/[^"]*\)".*/\1/')

if [ -z "$DEB_URLS" ]; then
    echo "Warning: No .deb files found in release ${TAG}"
    exit 0
fi

for url in $DEB_URLS; do
    filename=$(basename "$url")
    echo "  Downloading ${filename}..."
    curl -fsSL -L -H "Authorization: token ${GITHUB_TOKEN}" "$url" -o "${filename}"
    cp "${filename}" "${GIT_DIR}/${APT_DIR}/pool/${COMPONENT}/"
done

cd "$GIT_DIR"

# Generate Packages files for each architecture
echo "Generating Packages files..."
for arch in $ARCHITECTURES; do
    PKG_DIR="${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
    mkdir -p "$PKG_DIR"
    > "${PKG_DIR}/Packages"

    for deb in ${APT_DIR}/pool/${COMPONENT}/*.deb; do
        [ -f "$deb" ] || continue
        # Extract control file info
        control=$(dpkg-deb -f "$deb" 2>/dev/null || true)
        if [ -z "$control" ]; then
            continue
        fi

        # Get package info
        pkg_name=$(dpkg-deb -f "$deb" Package)
        pkg_version=$(dpkg-deb -f "$deb" Version)
        pkg_arch=$(dpkg-deb -f "$deb" Architecture)
        pkg_maintainer=$(dpkg-deb -f "$deb" Maintainer)
        pkg_description=$(dpkg-deb -f "$deb" Description)
        pkg_depends=$(dpkg-deb -f "$deb" Depends 2>/dev/null || true)
        pkg_section=$(dpkg-deb -f "$deb" Section 2>/dev/null || echo "utils")
        pkg_priority=$(dpkg-deb -f "$deb" Priority 2>/dev/null || echo "optional")

        # Only include packages matching this architecture
        if [ "$pkg_arch" != "$arch" ] && [ "$pkg_arch" != "all" ]; then
            continue
        fi

        deb_size=$(stat -f%z "$deb" 2>/dev/null || stat -c%s "$deb" 2>/dev/null)
        deb_md5=$(md5sum "$deb" | cut -d' ' -f1)
        deb_sha1=$(sha1sum "$deb" | cut -d' ' -f1)
        deb_sha256=$(sha256sum "$deb" | cut -d' ' -f1)
        deb_filename="pool/${COMPONENT}/$(basename "$deb")"

        cat >> "${PKG_DIR}/Packages" <<EOF
Package: ${pkg_name}
Version: ${pkg_version}
Architecture: ${pkg_arch}
Maintainer: ${pkg_maintainer}
Filename: ${deb_filename}
Size: ${deb_size}
MD5sum: ${deb_md5}
SHA1: ${deb_sha1}
SHA256: ${deb_sha256}
Section: ${pkg_section}
Priority: ${pkg_priority}
EOF
        if [ -n "$pkg_depends" ]; then
            echo "Depends: ${pkg_depends}" >> "${PKG_DIR}/Packages"
        fi
        echo "Description: ${pkg_description}" >> "${PKG_DIR}/Packages"
        echo "" >> "${PKG_DIR}/Packages"
    done

    # Compress Packages file
    gzip -k -f "${PKG_DIR}/Packages" || true
    echo "  ${arch}: $(grep -c '^Package:' "${PKG_DIR}/Packages" 2>/dev/null || echo 0) packages"
done

# Generate Release file
echo "Generating Release file..."
RELEASE_FILE="${APT_DIR}/dists/${DIST}/Release"

cat > "$RELEASE_FILE" <<EOF
Origin: DaemonHound
Label: DaemonHound APT Repository
Suite: ${DIST}
Codename: ${DIST}
Version: 1.0
Architectures: ${ARCHITECTURES}
Components: ${COMPONENT}
Description: DaemonHound APT repository
Date: $(date -Ru)
EOF

# Add hashes for each architecture
for arch in $ARCHITECTURES; do
    PKG_DIR="${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
    for file in Packages Packages.gz; do
        filepath="${PKG_DIR}/${file}"
        if [ -f "$filepath" ]; then
            size=$(stat -f%z "$filepath" 2>/dev/null || stat -c%s "$filepath" 2>/dev/null)
            md5=$(md5sum "$filepath" | cut -d' ' -f1)
            sha1=$(sha1sum "$filepath" | cut -d' ' -f1)
            sha256=$(sha256sum "$filepath" | cut -d' ' -f1)
            echo " ${md5} ${size} ${COMPONENT}/binary-${arch}/${file}" >> "$RELEASE_FILE"
        fi
    done
done

# Add MD5Sum, SHA1, SHA256 sections properly
echo "" >> "$RELEASE_FILE"
echo "MD5Sum:" >> "$RELEASE_FILE"
for arch in $ARCHITECTURES; do
    PKG_DIR="${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
    for file in Packages Packages.gz; do
        filepath="${PKG_DIR}/${file}"
        if [ -f "$filepath" ]; then
            size=$(stat -f%z "$filepath" 2>/dev/null || stat -c%s "$filepath" 2>/dev/null)
            md5=$(md5sum "$filepath" | cut -d' ' -f1)
            echo " ${md5} ${size} ${COMPONENT}/binary-${arch}/${file}" >> "$RELEASE_FILE"
        fi
    done
done

echo "" >> "$RELEASE_FILE"
echo "SHA1:" >> "$RELEASE_FILE"
for arch in $ARCHITECTURES; do
    PKG_DIR="${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
    for file in Packages Packages.gz; do
        filepath="${PKG_DIR}/${file}"
        if [ -f "$filepath" ]; then
            size=$(stat -f%z "$filepath" 2>/dev/null || stat -c%s "$filepath" 2>/dev/null)
            sha1=$(sha1sum "$filepath" | cut -d' ' -f1)
            echo " ${sha1} ${size} ${COMPONENT}/binary-${arch}/${file}" >> "$RELEASE_FILE"
        fi
    done
done

echo "" >> "$RELEASE_FILE"
echo "SHA256:" >> "$RELEASE_FILE"
for arch in $ARCHITECTURES; do
    PKG_DIR="${APT_DIR}/dists/${DIST}/${COMPONENT}/binary-${arch}"
    for file in Packages Packages.gz; do
        filepath="${PKG_DIR}/${file}"
        if [ -f "$filepath" ]; then
            size=$(stat -f%z "$filepath" 2>/dev/null || stat -c%s "$filepath" 2>/dev/null)
            sha256=$(sha256sum "$filepath" | cut -d' ' -f1)
            echo " ${sha256} ${size} ${COMPONENT}/binary-${arch}/${file}" >> "$RELEASE_FILE"
        fi
    done
done

# Commit and push to gh-pages
echo "Committing and pushing to gh-pages..."
git config user.email "bot@daemonhound.dev"
git config user.name "DaemonHound Bot"
git add -A
git commit -m "Update APT repo for ${TAG}" || echo "No changes to commit"
git push origin gh-pages

echo ""
echo "=== APT repository updated successfully ==="
echo "Repository URL: https://0xdps.github.io/daemon-hound/apt"
echo ""
echo "Users can now add it with:"
echo "  echo 'deb [trusted=yes] https://0xdps.github.io/daemon-hound/apt stable main' | sudo tee /etc/apt/sources.list.d/daemon-hound.list"
echo "  sudo apt update"
echo "  sudo apt install daemon-hound"
