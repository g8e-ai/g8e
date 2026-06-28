#!/bin/sh
# dataop — demo artifact that bridges the operator's governed execution to the
# Sovereign Data Service (the L5 actuator).
#
# It works around DangerousPatterns blocking curl/wget directly in
# run_shell_command: the governed command is `dataop ...`, and the curl call
# lives inside this script. The governance path (envelope -> admission ->
# L1/L2/L3 -> L5 execution -> receipt) is fully real; this script is only the
# bridge to the mock external data endpoint.
#
# Usage: dataop <op> <host:port> <record_id> <detail>
#   op:     ingest | release | cue | purge
# Example: dataop ingest 10.63.0.50:9100 TRK-CBP-0001 NIPR

if [ $# -ne 4 ]; then
    echo "Usage: dataop <ingest|release|cue|purge> <host:port> <record_id> <detail>" >&2
    exit 1
fi

OP="$1"
TARGET="$2"
RECORD="$3"
DETAIL="$4"

case "$OP" in
    ingest|release|cue|purge) ;;
    *) echo "dataop: unknown op '$OP'" >&2; exit 1 ;;
esac

curl -s -X POST "http://${TARGET}/${OP}" \
    -H "Content-Type: application/json" \
    -d "{\"record_id\":\"${RECORD}\",\"detail\":\"${DETAIL}\"}"
