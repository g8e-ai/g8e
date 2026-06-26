#!/usr/bin/env python3
"""
Verify that the mock gimbal recorded slew commands.

Queries the gimbal HTTP endpoint for recorded slews and prints them.
Exits 0 if at least one slew was recorded, 1 otherwise.
Used by the DoW demo Scenario 1, Step 6.
"""

import json
import sys
import urllib.request

GIMBAL_URL = "http://localhost:9000/slews"


def main():
    try:
        resp = urllib.request.urlopen(GIMBAL_URL)
        data = json.loads(resp.read())
    except Exception as e:
        print(f"ERROR: failed to query gimbal: {e}", file=sys.stderr)
        sys.exit(1)

    slews = data.get("slews", [])
    print(f"  Slews recorded: {len(slews)}")
    for s in slews:
        az = s["az"]
        el = s["el"]
        ts = s["timestamp"]
        print(f"  az={az}, el={el}, ts={ts}")

    sys.exit(0 if slews else 1)


if __name__ == "__main__":
    main()
