# shellcheck shell=bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Shared helpers for the Unix dev-setup scripts and scripts/dev-check.sh. Source
# from linux-setup.sh, macos-setup.sh, or dev-check.sh after setting SCRIPT_DIR.
# Must stay compatible with bash 3.2 (the macOS system bash): no associative
# arrays, no mapfile, no ${var,,}.
#
# Tool versions are NOT defined here. Go comes from go.mod; every other pin
# (uv, Python, buf, protoc plugins, golangci-lint) is read from the root Makefile
# so there is a single place to bump them (see g8e_make_var).

G8E_NODE_MIN_MAJOR=22
G8E_EXPLORER_DIR="dashboard/g8e-adapter/evaluation-explorer"
G8E_EXPLORER_DIST="${G8E_EXPLORER_DIR}/dist/index.html"
G8E_PATH_MARKER="# g8e: add repository root to PATH"

# Reads `NAME := value` from the root Makefile.
g8e_make_var() {
    sed -n "s/^$1[[:space:]]*:=[[:space:]]*//p" Makefile | head -1 | tr -d '[:space:]'
}

g8e_usage() {
    cat <<EOF
Usage: bash scripts/$(basename "$0") [options]

Installs the toolchain needed to build g8e and, by default, to run 'make ci'
(Go dev tools, a Python venv, and Node dependencies).

Options:
  -y, --yes       Install missing tooling without prompting (or set G8E_SETUP_YES=1)
  --build-only    Only set up what 'make build' needs; skip the contributor toolchain
  -h, --help      Show this help
EOF
}

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
    G8E_UV_VERSION="$(g8e_make_var UV_VERSION)"
    G8E_PYTHON_VERSION="$(g8e_make_var PYTHON_VERSION)"
    if [[ -z "$G8E_UV_VERSION" || -z "$G8E_PYTHON_VERSION" ]]; then
        echo "FATAL: could not read UV_VERSION / PYTHON_VERSION from the Makefile."
        exit 1
    fi

    AUTO_YES=false
    G8E_BUILD_ONLY=false
    for arg in "$@"; do
        case "$arg" in
            -y|--yes) AUTO_YES=true ;;
            --build-only) G8E_BUILD_ONLY=true ;;
            -h|--help) g8e_usage; exit 0 ;;
            *) echo "Unknown option: $arg"; g8e_usage; exit 2 ;;
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

# Pulls the first dotted number out of a tool's version banner:
# "go version go1.26.6 linux/amd64" -> 1.26.6, "v22.23.3" -> 22.23.3.
g8e_extract_version() {
    echo "$1" | sed -En 's/^[^0-9]*([0-9]+(\.[0-9]+)*).*/\1/p' | head -1
}

# True when $1 >= $2, comparing up to three dotted components. A missing
# component counts as 0, so "1.26" is older than "1.26.6".
g8e_version_ge() {
    local IFS=.
    local -a have need
    local i h n
    # Splitting on the dots (IFS=.) is the point.
    # shellcheck disable=SC2206
    have=($1)
    # shellcheck disable=SC2206
    need=($2)
    for i in 0 1 2; do
        h="${have[$i]:-0}"
        n="${need[$i]:-0}"
        if (( 10#$h > 10#$n )); then
            return 0
        fi
        if (( 10#$h < 10#$n )); then
            return 1
        fi
    done
    return 0
}

# True when `command` resolves to a native tool. On WSL the Windows PATH is
# appended to the Linux one and exposes extensionless Windows shims (npm, npx);
# those must not count as an installed Linux toolchain.
g8e_have() {
    local resolved
    resolved="$(command -v "$1" 2>/dev/null)" || return 1
    case "$resolved" in
        /mnt/[a-z]/*) return 1 ;;
    esac
    return 0
}

g8e_sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# --- prerequisite checks: print one line each, return 1 when missing -----------

g8e_check_git() {
    if g8e_have git; then
        echo "  git: detected ($(git --version | awk '{print $3}'))"
        return 0
    fi
    echo "  git: missing"
    return 1
}

g8e_check_make() {
    if g8e_have make; then
        echo "  make: detected"
        return 0
    fi
    echo "  make: missing"
    return 1
}

g8e_check_curl() {
    if g8e_have curl; then
        echo "  curl: detected"
        return 0
    fi
    echo "  curl: missing (used to download Go, Node.js, and uv)"
    return 1
}

g8e_check_go() {
    if ! g8e_have go; then
        echo "  go: missing (need >= $G8E_GO_MIN from go.mod)"
        return 1
    fi

    local version
    version="$(g8e_extract_version "$(go version)")"
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
    if ! g8e_have node; then
        echo "  node: missing (need >= $G8E_NODE_MIN_MAJOR for the evaluation-explorer build and dashboard)"
        return 1
    fi
    if ! g8e_have npm; then
        echo "  npm: missing (an npm found only under /mnt/<drive> is a Windows shim and does not count)"
        return 1
    fi

    local version
    version="$(g8e_extract_version "$(node --version)")"
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

# Any python3 will do: the Makefile uses it for json.tool and the BSL header
# check. The project venv gets its own interpreter from uv.
g8e_check_python3() {
    if g8e_have python3; then
        echo "  python3: detected ($(g8e_extract_version "$(python3 --version 2>&1)"))"
        return 0
    fi
    echo "  python3: missing (used by make validate-doctrines and check-bsl-headers)"
    return 1
}

# uv is pinned because 'make proto' re-locks ensemble/uv.lock with it. A
# different version still works, so it only warns.
g8e_check_uv() {
    if ! g8e_have uv; then
        echo "  uv: missing (need ${G8E_UV_VERSION}; creates the Python ${G8E_PYTHON_VERSION} venv and locks ensemble deps)"
        return 1
    fi
    local version
    version="$(g8e_extract_version "$(uv --version 2>&1)")"
    if [[ "$version" == "$G8E_UV_VERSION" ]]; then
        echo "  uv: detected (v$version)"
    else
        echo "  uv: detected (v$version; CI pins v${G8E_UV_VERSION}, 'make proto' may re-lock ensemble/uv.lock differently)"
    fi
    return 0
}

g8e_check_ripgrep() {
    if g8e_have rg; then
        echo "  rg: detected"
        return 0
    fi
    echo "  rg: missing (ripgrep; used by make dashboard-boundary-check)"
    return 1
}

g8e_check_bc() {
    if g8e_have bc; then
        echo "  bc: detected"
        return 0
    fi
    echo "  bc: missing (used by the make ci coverage threshold)"
    return 1
}

# 'go test -race' (make ci, make test-integration) needs cgo, hence a C compiler.
g8e_check_cc() {
    local cc
    for cc in cc gcc clang; do
        if g8e_have "$cc"; then
            echo "  C compiler: detected ($cc; required by the Go race detector)"
            return 0
        fi
    done
    echo "  C compiler: missing (required by 'go test -race')"
    return 1
}

# Fills MISSING with the logical names of absent prerequisites. The contributor
# toolchain is skipped with --build-only.
g8e_collect_missing() {
    MISSING=()
    g8e_check_git || MISSING+=("git")
    g8e_check_make || MISSING+=("make")
    g8e_check_curl || MISSING+=("curl")
    g8e_check_go || MISSING+=("go")
    g8e_check_node || MISSING+=("node")
    if [[ "$G8E_BUILD_ONLY" != true ]]; then
        g8e_check_python3 || MISSING+=("python3")
        g8e_check_uv || MISSING+=("uv")
        g8e_check_ripgrep || MISSING+=("rg")
        g8e_check_bc || MISSING+=("bc")
        g8e_check_cc || MISSING+=("cc")
    fi
}

# --- contributor-toolchain checks (used by dev-check.sh) -----------------------

# Checks a Go-installed tool, optionally against a pinned version. A version
# mismatch warns rather than fails: it still runs, but may not match CI output.
# usage: g8e_check_tool <command> [pinned-version] [version-flag]
g8e_check_tool() {
    local cmd="$1" pinned="${2:-}" flag="${3:---version}" got
    pinned="${pinned#v}"
    if ! g8e_have "$cmd"; then
        echo "  $cmd: missing (run: make dev-tools)"
        return 1
    fi
    if [[ -z "$pinned" ]]; then
        echo "  $cmd: detected"
        return 0
    fi
    got="$(g8e_extract_version "$("$cmd" "$flag" 2>&1 | head -1)")"
    if [[ "$got" == "$pinned" ]]; then
        echo "  $cmd: detected (v$got)"
    else
        echo "  $cmd: detected (v${got:-unknown}; CI pins v$pinned, run: make dev-tools)"
    fi
    return 0
}

g8e_check_go_tools() {
    local failed=0
    g8e_check_tool buf "$(g8e_make_var BUF_VERSION)" || failed=1
    g8e_check_tool protoc-gen-go "$(g8e_make_var PROTOC_GEN_GO_VERSION)" || failed=1
    g8e_check_tool protoc-gen-go-grpc "$(g8e_make_var PROTOC_GEN_GO_GRPC_VERSION)" || failed=1
    g8e_check_tool protoc-gen-doc || failed=1
    g8e_check_tool golangci-lint "$(g8e_make_var GOLANGCI_LINT_VERSION)" || failed=1
    g8e_check_tool govulncheck || failed=1
    g8e_check_tool swag || failed=1
    return $failed
}

g8e_check_python_env() {
    local py=".venv/bin/python" ver
    if [[ ! -x "$py" ]]; then
        echo "  .venv: missing (run: make dev-python)"
        return 1
    fi
    ver="$(g8e_extract_version "$("$py" --version 2>&1)")"
    if [[ "$ver" != "$G8E_PYTHON_VERSION".* ]]; then
        echo "  .venv: Python $ver but ensemble needs $G8E_PYTHON_VERSION (remove .venv, then: make dev-python)"
        return 1
    fi
    if ! "$py" -c "import grpc_tools, g8e" >/dev/null 2>&1; then
        echo "  .venv: Python $ver but grpcio-tools / the in-tree g8e package are not installed (run: make dev-python)"
        return 1
    fi
    local tool
    for tool in ruff pyright pytest; do
        if [[ ! -x ".venv/bin/$tool" ]]; then
            echo "  .venv: $tool missing (run: make dev-python)"
            return 1
        fi
    done
    echo "  .venv: ready (Python $ver, protocol + ensemble deps)"
    return 0
}

g8e_check_node_deps() {
    local failed=0 dir
    for dir in dashboard protocol/node dashboard/g8e-adapter; do
        if [[ -d "$dir/node_modules" ]]; then
            echo "  $dir/node_modules: present"
        else
            echo "  $dir/node_modules: missing (run: make dev-node)"
            failed=1
        fi
    done
    if [[ ! -f "$G8E_EXPLORER_DIST" ]]; then
        echo "  evaluation-explorer dist: missing (run the setup script, or: cd $G8E_EXPLORER_DIR && npm ci && npm run build)"
        failed=1
    fi
    return $failed
}

# --- installers shared by the Linux and macOS scripts --------------------------

# Installs the pinned uv with the standalone installer into ~/.local/bin and
# leaves shell profiles alone (g8e_configure_path_unix owns PATH).
g8e_install_uv() {
    local installer rc=0
    if ! g8e_have curl; then
        echo "  curl is required to install uv"
        return 1
    fi
    installer="$(mktemp "${TMPDIR:-/tmp}/g8e-uv-install.XXXXXX")"
    echo "Installing uv ${G8E_UV_VERSION} to \$HOME/.local/bin..."
    if ! curl -fsSL "https://astral.sh/uv/${G8E_UV_VERSION}/install.sh" -o "$installer"; then
        rm -f "$installer"
        return 1
    fi
    UV_INSTALL_DIR="$HOME/.local/bin" UV_NO_MODIFY_PATH=1 sh "$installer" || rc=$?
    rm -f "$installer"
    export PATH="$HOME/.local/bin:$PATH"
    return $rc
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

g8e_run_dev_setup() {
    make dev-setup
}

# --- PATH ------------------------------------------------------------------------

G8E_PROFILE_FILE=""

g8e_select_profile() {
    if [[ -n "${ZSH_VERSION:-}" || "${SHELL:-}" == */zsh ]]; then
        G8E_PROFILE_FILE="$HOME/.zshrc"
    elif [[ -n "${BASH_VERSION:-}" || "${SHELL:-}" == */bash ]]; then
        G8E_PROFILE_FILE="$HOME/.bashrc"
    else
        G8E_PROFILE_FILE="$HOME/.profile"
    fi
}

# Appends `export PATH="$PATH:<dir>"` to the profile unless the profile already
# mentions <dir> (literally, or as $HOME/..., ${HOME}/..., ~/...), and puts <dir>
# on PATH for this script. Profiles are only ever appended to, never rewritten.
# Pass "prepend" as the third argument when <dir> must win over an older tool
# already on PATH (the Go tarball in /usr/local/go versus a distro golang).
# usage: g8e_add_to_path <dir> [comment] [prepend]
g8e_add_to_path() {
    local dir="$1" comment="${2:-# g8e: add $1 to PATH}" mode="${3:-append}" rel variant found=false
    local -a variants
    variants=("$dir")
    rel="${dir#"$HOME"/}"
    if [[ "$rel" != "$dir" ]]; then
        # The tilde is matched literally: it is how a profile spells the path.
        # shellcheck disable=SC2088
        variants+=("\$HOME/$rel" "\${HOME}/$rel" "~/$rel")
    fi
    for variant in "${variants[@]}"; do
        if grep -qF -- "$variant" "$G8E_PROFILE_FILE" 2>/dev/null; then
            found=true
            break
        fi
    done

    if [[ "$found" == true ]]; then
        echo "  $dir already present in $G8E_PROFILE_FILE — skipping."
    else
        {
            echo ""
            echo "$comment"
            if [[ "$mode" == "prepend" ]]; then
                echo "export PATH=\"$dir:\$PATH\""
            else
                echo "export PATH=\"\$PATH:$dir\""
            fi
        } >>"$G8E_PROFILE_FILE"
        echo "  added $dir to PATH in $G8E_PROFILE_FILE"
    fi

    case ":$PATH:" in
        *":$dir:"*) ;;
        *)
            if [[ "$mode" == "prepend" ]]; then
                export PATH="$dir:$PATH"
            else
                export PATH="$PATH:$dir"
            fi
            ;;
    esac
}

g8e_configure_path_unix() {
    g8e_select_profile
    g8e_add_to_path "$REPO_ROOT" "$G8E_PATH_MARKER"

    if [[ "$G8E_BUILD_ONLY" == true ]]; then
        return 0
    fi

    # Directories the contributor toolchain installs into. Only those that exist
    # are added, so a custom GOPATH/GOBIN or an unused ~/.local/bin is not
    # written into the profile for nothing.
    local gobin
    if g8e_have go; then
        gobin="$(go env GOBIN)"
        if [[ -z "$gobin" ]]; then
            gobin="$(go env GOPATH)/bin"
        fi
        if [[ -d "$gobin" ]]; then
            g8e_add_to_path "$gobin" "# g8e: Go-installed dev tools (buf, protoc plugins, golangci-lint, ...)"
        fi
    fi
    if [[ -d "$HOME/.local/bin" ]]; then
        g8e_add_to_path "$HOME/.local/bin" "# g8e: uv"
    fi
    if [[ "$(command -v go 2>/dev/null)" == /usr/local/go/bin/go ]]; then
        g8e_add_to_path /usr/local/go/bin "# g8e: Go toolchain" prepend
    fi
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
EOF

    if [[ "$G8E_BUILD_ONLY" != true ]]; then
        cat <<EOF
  make dev-check        # confirms every tool 'make ci' needs is installed

Contributor workflow:
  make ci               # full local CI (platform, ensemble, dashboard)
  make help             # all targets
EOF
    fi

    cat <<EOF

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
      to use g8e and the dev tools from other shells.
EOF
}
