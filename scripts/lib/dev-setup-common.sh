# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Shared helpers for the Unix dev-setup scripts. Source from linux-setup.sh
# or macos-setup.sh after setting SCRIPT_DIR.

G8E_NODE_MIN_MAJOR=22
G8E_EXPLORER_DIR="dashboard/g8e-adapter/evaluation-explorer"
G8E_EXPLORER_DIST="${G8E_EXPLORER_DIR}/dist/index.html"
G8E_PATH_MARKER="# g8e: add repository root to PATH"

g8e_setup_init() {
    REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
    cd "$REPO_ROOT"

    if [[ ! -f go.mod ]]; then
        echo "FATAL: run this script from a g8e repository clone (go.mod not found)."
        exit 1
    fi

    G8E_GO_MIN="$(sed -n 's/^go //p' go.mod | head -1 | tr -d '[:space:]')"
    if [[ -z "$G8E_GO_MIN" ]]; then
        echo "FATAL: could not read the required Go version from go.mod."
        exit 1
    fi

    AUTO_YES=false
    for arg in "$@"; do
        case "$arg" in
            -y|--yes) AUTO_YES=true ;;
        esac
    done
    if [[ "${G8E_SETUP_YES:-}" == "1" ]]; then
        AUTO_YES=true
    fi
}

g8e_setup_confirm() {
    local prompt="$1"
    if [[ "$AUTO_YES" == true ]]; then
        return 0
    fi
    read -r -p "$prompt" response
    [[ "$response" =~ ^[Yy]$ ]]
}

g8e_version_major_minor() {
    echo "$1" | sed -E 's/^[^0-9]*([0-9]+)\.([0-9]+).*/\1.\2/'
}

g8e_version_ge() {
    local have="$1"
    local need="$2"
    local have_major have_minor need_major need_minor
    have_major="$(echo "$have" | cut -d. -f1)"
    have_minor="$(echo "$have" | cut -d. -f2)"
    need_major="$(echo "$need" | cut -d. -f1)"
    need_minor="$(echo "$need" | cut -d. -f2)"
    if [[ "$have_major" -gt "$need_major" ]]; then
        return 0
    fi
    if [[ "$have_major" -eq "$need_major" && "$have_minor" -ge "$need_minor" ]]; then
        return 0
    fi
    return 1
}

g8e_check_git() {
    if command -v git >/dev/null 2>&1; then
        echo "  git: detected ($(git --version | awk '{print $3}'))"
        return 0
    fi
    echo "  git: missing"
    return 1
}

g8e_check_make() {
    if command -v make >/dev/null 2>&1; then
        echo "  make: detected"
        return 0
    fi
    echo "  make: missing"
    return 1
}

g8e_check_go() {
    if ! command -v go >/dev/null 2>&1; then
        echo "  go: missing (need >= $G8E_GO_MIN from go.mod)"
        return 1
    fi

    local version
    version="$(g8e_version_major_minor "$(go version)")"
    if [[ -z "$version" ]]; then
        echo "  go: detected but version unknown"
        return 1
    fi
    if g8e_version_ge "$version" "$G8E_GO_MIN"; then
        echo "  go: detected (v$version, need >= $G8E_GO_MIN)"
        return 0
    fi
    echo "  go: detected (v$version) but go.mod requires >= $G8E_GO_MIN"
    return 1
}

g8e_check_node() {
    if ! command -v node >/dev/null 2>&1; then
        echo "  node: missing (need >= $G8E_NODE_MIN_MAJOR for evaluation-explorer build)"
        return 1
    fi
    if ! command -v npm >/dev/null 2>&1; then
        echo "  npm: missing"
        return 1
    fi

    local version
    version="$(g8e_version_major_minor "$(node --version)")"
    if [[ -z "$version" ]]; then
        echo "  node: detected but version unknown"
        return 1
    fi
    if g8e_version_ge "$version" "${G8E_NODE_MIN_MAJOR}.0"; then
        echo "  node: detected (v$version)"
        echo "  npm: detected ($(npm --version))"
        return 0
    fi
    echo "  node: detected (v$version) but need >= ${G8E_NODE_MIN_MAJOR}.0"
    return 1
}

g8e_build_evaluation_explorer() {
    if [[ -f "$G8E_EXPLORER_DIST" ]]; then
        echo "  evaluation-explorer dist already present — skipping frontend build"
        return 0
    fi

    echo "  building evaluation-explorer assets (required by make build)..."
    pushd "$G8E_EXPLORER_DIR" >/dev/null
    if [[ -f package-lock.json ]]; then
        npm ci
    else
        npm install
    fi
    npm run build
    popd >/dev/null
}

g8e_run_make_build() {
    make build
}

g8e_configure_path_unix() {
    local path_line="export PATH=\"\$PATH:$REPO_ROOT\""
    local profile_file

    if [[ -n "${ZSH_VERSION:-}" || "${SHELL:-}" == */zsh ]]; then
        profile_file="$HOME/.zshrc"
    elif [[ -n "${BASH_VERSION:-}" || "${SHELL:-}" == */bash ]]; then
        profile_file="$HOME/.bashrc"
    else
        profile_file="$HOME/.profile"
    fi

    if grep -qF "$G8E_PATH_MARKER" "$profile_file" 2>/dev/null || grep -qF "$REPO_ROOT" "$profile_file" 2>/dev/null; then
        echo "  repository root already present in $profile_file — skipping."
    else
        {
            echo ""
            echo "$G8E_PATH_MARKER"
            echo "$path_line"
        } >>"$profile_file"
        echo "  added $REPO_ROOT to PATH in $profile_file"
    fi

    export PATH="$PATH:$REPO_ROOT"
    G8E_PROFILE_FILE="$profile_file"
}

g8e_print_next_steps_unix() {
    local binary="./g8e"
    if [[ ! -f "$REPO_ROOT/g8e" && -f "$REPO_ROOT/g8e.exe" ]]; then
        binary="./g8e.exe"
    fi

    cat <<EOF

[SETUP COMPLETE]
---------------------------------------------------------------
Binary: $binary (repository root added to PATH for this session)

Verify:
  $binary --version

Recommended next steps:
  Docker stack (no local Go toolchain after this build):
    cp .env.example .env
    $binary docker start --full

  Native gateway:
    $binary gw start

  Owner enrollment against a running gateway:
    $binary auth enroll user -e localhost

Docs:
  docs/guides/getting_started.md
  docs/guides/unified_stack.md

Note: open a new terminal or run 'source ${G8E_PROFILE_FILE:-your shell profile}'
      to use g8e from other shells.
EOF
}
