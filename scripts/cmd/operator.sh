#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-}"

case "$SUB" in
    -h|--help|"")
        help_file="$SCRIPT_DIR/docs/general/cli_help.md"
        if [[ -f "$help_file" ]]; then
            awk '/^### operator/,/^### test/' "$help_file" | head -n -1
        else
            echo "[g8e] Help file not found: $help_file" >&2
            exit 1
        fi
        [[ -z "$SUB" ]] && exit 1 || exit 0
        ;;
    reauth)
        _banner "operator reauth"
        _ensure_operator
        _operator_curl POST "/api/operators/reauth" ;;

    init)
        _banner "operator init"
        (cd "$SCRIPT_DIR/services/g8eo" && make build-local)
        echo ""
        echo "Operator binary built on host: $SCRIPT_DIR/services/g8eo/build/linux-amd64/g8e.operator"
        exit 0 ;;
    build)
        _banner "operator build"
        exec bash "$SCRIPT_DIR/scripts/core/build.sh" operator-build ;;
    build-all)
        _banner "operator build-all"
        exec bash "$SCRIPT_DIR/scripts/core/build.sh" operator-build-all ;;
    deploy)
        _banner "operator deploy ${@:2}"
        _ensure_operator
        _DEPLOY_TARGET=""
        _DEPLOY_ARCH="amd64"
        _DEPLOY_DEST="./g8e.operator"
        _DEPLOY_ENDPOINT=""
        _DEPLOY_DEVICE_TOKEN=""
        _DEPLOY_API_KEY=""
        _DEPLOY_NO_GIT=false
        _DEPLOY_WSS_PORT=""
        _DEPLOY_HTTP_PORT=""
        _args=("${@:2}")
        set -- "${_args[@]}"
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --arch)         _DEPLOY_ARCH="$2";         shift 2 ;;
                --dest)         _DEPLOY_DEST="$2";         shift 2 ;;
                --endpoint)     _DEPLOY_ENDPOINT="$2";     shift 2 ;;
                --device-token) _DEPLOY_DEVICE_TOKEN="$2"; shift 2 ;;
                --key)          _DEPLOY_API_KEY="$2";      shift 2 ;;
                --wss-port)     _DEPLOY_WSS_PORT="$2";     shift 2 ;;
                --http-port)    _DEPLOY_HTTP_PORT="$2";    shift 2 ;;
                --no-git)       _DEPLOY_NO_GIT=true;        shift ;;
                -h|--help)
                    echo "Usage: ./g8e operator deploy <user@host> [options]"
                    echo "  --arch amd64|arm64|386       (default: amd64)"
                    echo "  --dest /path                 (default: ./g8e.operator)"
                    echo "  --endpoint <host>            Platform endpoint"
                    echo "  --device-token <tok>         Device link token"
                    echo "  --key <apikey>               API key auth (fallback)"
                    echo "  --wss-port <port>            WSS port for pub/sub (default: 443)"
                    echo "  --http-port <port>           HTTPS port for auth (default: 443)"
                    echo "  --no-git                     Disable ledger"
                    exit 0 ;;
                *) _DEPLOY_TARGET="$1"; shift ;;
            esac
        done
        if [[ -z "$_DEPLOY_TARGET" ]]; then
            echo "[g8e] operator deploy requires a target host" >&2
            echo "  Usage: ./g8e operator deploy <user@host> [options]" >&2
            exit 1
        fi
        _REMOTE_EXEC="${_DEPLOY_DEST}"
        [[ "$(basename "${_DEPLOY_DEST}")" != "g8e.operator" ]] && _REMOTE_EXEC="${_DEPLOY_DEST%/}/g8e.operator"
        trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
        if [[ ! -f "$trust_bundle" ]]; then
            echo "[g8e] Operator trust bundle not found at $trust_bundle — recreate runtime PKI with ./g8e platform clean && ./g8e platform start" >&2
            exit 1
        fi
        echo "Fetching linux/${_DEPLOY_ARCH} operator from host Operator blob store and copying to ${_DEPLOY_TARGET}:${_DEPLOY_DEST}..."
        curl -sf \
            -H "Authorization: Bearer ${G8E_OPERATOR_SESSION_ID}" \
            --cacert "$trust_bundle" \
            "${OPERATOR_HTTP_URL}/blob/operator-binary/linux-${_DEPLOY_ARCH}" \
            | ssh "${_DEPLOY_TARGET}" "cat > ${_DEPLOY_DEST} && chmod +x ${_DEPLOY_DEST}"
        echo "  Done."
        if [[ -n "$_DEPLOY_ENDPOINT" ]]; then
            _REMOTE_CMD="nohup ${_REMOTE_EXEC} -e ${_DEPLOY_ENDPOINT}"
            [[ -n "$_DEPLOY_DEVICE_TOKEN" ]] && _REMOTE_CMD+=" -D ${_DEPLOY_DEVICE_TOKEN}"
            [[ -n "$_DEPLOY_API_KEY" ]]      && _REMOTE_CMD+=" -k ${_DEPLOY_API_KEY}"
            [[ -n "$_DEPLOY_WSS_PORT" ]]     && _REMOTE_CMD+=" --wss-port ${_DEPLOY_WSS_PORT}"
            [[ -n "$_DEPLOY_HTTP_PORT" ]]    && _REMOTE_CMD+=" --http-port ${_DEPLOY_HTTP_PORT}"
            [[ "$_DEPLOY_NO_GIT" == "true" ]] && _REMOTE_CMD+=" --no-git"
            _REMOTE_CMD+=" > ./g8e.operator.log 2>&1 &"
            echo "Starting operator on ${_DEPLOY_TARGET}..."
            ssh "${_DEPLOY_TARGET}" "${_REMOTE_CMD}"
            echo "  Operator started. Logs: ./g8e.operator.log"
        fi
        exit 0 ;;
    stream)
        _bin="$(_operator_bin)"
        if [[ "${2:-}" == "-h" || "${2:-}" == "--help" || "${2:-}" == "" ]]; then
            exec "$_bin" stream --help
        fi
        _banner "operator stream ${@:2}"
        _stream_args=(stream)
        _args=("${@:2}")
        set -- "${_args[@]}"
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --arch|--hosts|--concurrency|--timeout|--endpoint|--device-token|--key|--ssh-config|--ssh-identity-file|--ssh-user|--binary-dir)
                    _stream_args+=("$1" "$2"); shift 2 ;;
                --no-git) _stream_args+=("--no-git"); shift ;;
                *) _stream_args+=("$1"); shift ;;
            esac
        done
        exec "$_bin" "${_stream_args[@]}" ;;
    ssh-config)
        _banner "operator ssh-config"
        exec bash "$SCRIPT_DIR/scripts/tools/setup-ssh.sh" ssh-config "${@:2}" ;;
    *)
        echo "[g8e] unknown operator subcommand: '$SUB'" >&2
        echo "  Valid: deploy, stream, ssh-config, reauth, build-all" >&2
        exit 1 ;;
esac
