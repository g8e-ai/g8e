#!/usr/bin/env bash
# Generate docker-compose batch files for the large-fleet demo profile.
#
# Produces N globally-distributed, featherweight nodes.
#
# Usage: generate-compose.sh [NODE_COUNT] [BATCH_SIZE]    (default: 100, 100)

set -euo pipefail

NODE_COUNT="${1:-100}"
BATCH_SIZE="${2:-100}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$(pwd)"

# ---------------------------------------------------------------------------
# Global locations (IATA-ish 3-letter codes).
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
# Assign nodes.
# ---------------------------------------------------------------------------
declare -a DEVICES=()

loc_idx=0
loc_count=${#LOCATIONS[@]}

for ((i=0; i<NODE_COUNT; i++)); do
    loc="${LOCATIONS[$((loc_idx % loc_count))]}"
    loc_idx=$((loc_idx + 1))
    DEVICES+=("$loc|healthy")
done

# ---------------------------------------------------------------------------
# Emit header to a batch file
# ---------------------------------------------------------------------------
_emit_header() {
    local batch_file="$1"
    cat > "$batch_file" <<'EOF'
# Large Fleet Simulator (GENERATED - do not edit by hand)

networks:
  large-fleet-net:
    external: true

x-node-device: &node-device
  image: operator-node:latest
  networks:
    - large-fleet-net
  extra_hosts:
    - "g8e.local:host-gateway"
    - "g8ed:host-gateway"
    - "g8es:host-gateway"
  environment:
    - DEVICE_TOKEN=${DEVICE_TOKEN:-}
    - G8E_ENDPOINT=${G8E_ENDPOINT:-g8e.local}
  labels:
    - "demo.service=operator-node"
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
    cat >> "$batch_file" <<EOF
  ${name}:
    <<: *node-device
    container_name: large-${name}
    hostname: ${name}

EOF
}

# ---------------------------------------------------------------------------
# Generate regional compose files
# ---------------------------------------------------------------------------
rm -f "$OUTPUT_DIR"/docker-compose.*.yml "$OUTPUT_DIR"/.batch-files

# Create network definition
cat > "$OUTPUT_DIR/docker-compose.network.yml" <<'EOF'
networks:
  large-fleet-net:
    driver: bridge
    ipam:
      config:
        - subnet: 10.220.0.0/16
EOF

# Group nodes by region
declare -A REGION_DEVICES
for i in "${!DEVICES[@]}"; do
    IFS='|' read -r loc profile <<< "${DEVICES[$i]}"
    region=$(_get_region "$loc")
    REGION_DEVICES[$region]+="$loc|$profile"$'\n'
done

# Emit files per region
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
        IFS='|' read -r loc _ <<< "$entry"
        
        if (( node_idx > 0 && node_idx % BATCH_SIZE == 0 )); then
            _emit_footer "$batch_file"
            batch_num=$((batch_num + 1))
            batch_file="$OUTPUT_DIR/docker-compose.${region}-${batch_num}.yml"
            _emit_header "$batch_file"
        fi
        
        count="${REGION_COUNTER[$loc]:-0}"
        count=$((count + 1))
        REGION_COUNTER[$loc]=$count
        seq=$(printf "%03d" "$count")
        name="node-${loc}-${seq}"
        
        _emit_node "$batch_file" "$name"
        node_idx=$((node_idx + 1))
    done
    
    _emit_footer "$batch_file"
done

# Create a list of all generated files for the Makefile
cd "$OUTPUT_DIR"
ls docker-compose.*.yml | grep -v "network" | sort > .batch-files
echo "Generated $(wc -l < .batch-files) regional compose files ($NODE_COUNT nodes total)"
