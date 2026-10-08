#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Run the real CLI deployment lifecycle on a fresh, owned Gateway. The test
# owns its Operator processes, including failure cleanup. Preserve Gateway logs
# and a binary hash/timing report; stop the Gateway on every exit.
set -euo pipefail
umask 077
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN="$REPO/g8e"
COUNT=3
ROOT=""
while (($#)); do
    case "$1" in
        --binary) BIN="${2:?--binary needs a path}"; shift 2 ;;
        --count) COUNT="${2:?--count needs a number}"; shift 2 ;;
        --root) ROOT="${2:?--root needs a path}"; shift 2 ;;
        --help) echo "Usage: $0 [--binary PATH] [--count 3] [--root NEW_DIRECTORY]"; exit 0 ;;
        *) echo "Unknown argument: $1" >&2; exit 2 ;;
    esac
done
if [[ ! "$COUNT" =~ ^[1-9][0-9]*$ ]] || ((COUNT > 4997)); then
    echo "--count must be an integer in 1..4997" >&2
    exit 2
fi
BIN="$(realpath "$BIN")"
[[ -x "$BIN" ]] || { echo "Build g8e with make build first" >&2; exit 2; }
# Probe before installing the cleanup trap: an occupied port belongs to someone
# else, so refusing the run must never stop that Gateway.
for port in 8080 8443; do
    if [[ -n "$(ss -Hltn "sport = :$port")" ]]; then
        echo "Port $port is occupied; lifecycle test requires an isolated Gateway" >&2
        exit 1
    fi
done
if [[ -z "$ROOT" ]]; then
    ROOT="$(mktemp -d -t g8e-deploy-lifecycle.XXXXXXXX)"
else
    [[ ! -e "$ROOT" ]] || { echo "--root must be a new directory" >&2; exit 2; }
    mkdir -p "$ROOT"
    ROOT="$(realpath "$ROOT")"
fi
mkdir -p "$ROOT/run" "$ROOT/out" "$ROOT/home" "$ROOT/tmp" "$ROOT/bin"
# Copy once so rebuilding the workspace binary cannot change this scenario.
cp "$BIN" "$ROOT/bin/g8e"
BIN="$ROOT/bin/g8e"
export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"
export GOMODCACHE="${GOMODCACHE:-$(go env GOMODCACHE)}"
export HOME="$ROOT/home"
export USERPROFILE="$ROOT/home"
export TMPDIR="$ROOT/tmp"
SUITE_PID=""
cleanup() {
    local status=$?
    trap - EXIT INT TERM
    if [[ -n "$SUITE_PID" ]]; then
        kill -TERM -- "-$SUITE_PID" 2>/dev/null || true
        wait "$SUITE_PID" 2>/dev/null || true
    fi
    # Go's hard test timeout bypasses t.Cleanup. Match detached workers only
    # inside this runner's scratch tree, including unregistered workers.
    if ! python3 - "$ROOT/tmp" >"$ROOT/out/operator-cleanup.log" 2>&1 <<'PY'
import errno
import os
from pathlib import Path
import signal
import sys
import time

root = Path(sys.argv[1]).resolve()

def owned(pid):
    try:
        args = Path(f"/proc/{pid}/cmdline").read_bytes().decode().rstrip("\0").split("\0")
        if len(args) < 3 or Path(args[0]).name != "g8e" or args[1:3] != ["operator", "start"]:
            return False
        i = args.index("--working-dir")
        return Path(args[i + 1]).resolve().is_relative_to(root)
    except (OSError, ValueError, IndexError):
        return False

def workers():
    return [int(p.name) for p in Path("/proc").iterdir() if p.name.isdigit() and owned(int(p.name))]

for sig in (signal.SIGTERM, signal.SIGKILL):
    for pid in workers():
        try:
            fd = os.pidfd_open(pid)
            try:
                if owned(pid):
                    signal.pidfd_send_signal(fd, sig)
                    print(f"signaled owned worker {pid}: {sig.name}")
            finally:
                os.close(fd)
        except OSError as exc:
            if exc.errno != errno.ESRCH:
                raise
    deadline = time.monotonic() + 5
    while workers() and time.monotonic() < deadline:
        time.sleep(0.05)
    if not workers():
        print("no owned Operator processes remain")
        break
else:
    raise SystemExit(f"Operator cleanup failed: {workers()}")
PY
    then
        echo "Operator teardown failed; see $ROOT/out/operator-cleanup.log" >&2
        status=1
    fi
    if ! (cd "$ROOT/run" && "$BIN" gw stop) >"$ROOT/out/gateway-stop.log" 2>&1; then
        echo "Gateway teardown failed; see $ROOT/out/gateway-stop.log" >&2
        status=1
    fi
    echo "Deployment lifecycle evidence: $ROOT/out (exit $status)"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
(cd "$ROOT/run" && "$BIN" gw start --cert-mode localhost --posture doctrine \
    --roles data --public-spectator=false --log info) >"$ROOT/out/gateway-start.log" 2>&1
(cd "$ROOT/run" && "$BIN" login --endpoint localhost) >"$ROOT/out/owner-login.log" 2>&1
cd "$REPO"
setsid env G8E_E2E_RUNTIME_ROOT="$ROOT/run" \
G8E_E2E_FLEET_SIZE="$COUNT" \
G8E_E2E_FLEET_BIN="$BIN" \
G8E_E2E_FLEET_REPORT="$ROOT/out/deployment-report.json" \
    "$BIN" test e2e --timeout 10m --run '^TestOperatorDeploy_LocalLifecycle$' \
    > >(tee "$ROOT/out/scenario.log") 2>&1 &
SUITE_PID=$!
wait "$SUITE_PID"
SUITE_PID=""
