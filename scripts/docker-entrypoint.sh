#!/bin/sh
set -e

# =============================================================================
# g8e Container Entrypoint
#
# Strict Order of Precedence for Executable Binary:
# 1. Explicit G8E_BIN environment variable override (if set and executable)
# 2. Host-mounted binary from /opt/g8e/bin/g8e (from local 'make build')
# 3. Host-mounted architecture binary from /opt/g8e/bin/g8e-linux-${ARCH}
# 4. Container image baked-in binary (/g8e)
# =============================================================================

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)       GOARCH="$ARCH" ;;
esac

TARGET_BIN=""

# 1. Explicit G8E_BIN override
if [ -n "$G8E_BIN" ] && [ -f "$G8E_BIN" ] && [ -x "$G8E_BIN" ]; then
  if "$G8E_BIN" version --help >/dev/null 2>&1; then
    TARGET_BIN="$G8E_BIN"
    echo "[g8e-entrypoint] Precedence 1: Using explicit G8E_BIN: $TARGET_BIN"
  else
    echo "[g8e-entrypoint] Warning: G8E_BIN ($G8E_BIN) is not a runnable binary for $(uname -s)/$ARCH; falling back"
  fi
fi

# 2. Host-mounted binary from /opt/g8e/bin/g8e
if [ -z "$TARGET_BIN" ] && [ -f "/opt/g8e/bin/g8e" ] && [ -x "/opt/g8e/bin/g8e" ]; then
  if /opt/g8e/bin/g8e version --help >/dev/null 2>&1; then
    TARGET_BIN="/opt/g8e/bin/g8e"
    echo "[g8e-entrypoint] Precedence 2: Using host-mounted binary /opt/g8e/bin/g8e (local 'make build' gospel)"
  else
    echo "[g8e-entrypoint] Warning: /opt/g8e/bin/g8e is not runnable on this container architecture ($ARCH); checking alternatives"
  fi
fi

# 3. Host-mounted arch-specific binary from /opt/g8e/bin/g8e-linux-${GOARCH}
if [ -z "$TARGET_BIN" ] && [ -f "/opt/g8e/bin/g8e-linux-${GOARCH}" ] && [ -x "/opt/g8e/bin/g8e-linux-${GOARCH}" ]; then
  if "/opt/g8e/bin/g8e-linux-${GOARCH}" version --help >/dev/null 2>&1; then
    TARGET_BIN="/opt/g8e/bin/g8e-linux-${GOARCH}"
    echo "[g8e-entrypoint] Precedence 3: Using host-mounted arch binary /opt/g8e/bin/g8e-linux-${GOARCH}"
  fi
fi

# 4. Baked-in container image binary
if [ -z "$TARGET_BIN" ]; then
  TARGET_BIN="/g8e"
  echo "[g8e-entrypoint] Precedence 4: Using container baked-in binary $TARGET_BIN (built in image via Makefile)"
fi

exec "$TARGET_BIN" "$@"
