#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-}"

case "$SUB" in
    -h|--help|"")
        echo "Usage: ./g8e user <subcommand> [options]"
        echo ""
        echo "Subcommands:"
        echo "  create   Create a new user on the Operator"
        echo ""
        echo "Options for 'create':"
        echo "  --email <email>    (required) User email address"
        echo "  --name  <name>     (optional) Display name"
        echo "  --role  <role>     (optional) Role to assign; may be specified multiple times"
        echo "                     Defaults: first user gets SUPERADMIN, subsequent get USER"
        echo ""
        echo "Examples:"
        echo "  ./g8e user create --email superadmin@g8e.local"
        echo "  ./g8e user create --email alice@example.com --name Alice --role SUPERADMIN"
        [[ -z "$SUB" ]] && exit 1 || exit 0
        ;;
    create)
        _banner "user create"
        _ensure_operator

        _EMAIL=""
        _NAME=""
        _ROLES=()
        _args=("${@:2}")
        set -- "${_args[@]}"
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --email) _EMAIL="$2"; shift 2 ;;
                --name)  _NAME="$2";  shift 2 ;;
                --role)  _ROLES+=("$2"); shift 2 ;;
                -h|--help)
                    exec bash "$0" --help ;;
                *)
                    echo "[g8e] unknown option: $1" >&2
                    exit 1 ;;
            esac
        done

        if [[ -z "$_EMAIL" ]]; then
            echo "[g8e] --email is required" >&2
            exit 1
        fi

        _BODY="{\"email\":\"$_EMAIL\""
        [[ -n "$_NAME" ]] && _BODY+=",\"name\":\"$_NAME\""
        if [[ ${#_ROLES[@]} -gt 0 ]]; then
            _ROLES_JSON=$(printf '"%s",' "${_ROLES[@]}")
            _ROLES_JSON="[${_ROLES_JSON%,}]"
            _BODY+=",\"roles\":$_ROLES_JSON"
        fi
        _BODY+="}"

        response=$(_operator_curl POST "/api/users" "$_BODY")
        echo "$response"

        if echo "$response" | grep -q '"success":true'; then
            user_id=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
            echo ""
            echo "[g8e] User created: $_EMAIL (id: $user_id)"
        else
            echo "[g8e] Failed to create user" >&2
            exit 1
        fi
        ;;
    *)
        echo "[g8e] unknown user subcommand: '$SUB'" >&2
        echo "  Valid: create" >&2
        exit 1 ;;
esac
