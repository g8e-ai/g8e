#!/usr/bin/env python3
"""Verify that the FedRAMP cloud service recorded governed operations.

Usage: python verify_ops.py <action> [action ...]
Exit 0 if at least one matching operation is found, 1 otherwise.
"""

import json
import os
import sys

LOG_PATH = os.environ.get("CLOUDSVC_LOG", "/var/cloudsvc/operations.jsonl")


def main():
    if len(sys.argv) < 2:
        print("Usage: verify_ops.py <action> [action ...]", file=sys.stderr)
        sys.exit(1)

    wanted = set(a.upper() for a in sys.argv[1:])

    if not os.path.exists(LOG_PATH):
        print(f"No operations log at {LOG_PATH}", file=sys.stderr)
        sys.exit(1)

    found = []
    with open(LOG_PATH) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            entry = json.loads(line)
            if entry.get("action", "").upper() in wanted:
                found.append(entry)

    if found:
        for entry in found:
            print(f"OK: {entry['action']} {entry['resource_id']} - {entry['detail']}")
        sys.exit(0)
    else:
        print(f"No matching operations found for: {wanted}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
