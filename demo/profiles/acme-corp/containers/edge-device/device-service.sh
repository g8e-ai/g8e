#!/bin/bash
# ACME edge-device simulated application.
#
# Reads /etc/device/config.yaml on startup, emits realistic logs + metrics,
# and honors the failure knobs set there. Designed to be diagnosed and fixed
# by an AI operator purely through log inspection + config edits + restart.

set -u

CONFIG="/etc/device/config.yaml"
CERT_FILE="/etc/device/certs/client.crt"
LOG_DIR="/var/log/device"
LOG_FILE="$LOG_DIR/service.log"
METRICS_FILE="$LOG_DIR/metrics.json"
CACHE_DIR="/var/lib/device/cache"

mkdir -p "$LOG_DIR" "$CACHE_DIR"

# ---------------------------------------------------------------------------
# Config parsing. Simple flat YAML - extract "leaf: value" under any section.
# ---------------------------------------------------------------------------
cfg() {
    local key="$1" default="${2:-}"
    local val
    val="$(awk -v k="$key" '$1 == k":" { sub(/^[^:]+: */, ""); print; exit }' "$CONFIG" 2>/dev/null)" || val=""
    [[ -z "$val" ]] && val="$default"
    echo "$val"
}

# Guard: cannot read config at all → bail out loud.
if ! [[ -r "$CONFIG" ]]; then
    ts="$(date -Iseconds)"
    echo "[$ts] [FATAL] cannot read config file $CONFIG: permission denied"
    exit 2
fi

NAME="$(cfg name unknown)"
FUNCTION="$(cfg function unknown)"
SITE="$(cfg site unknown)"
LOCATION="$(cfg location unknown)"
UPSTREAM_URL="$(cfg url "http://acme-ingest.internal")"
UPSTREAM_PORT="$(cfg port "8443")"
LOG_INTERVAL="$(cfg log_interval_sec 5)"
HEARTBEAT_INTERVAL="$(cfg heartbeat_interval_sec 30)"
ERROR_INJECT="$(cfg error_injection_percent 0)"
CRASH_ON_START="$(cfg crash_on_start false)"
STUCK_LOOP="$(cfg stuck_loop false)"
LEAK_RATE="$(cfg memory_leak_mb_per_min 0)"

log() {
    local level="$1"; shift
    local msg="$*"
    local ts
    ts="$(date -Iseconds)"
    printf '[%s] [%-5s] %s\n' "$ts" "$level" "$msg" | tee -a "$LOG_FILE"
}

# ---------------------------------------------------------------------------
# Function-specific "activity" phrases - makes logs feel purposeful and
# helps the AI recognise each device's role from its output alone.
# ---------------------------------------------------------------------------
activity() {
    case "$FUNCTION" in
        pos)        echo "processed transaction amount=\$$((RANDOM % 9000 + 100)).00 tender=card";;
        kiosk)      echo "session started checkin_flow=$((RANDOM % 4 + 1)) duration_ms=$((RANDOM % 1500 + 200))";;
        scanner)    echo "scanned barcode=$(printf '%012d' $((RANDOM * 1000)) ) bin=$((RANDOM % 999))";;
        camera)     echo "motion_detected zone=$((RANDOM % 8)) confidence=0.$((RANDOM % 100))";;
        printer)    echo "print_job pages=$((RANDOM % 20 + 1)) queue_depth=$((RANDOM % 5))";;
        badge)      echo "access_granted badge_id=B$((RANDOM * 100)) door=$((RANDOM % 16))";;
        sensor)     echo "reading temp_c=$((RANDOM % 20 + 15)).$((RANDOM % 10)) humidity=$((RANDOM % 40 + 30))";;
        controller) echo "plc_tick program=acme.prg step=$((RANDOM % 200)) cycle_ms=$((RANDOM % 50 + 10))";;
        gateway)    echo "forwarded msgs=$((RANDOM % 200 + 10)) uplink_latency_ms=$((RANDOM % 80 + 5))";;
        router)     echo "bgp_keepalive peers=$((RANDOM % 8 + 2)) routes=$((RANDOM % 5000 + 1000))";;
        logger)     echo "rotated segment seq=$((RANDOM % 9999)) size_mb=$((RANDOM % 500 + 100))";;
        probe)      echo "probe target=acme-core.internal rtt_ms=$((RANDOM % 30 + 1))";;
        *)          echo "heartbeat";;
    esac
}

# ---------------------------------------------------------------------------
# Metrics writer - a single JSON object refreshed on each tick.
# ---------------------------------------------------------------------------
write_metrics() {
    local uptime mem_mb disk_pct
    uptime=$(awk '{print int($1)}' /proc/uptime 2>/dev/null || echo 0)
    mem_mb=$(awk '/MemAvailable/ {print int((t - $2) / 1024)} /MemTotal/ {t=$2}' /proc/meminfo 2>/dev/null)
    [[ -z "$mem_mb" ]] && mem_mb=0
    disk_pct=$(df -P /var/lib/device 2>/dev/null | awk 'NR==2 {sub(/%/,"",$5); print $5}')
    [[ -z "$disk_pct" ]] && disk_pct=0

    # Simulated memory leak (reports higher each call if leak is active)
    local leak_extra=$(( LEAK_RATE * uptime / 60 ))
    mem_mb=$(( mem_mb + leak_extra ))

    cat > "$METRICS_FILE" <<EOF
{
  "device": "$NAME",
  "function": "$FUNCTION",
  "site": "$SITE",
  "location": "$LOCATION",
  "uptime_seconds": $uptime,
  "memory_mb": $mem_mb,
  "disk_usage_percent": $disk_pct,
  "upstream": "$UPSTREAM_URL:$UPSTREAM_PORT"
}
EOF
}

# ---------------------------------------------------------------------------
# Startup-time failure checks
# ---------------------------------------------------------------------------
log INFO "device-service starting name=$NAME function=$FUNCTION site=$SITE location=$LOCATION"

if [[ "$CRASH_ON_START" == "true" ]]; then
    log FATAL "crash_on_start=true in config; aborting. Remove this flag and restart."
    exit 1
fi

# Cert sanity check (parses the simulated cert's not_after date)
if [[ -r "$CERT_FILE" ]]; then
    not_after="$(awk '/not_after:/ {print $2}' "$CERT_FILE" | head -n1)"
    if [[ -n "$not_after" ]]; then
        exp_epoch=$(date -d "$not_after" +%s 2>/dev/null || echo 0)
        now_epoch=$(date +%s)
        if (( exp_epoch > 0 && exp_epoch < now_epoch )); then
            log ERROR "TLS client cert expired at $not_after - upstream sync will fail until cert is rotated (/opt/device/rotate-cert.sh)"
        fi
    fi
fi

if [[ "$STUCK_LOOP" == "true" ]]; then
    log WARN "stuck_loop=true in config; entering no-op wait. Flip to false and restart to recover."
    # Block forever until killed - a classic hung service.
    while true; do sleep 3600; done
fi

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------
tick=0
next_heartbeat=0

while true; do
    tick=$((tick + 1))

    # --- Disk-full watch (fires if staging.bin is lurking)
    if [[ -f "$CACHE_DIR/staging.bin" ]]; then
        size_mb=$(du -m "$CACHE_DIR/staging.bin" 2>/dev/null | awk '{print $1}')
        log ERROR "cache disk pressure: $CACHE_DIR/staging.bin is ${size_mb}MB - cannot flush telemetry"
    fi

    # --- Upstream sync (simulated; detect configured-but-bogus endpoints)
    if [[ "$UPSTREAM_URL" != "http://acme-ingest.internal" ]]; then
        log ERROR "upstream sync failed: DNS resolution failed for '$UPSTREAM_URL' (check upstream.url in $CONFIG)"
    elif [[ "$UPSTREAM_PORT" != "8443" ]]; then
        log ERROR "upstream sync failed: connection refused to ${UPSTREAM_URL}:${UPSTREAM_PORT} (check upstream.port)"
    fi

    # --- Normal activity (with optional error injection)
    if (( ERROR_INJECT > 0 )) && (( RANDOM % 100 < ERROR_INJECT )); then
        log ERROR "task failed: unexpected error in $FUNCTION pipeline (error_injection=$ERROR_INJECT%)"
    else
        log INFO "$(activity)"
    fi

    # --- Heartbeat (writes metrics less frequently)
    if (( tick >= next_heartbeat )); then
        log INFO "heartbeat ok uptime=${tick}ticks upstream=${UPSTREAM_URL}:${UPSTREAM_PORT}"
        write_metrics
        next_heartbeat=$(( tick + HEARTBEAT_INTERVAL / LOG_INTERVAL ))
    fi

    sleep "$LOG_INTERVAL"
done
