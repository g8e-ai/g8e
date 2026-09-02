#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
	exit 2
fi

run_id="$1"
scenario_id="$2"
network_name="$3"
container_name="$4"
containers="$(docker network inspect "$network_name" --format '{{range .Containers}}{{println .Name}}{{end}}')"
connected=false
if printf '%s\n' "$containers" | grep -Fxq "$container_name"; then
	connected=true
fi
observed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '{"connected":%s,"container_name":"%s","network_name":"%s","observed_at":"%s","run_id":"%s","scenario_id":"%s"}\n' "$connected" "$container_name" "$network_name" "$observed_at" "$run_id" "$scenario_id"
