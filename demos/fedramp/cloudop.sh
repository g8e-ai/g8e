#!/bin/sh
# cloudop - wrapper script for governed cloud resource operations.
# Usage: cloudop <action> <target_host:port> <resource_id> <detail>
# Actions: provision, configure, destroy, revert
set -e

ACTION="$1"
TARGET="$2"
RESOURCE_ID="$3"
DETAIL="$4"

if [ -z "$ACTION" ] || [ -z "$TARGET" ] || [ -z "$RESOURCE_ID" ]; then
    echo "Usage: cloudop <action> <target_host:port> <resource_id> <detail>" >&2
    exit 1
fi

ENDPOINT=""
case "$ACTION" in
    provision) ENDPOINT="/provision" ;;
    configure) ENDPOINT="/configure" ;;
    destroy)   ENDPOINT="/destroy" ;;
    revert)    ENDPOINT="/revert" ;;
    *)
        echo "Unknown action: $ACTION" >&2
        exit 1
        ;;
esac

PAYLOAD=$(printf '{"action":"%s","resource_id":"%s","detail":"%s"}' "$ACTION" "$RESOURCE_ID" "$DETAIL")

curl -sf -X POST "http://${TARGET}${ENDPOINT}" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD"
