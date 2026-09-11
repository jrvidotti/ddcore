#!/bin/sh
set -e

# Installation script for the ddcore CLI
# Usage: curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh

REPO="jrvidotti/ddcore"
BIN_NAME="ddcore"

# Determine target binary directory
if [ -z "$BIN_DIR" ]; then
    if [ -w "/usr/local/bin" ]; then
        BIN_DIR="/usr/local/bin"
    else
        BIN_DIR="$HOME/.local/bin"
    fi
fi

# Detect operating system
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)
        echo "Error: Operating system '$OS' is not supported by this installer." >&2
        exit 1
        ;;
esac

# Detect processor architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)   ARCH="amd64" ;;
    arm64|aarch64)  ARCH="arm64" ;;
    *)
        echo "Error: Processor architecture '$ARCH' is not supported." >&2
        exit 1
        ;;
esac

TAG="${VERSION:-latest}"
if [ "$TAG" = "latest" ]; then
    DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/ddcore-${OS}-${ARCH}.tar.gz"
else
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/ddcore-${OS}-${ARCH}.tar.gz"
fi

echo "==> Installing ddcore (${OS}/${ARCH})..."
mkdir -p "$BIN_DIR"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ARCHIVE="${TMP_DIR}/ddcore.tar.gz"

if command -v curl >/dev/null 2>&1; then
    HTTP_CODE="$(curl -sSL -w "%{http_code}" -o "$ARCHIVE" "$DOWNLOAD_URL")"
elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$ARCHIVE" "$DOWNLOAD_URL"
    HTTP_CODE="200"
else
    echo "Error: curl or wget is required to download the binary." >&2
    exit 1
fi

if [ ! -s "$ARCHIVE" ] || [ "$HTTP_CODE" = "404" ]; then
    echo "Error: Binary could not be found at $DOWNLOAD_URL." >&2
    echo "Please check if the release exists at https://github.com/${REPO}/releases." >&2
    exit 1
fi

tar -xzf "$ARCHIVE" -C "$TMP_DIR"
if [ ! -f "$TMP_DIR/$BIN_NAME" ]; then
    echo "Error: '$BIN_NAME' binary was not found in the downloaded archive." >&2
    exit 1
fi

mv "$TMP_DIR/$BIN_NAME" "$BIN_DIR/$BIN_NAME"
chmod +x "$BIN_DIR/$BIN_NAME"

echo "==> ddcore installed successfully at $BIN_DIR/$BIN_NAME"

# Check if BIN_DIR is in PATH
case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
        echo ""
        echo "Warning: '$BIN_DIR' is not in your PATH."
        echo "Add it to your shell configuration file (~/.zshrc or ~/.bashrc):"
        echo "  export PATH=\"\$PATH:$BIN_DIR\""
        echo ""
        ;;
esac

"$BIN_DIR/$BIN_NAME" help >/dev/null 2>&1 || true
echo "All set! Run 'ddcore' to see available commands."
