#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Serve this frontend on http://localhost:3003 for ./g8e gw connect
set -euo pipefail
cd "$(dirname "$0")"
PORT="${PORT:-3003}"
echo "Serving g8e external frontend at http://localhost:${PORT}"
echo "Then run from the g8e repo:  ./g8e gw connect http://localhost:${PORT}"
if command -v python3 >/dev/null 2>&1; then
  exec python3 -m http.server "$PORT" --bind 127.0.0.1
elif command -v python >/dev/null 2>&1; then
  exec python -m http.server "$PORT" --bind 127.0.0.1
else
  echo "Need python3 to serve static files" >&2
  exit 1
fi
