#!/bin/bash
# ----------------------------------------------------------
# g8e Linux Dev Setup Script
# Bootstraps a developer workspace from fresh clone to working binary.
# Validates dependencies, installs missing tooling, builds, and adds to PATH.
# See: docs/architecture/scripts.md and docs/guides/getting_started.md
# ==========================================================

set -e  # Exit on error

G8E_GO_MIN="1.26"

echo -e "\n[SETUP] Starting g8e Dev Environment Setup...\n"

# --- SECTION 1: Dependency Validation & Installation ---
echo "[STEP 1/3] Validating required dependencies (make, go >= $G8E_GO_MIN)..."

MISSING=()

if ! command -v make &> /dev/null; then
    MISSING+=("make")
else
    echo "  make: detected"
fi

GO_OK=false
if command -v go &> /dev/null; then
    GO_VERSION=$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | head -1 | sed 's/go//')
    if [ -n "$GO_VERSION" ]; then
        GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
        GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
        MIN_MAJOR=$(echo "$G8E_GO_MIN" | cut -d. -f1)
        MIN_MINOR=$(echo "$G8E_GO_MIN" | cut -d. -f2)
        if [ "$GO_MAJOR" -gt "$MIN_MAJOR" ] || { [ "$GO_MAJOR" -eq "$MIN_MAJOR" ] && [ "$GO_MINOR" -ge "$MIN_MINOR" ]; }; then
            echo "  go: detected (v$GO_VERSION)"
            GO_OK=true
        else
            echo "  go: detected (v$GO_VERSION) but v$G8E_GO_MIN+ is required"
            MISSING+=("golang")
        fi
    else
        echo "  go: detected but version unknown"
        MISSING+=("golang")
    fi
else
    MISSING+=("golang")
fi

if [ ${#MISSING[@]} -gt 0 ]; then
    echo -e "\nThe following dependencies are missing or outdated: ${MISSING[*]}"

    # Detect package manager and map package names
    PKG_MGR=""
    INSTALL_CMD=""
    if command -v apt-get &> /dev/null; then
        PKG_MGR="apt-get"
        PKG_NAMES=$(echo "${MISSING[@]}" | sed 's/golang/golang-go/g')
        INSTALL_CMD="sudo apt-get update && sudo apt-get install -y $PKG_NAMES"
        MANUAL_HINT="sudo apt-get install -y $PKG_NAMES"
    elif command -v dnf &> /dev/null; then
        PKG_MGR="dnf"
        PKG_NAMES=$(echo "${MISSING[@]}" | sed 's/golang/golang/g')
        INSTALL_CMD="sudo dnf install -y $PKG_NAMES"
        MANUAL_HINT="sudo dnf install -y $PKG_NAMES"
    elif command -v pacman &> /dev/null; then
        PKG_MGR="pacman"
        PKG_NAMES=$(echo "${MISSING[@]}" | sed 's/golang/go/g')
        INSTALL_CMD="sudo pacman -S --noconfirm $PKG_NAMES"
        MANUAL_HINT="sudo pacman -S $PKG_NAMES"
    elif command -v zypper &> /dev/null; then
        PKG_MGR="zypper"
        PKG_NAMES=$(echo "${MISSING[@]}" | sed 's/golang/go/g')
        INSTALL_CMD="sudo zypper install -y $PKG_NAMES"
        MANUAL_HINT="sudo zypper install $PKG_NAMES"
    else
        echo "FATAL: No supported package manager found (apt-get, dnf, pacman, zypper)."
        echo "Please install manually: ${MISSING[*]}"
        exit 1
    fi

    echo "Package manager detected: $PKG_MGR"
    echo "They can be installed via: $MANUAL_HINT"
    read -p "Would you like to install them now? [y/N] " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        echo "Installing: ${MISSING[*]}"
        eval "$INSTALL_CMD"
        echo "Installation complete."
    else
        echo "FATAL: Required dependencies not installed. Please install them manually: $MANUAL_HINT"
        exit 1
    fi
fi

# --- SECTION 2: Build ---
echo -e "\n[STEP 2/3] Building g8e..."
make build
echo "Build successful."

# --- SECTION 3: Add g8e to PATH ---
echo -e "\n[STEP 3/3] Adding g8e to PATH..."
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

# --- Complete ---
echo -e "\n[SETUP COMPLETE]"
echo "---------------------------------------------------------------"
echo "Binary available at: g8e (in PATH)"
echo "Start the gateway with: g8e gw start"
echo "Note: Open a new terminal or run 'source $PROFILE_FILE' to use g8e in other shells."
