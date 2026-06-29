#!/bin/bash
# ----------------------------------------------------------
# g8e_macos-setup.sh - Setup and validation for macOS environments
# Single Script for full setup and validation on macOS.
# ==========================================================

set -e  # Exit on error

echo -e "\n[SETUP] Starting g8e Environment Setup and Validation...\n"

# --- SECTION 1: Dependency Validation & Installation ---
echo "[STEP 1/3] Validating required dependencies (make, go)..."

MISSING=()

if ! command -v make &> /dev/null; then
    MISSING+=("make")
else
    echo "  make: detected"
fi

if ! command -v go &> /dev/null; then
    MISSING+=("go")
else
    echo "  go: detected"
fi

if [ ${#MISSING[@]} -gt 0 ]; then
    echo -e "\nThe following dependencies are missing: ${MISSING[*]}"
    echo "They can be installed via: brew install ${MISSING[*]}"

    # Check if Homebrew is installed
    if ! command -v brew &> /dev/null; then
        echo "FATAL: Homebrew is not installed, which is required to install dependencies on macOS."
        echo "Install Homebrew first: https://brew.sh"
        echo "Then run: brew install ${MISSING[*]}"
        exit 1
    fi

    read -p "Would you like to install them now? [y/N] " response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        echo "Installing: ${MISSING[*]}"
        brew install "${MISSING[@]}"
        echo "Installation complete."
    else
        echo "FATAL: Required dependencies not installed. Please install them manually: brew install ${MISSING[*]}"
        exit 1
    fi
fi

# --- SECTION 2: Build ---
echo -e "\n[STEP 2/3] Building g8e..."
make build
echo "Build successful."

# --- SECTION 3: Complete ---
echo -e "\n[SETUP COMPLETE]"
echo "---------------------------------------------------------------"
echo "Binary available at: ./g8e"
echo "Start the gateway with: ./g8e gw start"
