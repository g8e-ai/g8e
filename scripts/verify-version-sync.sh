#!/usr/bin/env bash
# Verify (default) or sync (--sync) version-bearing files from VERSION.
set -euo pipefail

SYNC=false
if [[ "${1:-}" == "--sync" ]]; then
  SYNC=true
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION=$(cat VERSION | tr -d '\n\r' | sed 's/^v//')
errors=0

check_or_sync_field() {
  local label="$1"
  local file="$2"
  local current="$3"
  local sed_expr="$4"

  if [ "$current" != "$VERSION" ]; then
    if [ "$SYNC" = true ]; then
      echo "Syncing $label: $current -> $VERSION"
      sed -i.bak -E "$sed_expr" "$file"
      rm -f "${file}.bak"
    else
      echo "Error: $label ($current) != VERSION ($VERSION)"
      errors=1
    fi
  elif [ "$SYNC" = true ]; then
    echo "  $label already in sync."
  fi
}

PY_FILE=protocol/python/pyproject.toml
PY_INIT=protocol/python/g8e/__init__.py
PY_LOCK=protocol/python/uv.lock

PY_VERSION=$(grep -E '^version = ' "$PY_FILE" | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
PY_INIT_VERSION=$(grep -E '^__version__ = ' "$PY_INIT" | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
PY_LOCK_VERSION=$(sed -n -E '/^name = "g8e"$/{n;s/^version = "([^"]+)"/\1/p;}' "$PY_LOCK")

check_or_sync_field "$PY_FILE" "$PY_FILE" "$PY_VERSION" 's/^version = "[^"]+"/version = "'"$VERSION"'"/'
check_or_sync_field "$PY_INIT" "$PY_INIT" "$PY_INIT_VERSION" 's/^__version__ = "[^"]+"/__version__ = "'"$VERSION"'"/'
check_or_sync_field "$PY_LOCK (g8e entry)" "$PY_LOCK" "$PY_LOCK_VERSION" '/^name = "g8e"$/{n;s/^version = "[^"]+"/version = "'"$VERSION"'"/;}'

for doc in a2a.md constants.md mcp.md spec.md; do
  path="protocol/docs/$doc"
  DOC_VERSION=$(grep -E '^Version: v' "$path" | head -1 | sed 's/^Version: v//')
  check_or_sync_field "$path" "$path" "$DOC_VERSION" 's/^Version: v[^[:space:]]+/Version: v'"$VERSION"'/'
done

if [ "$errors" -ne 0 ]; then
  echo "Version sync failed. Run 'bash scripts/verify-version-sync.sh --sync' to fix."
  exit 1
fi

echo "Version sync OK: $VERSION"
