#!/usr/bin/env python3
"""
Verify that the Sovereign Data Service recorded governed data operations.

Queries the data service for recorded operations and prints them. Optionally
filters by action (INGEST/RELEASE/CUE/PURGE). Exits 0 if at least one matching
operation was recorded, 1 otherwise. Used by the DHS demo scenarios to prove
that a governed envelope actually drove the L5 actuator.

Usage: verify_ops.py [ACTION]
"""

import json
import sys
import urllib.request

DATASVC_URL = "http://localhost:9100/operations"


def main():
    want = sys.argv[1].upper() if len(sys.argv) > 1 else None
    try:
        resp = urllib.request.urlopen(DATASVC_URL)
        data = json.loads(resp.read())
    except Exception as e:
        print(f"ERROR: failed to query sovereign data service: {e}", file=sys.stderr)
        sys.exit(1)

    ops = data.get("operations", [])
    if want:
        ops = [o for o in ops if o.get("action") == want]

    label = want or "operation"
    print(f"  Governed {label.lower()} operations recorded: {len(ops)}")
    for o in ops:
        print(f"  {o['action']:8} record={o['record_id']:14} detail={o['detail']:24} ts={o['timestamp']}")

    sys.exit(0 if ops else 1)


if __name__ == "__main__":
    main()
