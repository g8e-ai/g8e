#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# sample-fleet.sh <fleet-dir> <out.csv> [interval=60]
# Periodically sample the memory and CPU of every local Operator process whose
# command line contains <fleet-dir>/op- and append one CSV row per sample:
# timestamp, process count, summed RSS/PSS/private KB (from smaps_rollup),
# per-Operator PSS and private KB, summed threads, and summed CPU ticks
# (utime+stime; diff two rows and divide by CLK_TCK for CPU seconds).
# Loops until SIGINT/SIGTERM.

set -euo pipefail

[[ $# -ge 2 ]] || { echo "Usage: sample-fleet.sh <fleet-dir> <out.csv> [interval=60]" >&2; exit 1; }
fleet_dir="$1"
out_csv="$2"
interval="${3:-60}"

if [[ ! -f "$out_csv" ]]; then
  echo "timestamp,procs,rss_kb,pss_kb,private_kb,pss_kb_per_op,private_kb_per_op,threads,cpu_ticks,clk_tck" >> "$out_csv"
fi
clk_tck=$(getconf CLK_TCK)
trap 'exit 0' INT TERM

while true; do
  ts=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
  mapfile -t pids < <(pgrep -f -- "$fleet_dir/op-" || true)
  rss=0 pss=0 priv=0 threads=0 ticks=0 n=0
  for pid in "${pids[@]}"; do
    rollup="/proc/$pid/smaps_rollup"
    [[ -r "$rollup" ]] || continue
    read -r r p v < <(awk '/^Rss:/{r=$2} /^Pss:/{p=$2} /^Private_(Clean|Dirty):/{v+=$2} END{print r+0, p+0, v+0}' "$rollup" 2>/dev/null) || continue
    t=$(awk '/^Threads:/{print $2}' "/proc/$pid/status" 2>/dev/null) || continue
    # Fields 14 and 15 of /proc/<pid>/stat, counted after the ")" that ends comm.
    c=$(sed 's/.*) //' "/proc/$pid/stat" 2>/dev/null | awk '{print $12 + $13}') || continue
    rss=$((rss + r)) pss=$((pss + p)) priv=$((priv + v)) threads=$((threads + t)) ticks=$((ticks + c)) n=$((n + 1))
  done
  per_pss=0 per_priv=0
  if (( n > 0 )); then
    per_pss=$((pss / n)) per_priv=$((priv / n))
  fi
  echo "$ts,$n,$rss,$pss,$priv,$per_pss,$per_priv,$threads,$ticks,$clk_tck" >> "$out_csv"
  sleep "$interval"
done
