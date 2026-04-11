#!/usr/bin/env bash
set -euo pipefail

# Dependency license scanner for g8e
#
# Scans all three component stacks for license compatibility:
#   - VSA (Go)    — via go-licenses
#   - VSE (Python) — via pip-licenses
#   - VSOD (Node) — via license-checker
#
# Flags licenses that are incompatible with commercial distribution under BUSL-1.1.
# Outputs a summary report and exits non-zero if any flagged licenses are found.
#
# Usage:
#   ./scripts/security/scan-licenses.sh             # scan all components
#   ./scripts/security/scan-licenses.sh --vsa        # Go only
#   ./scripts/security/scan-licenses.sh --vse        # Python only
#   ./scripts/security/scan-licenses.sh --vsod       # Node only
#   ./scripts/security/scan-licenses.sh --report     # write reports/ in addition to stdout
#   ./scripts/security/scan-licenses.sh --csv        # output CSV format

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
REPORTS_DIR="$PROJECT_ROOT/security-reports/licenses"

RED=$'\033[0;31m'
YELLOW=$'\033[1;33m'
GREEN=$'\033[0;32m'
BLUE=$'\033[0;34m'
BOLD=$'\033[1m'
NC=$'\033[0m'

log_header() { printf "\n%s%s==== %s ====%s\n\n" "$BLUE" "$BOLD" "$1" "$NC"; }
log_ok()     { printf "%s[OK]%s    %s\n" "$GREEN" "$NC" "$1"; }
log_warn()   { printf "%s[WARN]%s  %s\n" "$YELLOW" "$NC" "$1"; }
log_flag()   { printf "%s[FLAG]%s  %s\n" "$RED" "$NC" "$1"; }
log_info()   { printf "        %s\n" "$1"; }

# Licenses that are incompatible with BUSL-1.1 commercial distribution.
# GPL/AGPL require source disclosure of the combined work.
# SSPL is similarly viral.
FLAGGED_LICENSES=(
    "GPL-2.0"
    "GPL-2.0-only"
    "GPL-2.0-or-later"
    "GPL-3.0"
    "GPL-3.0-only"
    "GPL-3.0-or-later"
    "AGPL-3.0"
    "AGPL-3.0-only"
    "AGPL-3.0-or-later"
    "SSPL-1.0"
    "LGPL-2.0"
    "LGPL-2.0-only"
    "LGPL-2.0-or-later"
    "LGPL-2.1"
    "LGPL-2.1-only"
    "LGPL-2.1-or-later"
    "LGPL-3.0"
    "LGPL-3.0-only"
    "LGPL-3.0-or-later"
    "OSL-3.0"
    "EUPL-1.1"
    "EUPL-1.2"
    "CC-BY-SA-4.0"
    "CC-BY-NC"
)

SCAN_VSA=false
SCAN_VSE=false
SCAN_VSOD=false
SCAN_ALL=true
WRITE_REPORT=false
CSV_OUTPUT=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --vsa)    SCAN_VSA=true;  SCAN_ALL=false; shift ;;
        --vse)    SCAN_VSE=true;  SCAN_ALL=false; shift ;;
        --vsod)   SCAN_VSOD=true; SCAN_ALL=false; shift ;;
        --report) WRITE_REPORT=true; shift ;;
        --csv)    CSV_OUTPUT=true; shift ;;
        -h|--help)
            sed -n '/^# Usage:/,/^$/p' "$0" | grep -v '^#' || true
            grep '^#' "$0" | head -20 | sed 's/^# \?//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

if [[ "$SCAN_ALL" == "true" ]]; then
    SCAN_VSA=true
    SCAN_VSE=true
    SCAN_VSOD=true
fi

TOTAL_FLAGGED=0
TOTAL_WARNED=0

_is_flagged() {
    local license="$1"
    for flagged in "${FLAGGED_LICENSES[@]}"; do
        if [[ "$license" == "$flagged" || "$license" == *"$flagged"* ]]; then
            return 0
        fi
    done
    return 1
}

_init_report_dir() {
    if [[ "$WRITE_REPORT" == "true" ]]; then
        mkdir -p "$REPORTS_DIR"
    fi
}

# =============================================================================
# Go (VSA)
# =============================================================================
scan_vsa() {
    log_header "VSA — Go dependencies (go-licenses)"

    if ! command -v go-licenses &>/dev/null; then
        log_warn "go-licenses not installed. Install with:"
        log_info  "  go install github.com/google/go-licenses@latest"
        log_warn "Falling back to manual go.mod license review..."
        _scan_vsa_manual
        return
    fi

    local vsa_dir="$PROJECT_ROOT/components/vsa"
    local flagged=0

    pushd "$vsa_dir" >/dev/null

    local output
    output=$(go-licenses csv . 2>/dev/null || true)

    if [[ -z "$output" ]]; then
        log_warn "go-licenses returned no output — check that dependencies are vendored"
        popd >/dev/null
        return
    fi

    if [[ "$WRITE_REPORT" == "true" ]]; then
        echo "$output" > "$REPORTS_DIR/vsa-licenses.csv"
        log_info "Report: $REPORTS_DIR/vsa-licenses.csv"
    fi

    local pkg license url
    while IFS=',' read -r pkg url license; do
        license="${license// /}"
        if _is_flagged "$license"; then
            log_flag "Go: $pkg ($license)"
            ((flagged++)) || true
        elif [[ "$CSV_OUTPUT" == "true" ]]; then
            echo "vsa,$pkg,$license,$url"
        else
            log_ok "Go: $pkg ($license)"
        fi
    done <<< "$output"

    popd >/dev/null

    TOTAL_FLAGGED=$((TOTAL_FLAGGED + flagged))
    if [[ "$flagged" -eq 0 ]]; then
        log_ok "VSA: no flagged licenses"
    else
        log_flag "VSA: $flagged flagged license(s)"
    fi
}

_scan_vsa_manual() {
    local vsa_dir="$PROJECT_ROOT/components/vsa"
    local flagged=0

    log_info "Parsing go.mod for known license patterns..."

    while IFS= read -r line; do
        if [[ "$line" =~ ^[[:space:]]*(github\.com|golang\.org|modernc\.org)/([^[:space:]]+)[[:space:]] ]]; then
            local pkg="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
            log_info "dep: $pkg (review manually at https://pkg.go.dev/$pkg)"
        fi
    done < "$vsa_dir/go.mod"

    log_warn "Manual review required — install go-licenses for automated scanning"
}

# =============================================================================
# Python (VSE)
# =============================================================================
scan_vse() {
    log_header "VSE — Python dependencies (pip-licenses)"

    local venv="$PROJECT_ROOT/.venv"
    local pip_licenses=""

    if [[ -x "$venv/bin/pip-licenses" ]]; then
        pip_licenses="$venv/bin/pip-licenses"
    elif command -v pip-licenses &>/dev/null; then
        pip_licenses="pip-licenses"
    else
        log_warn "pip-licenses not installed. Installing into venv..."
        if [[ -x "$venv/bin/pip" ]]; then
            "$venv/bin/pip" install --quiet pip-licenses
            pip_licenses="$venv/bin/pip-licenses"
        else
            log_warn "Cannot install pip-licenses — no venv found at $venv"
            log_info  "Install with: source $venv/bin/activate && pip install pip-licenses"
            return
        fi
    fi

    local flagged=0
    local output
    output=$("$pip_licenses" --format=csv 2>/dev/null || true)

    if [[ -z "$output" ]]; then
        log_warn "pip-licenses returned no output"
        return
    fi

    if [[ "$WRITE_REPORT" == "true" ]]; then
        echo "$output" > "$REPORTS_DIR/vse-licenses.csv"
        log_info "Report: $REPORTS_DIR/vse-licenses.csv"
    fi

    local first=true
    while IFS=',' read -r pkg version license; do
        if [[ "$first" == "true" ]]; then
            first=false
            continue
        fi
        pkg="${pkg//\"/}"
        version="${version//\"/}"
        license="${license//\"/}"

        if _is_flagged "$license"; then
            log_flag "Python: $pkg $version ($license)"
            ((flagged++)) || true
        elif [[ "$CSV_OUTPUT" == "true" ]]; then
            echo "vse,$pkg,$version,$license"
        else
            log_ok "Python: $pkg $version ($license)"
        fi
    done <<< "$output"

    TOTAL_FLAGGED=$((TOTAL_FLAGGED + flagged))
    if [[ "$flagged" -eq 0 ]]; then
        log_ok "VSE: no flagged licenses"
    else
        log_flag "VSE: $flagged flagged license(s)"
    fi
}

# =============================================================================
# Node.js (VSOD)
# =============================================================================
scan_vsod() {
    log_header "VSOD — Node.js dependencies (license-checker)"

    local vsod_dir="$PROJECT_ROOT/components/vsod"
    local license_checker=""

    if [[ -x "$vsod_dir/node_modules/.bin/license-checker" ]]; then
        license_checker="$vsod_dir/node_modules/.bin/license-checker"
    elif command -v license-checker &>/dev/null; then
        license_checker="license-checker"
    else
        log_warn "license-checker not installed. Installing locally..."
        pushd "$vsod_dir" >/dev/null
        npm install --save-dev --no-audit --silent license-checker 2>/dev/null || {
            log_warn "npm install failed — ensure node_modules exist: cd components/vsod && npm ci"
            popd >/dev/null
            return
        }
        license_checker="$vsod_dir/node_modules/.bin/license-checker"
        popd >/dev/null
    fi

    local flagged=0
    local output
    output=$(cd "$vsod_dir" && "$license_checker" --csv --excludePrivatePackages 2>/dev/null || true)

    if [[ -z "$output" ]]; then
        log_warn "license-checker returned no output"
        return
    fi

    if [[ "$WRITE_REPORT" == "true" ]]; then
        echo "$output" > "$REPORTS_DIR/vsod-licenses.csv"
        log_info "Report: $REPORTS_DIR/vsod-licenses.csv"
    fi

    local first=true
    while IFS=',' read -r pkg version description license repo; do
        if [[ "$first" == "true" ]]; then
            first=false
            continue
        fi
        pkg="${pkg//\"/}"
        license="${license//\"/}"

        if _is_flagged "$license"; then
            log_flag "Node: $pkg ($license)"
            ((flagged++)) || true
        elif [[ "$CSV_OUTPUT" == "true" ]]; then
            echo "vsod,$pkg,$license"
        else
            log_ok "Node: $pkg ($license)"
        fi
    done <<< "$output"

    TOTAL_FLAGGED=$((TOTAL_FLAGGED + flagged))
    if [[ "$flagged" -eq 0 ]]; then
        log_ok "VSOD: no flagged licenses"
    else
        log_flag "VSOD: $flagged flagged license(s)"
    fi
}

# =============================================================================
# Main
# =============================================================================

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  g8e Dependency License Scanner"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Flagged license families: GPL, AGPL, LGPL, SSPL, OSL, EUPL, CC-BY-SA"
echo ""

_init_report_dir

[[ "$SCAN_VSA" == "true" ]]  && scan_vsa
[[ "$SCAN_VSE" == "true" ]]  && scan_vse
[[ "$SCAN_VSOD" == "true" ]] && scan_vsod

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Summary"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [[ "$TOTAL_FLAGGED" -eq 0 ]]; then
    echo -e "${GREEN}${BOLD}PASS${NC} — No incompatible licenses detected."
    echo ""
    exit 0
else
    echo -e "${RED}${BOLD}FAIL${NC} — $TOTAL_FLAGGED incompatible license(s) detected."
    echo ""
    echo "Review flagged packages before commercial distribution."
    echo "Options: replace the dependency, dual-license it, or obtain a commercial exception."
    echo ""
    exit 1
fi
