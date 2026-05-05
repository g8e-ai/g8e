#!/usr/bin/env bash
# Generate docker-compose batch files for the acme-corp demo profile.
#
# Produces N globally-distributed, named edge devices whose hostnames encode
# function + site + location so the AI can reason about them from names alone.
# A deterministic subset (~FAILURE_RATIO percent) is assigned broken profiles
# spread across the failure-mode catalog.
#
# Splits the fleet into multiple batch compose files (default 100 per batch)
# to avoid Docker Compose timeout issues with large service counts.
#
# Usage: generate-compose.sh [NODE_COUNT] [BATCH_SIZE]    (default: 100, 100)

set -euo pipefail

NODE_COUNT="${1:-100}"
BATCH_SIZE="${2:-100}"

# ---------------------------------------------------------------------------
# Resource Validation
# Approx 5MB per node. Warn if memory usage exceeds 70% of available RAM.
# ---------------------------------------------------------------------------
check_resources() {
    local total_nodes="$1"
    local mem_per_node_mb=5
    local total_req_mb=$((total_nodes * mem_per_node_mb))
    
    # Get available RAM in MB
    if [[ -f /proc/meminfo ]]; then
        local mem_avail_kb=$(grep MemAvailable /proc/meminfo | awk '{print $2}')
        local mem_avail_mb=$((mem_avail_kb / 1024))
        local threshold=$((mem_avail_mb * 70 / 100))
        
        if (( total_req_mb > threshold )); then
            echo "WARNING: Requested ${total_nodes} nodes will use ~${total_req_mb}MB RAM." >&2
            echo "         Available RAM: ${mem_avail_mb}MB (Threshold: ${threshold}MB)." >&2
            echo "         This may cause system instability or OOM kills." >&2
            read -p "Continue anyway? [y/N] " confirm
            if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
                echo "Aborting." >&2
                exit 1
            fi
        fi
    fi
}

check_resources "$NODE_COUNT"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$(pwd)"
COMPOSE_FILE="$OUTPUT_DIR/docker-compose.yml"

# ---------------------------------------------------------------------------
# Device taxonomy: function|site|share-per-1000
# Shares sum to 1000; scaled proportionally to NODE_COUNT.
# ---------------------------------------------------------------------------
TAXONOMY=(
    "pos|store|250"
    "kiosk|airport|80"
    "scanner|warehouse|100"
    "camera|hq|60"
    "camera|store|60"
    "printer|office|100"
    "badge|office|60"
    "sensor|factory|100"
    "sensor|warehouse|50"
    "controller|factory|60"
    "gateway|branch|40"
    "router|hub|20"
    "logger|dc|15"
    "probe|dc|5"
)

# ---------------------------------------------------------------------------
# Global locations (IATA-ish 3-letter codes).
# Grouped by region so cross-region fleet ops demos are realistic.
# ---------------------------------------------------------------------------
AMERICAS=(nyc lax chi sfo mia sea dfw bos tor mex)
EUROPE=(lon par ber ams mad rom dub sto mil zur)
APAC=(tok sin syd hkg sha mum del bkk)
MEA=(dxb jnb)

LOCATIONS=("${AMERICAS[@]}" "${EUROPE[@]}" "${APAC[@]}" "${MEA[@]}")

_get_region() {
    local loc="$1"
    for r in "${AMERICAS[@]}"; do [[ "$loc" == "$r" ]] && echo "americas" && return; done
    for r in "${EUROPE[@]}"; do [[ "$loc" == "$r" ]] && echo "europe" && return; done
    for r in "${APAC[@]}"; do [[ "$loc" == "$r" ]] && echo "apac" && return; done
    for r in "${MEA[@]}"; do [[ "$loc" == "$r" ]] && echo "mea" && return; done
    echo "unknown"
}

# ---------------------------------------------------------------------------
# Failure catalog. Each broken device gets exactly one of these profiles.
# Healthy devices use the "healthy" profile.
# ---------------------------------------------------------------------------
FAILURE_MODES=(
    crashed
    wrong_config
    disk_full
    cert_expired
    stuck_loop
    permission_denied
    bad_upstream
    high_error_rate
    memory_leak
)

FAILURE_RATIO_PERCENT="${FAILURE_RATIO_PERCENT:-5}"

# ---------------------------------------------------------------------------
# Compute per-category counts proportional to NODE_COUNT.
# Each category gets at least 1 device (so every type is represented even at
# small N), then we truncate the global list to exactly NODE_COUNT.
# ---------------------------------------------------------------------------
declare -a DEVICES=()   # entries of form: function|site|location|profile

loc_idx=0
loc_count=${#LOCATIONS[@]}

for entry in "${TAXONOMY[@]}"; do
    IFS='|' read -r fn site share <<< "$entry"
    # count = max(1, round(share * N / 1000))
    count=$(( (share * NODE_COUNT + 500) / 1000 ))
    (( count < 1 )) && count=1

    for ((i=0; i<count; i++)); do
        loc="${LOCATIONS[$((loc_idx % loc_count))]}"
        loc_idx=$((loc_idx + 1))
        DEVICES+=("$fn|$site|$loc|healthy")
    done
done

# Truncate to exactly NODE_COUNT
total=${#DEVICES[@]}
if (( total > NODE_COUNT )); then
    DEVICES=("${DEVICES[@]:0:$NODE_COUNT}")
elif (( total < NODE_COUNT )); then
    # Pad with additional POS devices (the largest category) if we're short.
    shortage=$((NODE_COUNT - total))
    for ((i=0; i<shortage; i++)); do
        loc="${LOCATIONS[$((loc_idx % loc_count))]}"
        loc_idx=$((loc_idx + 1))
        DEVICES+=("pos|store|$loc|healthy")
    done
fi

# ---------------------------------------------------------------------------
# Assign failure profiles deterministically.
# Uses sha256(index) to select which devices break; stable across runs.
# ---------------------------------------------------------------------------
total=${#DEVICES[@]}
broken_target=$(( (total * FAILURE_RATIO_PERCENT + 50) / 100 ))
(( broken_target < 1 && FAILURE_RATIO_PERCENT > 0 )) && broken_target=1

fmode_count=${#FAILURE_MODES[@]}

# Pick broken indices via deterministic stride so failures are spread across
# the list (rather than clustered in one category).
if (( broken_target > 0 )); then
    stride=$(( total / broken_target ))
    (( stride < 1 )) && stride=1
    broken_idx=0
    for ((i=0; i<broken_target; i++)); do
        pos=$(( (i * stride + 3) % total ))   # +3 offset so we don't always start at index 0
        IFS='|' read -r fn site loc _ <<< "${DEVICES[$pos]}"
        fmode="${FAILURE_MODES[$((i % fmode_count))]}"
        DEVICES[$pos]="$fn|$site|$loc|$fmode"
    done
fi

# ---------------------------------------------------------------------------
# Emit services to batch files. Counter per (function, site, location) triple
# so names like pos-store-nyc-001, pos-store-nyc-002 are unique.
# ---------------------------------------------------------------------------
declare -A TRIPLE_COUNTER=()

# ---------------------------------------------------------------------------
# Emit header to a batch file
# ---------------------------------------------------------------------------
_emit_header() {
    local batch_file="$1"
    cat > "$batch_file" <<'EOF'
# ACME Corp Global Fleet Demo (GENERATED - do not edit by hand)
#
# Batch of devices. See docker-compose.yml for full fleet configuration.
# Regenerate via `make generate-compose` or `./scripts/generate-compose.sh`.

networks:
  acme-net:
    external: true

x-edge-env: &edge-env
  DEVICE_TOKEN: ${DEVICE_TOKEN:-}
  G8E_ENDPOINT: ${G8E_ENDPOINT:-g8e.local}

x-edge-device: &edge-device
  image: acme-edge-device:latest
  networks:
    - acme-net
  # Simulate internet connectivity to the g8e platform via public endpoints.
  # This keeps the "simulated company" isolated on its own network.
  extra_hosts:
    - "g8e.local:host-gateway"
    - "g8ed:host-gateway"
    - "g8es:host-gateway"
  labels:
    - "demo.service=acme-edge"
  volumes:
    - g8es-ssl:/g8es:ro
  restart: unless-stopped

services:
EOF
}

# ---------------------------------------------------------------------------
# Emit footer to a batch file
# ---------------------------------------------------------------------------
_emit_footer() {
    local batch_file="$1"
    cat >> "$batch_file" <<'EOF'

volumes:
  g8es-ssl:
    external: true
EOF
}

# ---------------------------------------------------------------------------
# Emit the dashboard compose file
# ---------------------------------------------------------------------------
_emit_dashboard() {
    local dash_file="$OUTPUT_DIR/docker-compose.dashboard.yml"
    cat > "$dash_file" <<'EOF'
# ACME Corp Dashboard (GENERATED - do not edit by hand)
networks:
  acme-net:
    external: true

services:
  dashboard:
    image: acme-dashboard:latest
    container_name: acme-dashboard
    hostname: dashboard
    networks:
      - acme-net
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    labels:
      - "demo.service=acme-dashboard"
    restart: unless-stopped
EOF
}

# ---------------------------------------------------------------------------
# Emit a device service to a batch file
# ---------------------------------------------------------------------------
_emit_device() {
    local batch_file="$1"
    local name="$2"
    local fn="$3"
    local site="$4"
    local loc="$5"
    local profile="$6"

    cat >> "$batch_file" <<EOF
  ${name}:
    <<: *edge-device
    container_name: acme-${name}
    hostname: ${name}
    environment:
      <<: *edge-env
      DEVICE_NAME: ${name}
      DEVICE_FUNCTION: ${fn}
      DEVICE_SITE: ${site}
      DEVICE_LOCATION: ${loc}
      DEVICE_PROFILE: ${profile}

EOF
}

# ---------------------------------------------------------------------------
# Generate regional compose files
# ---------------------------------------------------------------------------
rm -f "$OUTPUT_DIR"/docker-compose.*.yml "$OUTPUT_DIR"/.batch-files

# Create dashboard definition
_emit_dashboard

# Create network definition
cat > "$OUTPUT_DIR/docker-compose.network.yml" <<'EOF'
networks:
  acme-net:
    driver: bridge
    ipam:
      config:
        - subnet: 10.230.0.0/16
EOF

# Group devices by region
declare -A REGION_DEVICES
for i in "${!DEVICES[@]}"; do
    IFS='|' read -r fn site loc profile <<< "${DEVICES[$i]}"
    region=$(_get_region "$loc")
    REGION_DEVICES[$region]+="$fn|$site|$loc|$profile"$'\n'
done

# Emit files per region, with batching if a region is too large
for region in "americas" "europe" "apac" "mea"; do
    [[ -z "${REGION_DEVICES[$region]:-}" ]] && continue
    
    # Read devices for this region into an array
    IFS=$'\n' read -d '' -ra region_list <<< "${REGION_DEVICES[$region]}" || true
    
    total_in_region=${#region_list[@]}
    batch_num=1
    device_idx=0
    
    batch_file="$OUTPUT_DIR/docker-compose.${region}-${batch_num}.yml"
    _emit_header "$batch_file"
    
    for entry in "${region_list[@]}"; do
        [[ -z "$entry" ]] && continue
        IFS='|' read -r fn site loc profile <<< "$entry"
        
        # Start new batch within region if current one is full
        if (( device_idx > 0 && device_idx % BATCH_SIZE == 0 )); then
            _emit_footer "$batch_file"
            batch_num=$((batch_num + 1))
            batch_file="$OUTPUT_DIR/docker-compose.${region}-${batch_num}.yml"
            _emit_header "$batch_file"
        fi
        
        key="${fn}-${site}-${loc}"
        idx="${TRIPLE_COUNTER[$key]:-0}"
        idx=$((idx + 1))
        TRIPLE_COUNTER[$key]=$idx
        seq=$(printf "%03d" "$idx")
        name="${fn}-${site}-${loc}-${seq}"
        
        _emit_device "$batch_file" "$name" "$fn" "$site" "$loc" "$profile"
        device_idx=$((device_idx + 1))
    done
    
    _emit_footer "$batch_file"
done

# Create a list of all generated files for the Makefile
cd "$OUTPUT_DIR"
ls docker-compose.*.yml | grep -v "network" | sort > .batch-files
echo "Generated $(wc -l < .batch-files) regional compose files ($NODE_COUNT devices total)"

# ---------------------------------------------------------------------------
# Summary (stderr so callers can capture the file cleanly)
# ---------------------------------------------------------------------------
healthy=0
declare -A MODE_TALLY=()
for entry in "${DEVICES[@]}"; do
    IFS='|' read -r _ _ _ profile <<< "$entry"
    if [[ "$profile" == "healthy" ]]; then
        healthy=$((healthy + 1))
    else
        MODE_TALLY[$profile]=$(( ${MODE_TALLY[$profile]:-0} + 1 ))
    fi
done

{
    echo "Generated $batch_num batch compose files"
    echo "  total devices: $total"
    echo "  healthy:       $healthy"
    echo "  broken:        $(( total - healthy )) (~${FAILURE_RATIO_PERCENT}%)"
    for m in "${FAILURE_MODES[@]}"; do
        c="${MODE_TALLY[$m]:-0}"
        (( c > 0 )) && printf "    %-20s %d\n" "$m" "$c" || true
    done
} >&2
