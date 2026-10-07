#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Operator fleet scenario: start an isolated Gateway, deploy N real Operator
# processes with `g8e operator deploy --local --approve`, then run the Tier 3
# scenario TestOperatorFleet_* (enrollment, heartbeat soak, fan-out, settle).
#
# Environment:
#   G8E_FLEET_SIZE     remote Operators to deploy (default 100)
#   G8E_FLEET_ROOT     scratch root for the runtime, fleet, and outputs
#                      (default: a new mktemp directory; outputs are in $ROOT/out)
#   G8E_BIN            g8e binary to use (default: make build, then ./g8e)
#   G8E_FLEET_NETNS    1 = run everything in a private network namespace
#                      (unshare --map-root-user --net). Workers always dial the
#                      default Gateway ports 8080/8443, so use this when another
#                      Gateway already owns them (a developer machine).
#   G8E_FLEET_TIMEOUT  overall `g8e test e2e` time limit (default 20m)
#   G8E_E2E_FLEET_SOAK / _CONCURRENCY / _ROUNDS   passed through to the scenario
#
# Never touches a Gateway or Operators it did not start: it uses its own
# runtime directory and stops only processes under that directory.
set -euo pipefail
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$REPO/scripts/ci/operator-fleet-smoke.sh"

if [[ "${G8E_FLEET_NETNS:-0}" == 1 && -z "${G8E_FLEET_IN_NETNS:-}" ]]; then
    exec unshare --map-root-user --net env G8E_FLEET_IN_NETNS=1 bash "$SCRIPT" "$@"
fi
if [[ -n "${G8E_FLEET_IN_NETNS:-}" ]]; then
    ip link set lo up
fi

FLEET_SIZE="${G8E_FLEET_SIZE:-100}"
ROOT="${G8E_FLEET_ROOT:-$(mktemp -d)}"
RUN="$ROOT/run"      # Gateway working directory (its .g8e/ lives here)
FLEET="$ROOT/fleet"  # one op-NNNNN directory per Operator
OUT="$ROOT/out"
mkdir -p "$RUN" "$FLEET" "$OUT" "$ROOT/home"

# Isolate HOME from the developer's, but keep the Go caches so `g8e test e2e`
# neither recompiles the world nor needs the network.
export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"
export GOMODCACHE="${GOMODCACHE:-$(go env GOMODCACHE)}"
export HOME="$ROOT/home"

if [[ -z "${G8E_BIN:-}" ]]; then
    (cd "$REPO" && make build)
    G8E_BIN="$REPO/g8e"
fi
G8E_BIN="$(readlink -f "$G8E_BIN")"
SAMPLER_PID=""

cleanup() {
    local status=$?
    if [[ -n "$SAMPLER_PID" ]]; then
        kill "$SAMPLER_PID" 2>/dev/null || true
    fi
    if [[ $status -ne 0 ]]; then
        echo "operator-fleet-smoke FAILED (status $status); logs under $ROOT" >&2
        find "$RUN/.g8e" -name '*.log' -type f -print -exec tail -n 40 {} \; 2>/dev/null || true
    fi
    # Stop only this run's Operators, matched by their directory under $FLEET.
    pkill -TERM -f -- "$FLEET/op-" 2>/dev/null || true
    for _ in {1..20}; do
        pgrep -f -- "$FLEET/op-" >/dev/null 2>&1 || break
        sleep 0.5
    done
    pkill -KILL -f -- "$FLEET/op-" 2>/dev/null || true
    (cd "$RUN" && "$G8E_BIN" gw stop) >"$OUT/gateway-stop.log" 2>&1 || true
    exit "$status"
}
trap cleanup EXIT

# Workers always dial the default Gateway ports, so another listener there
# would receive this run's enrollments and commands. Refuse to start rather
# than risk pointing the deploy at a Gateway this script does not own.
for port in 8080 8443; do
    if ss -Hltn "sport = :$port" | grep -q .; then
        echo "port $port already has a listener; stop it or run with G8E_FLEET_NETNS=1" >&2
        exit 1
    fi
done

echo "== Gateway (isolated runtime: $RUN)"
cd "$RUN"
"$G8E_BIN" gw start --cert-mode localhost --posture doctrine --public-spectator=false --rate-limit-rps 0 --log info --quiet
"$G8E_BIN" auth enroll user -e localhost --headless

GATEWAY_PID="$(pgrep -f -- "$RUN/.g8e/bin/g8e gw start" | head -n1)"
# Confirm the listener on the Operator port belongs to the Gateway started above.
if ! ss -Hltnp "sport = :8443" | grep -q "pid=$GATEWAY_PID,"; then
    echo "port 8443 is not served by the Gateway this script started (pid ${GATEWAY_PID:-none})" >&2
    exit 1
fi
bash "$REPO/scripts/ci/sample-gateway.sh" "$GATEWAY_PID" "$RUN/.g8e/data" "$OUT/gateway-resources.csv" 10 &
SAMPLER_PID=$!

echo "== Deploy $FLEET_SIZE Operators"
deploy_started=$SECONDS
"$G8E_BIN" operator deploy --local --dest-dir "$FLEET" --count "$FLEET_SIZE" --roles data \
    -e localhost --background --approve --parallel 4 --log info 2>&1 | tee "$OUT/deploy.log"
deploy_seconds=$((SECONDS - deploy_started))
echo "deploy_seconds=$deploy_seconds operators=$FLEET_SIZE" | tee "$OUT/deploy-summary.txt"

echo "== Scenario TestOperatorFleet_*"
cd "$REPO"
G8E_E2E_RUNTIME_ROOT="$RUN" \
G8E_E2E_FLEET_SIZE="$FLEET_SIZE" \
G8E_E2E_FLEET_BIN="$G8E_BIN" \
G8E_E2E_FLEET_REPORT="$OUT/fleet-report.json" \
    "$G8E_BIN" test e2e --timeout "${G8E_FLEET_TIMEOUT:-20m}" --run '^TestOperatorFleet_' 2>&1 | tee "$OUT/scenario.log"

echo "Operator fleet scenario passed: $FLEET_SIZE Operators; results in $OUT"
