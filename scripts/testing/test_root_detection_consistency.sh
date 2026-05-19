#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Regression test for root detection consistency across Shell, Go, and Python.
# This test ensures all three language implementations return the same result.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$SCRIPT_DIR/scripts/core/path_utils.sh"
G8E_PROJECT_ROOT="$(resolve_g8e_root)"
export G8E_PROJECT_ROOT

echo "[test] Root detection consistency test"
echo "[test] Expected root: $G8E_PROJECT_ROOT"

# Test 1: Shell root detection from various directories
echo "[test] Testing Shell root detection..."
cd "$G8E_PROJECT_ROOT"
shell_root_from_root="$(resolve_g8e_root)"
if [ "$shell_root_from_root" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Shell root from project root: $shell_root_from_root (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

cd "$G8E_PROJECT_ROOT/services/g8eo"
shell_root_from_g8eo="$(resolve_g8e_root)"
if [ "$shell_root_from_g8eo" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Shell root from services/g8eo: $shell_root_from_g8eo (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

cd "$G8E_PROJECT_ROOT/services/g8ee"
shell_root_from_g8ee="$(resolve_g8e_root)"
if [ "$shell_root_from_g8ee" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Shell root from services/g8ee: $shell_root_from_g8ee (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

cd "$G8E_PROJECT_ROOT/scripts"
shell_root_from_scripts="$(resolve_g8e_root)"
if [ "$shell_root_from_scripts" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Shell root from scripts: $shell_root_from_scripts (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

echo "[PASS] Shell root detection consistent"

# Test 2: Python root detection from various directories
echo "[test] Testing Python root detection..."

cd "$G8E_PROJECT_ROOT"
python_root_from_root=$(python3 -c "
import sys
import importlib.util
spec = importlib.util.spec_from_file_location('_lib', '$G8E_PROJECT_ROOT/scripts/data/_lib.py')
lib = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lib)
print(lib.resolve_project_root())
")
if [ "$python_root_from_root" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Python root from project root: $python_root_from_root (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

cd "$G8E_PROJECT_ROOT/services/g8eo"
python_root_from_g8eo=$(python3 -c "
import sys
import importlib.util
spec = importlib.util.spec_from_file_location('_lib', '$G8E_PROJECT_ROOT/scripts/data/_lib.py')
lib = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lib)
print(lib.resolve_project_root())
")
if [ "$python_root_from_g8eo" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Python root from services/g8eo: $python_root_from_g8eo (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

cd "$G8E_PROJECT_ROOT/services/g8ee"
python_root_from_g8ee=$(python3 -c "
import sys
import importlib.util
spec = importlib.util.spec_from_file_location('_lib', '$G8E_PROJECT_ROOT/scripts/data/_lib.py')
lib = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lib)
print(lib.resolve_project_root())
")
if [ "$python_root_from_g8ee" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Python root from services/g8ee: $python_root_from_g8ee (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

cd "$G8E_PROJECT_ROOT/scripts/data"
python_root_from_data=$(python3 -c "
import sys
import importlib.util
spec = importlib.util.spec_from_file_location('_lib', '$G8E_PROJECT_ROOT/scripts/data/_lib.py')
lib = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lib)
print(lib.resolve_project_root())
")
if [ "$python_root_from_data" != "$G8E_PROJECT_ROOT" ]; then
    echo "[FAIL] Python root from scripts/data: $python_root_from_data (expected $G8E_PROJECT_ROOT)"
    exit 1
fi

echo "[PASS] Python root detection consistent"

# Test 3: Go root detection from various directories
echo "[test] Testing Go root detection..."

cd "$G8E_PROJECT_ROOT/services/g8eo"
go_root_test_output=$(go test -v -run TestResolveProjectRootConsistency ./internal/services/system/ 2>&1)
if echo "$go_root_test_output" | grep -q "FAIL"; then
    echo "[FAIL] Go root detection test failed"
    echo "$go_root_test_output"
    exit 1
fi

echo "[PASS] Go root detection consistent"

# Test 4: Cross-language consistency
echo "[test] Testing cross-language consistency..."
if [ "$shell_root_from_root" != "$python_root_from_root" ]; then
    echo "[FAIL] Shell and Python roots differ: $shell_root_from_root vs $python_root_from_root"
    exit 1
fi

echo "[PASS] All root detection implementations consistent"

echo "[test] Root detection consistency test passed"
