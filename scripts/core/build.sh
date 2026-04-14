#!/bin/bash
# Platform lifecycle management for the local g8e environment.
#
# Service categories:
#   Managed:  g8es, g8ee, g8ed, g8ep  (in scope for up/rebuild/clean)
#   Data volumes:
#     g8es-data     (g8es -- SQLite DB, users, settings; wiped by reset)
#     g8es-ssl      (g8es -- TLS certs; NEVER wiped by reset or wipe)
#     g8ee-data    (g8ee   -- app data; wiped by reset)
#     g8ed-data (g8ed  -- app data; wiped by reset)
#   Excluded from reset: g8ep (reset targets core data services only)
#
# Prerequisites:
#   - Docker and docker compose available
#
# Invoked via: ./g8e platform <subcommand>

set -e

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  build.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  build.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

MANAGED_SERVICES=(g8es g8ee g8ed g8ep)

_service_volume() {
    case "$1" in
        g8es)     echo "g8es-data" ;;
        g8ee)   echo "g8ee-data" ;;
        g8ed) echo "g8ed-data" ;;
    esac
}

# SSL volume is never wiped — preserved across reset, wipe, and rebuild.
SSL_VOLUME="g8es-ssl"

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [options]

Commands:
  status                          Show container status and component versions
  up [component ...]              Start managed services -- no build
                                  Default (no components): g8es g8ee g8ed g8ep
                                  Valid: g8es g8ee g8ed g8ep
  down                            Stop managed containers -- nothing is removed
  rebuild [component ...]         Rebuild + restart of managed services using layer cache (no volume wipe)
                                  Default (no components): g8es g8ee g8ed g8ep
                                  Valid: g8es g8ee g8ed g8ep
  reset                           Wipe DB data volumes + rebuild images from scratch
                                  Removes: g8es, g8ee, g8ed volumes; SSL certs preserved
  wipe                            Clear app data from the database (all collections except platform settings)
                                  g8es stays up; preserves: platform settings, SSL certs, auth token
  clean                           Nuke all managed Docker resources (containers, images,
                                  volumes, networks), dangling images, orphaned networks,
                                  and all build cache layers (docker builder prune -f)
  setup                           Full first-time setup: no-cache build of all images, start platform
  operator-build                  Build linux/amd64 operator binary inside g8ep (no compression)
  operator-build-all              Build all operator architectures with compression (for distribution)

Examples:
  $(basename "$0") status                       Show container status and versions
  $(basename "$0") up                           Start the environment (no build)
  $(basename "$0") up g8ep                  Start only the g8ep container
  $(basename "$0") down                         Stop containers (preserve everything)
  $(basename "$0") rebuild                      Rebuild g8es, g8ee, g8ed images (preserve volumes)
  $(basename "$0") rebuild g8ee g8ed  Rebuild g8ee and g8ed only (preserve volumes)
  $(basename "$0") rebuild g8ep             Rebuild the g8ep image
  $(basename "$0") wipe                         Clear app data from the database; restart g8ee/g8ed
  $(basename "$0") reset                        Wipe ALL data volumes and rebuild from scratch
  $(basename "$0") clean                        Full Docker cleanup (managed services only)
EOF
}

COMMAND=""
REBUILD_COMPONENTS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        setup|up|down|restart|reset|wipe|clean|status|operator-build|operator-build-all)
            COMMAND="$1"
            shift
            while [[ $# -gt 0 && ! "$1" =~ ^- ]]; do
                if ! printf '%s\n' g8es g8ee g8ed g8ep | grep -qx "$1"; then
                    echo "Error: Invalid component '$1'" >&2
                    echo "Valid: g8es g8ee g8ed g8ep" >&2
                    exit 1
                fi
                REBUILD_COMPONENTS+=("$1")
                shift
            done
            ;;
        rebuild)
            COMMAND="rebuild"
            shift
            while [[ $# -gt 0 && ! "$1" =~ ^- ]]; do
                if ! printf '%s\n' g8es g8ee g8ed g8ep | grep -qx "$1"; then
                    echo "Error: Invalid component '$1'" >&2
                    echo "Valid: g8es g8ee g8ed g8ep" >&2
                    exit 1
                fi
                REBUILD_COMPONENTS+=("$1")
                shift
            done
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

# ─── Helpers ──────────────────────────────────────────────────────────────────


_is_running() {
    docker ps --filter "name=^$1$" --filter "status=running" --format "{{.Names}}" | grep -q "^$1$"
}

_ensure_g8ep() {
    if ! _is_running "g8ep"; then
        echo "g8ep container is not running — starting it..."
        docker compose -f "$PROJECT_ROOT/docker-compose.yml" up -d g8ep
    fi
}

_wait_healthy() {
    local name="$1" timeout_s="$2" interval="${3:-1}"
    local waited=0
    echo "  $name: waiting for healthy status..."
    
    # Start tailing logs in the background, filtered for entrypoint/startup messages
    docker logs -f "$name" 2>&1 | grep --line-buffered -E "\[.*ENTRYPOINT\]|uvicorn|started|error|ERROR|Ready|ready" | sed "s/^/    [$name] /" &
    local log_pid=$!
    
    # Ensure log tailing is killed on exit
    trap "kill $log_pid 2>/dev/null || true" RETURN

    until [ "$(docker inspect --format='{{.State.Health.Status}}' "$name" 2>/dev/null)" = "healthy" ]; do
        if (( waited >= timeout_s )); then
            kill $log_pid 2>/dev/null || true
            echo -e "  $name: \033[0;31mTIMEOUT\033[0m"
            echo "  $name did not become healthy within ${timeout_s}s. Last logs:"
            docker logs --tail 30 "$name" 2>&1 | sed 's/^/    /'
            exit 1
        fi
        local status
        status=$(docker inspect --format='{{.State.Health.Status}}' "$name" 2>/dev/null || echo "starting")
        # Update status line without interfering with streamed logs
        # echo -ne "\r  $name: \033[0;33m$status\033[0m (${waited}s)   " >&2
        sleep "$interval"
        waited=$(( waited + interval ))
    done
    
    kill $log_pid 2>/dev/null || true
    echo -e "  $name: \033[0;32mready\033[0m (${waited}s)"
}

_wait_curl() {
    local name="$1" url="$2" grep_pattern="$3" timeout_s="$4" interval="$5"
    local waited=0
    echo "  $name: waiting for endpoint $url..."

    # Start tailing logs in the background
    docker logs -f "$name" 2>&1 | grep --line-buffered -E "\[.*ENTRYPOINT\]|uvicorn|started|error|ERROR|Ready|ready|GET|POST" | sed "s/^/    [$name] /" &
    local log_pid=$!
    
    trap "kill $log_pid 2>/dev/null || true" RETURN

    until curl -sfk "$url" 2>/dev/null | grep -q "$grep_pattern"; do
        if (( waited >= timeout_s )); then
            kill $log_pid 2>/dev/null || true
            echo -e "  $name: \033[0;31mTIMEOUT\033[0m"
            echo "  $name did not become healthy within ${timeout_s}s. Last logs:"
            docker logs --tail 30 "$name" 2>&1 | sed 's/^/    /'
            exit 1
        fi
        sleep "$interval"
        waited=$(( waited + interval ))
    done
    
    kill $log_pid 2>/dev/null || true
    echo -e "  $name: \033[0;32mready\033[0m (${waited}s)"
}

_load_env() {
    if [[ -z "${DOCKER_GID:-}" ]]; then
        DOCKER_GID="$(getent group docker 2>/dev/null | cut -d: -f3 || true)"
        [[ -z "$DOCKER_GID" ]] && DOCKER_GID="0"
        export DOCKER_GID
    fi
    if [[ -z "${G8E_VERSION:-}" ]]; then
        G8E_VERSION="$(cat "$PROJECT_ROOT/VERSION" 2>/dev/null | tr -d '[:space:]' || echo 'dev')"
        export G8E_VERSION
    fi
}

_preflight() {
    _load_env

    HOST_IPS=""
    if command -v ip >/dev/null 2>&1; then
        HOST_IPS=$(ip -4 addr show scope global | awk '/inet / {split($2,a,"/"); print a[1]}' | tr '\n' ',' | sed 's/,$//')
    elif command -v ifconfig >/dev/null 2>&1; then
        HOST_IPS=$(ifconfig | awk '/inet / && !/127\.0\.0\.1/ {print $2}' | sed 's/addr://' | tr '\n' ',' | sed 's/,$//')
    fi
    export HOST_IPS
}

# ─── Startup ──────────────────────────────────────────────────────────────────

_load_env

_print_platform_info() {
    local _https_port="${HTTPS_PORT:-443}"
    local _app_url="${APP_URL:-}"
    local _dashboard_url
    local _ssl_setup_url="https://127.0.0.1/trust"
    
    if [[ -n "$_app_url" ]]; then
        _dashboard_url="$_app_url"
    elif [[ "$_https_port" == "443" ]]; then
        _dashboard_url="https://localhost"
    else
        _dashboard_url="https://localhost"
    fi
    
    echo "  Dashboard : https://localhost"
    echo "  SSL Setup  : https://127.0.0.1/trust"
}

if [[ -z "$COMMAND" ]]; then
    echo "Error: no command specified." >&2
    usage >&2
    exit 1
fi

cd "$PROJECT_ROOT"

# ─── status ───────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "status" ]]; then
    _VER="$(cat "$PROJECT_ROOT/VERSION" 2>/dev/null | tr -d '[:space:]' \
        || git -C "$PROJECT_ROOT" describe --tags --abbrev=0 2>/dev/null \
        || echo 'unknown')"
    [[ "$_VER" != v* ]] && _VER="v$_VER"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Component Versions"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    for svc in platform g8es g8ee g8ed g8ep; do
        printf "  %-14s  %s\n" "$svc" "$_VER"
    done
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Container Status"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
    exit 0
fi

# ─── down ─────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "down" ]]; then
    echo "Stopping managed containers (g8es, g8ee, g8ed, g8ep)..."
    docker compose stop g8es g8ee g8ed g8ep 2>/dev/null || true
    echo "Done. Volumes, images, and networks are preserved."
    exit 0
fi

# ─── restart ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "restart" ]]; then
    _preflight
    echo "Restarting managed containers (g8es, g8ee, g8ed, g8ep)..."
    docker compose stop g8es g8ee g8ed g8ep 2>/dev/null || true
    docker compose up -d g8es g8ee g8ed g8ep
    echo ""
    echo "Waiting for services..."
    _wait_healthy g8es     60  1
    _wait_healthy g8ee    120 2
    _wait_curl    g8ed "https://localhost/health" '"status"' 120 2
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Restart complete."
    echo ""
    docker compose ps --format "table {{.Name}}\t{{.Status}}"
    exit 0
fi

# ─── reset ───────────────────────────────────────────────────────────────────
# Wipes DB data volumes (g8es, g8ee, g8ed). SSL certs are preserved in the
# separate g8es-ssl volume — no need to re-trust after reset.
# Use 'clean' to remove everything including SSL.

if [[ "$COMMAND" == "reset" ]]; then
    REBUILD_COMPONENTS=(g8es g8ee g8ed)

    echo "Wiping DB data volumes (g8es, g8ee, g8ed) — SSL certs preserved..."
    docker compose stop g8es g8ee g8ed g8ep 2>/dev/null || true
    docker ps -aq --filter "name=^g8es$|^g8ee$|^g8ed$|^g8ep$" 2>/dev/null | xargs -r docker rm -f 2>/dev/null || true
    for svc in g8es g8ee g8ed; do
        vol="$(_service_volume "$svc")"
        [[ -n "$vol" ]] && docker volume rm "$vol" 2>/dev/null || true
    done
    docker volume rm g8ed-node-modules 2>/dev/null || true
    echo ""

    _preflight

    echo "Rebuilding all images (no cache)..."
    docker compose build --no-cache --pull=false --parallel g8es g8ee g8ed g8ep

    echo "Starting g8es (compiles and publishes operator binaries)..."
    docker compose up -d --force-recreate g8es
    _wait_healthy g8es 300 2

    echo "Starting remaining services..."
    docker compose up -d --force-recreate g8ee g8ed g8ep
    echo ""
    echo "Waiting for services..."
    _wait_healthy g8ee    120 2
    _wait_curl    g8ed "https://localhost/health" '"status"' 120 2

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Reset complete. SSL certs preserved — no need to re-trust."
    echo ""
    docker compose ps --format "table {{.Name}}\t{{.Status}}"
    exit 0
fi

# ─── wipe ─────────────────────────────────────────────────────────────────────
# Clears all app data from the g8es database via the HTTP API.
# Preserves: platform settings (components collection), SSL certs, auth token, LLM data.
# g8es is restarted to flush in-memory state; no volume wipe, no rebuild.
# Use 'reset' to wipe DB data volumes and rebuild from scratch (SSL still preserved).

if [[ "$COMMAND" == "wipe" ]]; then
    _preflight
    _ensure_g8ep

    echo "Stopping g8ee, g8ed, and g8es..."
    docker compose stop g8ee g8ed g8es 2>/dev/null || true
    docker compose rm -f g8ee g8ed g8es 2>/dev/null || true
    echo ""

    echo "Restarting g8es (compiles and publishes operator binaries)..."
    docker compose up -d --force-recreate g8es
    _wait_healthy g8es 120 2

    echo "Clearing app data from g8es..."
    docker exec -i g8ep python3 /app/scripts/data/manage-g8es.py store wipe
    echo ""

    echo "Restarting g8ee and g8ed..."
    docker compose up -d --force-recreate g8ee g8ed
    echo ""
    echo "Waiting for services..."
    _wait_healthy g8ee    120 2
    _wait_curl    g8ed "https://localhost/health" '"status"' 120 2
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Wipe complete. Platform settings, SSL certs, and auth token preserved."
    echo ""
    docker compose ps --format "table {{.Name}}\t{{.Status}}"
    exit 0
fi

# ─── clean ────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "clean" ]]; then
    echo "Cleaning managed Docker resources..."

    FILTER_MANAGED="label=io.g8e.managed=true"

    # 1. Identify and remove containers
    containers=$(docker ps -aq --filter "$FILTER_MANAGED" 2>/dev/null)
    if [[ -n "$containers" ]]; then
        echo "Removing containers..."
        echo "$containers" | xargs -r docker rm -f 2>/dev/null || true
    fi

    # 2. Identify and remove volumes
    volumes=$(docker volume ls -q --filter "$FILTER_MANAGED" 2>/dev/null)
    if [[ -n "$volumes" ]]; then
        echo "Removing managed volumes..."
        echo "$volumes" | xargs -r docker volume rm 2>/dev/null || true
    fi

    # 3. Identify and remove networks
    networks=$(docker network ls -q --filter "$FILTER_MANAGED" 2>/dev/null)
    if [[ -n "$networks" ]]; then
        echo "Removing managed networks..."
        echo "$networks" | xargs -r docker network rm 2>/dev/null || true
    fi

    # 4. Identify and remove images
    images=$(docker images -q --filter "$FILTER_MANAGED" 2>/dev/null | sort -u)
    if [[ -n "$images" ]]; then
        echo "Removing managed images..."
        echo "$images" | xargs -r docker rmi -f 2>/dev/null || true
    fi

    # 5. Final prune for any orphaned project resources (anonymous volumes, etc.)
    echo "Pruning remaining project-labeled resources..."
    docker image prune -f --filter "label=com.docker.compose.project=g8e" 2>/dev/null || true
    docker volume prune -f --filter "label=com.docker.compose.project=g8e" 2>/dev/null || true

    # 6. Prune dangling images (untagged intermediate layers not caught by label filters)
    echo "Pruning dangling images..."
    docker image prune -f 2>/dev/null || true

    # 7. Prune orphaned networks not covered by label filters
    echo "Pruning orphaned networks..."
    docker network prune -f 2>/dev/null || true

    # 8. Prune dangling build cache layers (preserves base image cache)
    echo "Pruning dangling build cache..."
    docker builder prune -f 2>/dev/null || true

    echo ""
    echo "Done."
    exit 0
fi

# ─── Preflight (up and rebuild) ───────────────────────────────────────────────

_preflight

# ─── up ───────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "up" ]]; then
    UP_COMPONENTS=("${REBUILD_COMPONENTS[@]}")
    if [[ ${#UP_COMPONENTS[@]} -eq 0 ]]; then
        UP_COMPONENTS=(g8es g8ee g8ed g8ep)
    fi
    echo "Starting services (no build): ${UP_COMPONENTS[*]}..."
    docker compose up -d $(printf '%s\n' "${UP_COMPONENTS[@]}" | tr '\n' ' ')
    echo ""
    echo "Waiting for services..."
    printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx g8es     && _wait_healthy g8es     60  1
    printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx g8ee  && _wait_healthy g8ee    120 2
    printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx g8ed && _wait_curl    g8ed "https://localhost/health" '"status"' 120 2

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Environment ready."
    echo ""
    docker compose ps --format "table {{.Name}}\t{{.Status}}"
    echo ""
    _print_platform_info
    exit 0
fi

# ─── setup ───────────────────────────────────────────────────────────────────
# Full first-time setup: no-cache build of all images, then start the platform.
# Does NOT wipe data volumes — safe to run on an existing installation.
# g8es image bakes all 3 operator binaries (amd64/arm64/386) with UPX compression;
# on container start, g8es uploads them to the blob store automatically.

if [[ "$COMMAND" == "setup" ]]; then
    echo "Stopping all managed containers..."
    docker compose stop g8es g8ee g8ed g8ep 2>/dev/null || true
    docker compose rm -f g8es g8ee g8ed g8ep 2>/dev/null || true

    echo "Building all images (no cache)..."
    docker compose build --no-cache --pull=false --parallel g8es g8ee g8ed g8ep

    echo "Starting g8es (compiles and publishes operator binaries)..."
    docker compose up -d --force-recreate g8es
    _wait_healthy g8es 300 2

    echo "Starting remaining services..."
    docker compose up -d --force-recreate g8ee g8ed g8ep
    echo ""
    echo "Waiting for services..."
    _wait_healthy g8ee    120 2
    _wait_curl    g8ed "https://localhost/health" '"status"' 120 2

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Setup complete."
    echo ""
    docker compose ps --format "table {{.Name}}\t{{.Status}}"
    echo ""
    _print_platform_info
    exit 0
fi

# ─── rebuild ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "rebuild" ]]; then
    if [[ ${#REBUILD_COMPONENTS[@]} -eq 0 ]]; then
        REBUILD_COMPONENTS=(g8es g8ee g8ed g8ep)
    fi

    echo "Removing containers for: ${REBUILD_COMPONENTS[*]}..."
    docker compose rm -f "${REBUILD_COMPONENTS[@]}" 2>/dev/null || true

    echo "Rebuilding (with cache): ${REBUILD_COMPONENTS[*]}..."
    docker compose build --pull=false --parallel "${REBUILD_COMPONENTS[@]}"

    NEEDS_G8E_DATA_FIRST=false
    if printf '%s\n' "${REBUILD_COMPONENTS[@]}" | grep -qx g8es; then
        NEEDS_G8E_DATA_FIRST=true
    fi

    if [[ "$NEEDS_G8E_DATA_FIRST" == "true" ]]; then
        echo "Starting g8es (compiles and publishes operator binaries)..."
        docker compose up -d --force-recreate g8es
        _wait_healthy g8es 300 2
    fi

    STARTABLE=($(printf '%s\n' "${REBUILD_COMPONENTS[@]}" | grep -v '^g8es$'))
    if [[ ${#STARTABLE[@]} -gt 0 ]]; then
        echo ""
        echo "Restarting: ${STARTABLE[*]}..."
        docker compose up -d --force-recreate "${STARTABLE[@]}"
        echo ""
        echo "Waiting for services..."
        printf '%s\n' "${STARTABLE[@]}" | grep -qx g8ee  && _wait_healthy g8ee    120 2
        printf '%s\n' "${STARTABLE[@]}" | grep -qx g8ed && _wait_curl    g8ed "https://localhost/health" '"status"' 30 2
    fi

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Rebuild complete."
    echo ""
    docker compose ps --format "table {{.Name}}\t{{.Status}}"
    echo ""
    _print_platform_info
    exit 0
fi

# ─── operator-build ─────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "operator-build" ]]; then
    _ensure_g8ep
    echo "Building linux/amd64 operator binary and uploading to g8es blob store..."
    docker exec g8ep bash -c "cd /app/components/g8eo && make build"
    echo ""
    echo "Operator binary built and uploaded to g8es blob store."
    exit 0
fi

# ─── operator-build-all ─────────────────────────────────────────────────────────

if [[ "$COMMAND" == "operator-build-all" ]]; then
    _ensure_g8ep
    echo "Building all operator architectures and uploading to g8es blob store..."
    docker exec g8ep bash -c "cd /app/components/g8eo && make build-all"
    echo ""
    echo "All operator binaries built and uploaded to g8es blob store."
    exit 0
fi
