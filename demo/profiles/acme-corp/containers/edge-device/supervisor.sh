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

# Tiny service supervisor.
#
# Keeps /opt/device/device-service.sh running. If the service exits (clean or
# crash), it's restarted after a short cooldown. This makes failure modes like
# `crashed` visible as a repeating crash-loop in `docker logs`, and means the
# AI can simply `pkill` the service to force a restart after editing config.

set -u

SERVICE="/opt/device/device-service.sh"
COOLDOWN_SEC=5

trap 'echo "[supervisor] shutting down"; kill -TERM "${child:-0}" 2>/dev/null; exit 0' TERM INT

attempt=0
while true; do
    attempt=$((attempt + 1))
    echo "[supervisor] starting device-service (attempt $attempt)"
    "$SERVICE" &
    child=$!
    wait "$child"
    rc=$?
    echo "[supervisor] device-service exited with code=$rc; restart in ${COOLDOWN_SEC}s"
    sleep "$COOLDOWN_SEC"
done
