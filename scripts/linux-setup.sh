#!/bin/bash
# ----------------------------------------------------------
# g8e_linux-setup.sh - Setup and validation for Linux environments
# Single Script for full setup and validation on Linux.
# ==========================================================

set -e  # Exit on error

echo -e "\n[SETUP] Starting g8e Environment Setup and Validation...\n"

# --- SECTION 1: Dependency Validation & Installation ---
echo "[STEP 1/4] Validating required dependencies (make, go)..."

MISSING=()

if ! command -v make &> /dev/null; then
    MISSING+=("make")
else
    echo "  make: detected"
fi

if ! command -v go &> /dev/null; then
    MISSING+=("golang")
else
    echo "  go: detected"
fi

if [ ${#MISSING[@]} -gt 0 ]; then
    echo -e "\nThe following dependencies are missing: ${MISSING[*]}"
    echo "They can be installed via: sudo apt-get install -y ${MISSING[*]}"
    read -p "Would you like to install them now? [y/N] " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        echo "Installing: ${MISSING[*]}"
        sudo apt-get update && sudo apt-get install -y "${MISSING[@]}"
        echo "Installation complete."
    else
        echo "FATAL: Required dependencies not installed. Please install them manually: sudo apt-get install -y ${MISSING[*]}"
        exit 1
    fi
fi

# --- SECTION 2: Build ---
echo -e "\n[STEP 2/4] Building g8e..."
make build
echo "Build successful."

# --- SECTION 3: Add g8e to PATH ---
echo -e "\n[STEP 3/4] Adding g8e to PATH..."
G8E_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PATH_LINE="export PATH=\"\$PATH:$G8E_DIR\""

# Detect shell profile
if [ -n "$ZSH_VERSION" ] || [ "$SHELL" = "/bin/zsh" ]; then
    PROFILE_FILE="$HOME/.zshrc"
elif [ -n "$BASH_VERSION" ] || [ "$SHELL" = "/bin/bash" ]; then
    PROFILE_FILE="$HOME/.bashrc"
else
    PROFILE_FILE="$HOME/.profile"
fi

if grep -qF "$G8E_DIR" "$PROFILE_FILE" 2>/dev/null; then
    echo "  g8e directory already in $PROFILE_FILE — skipping."
else
    echo "" >> "$PROFILE_FILE"
    echo "# g8e: add binary to PATH" >> "$PROFILE_FILE"
    echo "$PATH_LINE" >> "$PROFILE_FILE"
    echo "  Added $G8E_DIR to PATH in $PROFILE_FILE."
fi

# Also export for the current session
export PATH="$PATH:$G8E_DIR"

# --- SECTION 4: Complete ---
echo -e "\n[SETUP COMPLETE]"
echo "---------------------------------------------------------------"
echo "Binary available at: g8e (in PATH)"
echo "Start the gateway with: g8e gw start"
echo "Note: Open a new terminal or run 'source $PROFILE_FILE' to use g8e in other shells."
