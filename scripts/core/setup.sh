#!/bin/bash
# g8e Platform Setup
#
# Delegates LLM provider configuration to setup-llm.sh, which writes to the
# platform DB. Platform hostname and SSL are configured through the browser
# setup wizard on first run (https://localhost).
#
# Pass-through: all arguments are forwarded to setup-llm.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

exec "$SCRIPT_DIR/../tools/setup-llm.sh" "$@"
