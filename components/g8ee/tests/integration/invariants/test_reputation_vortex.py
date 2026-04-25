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

import ast
import os
from pathlib import Path

import pytest

# GDD §3 Vortex Invariant: Reputation data is highly sensitive and its
# visibility must be strictly controlled to prevent persona prompt leakage.
# Only the Auditor (reading) and ReputationService (writing) should touch
# the reputation_data_service.

ALLOW_LIST = {
    "app/services/data/reputation_data_service.py",  # The service itself
    "app/services/service_factory.py",                # Dependency injection
    "app/services/ai/auditor_service.py",            # Reader (Artifact B)
    "app/services/ai/reputation_service.py",          # Writer (Artifact A)
    "app/services/ai/tool_service.py",               # Exposer for tribunal
}

PROJECT_ROOT = Path("/home/bob/g8e/components/g8ee")


def get_all_python_files(root: Path):
    for path in root.rglob("*.py"):
        if "tests" in path.parts:
            continue
        if ".ruff_cache" in path.parts:
            continue
        yield path


def check_file_for_reputation_import(file_path: Path) -> bool:
    """Returns True if the file imports ReputationDataService or reputation_data_service."""
    with open(file_path, "r", encoding="utf-8") as f:
        try:
            tree = ast.parse(f.read())
        except SyntaxError:
            return False

    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                if "reputation_data_service" in alias.name:
                    return True
        elif isinstance(node, ast.ImportFrom):
            if node.module and "reputation_data_service" in node.module:
                return True
            for alias in node.names:
                if alias.name == "ReputationDataService":
                    return True
    return False


@pytest.mark.integration
def test_reputation_data_service_import_boundary():
    """Vortex Invariant: Only allow-listed services may import ReputationDataService."""
    violations = []
    
    for py_file in get_all_python_files(PROJECT_ROOT):
        relative_path = str(py_file.relative_to(PROJECT_ROOT))
        
        if relative_path in ALLOW_LIST:
            continue
            
        if check_file_for_reputation_import(py_file):
            violations.append(relative_path)
            
    assert not violations, (
        f"GDD §3 Vortex Invariant violation! The following files import "
        f"reputation_data_service but are not in the allow-list: {violations}. "
        f"Reputation state must only be visible to the Auditor and ReputationService."
    )
