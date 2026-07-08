#!/bin/sh
#
# AgentUp uninstaller for macOS and Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/rockychang7/agentup/main/uninstall.sh | bash

set -e

# --- Locate the binary ---
BINARY=""
if [ -f "/usr/local/bin/agentup" ]; then
    BINARY="/usr/local/bin/agentup"
elif [ -f "$HOME/.local/bin/agentup" ]; then
    BINARY="$HOME/.local/bin/agentup"
else
    BINARY=$(command -v agentup 2>/dev/null || true)
fi

if [ -z "$BINARY" ]; then
    echo "agentup is not installed."
    exit 0
fi

echo "Uninstalling agentup..."
echo "  Binary: $BINARY"

# --- Remove the binary ---
if [ -w "$(dirname "$BINARY")" ]; then
    rm -f "$BINARY"
else
    sudo rm -f "$BINARY"
fi

echo "  Removed: $BINARY"

# --- Suggest PATH cleanup for non-standard locations ---
INSTALL_DIR=$(dirname "$BINARY")
case "$INSTALL_DIR" in
    /usr/local/bin|/usr/bin) ;;
    *)
        echo ""
        echo "If you added $INSTALL_DIR to your PATH, remove it from your shell config:"
        SHELL_NAME=$(basename "$SHELL" 2>/dev/null || echo "")
        case "$SHELL_NAME" in
            zsh)  echo "  ~/.zshrc" ;;
            bash) echo "  ~/.bashrc" ;;
            *)    echo "  your shell profile" ;;
        esac
        ;;
esac

echo ""
echo "agentup has been uninstalled."
