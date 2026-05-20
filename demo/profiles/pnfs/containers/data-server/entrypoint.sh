#!/bin/bash
set -e

echo "/data/pnfs *(rw,sync,no_subtree_check,no_root_squash)" >> /etc/exports

# Ensure rpcbind is running
service rpcbind start

# Export the filesystems
exportfs -ra

# Start nfs-server in foreground
exec /usr/sbin/rpc.nfsd -N 2 -N 3 -V 4 -V 4.1 --debug 8 &
exec /usr/sbin/rpc.mountd -F
