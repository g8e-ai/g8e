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
    "app/services/ai/agent_tool_loop.py",            # Hook point for stake resolution
    "app/services/infra/g8ed_event_service.py",      # SSE emission
    "app/models/reputation.py",                      # Model definitions
    "app/models/agents/tribunal.py",                 # Result models
    "app/models/tribunal_commands.py",               # Auditor model
    "app/services/ai/generator.py",                  # Threads reputation id
}

REPUTATION_MODELS = {
    "ReputationState",
    "ReputationCommitment",
    "StakeResolution",
}

REPUTATION_FIELDS = {
    "reputation_commitment_id",
}

PROJECT_ROOT = Path("/home/bob/g8e/components/g8ee")


def get_all_python_files(root: Path):
    for path in root.rglob("*.py"):
        if "tests" in path.parts:
            continue
        if ".ruff_cache" in path.parts:
            continue
        yield path


def check_file_for_violations(file_path: Path) -> list[str]:
    """Returns a list of violation descriptions found in the file."""
    with open(file_path, "r", encoding="utf-8") as f:
        try:
            tree = ast.parse(f.read())
        except SyntaxError:
            return []

    violations = []
    
    for node in ast.walk(tree):
        # 1. Check for imports of the data service
        if isinstance(node, ast.Import):
            for alias in node.names:
                if "reputation_data_service" in alias.name:
                    violations.append(f"Import of reputation_data_service: {alias.name}")
        elif isinstance(node, ast.ImportFrom):
            if node.module and "reputation_data_service" in node.module:
                violations.append(f"Import from reputation_data_service: {node.module}")
            for alias in node.names:
                if alias.name == "ReputationDataService":
                    violations.append("Import of ReputationDataService class")
                if alias.name in REPUTATION_MODELS:
                    violations.append(f"Import of reputation model: {alias.name}")

        # 2. Check for usage of reputation models in code (e.g. as type hints or constructors)
        if isinstance(node, ast.Name):
            if node.id in REPUTATION_MODELS:
                violations.append(f"Direct usage of reputation model: {node.id}")

        # 3. Check for access to reputation-related fields
        if isinstance(node, ast.Attribute):
            if node.attr in REPUTATION_FIELDS:
                violations.append(f"Access to reputation-related field: {node.attr}")

    return list(set(violations))  # Unique violations


@pytest.mark.integration
def test_reputation_vortex_invariant():
    """Vortex Invariant: Only allow-listed services may touch reputation data or models."""
    all_violations = {}
    
    for py_file in get_all_python_files(PROJECT_ROOT):
        relative_path = str(py_file.relative_to(PROJECT_ROOT))
        
        if relative_path in ALLOW_LIST:
            continue
            
        file_violations = check_file_for_violations(py_file)
        if file_violations:
            all_violations[relative_path] = file_violations
            
    if all_violations:
        msg = "GDD §3 Vortex Invariant violation! The following files touch reputation data or models but are not in the allow-list:\n"
        for path, issues in all_violations.items():
            msg += f"  - {path}: {', '.join(issues)}\n"
        msg += "\nReputation state must only be visible to the Auditor and ReputationService to preserve the information quarantine."
        pytest.fail(msg)
