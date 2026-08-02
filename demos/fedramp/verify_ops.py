#!/usr/bin/env python3
"""Verify that the FedRAMP cloud service recorded governed operations.

Usage: python verify_ops.py <action> [action ...]
       python verify_ops.py --ksi-result
Exit 0 if verification passes, 1 otherwise.
"""

import json
import os
import sys

LOG_PATH = os.environ.get("CLOUDSVC_LOG", "/var/cloudsvc/operations.jsonl")
KSI_RESULT_PATH = os.environ.get(
    "KSI_RESULT_PATH", "/root/.g8e/data/compliance/ksi-history"
)


def verify_operations(wanted):
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
        return True
    else:
        print(f"No matching operations found for: {wanted}", file=sys.stderr)
        return False


def verify_ksi_result():
    if not os.path.isdir(KSI_RESULT_PATH):
        print(f"No KSI history directory at {KSI_RESULT_PATH}", file=sys.stderr)
        return False

    snapshots = [f for f in os.listdir(KSI_RESULT_PATH) if f.endswith(".json")]
    if not snapshots:
        print(f"No KSI result snapshots found in {KSI_RESULT_PATH}", file=sys.stderr)
        return False

    for snapshot_file in sorted(snapshots):
        path = os.path.join(KSI_RESULT_PATH, snapshot_file)
        with open(path) as f:
            data = json.load(f)
        results = data.get("results", [])
        satisfied = sum(1 for r in results if r.get("status") == "satisfied")
        total = len(results)
        print(f"OK: KSI snapshot {snapshot_file} - {satisfied}/{total} satisfied")
    return True


def main():
    args = sys.argv[1:]

    if not args:
        print("Usage: verify_ops.py <action> [action ...]", file=sys.stderr)
        print("       verify_ops.py --ksi-result", file=sys.stderr)
        sys.exit(1)

    if args[0] == "--ksi-result":
        ok = verify_ksi_result()
        sys.exit(0 if ok else 1)

    wanted = set(a.upper() for a in args)
    ok = verify_operations(wanted)
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
