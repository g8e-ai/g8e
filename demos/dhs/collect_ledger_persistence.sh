#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
	exit 2
fi

run_id="$1"
scenario_id="$2"
container="$3"
directory="$4"
persisted=false
entry_count=0
if docker compose exec -T "$container" sh -c "test -d '$directory' && test -n \"\$(ls -A '$directory')\"" >/dev/null 2>&1; then
	persisted=true
	entry_count=$(docker compose exec -T "$container" sh -c "ls -A '$directory' | wc -l" | tr -d '[:space:]')
fi
observed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '{"persisted":%s,"directory":"%s","entry_count":%s,"observed_at":"%s","run_id":"%s","scenario_id":"%s"}\n' "$persisted" "$directory" "$entry_count" "$observed_at" "$run_id" "$scenario_id"
