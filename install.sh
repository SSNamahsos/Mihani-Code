#!/usr/bin/env sh
# Mihani Code installer for Linux and macOS.
# Downloads the latest release binary, or falls back to building from source.
set -eu

REPO="SSNamahsos/Mihani-Code"
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux*) OS_NAME="linux" ;;
  Darwin*) OS_NAME="darwin" ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH_NAME="amd64" ;;
  aarch64|arm64) ARCH_NAME="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

DEST="$HOME/.mihani/bin"
ASSET="mihani-$OS_NAME-$ARCH_NAME"
mkdir -p "$DEST"

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    echo "need curl or wget" >&2
    return 1
  fi
}

echo "Installing Mihani Code..."
if TAG_JSON="$(download "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null)" &&
   URL="$(printf '%s' "$TAG_JSON" | grep -o "\"browser_download_url\": *\"[^\"]*/$ASSET\"" | head -n 1 | sed 's/.*"\(https[^"]*\)"/\1/')" &&
   [ -n "$URL" ]; then
  download "$URL" > "$DEST/mihani.tmp" && mv "$DEST/mihani.tmp" "$DEST/mihani"
else
  echo "Release download unavailable — falling back to go install (requires Go 1.24+)..."
  go install "github.com/$REPO/cmd/mihani@latest"
fi

chmod +x "$DEST/mihani"

case ":$PATH:" in
  *":$DEST:"*) ;;
  *)
    echo "Add this to your shell profile to use 'mihani' anywhere:"
    echo "  export PATH=\"\$PATH:$DEST\""
    ;;
esac

echo ""
echo "Done. Run '$DEST/mihani' inside any project directory."
