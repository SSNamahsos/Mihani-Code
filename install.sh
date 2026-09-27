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
   EXPECTED="$(printf '%s' "$TAG_JSON" | tr ',' '\n' | grep -A 3 "\"name\": *\"$ASSET\"" | grep -o '"size": *[0-9]*' | head -n 1 | grep -o '[0-9]*')" &&
   [ -n "$URL" ]; then
  download "$URL" > "$DEST/mihani.tmp" || true
  GOT="$(wc -c < "$DEST/mihani.tmp" | tr -d ' ')"
  # A dropped connection leaves a TRUNCATED binary that still looks runnable
  # but fails at execution time, so verify the published size before moving it
  # into place. Fall back to go install rather than install a broken file.
  if [ -n "${EXPECTED:-}" ] && [ "$GOT" != "$EXPECTED" ]; then
    rm -f "$DEST/mihani.tmp"
    echo "Incomplete download: got $GOT of $EXPECTED bytes — retrying via source build..." >&2
    go install "github.com/$REPO/cmd/mihani@latest"
  elif [ "$GOT" -lt 1000000 ]; then
    rm -f "$DEST/mihani.tmp"
    echo "Downloaded file is only $GOT bytes — retrying via source build..." >&2
    go install "github.com/$REPO/cmd/mihani@latest"
  else
    mv "$DEST/mihani.tmp" "$DEST/mihani"
  fi
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
