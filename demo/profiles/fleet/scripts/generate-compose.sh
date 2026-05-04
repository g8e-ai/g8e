#!/usr/bin/env bash
# Generate docker-compose batch files for the fleet demo profile.
#
# Produces N globally-distributed, named web nodes.
# A deterministic subset (~FAILURE_RATIO percent) is assigned broken profiles.
#
# Splits the fleet into multiple batch compose files (default 100 per batch)
# to avoid Docker Compose timeout issues with large service counts.
#
# Usage: generate-compose.sh [NODE_COUNT] [BATCH_SIZE]    (default: 10, 100)

set -euo pipefail

NODE_COUNT="${1:-10}"
BATCH_SIZE="${2:-100}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$(pwd)"

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
# Failure catalog. Each broken node gets exactly one of these profiles.
# ---------------------------------------------------------------------------
FAILURE_MODES=(
    bad_upstream
    ssl_expired
    wrong_root
    high_load
    crashed
)

FAILURE_RATIO_PERCENT="${FAILURE_RATIO_PERCENT:-50}"

# ---------------------------------------------------------------------------
# Assign nodes.
# ---------------------------------------------------------------------------
declare -a DEVICES=()   # entries of form: location|profile

loc_idx=0
loc_count=${#LOCATIONS[@]}

for ((i=0; i<NODE_COUNT; i++)); do
    loc="${LOCATIONS[$((loc_idx % loc_count))]}"
    loc_idx=$((loc_idx + 1))
    DEVICES+=("$loc|healthy")
done

# ---------------------------------------------------------------------------
# Assign failure profiles deterministically.
# ---------------------------------------------------------------------------
total=${#DEVICES[@]}
broken_target=$(( (total * FAILURE_RATIO_PERCENT + 50) / 100 ))
(( broken_target < 1 && FAILURE_RATIO_PERCENT > 0 )) && broken_target=1

fmode_count=${#FAILURE_MODES[@]}

if (( broken_target > 0 )); then
    stride=$(( total / broken_target ))
    (( stride < 1 )) && stride=1
    for ((i=0; i<broken_target; i++)); do
        pos=$(( (i * stride + 3) % total ))
        IFS='|' read -r loc _ <<< "${DEVICES[$pos]}"
        fmode="${FAILURE_MODES[$((i % fmode_count))]}"
        DEVICES[$pos]="$loc|$fmode"
    done
fi

# ---------------------------------------------------------------------------
# Emit header to a batch file
# ---------------------------------------------------------------------------
_emit_header() {
    local batch_file="$1"
    cat > "$batch_file" <<'EOF'
# Web App Fleet Demo (GENERATED - do not edit by hand)
#
# Batch of nodes. See docker-compose.yml for full fleet configuration.
# Regenerate via `make generate-compose` or `./scripts/generate-compose.sh`.

networks:
  fleet-net:
    external: true

x-operator-env: &operator-env
  DEVICE_TOKEN: ${DEVICE_TOKEN:-}
  G8E_ENDPOINT: ${G8E_ENDPOINT:-g8e.local}

x-web-node: &web-node
  image: web-node:latest
  networks:
    - fleet-net
  extra_hosts:
    - "g8e.local:host-gateway"
    - "g8ed:host-gateway"
    - "g8es:host-gateway"
  labels:
    - "demo.service=web-node"
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
# Emit a node service to a batch file
# ---------------------------------------------------------------------------
_emit_node() {
    local batch_file="$1"
    local name="$2"
    local loc="$3"
    local profile="$4"

    cat >> "$batch_file" <<EOF
  ${name}:
    <<: *web-node
    container_name: web-${name}
    hostname: ${name}
    environment:
      <<: *operator-env
      NODE_ID: ${name}
      NODE_LOCATION: ${loc}
      NODE_PROFILE: ${profile}

EOF
}

# ---------------------------------------------------------------------------
# Generate regional compose files
# ---------------------------------------------------------------------------
rm -f "$OUTPUT_DIR"/docker-compose.*.yml "$OUTPUT_DIR"/.batch-files

# Create network definition
cat > "$OUTPUT_DIR/docker-compose.network.yml" <<'EOF'
networks:
  fleet-net:
    driver: bridge
    ipam:
      config:
        - subnet: 10.210.0.0/16

services:
  dashboard:
    image: fleet-dashboard:latest
    container_name: fleet-dashboard
    hostname: dashboard
    networks:
      - fleet-net
    ports:
      - "3000:3000"
    labels:
      - "demo.service=dashboard"
EOF

# Group nodes by region
declare -A REGION_DEVICES
for i in "${!DEVICES[@]}"; do
    IFS='|' read -r loc profile <<< "${DEVICES[$i]}"
    region=$(_get_region "$loc")
    REGION_DEVICES[$region]+="$loc|$profile"$'\n'
done

# Emit files per region, with batching if a region is too large
declare -A REGION_COUNTER=()
for region in "americas" "europe" "apac" "mea"; do
    [[ -z "${REGION_DEVICES[$region]:-}" ]] && continue
    
    IFS=$'\n' read -d '' -ra region_list <<< "${REGION_DEVICES[$region]}" || true
    
    batch_num=1
    node_idx=0
    
    batch_file="$OUTPUT_DIR/docker-compose.${region}-${batch_num}.yml"
    _emit_header "$batch_file"
    
    for entry in "${region_list[@]}"; do
        [[ -z "$entry" ]] && continue
        IFS='|' read -r loc profile <<< "$entry"
        
        if (( node_idx > 0 && node_idx % BATCH_SIZE == 0 )); then
            _emit_footer "$batch_file"
            batch_num=$((batch_num + 1))
            batch_file="$OUTPUT_DIR/docker-compose.${region}-${batch_num}.yml"
            _emit_header "$batch_file"
        fi
        
        count="${REGION_COUNTER[$loc]:-0}"
        count=$((count + 1))
        REGION_COUNTER[$loc]=$count
        seq=$(printf "%02d" "$count")
        name="node-${loc}-${seq}"
        
        _emit_node "$batch_file" "$name" "$loc" "$profile"
        node_idx=$((node_idx + 1))
    done
    
    _emit_footer "$batch_file"
done

# Create a list of all generated files for the Makefile
cd "$OUTPUT_DIR"
ls docker-compose.*.yml | grep -v "network" | sort > .batch-files
echo "Generated $(wc -l < .batch-files) regional compose files ($NODE_COUNT nodes total)"
