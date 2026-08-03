#!/usr/bin/env bash
# CI guard: verify that finalized COSAiS overlays have detector OverlayIDs coverage.
#
# When NIST finalizes AI-specific control overlays (changing their status from
# "draft" to "finalized" in docs/reference/cosais-overlays.json), every finalized
# overlay ID must be referenced by at least one doctrine detector's overlay_ids
# field. This script enforces that requirement.
#
# Until all overlays remain in "draft" status, the check passes with a warning.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OVERLAY_FILE="$ROOT_DIR/docs/reference/cosais-overlays.json"

if [ ! -f "$OVERLAY_FILE" ]; then
    echo "ERROR: COSAiS overlay catalog not found at $OVERLAY_FILE"
    exit 1
fi

# Collect every doctrine directory under demos/*/doctrine/ so overlays declared
# by any demo doctrine (not just demos/fedramp/doctrine) are checked for
# detector coverage. Pass them all to the Python checker as trailing args.
DOCTRINE_DIRS=()
shopt -s nullglob
for d in "$ROOT_DIR"/demos/*/doctrine; do
    if [ -d "$d" ]; then
        DOCTRINE_DIRS+=("$d")
    fi
done
shopt -u nullglob

if [ "${#DOCTRINE_DIRS[@]}" -eq 0 ]; then
    echo "ERROR: No doctrine directories found under demos/*/doctrine/"
    exit 1
fi

# Use python3 to parse JSON and check coverage (python3 is a CI dependency).
# Usage: python3 ... <overlay_file> <doctrine_dir> [<doctrine_dir> ...]
python3 - "$OVERLAY_FILE" "${DOCTRINE_DIRS[@]}" <<'PYEOF'
import json
import os
import sys
import glob

overlay_file = sys.argv[1]
doctrine_dirs = sys.argv[2:]

with open(overlay_file) as f:
    catalog = json.load(f)

overlays = catalog.get("overlays", [])
finalized = [o["id"] for o in overlays if o.get("status") == "finalized"]

if not finalized:
    print("PASS: No finalized COSAiS overlays yet.")
    print("      Detector OverlayIDs will be checked when NIST finalizes.")
    sys.exit(0)

# Collect all overlay_ids from doctrine JSON files across every doctrine dir.
detector_overlay_ids = set()
doctrine_files = []
for doctrine_dir in doctrine_dirs:
    doctrine_files.extend(glob.glob(os.path.join(doctrine_dir, "*.json")))
for path in doctrine_files:
    with open(path) as f:
        data = json.load(f)
    for entry in data.get("doctrines", []):
        for oid in entry.get("overlay_ids", []):
            detector_overlay_ids.add(oid)

uncovered = [fid for fid in finalized if fid not in detector_overlay_ids]

if uncovered:
    print("FAIL: Finalized COSAiS overlays without detector coverage:")
    for fid in uncovered:
        print("  - {}".format(fid))
    print()
    print("Action required: populate overlay_ids in doctrine JSON files")
    print("to reference each finalized overlay, then re-run:")
    print("  ./g8e compliance ksi --class C")
    sys.exit(1)

print("PASS: All {} finalized COSAiS overlay(s) have detector coverage.".format(len(finalized)))
PYEOF
