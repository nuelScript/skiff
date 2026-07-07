#!/bin/sh
# Skiff CLI installer — puts the `skiff` command on your machine (macOS or Linux):
#
#   curl -fsSL https://useskiff.xyz/cli | sh
#
# Prefer Go (any platform)?  go install github.com/nuelScript/skiff@latest
#
# This installs the CLI you use from your laptop. To stand up the Skiff platform
# on a server, use https://useskiff.xyz/install instead.
set -eu

REPO="nuelScript/skiff"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  linux | darwin) ;;
  *) echo "error: unsupported OS '$os' — try: go install github.com/$REPO@latest" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) echo "error: unsupported architecture '$arch'" >&2; exit 1 ;;
esac

asset="skiff-$os-$arch"
echo "→ downloading skiff ($os/$arch)"
tmp="$(mktemp)"
curl -fsSL "https://github.com/$REPO/releases/latest/download/$asset" -o "$tmp" \
  || { echo "error: download failed — is a release published yet?" >&2; rm -f "$tmp"; exit 1; }
chmod +x "$tmp"

# Install to /usr/local/bin if we can, otherwise ~/.local/bin.
dest="/usr/local/bin"
if [ -w "$dest" ]; then
  mv "$tmp" "$dest/skiff"
elif command -v sudo >/dev/null 2>&1; then
  echo "→ installing to $dest (needs sudo)"
  sudo mv "$tmp" "$dest/skiff"
else
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
  mv "$tmp" "$dest/skiff"
fi

echo "✓ installed $("$dest/skiff" version 2>/dev/null || echo skiff) to $dest/skiff"
case ":$PATH:" in
  *":$dest:"*) echo "  run: skiff --help" ;;
  *) echo "  add it to your PATH:  export PATH=\"$dest:\$PATH\"" ;;
esac
