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
