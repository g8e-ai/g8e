#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Run only in a disposable checkout/container; this starts and enrolls a platform.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
if [[ -e .g8e || -e .venv || -e .env ]]; then
    echo "ERROR: onboarding smoke test requires a fresh disposable checkout" >&2
    exit 1
fi

cleanup() {
    local status=$?
    if [[ $status -ne 0 ]]; then
        find .local.dev/full .g8e "$HOME/.ollama/g8e" -name '*.log' -type f -print -exec tail -n 80 {} \; 2>/dev/null || true
    fi
    if [[ -x ./g8e ]]; then
        make down || true
    fi
    exit "$status"
}
trap cleanup EXIT

# No contributor Go plugins, linter, or Python tooling preinstalled.
bash scripts/linux-setup.sh --build-only -y
export PATH="/usr/local/go/bin:$HOME/.local/bin:$PATH"
make build
make up
./g8e auth enroll user -e localhost --headless
make ensemble-env

# No live model service or API keys are needed to start/enroll the platform.
export G8E_HOSTNAME=localhost
export G8E_OLLAMA_ENDPOINT=http://localhost:11434
export NO_PROXY='localhost,127.0.0.1,[::1]'
export no_proxy="$NO_PROXY"
.venv/bin/python -c 'import app.main; import ollama'
make full

# Apps and Operators enroll asynchronously; approve until all five are ready.
for attempt in {1..60}; do
    ./g8e auth enroll approve --all --yes
    if .venv/bin/python - <<'PY'
import importlib.util
from pathlib import Path
from urllib.request import urlopen

spec = importlib.util.spec_from_file_location("full", "scripts/full.py")
full = importlib.util.module_from_spec(spec)
spec.loader.exec_module(full)
for role in full.ROLES:
    directory = Path.home() / ".ollama/g8e" / role
    state, _ = full.workload_state(role, directory)
    if state != "connected":
        raise SystemExit(1)
try:
    with urlopen("http://127.0.0.1:8000/health", timeout=2) as response:
        if response.status != 200:
            raise SystemExit(1)
except OSError:
    raise SystemExit(1)
PY
    then
        ./g8e gw status
        echo 'Onboarding smoke test passed: Gateway, four Operators, and Ensemble ready.'
        exit 0
    fi
    sleep 2
done
echo 'ERROR: workloads did not become ready after enrollment approval' >&2
exit 1
