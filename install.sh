#!/bin/sh
set -e

REPO="sunmingdh/lazysm2"
BIN_NAME="lazysm2"
INSTALL_DIR="$HOME/.local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="x86_64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux)  EXT="tar.gz" ;;
  darwin) EXT="tar.gz" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Failed to fetch latest release version."
  exit 1
fi

FILENAME="${BIN_NAME}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/$REPO/releases/download/$LATEST/$FILENAME"

echo "Installing $BIN_NAME $LATEST ($OS/$ARCH)..."

mkdir -p "$INSTALL_DIR"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "$TMP/$FILENAME"
tar -xzf "$TMP/$FILENAME" -C "$TMP"
mv "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

echo "Installed $BIN_NAME to $INSTALL_DIR/$BIN_NAME"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Note: add $INSTALL_DIR to your PATH if not already set:"
     echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc" ;;
esac
