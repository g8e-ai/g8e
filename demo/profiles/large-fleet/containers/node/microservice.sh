#!/bin/bash
# Lightweight edge device simulator - generates logs for fleet operations demos

LOG_DIR="/var/log/edge-service"
LOG_FILE="$LOG_DIR/service.log"
METRICS_FILE="$LOG_DIR/metrics.json"

mkdir -p "$LOG_DIR"

# Initialize metrics
cat > "$METRICS_FILE" << EOF
{
  "hostname": "$(hostname)",
  "start_time": "$(date -Iseconds)",
  "uptime_seconds": 0,
  "requests_processed": 0,
  "errors_count": 0,
  "memory_mb": 0,
  "disk_usage_percent": 0
}
EOF

log() {
  local level="$1"
  local message="$2"
  local timestamp=$(date -Iseconds)
  echo "[$timestamp] [$level] $message" >> "$LOG_FILE"
}

update_metrics() {
  local uptime=$(cat /proc/uptime | awk '{print int($1)}')
  local mem=$(free -m | awk '/Mem:/ {print $3}')
  local disk=$(df -h / | awk 'NR==2 {print $5}' | sed 's/%//')
  
  cat > "$METRICS_FILE" << EOF
{
  "hostname": "$(hostname)",
  "uptime_seconds": $uptime,
  "requests_processed": $((RANDOM % 1000)),
  "errors_count": $((RANDOM % 10)),
  "memory_mb": $mem,
  "disk_usage_percent": $disk
}
EOF
}

# Main loop
iteration=0
while true; do
  iteration=$((iteration + 1))
  
  # Simulate periodic health checks
  if [ $((iteration % 5)) -eq 0 ]; then
    log "INFO" "Health check passed - all systems operational"
    update_metrics
  fi
  
  # Simulate request processing
  if [ $((iteration % 3)) -eq 0 ]; then
    local latency=$((RANDOM % 100))
    log "INFO" "Processed request in ${latency}ms"
  fi
  
  # Simulate occasional warnings
  if [ $((iteration % 20)) -eq 0 ]; then
    log "WARN" "High memory usage detected: $((RANDOM % 80 + 20))%"
  fi
  
  # Simulate rare errors
  if [ $((iteration % 50)) -eq 0 ]; then
    log "ERROR" "Connection timeout to upstream service"
  fi
  
  # Simulate data sync operations
  if [ $((iteration % 15)) -eq 0 ]; then
    local synced=$((RANDOM % 1000))
    log "INFO" "Synced $synced records to central storage"
  fi
  
  # Log rotation simulation
  if [ $((iteration % 100)) -eq 0 ]; then
    log "INFO" "Log rotation initiated - archiving old logs"
  fi
  
  sleep 2
done
