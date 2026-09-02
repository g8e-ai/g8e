#!/usr/bin/env python3
"""
Collect a typed observation of an exact governed Sovereign Data Service operation.

Usage: verify_ops.py RUN_ID SCENARIO_ID ACTION RECORD_ID DETAIL
"""

import json
import sys
import urllib.request
from datetime import datetime, timezone

DATASVC_URL = "http://localhost:9100/operations"


def main():
    if len(sys.argv) != 6:
        sys.exit(2)
    run_id, scenario_id, action, record_id, detail = sys.argv[1:]
    action = action.upper()
    try:
        resp = urllib.request.urlopen(DATASVC_URL)
        data = json.loads(resp.read())
    except Exception as e:
        print(f"ERROR: failed to query sovereign data service: {e}", file=sys.stderr)
        sys.exit(1)

    matches = [
        operation
        for operation in data.get("operations", [])
        if operation.get("action") == action
        and operation.get("record_id") == record_id
        and operation.get("detail") == detail
    ]
    operation = matches[-1] if matches else {}
    observed_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    print(json.dumps({
        "action": action,
        "detail": detail,
        "observed_at": observed_at,
        "operation_found": bool(matches),
        "operation_timestamp": operation.get("timestamp", ""),
        "record_id": record_id,
        "run_id": run_id,
        "scenario_id": scenario_id,
    }, separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    main()
