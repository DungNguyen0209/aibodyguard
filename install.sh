#!/bin/sh
set -e

REPO="DungNguyen0209/aibodyguard"
BASE_URL="https://github.com/${REPO}/releases/latest/download"

# Detect OS and arch
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)
    case "$ARCH" in
      x86_64)  BINARY="aibodyguard-linux-amd64" ;;
      aarch64) BINARY="aibodyguard-linux-arm64" ;;
      *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
    esac
    ;;
  Darwin)
    echo "On macOS, install via Homebrew instead:"
    echo ""
    echo "  brew install DungNguyen0209/tap/aibodyguard"
    echo ""
    exit 0
    ;;
  *)
    echo "Unsupported OS: $OS"
    echo "On Windows, install via Scoop instead:"
    echo ""
    echo "  scoop bucket add dung https://github.com/DungNguyen0209/scoop-bucket"
    echo "  scoop install aibodyguard"
    echo ""
    exit 1
    ;;
esac

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DEST="${INSTALL_DIR}/aibodyguard"

echo "Downloading ${BINARY}..."
curl -fsSL "${BASE_URL}/${BINARY}" -o "/tmp/aibodyguard"
chmod +x /tmp/aibodyguard

echo "Installing to ${DEST} (may require sudo)..."
if [ -w "$INSTALL_DIR" ]; then
  mv /tmp/aibodyguard "$DEST"
else
  sudo mv /tmp/aibodyguard "$DEST"
fi

echo ""
echo "aibodyguard installed successfully."
aibodyguard --version
