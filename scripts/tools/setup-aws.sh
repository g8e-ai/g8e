#!/bin/bash
# g8e AWS Setup
#
# Mounts your AWS credentials into g8e-pod so the operator can interact
# with AWS services. Run once; re-run to change the credentials path.
#
# Non-interactive usage:
#   --aws-dir  path to AWS credentials directory (default: ~/.aws)

set -euo pipefail

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  setup-aws.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  setup-aws.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

ARG_AWS_DIR=""
NON_INTERACTIVE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --aws-dir) ARG_AWS_DIR="$2"; shift 2 ;;
        --help|-h)
            echo "Usage: setup-aws.sh [options]"
            echo ""
            echo "Options:"
            echo "  --aws-dir  path to AWS credentials directory to mount (default: ~/.aws)"
            exit 0 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

if [[ -n "$ARG_AWS_DIR" ]]; then
    NON_INTERACTIVE=true
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
BLUE=$'\033[0;34m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
NC=$'\033[0m'

_header() { echo -e "\n${BLUE}${BOLD}$1${NC}"; }
_ok()     { echo -e "  ${GREEN}[OK]${NC} $1"; }
_warn()   { echo -e "  ${YELLOW}[WARN]${NC} $1"; }
_err()    { echo -e "  ${RED}[ERROR]${NC} $1" >&2; }
_info()   { echo -e "  ${CYAN}$1${NC}"; }

_header "AWS Credentials Configuration"
echo
_info "g8e-pod mounts your AWS credentials so the operator can access AWS services."
_info "The mounted directory is configured in docker-compose.yml as \${HOME}/.aws."
echo
_info "  Default: ~/.aws  — uses your existing AWS CLI credentials and config"
_info "  To use a custom directory, update the volume mount in docker-compose.yml"
echo
_warn "Only mount credentials that have the minimum permissions required."
echo

_default_aws_dir="${HOME}/.aws"

if [[ "$NON_INTERACTIVE" == true ]]; then
    AWS_DIR="$ARG_AWS_DIR"
else
    printf "  AWS credentials directory [%s]: " "$_default_aws_dir"
    IFS= read -r _input
    AWS_DIR="${_input:-$_default_aws_dir}"
fi

AWS_DIR="${AWS_DIR/#\~/$HOME}"

if [[ ! -d "$AWS_DIR" ]]; then
    _warn "Directory not found: $AWS_DIR"
    _info "It will be created when you run 'aws configure' or add credentials manually."
    mkdir -p "$AWS_DIR"
    _ok "Created $AWS_DIR"
fi

_ok "AWS directory exists: $AWS_DIR"

echo
_info "g8e-pod mounts \${HOME}/.aws from docker-compose.yml."
_info "If you need a custom path, update the aws volume mount in docker-compose.yml"
_info "and run: ./g8e platform restart"
echo
