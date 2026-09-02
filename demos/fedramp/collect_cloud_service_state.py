#!/usr/bin/env python3

import json
import os
import sys
from datetime import datetime, timezone


def timestamp():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def entries(path):
    if not os.path.isfile(path):
        return []
    with open(path, encoding="utf-8") as stream:
        return [json.loads(line) for line in stream if line.strip()]


def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))


def operation(args):
    if len(args) != 7:
        return 2
    run_id, scenario_id, path, action, resource_id, detail = args[1:]
    matched = [entry for entry in entries(path) if entry.get("action") == action and entry.get("resource_id") == resource_id and entry.get("detail") == detail]
    entry = matched[-1] if matched else {}
    emit({
        "action": action,
        "detail": detail,
        "observed_at": timestamp(),
        "operation_found": bool(matched),
        "operation_timestamp": entry.get("timestamp", ""),
        "resource_id": resource_id,
        "run_id": run_id,
        "scenario_id": scenario_id,
    })
    return 0


def log_state(args):
    if len(args) != 4:
        return 2
    run_id, scenario_id, path = args[1:]
    recorded = entries(path)
    emit({
        "entry_count": len(recorded),
        "log_path": path,
        "observed_at": timestamp(),
        "persisted": os.path.isfile(path) and os.path.getsize(path) > 0,
        "run_id": run_id,
        "scenario_id": scenario_id,
        "size_bytes": os.path.getsize(path) if os.path.isfile(path) else 0,
    })
    return 0


def main():
    if len(sys.argv) < 2:
        return 2
    if sys.argv[1] == "operation":
        return operation(sys.argv[1:])
    if sys.argv[1] == "log":
        return log_state(sys.argv[1:])
    return 2


if __name__ == "__main__":
    sys.exit(main())
