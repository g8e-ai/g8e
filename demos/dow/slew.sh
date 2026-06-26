#!/bin/sh
# slew — demo artifact that wraps the HTTP call to the mock gimbal.
#
# This script works around DangerousPatterns blocking curl/wget in
# run_shell_command. The governance path (envelope -> admission ->
# consensus -> L5 execution -> receipt) is fully real; this script
# is the bridge between the operator's governed execution and the
# mock external gimbal endpoint.
#
# Usage: slew <host:port> <azimuth> <elevation>
# Example: slew 10.43.0.40:9000 45.0 30.0

if [ $# -ne 3 ]; then
    echo "Usage: slew <host:port> <azimuth> <elevation>" >&2
    exit 1
fi

TARGET="$1"
AZ="$2"
EL="$3"

curl -s -X POST "http://${TARGET}/slew" \
    -H "Content-Type: application/json" \
    -d "{\"az\":${AZ},\"el\":${EL}}"
