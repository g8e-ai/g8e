#!/bin/sh
# Verify both Gateway paths without disabling TLS verification. The dial target
# remains the supplied host while certificate identity is g8e.local.
set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: gateway-preflight.sh <gateway-host>" >&2
    exit 2
fi

gateway_host=$1
ca_file=$(mktemp)
trap 'rm -f "$ca_file"' EXIT HUP INT TERM

curl --fail --silent --show-error --connect-timeout 5 \
    "http://${gateway_host}:8080/.well-known/g8e/pki/ca-bundle" > "$ca_file"
curl --silent --show-error --connect-timeout 5 --output /dev/null \
    --cacert "$ca_file" \
    --connect-to "g8e.local:8443:${gateway_host}:8443" \
    https://g8e.local:8443/api/v1/health
