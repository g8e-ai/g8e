#!/usr/bin/env python3
"""
Inspect PNT sources from tactical_environment.json.

Reads the tactical environment file and prints each PNT source with
its key attributes, highlighting spoofed sources. Used by the DoW
demo Scenario 2, Step 1.
"""

import json
import sys
from pathlib import Path

ENV_PATH = Path("/var/g8e/target/tactical_environment.json")


def main():
    if not ENV_PATH.exists():
        print(f"ERROR: {ENV_PATH} not found", file=sys.stderr)
        sys.exit(1)

    with open(ENV_PATH) as f:
        env = json.load(f)

    sources = env.get("pnt_sources", [])
    if not sources:
        print("  No PNT sources found.")
        sys.exit(0)

    for s in sources:
        source_id = s["source_id"]
        src_type = s["type"]
        coords = s["coordinates"]
        trusted = s.get("trusted", True)
        spoofed = s.get("spoofed", False)
        tag = " [SPOOFED]" if spoofed else ""
        print(f"  {source_id}: {src_type} -> {coords} (trusted: {trusted}){tag}")


if __name__ == "__main__":
    main()
