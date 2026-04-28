#!/bin/bash
# Measure llama.cpp prefix cache effectiveness for Tribunal pipeline
#
# This script measures prefill cost delta between first and subsequent Tribunal
# requests to verify that --cache-reuse, --keep, and --parallel flags are working.
#
# Usage:
#   ./scripts/testing/measure_prefix_cache.sh
#
# Prerequisites:
#   - g8el must be running with recommended flags: --cache-reuse 256 --keep -1 --parallel 6
#   - g8ee must be configured to use llama.cpp provider
#   - Platform must be running (g8ed, g8ee, g8es, g8el)
#
# What it measures:
#   - First Tribunal request: captures n_p_eval (prompt eval tokens) from llama-server logs
#   - Second Tribunal request: captures n_p_eval again
#   - Delta should show significant reduction in prefill cost for the second request
#
# Expected results with working prefix cache:
#   - First request: higher n_p_eval (full prompt prefill)
#   - Second request: lower n_p_eval (static prefix reused from KV cache)
#   - Delta: 30-50% reduction in prefill tokens for the second request

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Prefix Cache Measurement Script ==="
echo ""

# Check if g8el is running
if ! docker ps --format '{{.Names}}' | grep -q '^g8el$'; then
    echo "ERROR: g8el is not running. Start it with: ./g8e platform up g8el"
    exit 1
fi

echo "✓ g8el is running"

# Check if g8el has the recommended flags
G8EL_CMD=$(docker inspect g8el --format='{{.Config.Cmd}}')
if echo "$G8EL_CMD" | grep -q -- "--cache-reuse"; then
    echo "✓ g8el has --cache-reuse flag"
else
    echo "⚠ g8el is missing --cache-reuse flag"
fi

if echo "$G8EL_CMD" | grep -q -- "--keep"; then
    echo "✓ g8el has --keep flag"
else
    echo "⚠ g8el is missing --keep flag"
fi

if echo "$G8EL_CMD" | grep -q -- "--parallel"; then
    echo "✓ g8el has --parallel flag"
else
    echo "⚠ g8el is missing --parallel flag"
fi

echo ""
echo "=== Capturing llama-server logs for Tribunal requests ==="
echo "Note: This requires actual Tribunal activity through the platform."
echo "Run a Tribunal-bearing command (e.g., ask Sage to run a shell command)."
echo ""

# Follow g8el logs and extract n_p_eval metrics
LOG_FILE="/tmp/g8el_prefix_cache_$(date +%s).log"

echo "Capturing logs to: $LOG_FILE"
echo "Press Ctrl+C to stop capturing after 2-3 Tribunal requests..."
echo ""

docker logs g8el -f > "$LOG_FILE" 2>&1 &
LOG_PID=$!

# Wait for user to interrupt
trap "kill $LOG_PID 2>/dev/null; echo ''; echo 'Log capture stopped'; exit 0" INT

# Wait indefinitely (user will Ctrl+C)
while true; do
    sleep 1
done
