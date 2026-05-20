#!/bin/bash
set -e

# Wait for servers and mount
echo "Waiting for NFS servers..."
sleep 10

echo "Mounting pNFS share..."
mount -t nfs metadata-server:/data/pnfs /mnt/pnfs || {
    echo "FAILED TO MOUNT. Retrying in 5s..."
    sleep 5
    mount -t nfs metadata-server:/data/pnfs /mnt/pnfs
}

echo "pNFS client ready."
exec tail -f /dev/null
