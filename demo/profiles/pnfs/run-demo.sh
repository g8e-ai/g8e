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

# pNFS Governance Demo Helper Script
# This script runs inside the client-node container or on the host.

set -e

CLIENT_CONTAINER="pnfs-client-node"

echo "========================================"
echo "  pNFS Governance Demo"
echo "========================================"

case "$1" in
    setup)
        echo "[1/3] Switching to pnfs profile..."
        ./g8e demo profile switch pnfs
        
        echo "[2/3] Starting pNFS cluster..."
        ./g8e demo up
        
        echo "[3/3] Demo ready. Deploy operator to continue."
        echo "Run: ./g8e demo deploy -d YOUR_TOKEN"
        ;;

    demo-l1)
        echo "Step 1: Testing L1 Hard Gates (Forbidden Patterns)"
        echo "The Operator is configured to block any access to '/mnt/pnfs/private/*'"
        echo ""
        echo "Executing FsList on /mnt/pnfs/private..."
        # This would be a real g8e command in a full demo, but here we describe the flow.
        echo "RESULT: Access Denied (TX_L1_FAILED: field path violates pattern /mnt/pnfs/private/*)"
        ;;

    demo-l3)
        echo "Step 2: Testing L3 Authorization (Human-in-the-loop)"
        echo "The Operator is configured to require approval for '/mnt/pnfs/data/secrets.txt'"
        echo ""
        echo "Executing FsRead on /mnt/pnfs/data/secrets.txt..."
        echo "RESULT: Awaiting L3 Authorization (WebAuthn/Passkey required)"
        ;;

    *)
        echo "Usage: $0 {setup|demo-l1|demo-l3}"
        exit 1
        ;;
esac
