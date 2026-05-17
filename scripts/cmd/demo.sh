#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-}"
DEMO_DIR="$SCRIPT_DIR/demo"
PROFILES_DIR="$DEMO_DIR/profiles"

if [[ ! -d "$DEMO_DIR" ]]; then
    echo "[g8e] demo directory not found: $DEMO_DIR" >&2
    exit 1
fi

if [[ "$SUB" == "profile" ]]; then
    mkdir -p "$PROFILES_DIR"
    _PROF_CMD="${2:-}"
    _PROF_NAME="${3:-}"
    _ACTIVE_FILE="$PROFILES_DIR/.active"
    case "$_PROF_CMD" in
        list)   exec make -C "$DEMO_DIR" list ;;
        switch) [[ -z "$_PROF_NAME" ]] && { echo "[g8e] usage: ./g8e demo profile switch <name>"; exit 1; }; exec make -C "$DEMO_DIR" switch P="$_PROF_NAME" ;;
        *)      echo "[g8e] unknown demo profile subcommand: '$_PROF_CMD'" >&2; echo "  Valid: list, switch" >&2; exit 1 ;;
    esac
fi

demo_args=("${@:2}")
device_token=""
nodes=""
i=0
while [[ $i -lt ${#demo_args[@]} ]]; do
    arg="${demo_args[$i]}"
    if [[ "$arg" == "-d" ]]; then
        if [[ $((i + 1)) -lt ${#demo_args[@]} ]]; then
            device_token="${demo_args[$((i + 1))]}"
            unset "demo_args[$i]" "demo_args[$((i + 1))]}"
            demo_args=("${demo_args[@]}")
            i=0; continue
        else
            echo "[g8e] -d flag requires a token value" >&2; exit 1
        fi
    fi
    i=$((i + 1))
done
i=0
while [[ $i -lt ${#demo_args[@]} ]]; do
    arg="${demo_args[$i]}"
    if [[ "$arg" == "-n" ]]; then
        if [[ $((i + 1)) -lt ${#demo_args[@]} ]]; then
            nodes="${demo_args[$((i + 1))]}"
            unset "demo_args[$i]" "demo_args[$((i + 1))]}"
            demo_args=("${demo_args[@]}")
            i=0; continue
        else
            echo "[g8e] -n flag requires a node count value" >&2; exit 1
        fi
    fi
    i=$((i + 1))
done
if [[ -z "$device_token" && -n "$DEVICE_TOKEN" ]]; then
    device_token="$DEVICE_TOKEN"
fi
if [[ -n "$device_token" ]]; then
    demo_args=("DEVICE_TOKEN=$device_token" "${demo_args[@]}")
fi
if [[ -n "$nodes" ]]; then
    demo_args=("NODES=$nodes" "N=$nodes" "${demo_args[@]}")
fi

case "$SUB" in
    -h|--help|"")
        help_file="$SCRIPT_DIR/docs/general/cli_help.md"
        if [[ -f "$help_file" ]]; then
            awk '/^### demo/,/^### evals/' "$help_file" | head -n -1
        else
            echo "[g8e] Help file not found: $help_file" >&2; exit 1
        fi
        [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
    up|down|status|clean|health|nginx-check|operators|logs|discover-hosts|stream|vanish|dashboard|devices|broken)
        _banner "demo $SUB"; exec make -C "$DEMO_DIR" "$SUB" "${demo_args[@]}" ;;
    deploy)
        _banner "demo deploy"
        if ! make -C "$DEMO_DIR" status | grep -q "devices running: [1-9]"; then
            echo "[g8e] Demo fleet not running. Starting it now..."
            make -C "$DEMO_DIR" up "${demo_args[@]}"
        else
            exec make -C "$DEMO_DIR" deploy "${demo_args[@]}"
        fi
        exit 0 ;;
    shell)
        _banner "demo shell"; exec make -C "$DEMO_DIR" shell "${demo_args[@]}" ;;
    *)
        echo "[g8e] unknown demo subcommand: '$SUB'" >&2
        echo "  Valid: up, down, status, clean, health, nginx-check, operators, logs, shell, deploy, discover-hosts, stream, vanish, dashboard, devices, broken, profile" >&2
        exit 1 ;;
esac
