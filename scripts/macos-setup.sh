#!/bin/bash
# ----------------------------------------------------------
# g8e_macos-setup.sh - Setup and validation for macOS environments
# Single Script for full setup and validation on macOS.
# ==========================================================

set -e  # Exit on error

echo -e "\n[SETUP] Starting g8e Environment Setup and Validation...\n"

# --- SECTION 1: Dependency Validation ---
echo "[STEP 1/3] Validating required dependency (go)..." 

# Validate Go Environment
if ! command -v go &> /dev/null; then
    echo "FATAL: 'go' command not found in PATH. Please install Go to proceed."
    echo "Install via: brew install go"
    exit 1
else
    echo "Go environment detected."
fi

# --- SECTION 2: Build ---
echo -e "\n[STEP 2/3] Building g8e..."
if [ -f "./g8e" ]; then
    echo "Binary already exists at ./g8e, skipping build."
    echo "To rebuild, run: make build"
else
    make build
    echo "Build successful."
fi

# --- SECTION 3: Complete ---
echo -e "\n[SETUP COMPLETE]"
echo "---------------------------------------------------------------"
echo "Binary available at: ./g8e"
echo "Start the gateway with: ./g8e gw start"
