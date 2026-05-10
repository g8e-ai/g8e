#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
STUB_DIR="$(mktemp -d)"
trap 'rm -rf "$STUB_DIR"' EXIT

cat > "$STUB_DIR/docker" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "compose" ]]; then
    shift
    if [[ "${1:-}" == "version" ]]; then
        echo "Docker Compose version v2.0.0"
        exit 0
    fi
    args=("$@")
    for ((i=0; i<${#args[@]}; i++)); do
        if [[ "${args[$i]}" == "python3" ]]; then
            printf '%s\n' "${args[@]:$i}" > "${G8E_DOCKER_CAPTURE_FILE:?}"
            exit 0
        fi
    done
    printf 'python3 command not found in docker compose invocation: %s\n' "$*" >&2
    exit 1
fi
printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
STUB
chmod +x "$STUB_DIR/docker"

assert_eval_invocation() {
    local gold_set="$1"
    local expected_gold_set="$2"
    local capture_file
    capture_file="$(mktemp)"
    PATH="$STUB_DIR:$PATH" G8E_DOCKER_CAPTURE_FILE="$capture_file" \
        "$PROJECT_ROOT/g8e" evals run --gold-set "$gold_set"

    mapfile -t argv < "$capture_file"
    rm -f "$capture_file"

    [[ "${argv[0]}" == "python3" ]]
    [[ "${argv[1]}" == "-m" ]]
    [[ "${argv[2]}" == "app.evals.runner.cli" ]]
    [[ "${argv[3]}" == "run" ]]
    [[ "${argv[4]}" == "--gold-set" ]]
    [[ "${argv[5]}" == "$expected_gold_set" ]]
}

assert_eval_invocation "/app/components/g8ee/evals/gold_sets/accuracy.json" "/app/components/g8ee/evals/gold_sets/accuracy.json"
assert_eval_invocation "components/g8ee/evals/gold_sets/accuracy.json" "/app/components/g8ee/evals/gold_sets/accuracy.json"
assert_eval_invocation "evals/gold_sets/accuracy.json" "/app/components/g8ee/evals/gold_sets/accuracy.json"

echo "g8e evals wrapper tests passed"
