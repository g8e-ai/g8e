#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd)"
export G8E_PROJECT_ROOT="$SCRIPT_DIR"

test_help_g8eo() {
    output=$("$SCRIPT_DIR/g8e" test g8eo -h)
    if ! echo "$output" | grep -q "Go Operator Gateway tests"; then
        echo "FAIL: g8eo help output is missing expected content"
        exit 1
    fi
    if echo "$output" | grep -q "LLM Configuration"; then
        echo "FAIL: g8eo help output contains g8ee-only options"
        exit 1
    fi
    echo "PASS: g8eo help output is correct and clean"
}

test_help_g8ee() {
    output=$("$SCRIPT_DIR/g8e" test g8ee -h)
    if ! echo "$output" | grep -q "Python Engine adapter tests"; then
        echo "FAIL: g8ee help output is missing expected content"
        exit 1
    fi
    if ! echo "$output" | grep -q "LLM Configuration"; then
        echo "FAIL: g8ee help output is missing LLM options"
        exit 1
    fi
    echo "PASS: g8ee help output is correct and clean"
}

test_help_chaos() {
    output=$("$SCRIPT_DIR/g8e" test chaos -h)
    if ! echo "$output" | grep -q "Chaos Tester"; then
        echo "FAIL: chaos help output is missing expected content"
        exit 1
    fi
    echo "PASS: chaos help output is correct and clean"
}

test_help_ci() {
    output=$("$SCRIPT_DIR/g8e" test ci -h)
    if ! echo "$output" | grep -q "g8e CI workflow"; then
        echo "FAIL: ci help output is missing expected content"
        exit 1
    fi
    echo "PASS: ci help output is correct and clean"
}

test_help_general() {
    output=$("$SCRIPT_DIR/g8e" test -h)
    if ! echo "$output" | grep -q "Go Operator Gateway tests"; then
        echo "FAIL: general test help output is missing expected content"
        exit 1
    fi
    echo "PASS: general test help output is correct"
}

test_help_g8eo
test_help_g8ee
test_help_chaos
test_help_ci
test_help_general

echo "All test help verification checks passed!"
