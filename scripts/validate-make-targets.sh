#!/bin/bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, 2.0.

# Validate every Makefile target (except `release`, which tags and pushes
# shared state). Targets are grouped into phases of related commands so a
# failure in one phase does not block unrelated phases. Each target's
# pass/fail is printed inline and a summary is printed at the end.
#
# Usage:
#   scripts/validate-make-targets.sh            # run all phases
#   scripts/validate-make-targets.sh PHASE=lint # run a single phase by name

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
SKIPPED_TARGETS="release"
FAILED_TARGETS=()

# Run a single make target, stream output, record result.
run_target() {
    local target="$1"

    if [[ " $SKIPPED_TARGETS " == *" $target "* ]]; then
        echo "  SKIP  $target (shared-state mutation — run manually)"
        SKIP_COUNT=$((SKIP_COUNT + 1))
        return
    fi

    printf "  RUN   %-30s " "$target"
    local start=$SECONDS
    if make "$target" >/dev/null 2>&1; then
        local dur=$((SECONDS - start))
        printf "PASS (%ds)\n" "$dur"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        local dur=$((SECONDS - start))
        printf "FAIL (%ds)\n" "$dur"
        FAILED_TARGETS+=("$target")
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

run_phase() {
    local name="$1"
    shift
    echo ""
    echo "================================================================"
    echo "  PHASE: $name"
    echo "================================================================"
    for target in "$@"; do
        run_target "$target"
    done
}

# Phase definitions (order matters: build before tests, cleanup last).
PHASE_HELP="help fmt"
PHASE_BUILD="build build-linux build-darwin build-windows build-all build-compressed build-fips verify-fips"
PHASE_PROTO="buf-install protoc-install proto generate proto-python proto-force"
PHASE_LINT="lint-no-embedded-newlines validate-doctrines validate-cosais swagger-generate readme readme-check vulncheck lint"
PHASE_TEST="test-unit test-integration test test-coverage test-airgap readme-test"
PHASE_PYTHON="python-build ensemble-lint evals-lint ensemble-test evals-test-unit evals-test-integration evals-test test-external build-ensemble"
PHASE_DASHBOARD="dashboard-test build-dashboard"
PHASE_DOCKER="up test-docker demo-verify"
PHASE_CI="ci-platform ci-ensemble ci-dashboard ci"
PHASE_DOCTRINE="ingest-doctrines update-doctrines"
PHASE_CLEAN="clean-harness down clean-docker clean"

ALL_PHASES=(
    "help:$PHASE_HELP"
    "build:$PHASE_BUILD"
    "proto:$PHASE_PROTO"
    "lint:$PHASE_LINT"
    "test:$PHASE_TEST"
    "python:$PHASE_PYTHON"
    "dashboard:$PHASE_DASHBOARD"
    "docker:$PHASE_DOCKER"
    "ci:$PHASE_CI"
    "doctrine:$PHASE_DOCTRINE"
    "clean:$PHASE_CLEAN"
)

SELECTOR="${PHASE:-}"

if [[ -n "$SELECTOR" ]]; then
    for entry in "${ALL_PHASES[@]}"; do
        local_name="${entry%%:*}"
        if [[ "$local_name" == "$SELECTOR" ]]; then
            run_phase "$SELECTOR" ${entry#*:}
            break
        fi
    done
else
    for entry in "${ALL_PHASES[@]}"; do
        run_phase "${entry%%:*}" ${entry#*:}
    done
fi

echo ""
echo "================================================================"
echo "  SUMMARY"
echo "================================================================"
echo "  PASS: $PASS_COUNT"
echo "  FAIL: $FAIL_COUNT"
echo "  SKIP: $SKIP_COUNT"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
    echo ""
    echo "Failed targets:"
    for t in "${FAILED_TARGETS[@]}"; do
        echo "  $t"
    done
fi
echo "================================================================"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    exit 1
fi
exit 0
