#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0.
#
# Air-gapped deployment helper for g8e demo environments.
#
# Usage:
#   ./airgap.sh export [output-dir]   — Save all images to tar files
#   ./airgap.sh import [input-dir]    — Load images from tar files
#   ./airgap.sh list                  — Show images in manifest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="${SCRIPT_DIR}/images.json"

usage() {
    cat <<EOF
g8e Air-Gapped Deployment Helper

Usage:
  airgap.sh export [output-dir]   Save all images from images.json to tar files
  airgap.sh import [input-dir]    Load images from tar files into local Docker
  airgap.sh list                  List all images in the manifest

The export subcommand pulls each image by digest and saves it as a .tar file.
The import subcommand loads each .tar file into the local Docker daemon.

Examples:
  # On a connected machine:
  g8e demos pull
  ./airgap.sh export /tmp/g8e-images

  # Transfer /tmp/g8e-images to the air-gapped machine, then:
  ./airgap.sh import /tmp/g8e-images
EOF
}

get_images() {
    if [ ! -f "$MANIFEST" ]; then
        echo "Error: images manifest not found at $MANIFEST" >&2
        exit 1
    fi
    # Extract image@digest references using python (available in all demo envs)
    python3 -c "
import json, sys
with open('$MANIFEST') as f:
    for entry in json.load(f):
        print(f\"{entry['image']}@{entry['digest']}\")
"
}

cmd_export() {
    local out_dir="${1:-${SCRIPT_DIR}/images-export}"
    mkdir -p "$out_dir"

    local images
    images=$(get_images)
    local total
    total=$(echo "$images" | wc -l)
    local i=0

    echo "Exporting $total images to $out_dir..."
    while IFS= read -r ref; do
        i=$((i + 1))
        # Create a safe filename from the image reference
        local filename
        filename=$(echo "$ref" | tr '/:@' '_').tar
        local filepath="${out_dir}/${filename}"

        if [ -f "$filepath" ]; then
            echo "[$i/$total] Skipping $ref (already exists)"
            continue
        fi

        echo "[$i/$total] Pulling $ref..."
        docker pull "$ref" 2>&1 | tail -1

        echo "[$i/$total] Saving to $filename..."
        docker save -o "$filepath" "$ref"
    done <<< "$images"

    echo ""
    echo "Export complete. $total images saved to $out_dir/"
    echo "Transfer this directory to the air-gapped machine and run:"
    echo "  ./airgap.sh import $out_dir"
}

cmd_import() {
    local in_dir="${1:-${SCRIPT_DIR}/images-export}"

    if [ ! -d "$in_dir" ]; then
        echo "Error: directory $in_dir does not exist" >&2
        exit 1
    fi

    local files
    files=$(find "$in_dir" -name '*.tar' -type f | sort)
    local total
    total=$(echo "$files" | grep -c . || true)

    if [ "$total" -eq 0 ]; then
        echo "No .tar files found in $in_dir" >&2
        exit 1
    fi

    echo "Loading $total images from $in_dir..."
    local i=0
    while IFS= read -r filepath; do
        i=$((i + 1))
        echo "[$i/$total] Loading $(basename "$filepath")..."
        docker load -i "$filepath" 2>&1 | tail -1
    done <<< "$files"

    echo ""
    echo "Import complete. $total images loaded."
    echo "You can now build and run demos in air-gapped mode:"
    echo "  make build"
    echo "  g8e demos start <org>"
}

cmd_list() {
    if [ ! -f "$MANIFEST" ]; then
        echo "Error: images manifest not found at $MANIFEST" >&2
        exit 1
    fi
    echo "Images in manifest ($MANIFEST):"
    echo ""
    python3 -c "
import json
with open('$MANIFEST') as f:
    for entry in json.load(f):
        demos = ', '.join(entry.get('demos', []))
        print(f\"  {entry['image']}@{entry['digest']}\")
        print(f\"    tag: {entry.get('tag', 'n/a')}\")
        print(f\"    demos: {demos}\")
        print()
"
}

case "${1:-}" in
    export)
        cmd_export "${2:-}"
        ;;
    import)
        cmd_import "${2:-}"
        ;;
    list)
        cmd_list
        ;;
    -h|--help|help|"")
        usage
        ;;
    *)
        echo "Unknown command: $1" >&2
        echo ""
        usage
        exit 1
        ;;
esac
