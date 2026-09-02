#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	exit 2
fi

run_id="$1"
scenario_id="$2"
artifact_path="$3"
artifact_exists=false
if [ -e "$artifact_path" ]; then
	artifact_exists=true
fi
observed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf '{"artifact_exists":%s,"artifact_path":"%s","observed_at":"%s","run_id":"%s","scenario_id":"%s"}\n' "$artifact_exists" "$artifact_path" "$observed_at" "$run_id" "$scenario_id"
