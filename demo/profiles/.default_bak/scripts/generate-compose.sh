#!/usr/bin/env bash
set -e

NODE_COUNT=${1:-10}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="$(dirname "$SCRIPT_DIR")"
COMPOSE_FILE="$DEMO_DIR/docker-compose.yml"

# Node profiles to cycle through
PROFILES=(
    "healthy"
    "healthy"
    "healthy"
    "healthy"
    "healthy"
    "bad_upstream"
    "ssl_expired"
    "wrong_root"
    "high_load"
    "crashed"
)

cat > "$COMPOSE_FILE" <<'EOF'
# Broken Web App Fleet Demo - Nginx Nodes
# Each node runs nginx proxying to a Flask backend app.
# Various nodes have different misconfigurations for g8e to discover and fix.

networks:
  fleet-net:
    driver: bridge
    ipam:
      config:
        - subnet: 10.210.0.0/24
  g8e-network:
    external: true

# Shared operator env — merged into each node's environment map below.
x-operator-env: &operator-env
  G8E_ENDPOINT: ${G8E_ENDPOINT:-g8e.local}

x-web-node: &web-node
  build:
    context: ./containers/web-node
    dockerfile: Dockerfile
  networks:
    - fleet-net
    - g8e-network
  expose:
    - "22"
    - "5000"
    - "8181"
    - "8443"
  stdin_open: true
  tty: true
  labels:
    - "demo.service=web-node"
  volumes:
    - g8es-ssl:/g8es:ro

services:
EOF

# Generate node services
for i in $(seq 1 "$NODE_COUNT"); do
    NODE_NUM=$(printf "%02g" "$i")
    PROFILE_INDEX=$(( (i - 1) % ${#PROFILES[@]} ))
    PROFILE="${PROFILES[$PROFILE_INDEX]}"
    
    cat >> "$COMPOSE_FILE" <<EOF
  node-$NODE_NUM:
    <<: *web-node
    container_name: web-node-$NODE_NUM
    hostname: node-$NODE_NUM
    environment:
      <<: *operator-env
      NODE_ID: node-$NODE_NUM
      NODE_PROFILE: $PROFILE

EOF
done

# Add dashboard service
cat >> "$COMPOSE_FILE" <<'EOF'
  dashboard:
    build:
      context: ./containers/dashboard
      dockerfile: Dockerfile
    container_name: fleet-dashboard
    hostname: dashboard
    networks:
      - fleet-net
    ports:
      - "3000:3000"
    volumes:
      - ./containers/dashboard/server.js:/app/server.js
      - ./containers/dashboard/public:/app/public
    labels:
      - "demo.service=dashboard"


volumes:
  g8es-ssl:
    external: true
EOF

echo "Generated docker-compose.yml with $NODE_COUNT nodes"
