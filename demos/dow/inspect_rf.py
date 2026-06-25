#!/usr/bin/env python3
"""
Inspect the RF environment from tactical_environment.json.

Reads the tactical environment file and prints each RF signal with
its key attributes. Used by the DoW demo Scenario 1, Step 4.
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

    signals = env.get("rf_environment", {}).get("signals", [])
    if not signals:
        print("  No RF signals detected.")
        sys.exit(0)

    for s in signals:
        signal_id = s["signal_id"]
        sig_type = s["type"]
        freq = s["frequency_mhz"]
        conf = s["confidence"]
        classification = s["classification"]
        print(f"  {signal_id}: {sig_type} at {freq} MHz (conf: {conf}, class: {classification})")


if __name__ == "__main__":
    main()
