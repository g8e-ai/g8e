#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
	exit 2
fi

run_id="$1"
scenario_id="$2"
container="$3"
database_path="$4"
persisted=false
size_bytes=0
if docker compose exec -T "$container" sh -c "test -f '$database_path' && test -s '$database_path'" >/dev/null 2>&1; then
	persisted=true
	size_bytes=$(docker compose exec -T "$container" sh -c "wc -c < '$database_path'" | tr -d '[:space:]')
fi
observed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '{"persisted":%s,"database_path":"%s","size_bytes":%s,"observed_at":"%s","run_id":"%s","scenario_id":"%s"}\n' "$persisted" "$database_path" "$size_bytes" "$observed_at" "$run_id" "$scenario_id"
