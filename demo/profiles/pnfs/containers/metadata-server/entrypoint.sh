#!/bin/bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

echo "/data/pnfs *(rw,sync,no_subtree_check,no_root_squash)" >> /etc/exports

# Ensure rpcbind is running
service rpcbind start

# Export the filesystems
exportfs -ra

# Start nfs-server in foreground
exec /usr/sbin/rpc.nfsd -N 2 -N 3 -V 4 -V 4.1 --debug 8 &
exec /usr/sbin/rpc.mountd -F
