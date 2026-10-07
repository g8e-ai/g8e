#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# sample-gateway.sh <gateway-pid> <data-dir> <out.csv> [interval=10]
# Periodically sample Gateway resource usage and log to CSV.
# Appends rows: timestamp, RSS KB, CPU%, thread count, open FDs,
# established conns :8443, established conns :8080, DB file size, WAL file size,
# free used MB, free available MB, 1-min loadavg.
# Loops until PID disappears or SIGINT.

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: sample-gateway.sh <gateway-pid> <data-dir> <out.csv> [interval=10]
  gateway-pid: PID of the running Gateway process
  data-dir:    directory containing *.db and *.db-wal files
  out.csv:     output CSV file (header written once)
  interval:    sampling interval in seconds (default: 10)
EOF
  exit 1
}

[[ $# -ge 3 ]] || usage

gateway_pid="$1"
data_dir="$2"
out_csv="$3"
interval="${4:-10}"

# Validate interval
if ! [[ "$interval" =~ ^[0-9]+$ ]] || (( interval < 1 )); then
  echo "Error: interval must be a positive integer" >&2
  exit 1
fi

# Write header if file doesn't exist
if [[ ! -f "$out_csv" ]]; then
  cat >> "$out_csv" <<'EOF'
timestamp,rss_kb,cpu_percent,thread_count,open_fds,conns_8443,conns_8080,db_bytes,wal_bytes,free_used_mb,free_avail_mb,loadavg_1min
EOF
fi

# Trap to clean up on SIGINT
trap 'exit 0' SIGINT

while true; do
  # Check if PID still exists
  if ! kill -0 "$gateway_pid" 2>/dev/null; then
    break
  fi

  # Timestamp in ISO format
  ts=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

  # RSS and CPU% from ps
  if rss_cpu=$(ps -o rss=,"%cpu=" -p "$gateway_pid" 2>/dev/null); then
    read -r rss cpu <<< "$rss_cpu"
  else
    rss=0
    cpu=0
  fi

  # Thread count from /proc/<pid>/status
  thread_count=0
  if [[ -f "/proc/$gateway_pid/status" ]]; then
    thread_count=$(grep "^Threads:" "/proc/$gateway_pid/status" 2>/dev/null | awk '{print $2}' || echo 0)
  fi

  # Open FDs
  open_fds=0
  if [[ -d "/proc/$gateway_pid/fd" ]]; then
    open_fds=$(ls -1 "/proc/$gateway_pid/fd" 2>/dev/null | wc -l || echo 0)
  fi

  # Established connections on :8443 and :8080
  conns_8443=$(ss -Htn state established "( sport = :8443 )" 2>/dev/null | wc -l || echo 0)
  conns_8080=$(ss -Htn state established "( sport = :8080 )" 2>/dev/null | wc -l || echo 0)

  # DB and WAL file sizes (sum, or 0 if none exist)
  db_bytes=0
  wal_bytes=0
  if [[ -d "$data_dir" ]]; then
    # Sum of all *.db files
    for f in "$data_dir"/*.db; do
      if [[ -f "$f" ]]; then
        db_bytes=$((db_bytes + $(stat -c %s "$f" 2>/dev/null || echo 0)))
      fi
    done
    # Sum of all *.db-wal files
    for f in "$data_dir"/*.db-wal; do
      if [[ -f "$f" ]]; then
        wal_bytes=$((wal_bytes + $(stat -c %s "$f" 2>/dev/null || echo 0)))
      fi
    done
  fi

  # Free memory (used and available in MB)
  read -r free_used free_avail <<< "$(free -m | grep Mem | awk '{print $3, $7}')"

  # 1-min loadavg
  loadavg=$(cat /proc/loadavg 2>/dev/null | awk '{print $1}' || echo 0)

  # Append row
  echo "$ts,$rss,$cpu,$thread_count,$open_fds,$conns_8443,$conns_8080,$db_bytes,$wal_bytes,$free_used,$free_avail,$loadavg" >> "$out_csv"

  sleep "$interval"
done
