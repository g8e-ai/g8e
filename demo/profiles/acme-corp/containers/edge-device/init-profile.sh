#!/bin/bash
# Initialise /etc/device/config.yaml and any failure-mode artifacts.
#
# Runs ONCE per container start. The device-service reads config.yaml on each
# start, so the "fix" for most failure modes is: edit config → restart service.
# Some modes also leave on-disk artifacts (junk files, expired certs, chmod
# changes) that must be cleaned up for full recovery.

set -u

CONFIG="/etc/device/config.yaml"
CERT_DIR="/etc/device/certs"
CACHE_DIR="/var/lib/device/cache"
PROFILE="${DEVICE_PROFILE:-healthy}"

mkdir -p "$CACHE_DIR"

# Baseline healthy config. Each failure mode flips one knob.
write_config() {
    cat > "$CONFIG" <<EOF
# ACME Edge Device Configuration
# Generated at first boot. Safe to edit; restart service to apply:
#   pkill -f device-service.sh
device:
  name: ${DEVICE_NAME:-unknown}
  function: ${DEVICE_FUNCTION:-unknown}
  site: ${DEVICE_SITE:-unknown}
  location: ${DEVICE_LOCATION:-unknown}

upstream:
  url: http://acme-ingest.internal
  port: 8443
  tls_cert: /etc/device/certs/client.crt

runtime:
  log_interval_sec: 5
  heartbeat_interval_sec: 30
  error_injection_percent: 0
  crash_on_start: false
  stuck_loop: false
  memory_leak_mb_per_min: 0
EOF
}

write_healthy_cert() {
    # A plausible-looking (but obviously fake) cert file. Simulates a rotation
    # artifact - real TLS is never used against a real upstream in this demo.
    local not_before not_after
    not_before=$(date -u -d "30 days ago" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
    not_after=$(date -u -d "335 days" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
    cat > "$CERT_DIR/client.crt" <<EOF
# ACME device client certificate (simulated)
subject: CN=${DEVICE_NAME:-device}.acme.internal
issuer:  CN=ACME Internal CA
not_before: $not_before
not_after:  $not_after
serial: $(date +%s)$RANDOM
EOF
}

write_expired_cert() {
    cat > "$CERT_DIR/client.crt" <<EOF
# ACME device client certificate (EXPIRED)
subject: CN=${DEVICE_NAME:-device}.acme.internal
issuer:  CN=ACME Internal CA
not_before: 2022-01-01T00:00:00Z
not_after:  2023-01-01T00:00:00Z
serial: legacy-expired
EOF
}

# ---------------------------------------------------------------------------
# Start with healthy baseline
# ---------------------------------------------------------------------------
write_config
write_healthy_cert

# ---------------------------------------------------------------------------
# Apply profile-specific overrides
# ---------------------------------------------------------------------------
case "$PROFILE" in
    healthy)
        : ;;

    crashed)
        # Service exits immediately on start; supervisor logs crash loop.
        # Fix: edit config, set crash_on_start: false.
        sed -i 's/crash_on_start: false/crash_on_start: true/' "$CONFIG"
        ;;

    wrong_config)
        # Config points at a hostname that doesn't resolve.
        # Fix: edit upstream.url.
        sed -i 's|url: http://acme-ingest.internal|url: http://decommissioned-host-do-not-use|' "$CONFIG"
        ;;

    bad_upstream)
        # Config points at a port nothing listens on.
        # Fix: edit upstream.port back to 8443.
        sed -i 's/port: 8443/port: 1/' "$CONFIG"
        ;;

    disk_full)
        # Fill device cache with a 50MB junk file; service alarms on disk check.
        # Fix: delete /var/lib/device/cache/staging.bin.
        dd if=/dev/zero of="$CACHE_DIR/staging.bin" bs=1M count=50 2>/dev/null
        ;;

    cert_expired)
        # Replace cert with an obviously expired one.
        # Fix: regenerate via /opt/device/rotate-cert.sh OR edit the file.
        write_expired_cert
        ;;

    stuck_loop)
        # Service enters no-op sleep after first log line.
        # Fix: edit config (stuck_loop: false), pkill device-service.
        sed -i 's/stuck_loop: false/stuck_loop: true/' "$CONFIG"
        ;;

    permission_denied)
        # Config file is unreadable; service logs the error and exits.
        # Fix: chmod 644 /etc/device/config.yaml.
        chmod 000 "$CONFIG"
        ;;

    high_error_rate)
        # ~50% of log lines are ERRORs.
        # Fix: set error_injection_percent back to 0.
        sed -i 's/error_injection_percent: 0/error_injection_percent: 50/' "$CONFIG"
        ;;

    memory_leak)
        # Simulated memory growth reported in metrics.
        # Fix: set memory_leak_mb_per_min back to 0 and restart.
        sed -i 's/memory_leak_mb_per_min: 0/memory_leak_mb_per_min: 4/' "$CONFIG"
        ;;

    *)
        echo "[init-profile] unknown DEVICE_PROFILE='$PROFILE'; defaulting to healthy"
        ;;
esac

# Tiny helper the AI (or user) can run to rotate an expired cert. Intentionally
# placed at a predictable path so it's discoverable.
cat > /opt/device/rotate-cert.sh <<'EOF'
#!/bin/bash
# Rotate the ACME device client cert to a fresh (simulated) one-year cert.
set -e
not_before=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
not_after=$(date -u -d "335 days" +"%Y-%m-%dT%H:%M:%SZ")
cat > /etc/device/certs/client.crt <<INNER
# ACME device client certificate (rotated)
subject: CN=$(hostname).acme.internal
issuer:  CN=ACME Internal CA
not_before: $not_before
not_after:  $not_after
serial: $(date +%s)$RANDOM
INNER
echo "rotated: $not_before -> $not_after"
EOF
chmod +x /opt/device/rotate-cert.sh

echo "[init-profile] applied profile=$PROFILE"
