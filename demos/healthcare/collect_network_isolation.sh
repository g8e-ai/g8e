# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

#!/bin/sh
set -eu

if [ "$#" -ne 5 ]; then
	exit 2
fi

run_id="$1"
scenario_id="$2"
source_boundary="$3"
target_boundary="$4"
target_endpoint="$5"
reachable=false
if wget -qO /dev/null -T 5 "$target_endpoint"; then
	reachable=true
fi
observed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '{"observed_at":"%s","reachable":%s,"run_id":"%s","scenario_id":"%s","source_boundary":"%s","target_boundary":"%s","target_endpoint":"%s"}\n' "$observed_at" "$reachable" "$run_id" "$scenario_id" "$source_boundary" "$target_boundary" "$target_endpoint"
