# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	exit 2
fi

run_id="$1"
scenario_id="$2"
endpoint="$3"
available=false
if curl -sf "$endpoint" >/dev/null 2>&1; then
	available=true
fi
observed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '{"available":%s,"endpoint":"%s","observed_at":"%s","run_id":"%s","scenario_id":"%s"}\n' "$available" "$endpoint" "$observed_at" "$run_id" "$scenario_id"
