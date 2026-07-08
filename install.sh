#!/bin/sh
#
# AgentUp installer for macOS and Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/rockychang7/agentup/main/install.sh | bash
#
# Or with a custom repo:
#   curl -fsSL https://raw.githubusercontent.com/rockychang7/agentup/main/install.sh | bash -s -- --owner rockychang7

set -e

OWNER="rockychang7"
REPO="agentup"

# --- Parse args ---
while [ $# -gt 0 ]; do
    case "$1" in
        --owner) OWNER="$2"; shift 2 ;;
        --repo)  REPO="$2";  shift 2 ;;
        *) shift ;;
    esac
done

# --- Detect OS and architecture ---
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Darwin) OS="darwin" ;;
    Linux)  OS="linux"  ;;
    *)
        echo "ERROR: Unsupported OS: $OS"
        echo "This installer supports macOS and Linux only."
        echo "For Windows, use install.ps1"
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "ERROR: Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Fetching latest release from $OWNER/$REPO..."

# --- Fetch latest release tag ---
TAG=$(curl -fsSL "https://api.github.com/repos/$OWNER/$REPO/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
    echo "ERROR: Could not determine latest release version."
    exit 1
fi

echo "Latest version: $TAG"

# --- Build download URL ---
# goreleaser archive naming: agentup_<version>_<os>_<arch>.tar.gz
VERSION="${TAG#v}"
ARCHIVE_NAME="${REPO}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$OWNER/$REPO/releases/download/${TAG}/${ARCHIVE_NAME}"

echo "Downloading: $DOWNLOAD_URL"

# --- Download and extract ---
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL -o "$TMP_DIR/$ARCHIVE_NAME" "$DOWNLOAD_URL"

tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"

# --- Find the binary ---
BINARY="$TMP_DIR/agentup"
if [ ! -f "$BINARY" ]; then
    # Try looking in subdirectory
    BINARY=$(find "$TMP_DIR" -name "agentup" -type f | head -1)
fi

if [ -z "$BINARY" ] || [ ! -f "$BINARY" ]; then
    echo "ERROR: agentup binary not found in archive."
    exit 1
fi

# --- Install ---
if [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
else
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi

cp "$BINARY" "$INSTALL_DIR/agentup"
chmod +x "$INSTALL_DIR/agentup"

# --- Check PATH ---
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        echo ""
        echo "WARNING: $INSTALL_DIR is not in your PATH." >&2
        SHELL_NAME=$(basename "$SHELL")
        case "$SHELL_NAME" in
            zsh)  RC_FILE="$HOME/.zshrc" ;;
            bash) RC_FILE="$HOME/.bashrc" ;;
            *)    RC_FILE="$HOME/.profile" ;;
        esac
        echo "Add this line to $RC_FILE:" >&2
        echo "  export PATH=\"\$PATH:$INSTALL_DIR\"" >&2
        ;;
esac

# --- Verify ---
echo ""
echo "Installation complete!"
echo "  Version: $TAG"
echo "  Location: $INSTALL_DIR/agentup"
echo ""
"$INSTALL_DIR/agentup" version
