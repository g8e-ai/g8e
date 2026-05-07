#!/bin/bash
PUB_KEY="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA/3mumi4l9ryh9Ngk6K3IPygYnaiDYamvi+PVxy3c8s root@g8ep"
NODES=$(docker ps --format '{{.Names}}' | grep -E "web-node-|pos-|kiosk-|scanner-|camera-|printer-|badge-|sensor-|controller-|gateway-|router-|logger-|probe-")

for node in $NODES; do
    echo "Authorizing on $node..."
    docker exec -u 0 "$node" bash -c "mkdir -p /home/appuser/.ssh && echo '$PUB_KEY' >> /home/appuser/.ssh/authorized_keys && chown -R appuser:appuser /home/appuser/.ssh && chmod 700 /home/appuser/.ssh && chmod 600 /home/appuser/.ssh/authorized_keys"
done
