#!/bin/sh
# Install cellp + cellpd + celld + offshoot from GitHub Releases.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
# Pin a tag:
#   CELLP_VERSION=v0.1.0 curl -fsSL … | sh
# Private repo / rate limit:
#   GH_TOKEN=… curl -fsSL … | sh
set -eu

REPO="${CELLP_REPO:-KonghaYao/cellp}"
DEST="${CELLP_INSTALL_DIR:-${HOME}/.local/bin}"
TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) echo "Windows is not a native celld target yet. Use WSL, macOS, Linux, or Docker." >&2; exit 1 ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac

auth_hdr=""
if [ -n "$TOKEN" ]; then
  auth_hdr="Authorization: Bearer ${TOKEN}"
fi

if command -v curl >/dev/null 2>&1; then
  api() {
    if [ -n "$auth_hdr" ]; then
      curl -fsSL -H "$auth_hdr" -H "Accept: application/vnd.github+json" "$1"
    else
      curl -fsSL -H "Accept: application/vnd.github+json" "$1"
    fi
  }
  fetch() {
    if [ -n "$auth_hdr" ]; then
      curl -fsSL -H "$auth_hdr" -o "$2" "$1"
    else
      curl -fsSL -o "$2" "$1"
    fi
  }
elif command -v wget >/dev/null 2>&1; then
  api() {
    if [ -n "$TOKEN" ]; then
      wget -qO- --header="$auth_hdr" --header="Accept: application/vnd.github+json" "$1"
    else
      wget -qO- --header="Accept: application/vnd.github+json" "$1"
    fi
  }
  fetch() {
    if [ -n "$TOKEN" ]; then
      wget -qO "$2" --header="$auth_hdr" "$1"
    else
      wget -qO "$2" "$1"
    fi
  }
else
  echo "need curl or wget" >&2
  exit 1
fi

tag="${CELLP_VERSION:-}"
if [ -z "$tag" ]; then
  json=$(api "https://api.github.com/repos/${REPO}/releases/latest")
  tag=$(printf '%s' "$json" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
fi
if [ -z "$tag" ]; then
  echo "could not read latest release for ${REPO}" >&2
  echo "create a GitHub Release (tag v*) so binaries exist, or set CELLP_VERSION=vX.Y.Z" >&2
  exit 1
fi

asset="cellp_${tag}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "downloading ${url}"
fetch "$url" "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$DEST"
installed=0
for b in cellp cellpd celld offshoot esbuild; do
  if [ -f "$tmp/$b" ]; then
    install -m 755 "$tmp/$b" "$DEST/$b"
    echo "installed $DEST/$b"
    installed=$((installed + 1))
  fi
done
if [ "$installed" -lt 1 ]; then
  echo "archive had no binaries" >&2
  exit 1
fi

echo
echo "Add to PATH if needed:"
echo "  export PATH=\"${DEST}:\$PATH\""
echo
echo "Then:"
echo "  cellp doctor"
echo "  cellp dev"
echo
echo "Docs: https://konghayao.github.io/cellp/guides/install"
