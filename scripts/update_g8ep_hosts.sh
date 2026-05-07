#!/bin/bash
NODES=$(docker ps --format '{{.Names}}' | grep -E "web-node-|pos-|kiosk-|scanner-|camera-|printer-|badge-|sensor-|controller-|gateway-|router-|logger-|probe-")
TEMP_HOSTS=$(mktemp)

for node in $NODES; do
    IP=$(docker inspect "$node" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' | head -n 1)
    if [ -n "$IP" ]; then
        echo "$IP $node" >> "$TEMP_HOSTS"
    fi
done

docker exec -i -u 0 g8ep bash -c "grep -v -E '$(echo $NODES | tr ' ' '|')' /etc/hosts > /etc/hosts.new && cat >> /etc/hosts.new && cat /etc/hosts.new > /etc/hosts && rm /etc/hosts.new" < "$TEMP_HOSTS"
rm "$TEMP_HOSTS"
