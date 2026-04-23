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

"""AST guardrail: every MCP JSON-RPC tool-call request must go through
``build_tool_call_request`` so ``execution_id`` is always stamped into both the
envelope ``id`` and ``params.arguments["execution_id"]``.

Background: A recent regression had 5 of 10 ``build_tool_call_request`` call
sites pass ``request_id=exec_id`` but forget ``execution_id`` inside the
``arguments`` dict. The adapter now takes ``execution_id`` as a required
parameter and auto-injects it, making *that* footgun structurally impossible.

The remaining failure mode this test guards against is a caller bypassing
``build_tool_call_request`` entirely and hand-constructing a
``JSONRPCRequest(...)`` for a ``tools/call`` dispatch.

Policy: ``JSONRPCRequest`` may only be instantiated inside
``app/services/mcp/adapter.py`` (the single blessed constructor) and inside
tests. Any other instantiation is a violation.
"""

from __future__ import annotations

import ast
from pathlib import Path

import pytest

REPO_APP_ROOT = Path(__file__).resolve().parents[4] / "app"
ALLOWED_CONSTRUCTION_SITES: frozenset[Path] = frozenset({
    REPO_APP_ROOT / "services" / "mcp" / "adapter.py",
})


def _iter_app_python_files() -> list[Path]:
    return sorted(p for p in REPO_APP_ROOT.rglob("*.py") if p.is_file())


def _find_jsonrpc_request_constructions(path: Path) -> list[int]:
    """Return line numbers where ``JSONRPCRequest(...)`` is called in ``path``.

    Matches both ``JSONRPCRequest(...)`` and ``mcp.JSONRPCRequest(...)`` call
    forms. Does not match bare references (e.g. type annotations or imports).
    """
    tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    hits: list[int] = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        if isinstance(func, ast.Name) and func.id == "JSONRPCRequest":
            hits.append(node.lineno)
        elif isinstance(func, ast.Attribute) and func.attr == "JSONRPCRequest":
            hits.append(node.lineno)
    return hits


def test_jsonrpc_request_only_constructed_in_adapter() -> None:
    """Enforce that ``JSONRPCRequest`` is constructed only inside the adapter.

    Any new hand-rolled construction under ``app/`` must either move through
    ``build_tool_call_request`` or be explicitly added to
    ``ALLOWED_CONSTRUCTION_SITES`` with a written justification in the PR.
    """
    violations: list[str] = []
    for file_path in _iter_app_python_files():
        if file_path in ALLOWED_CONSTRUCTION_SITES:
            continue
        for lineno in _find_jsonrpc_request_constructions(file_path):
            rel = file_path.relative_to(REPO_APP_ROOT.parent)
            violations.append(f"{rel}:{lineno}")

    assert not violations, (
        "JSONRPCRequest(...) was constructed outside the blessed adapter. "
        "Use app.services.mcp.adapter.build_tool_call_request(...) instead so "
        "execution_id is always stamped into both the envelope id and "
        "params.arguments['execution_id'].\n"
        "Violations:\n  - " + "\n  - ".join(violations)
    )


def test_adapter_is_actually_the_allowed_site() -> None:
    """Sanity: the allow-list points at a file that exists and constructs
    JSONRPCRequest. If this ever fails the allow-list is stale."""
    adapter_path = REPO_APP_ROOT / "services" / "mcp" / "adapter.py"
    assert adapter_path.exists(), f"adapter not found at {adapter_path}"
    assert _find_jsonrpc_request_constructions(adapter_path), (
        "adapter.py no longer constructs JSONRPCRequest — allow-list is stale"
    )


def test_build_tool_call_request_is_only_defined_once() -> None:
    """The entire point of centralization is a single definition. Guard
    against someone adding a second helper that bypasses the invariant."""
    definitions: list[str] = []
    for file_path in _iter_app_python_files():
        tree = ast.parse(file_path.read_text(encoding="utf-8"), filename=str(file_path))
        for node in ast.walk(tree):
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name == "build_tool_call_request":
                rel = file_path.relative_to(REPO_APP_ROOT.parent)
                definitions.append(f"{rel}:{node.lineno}")

    assert len(definitions) == 1, (
        "Expected exactly one definition of build_tool_call_request under app/, "
        f"found {len(definitions)}: {definitions}"
    )


@pytest.mark.parametrize("file_rel", [
    "services/operator/execution_service.py",
    "services/operator/command_service.py",
    "services/operator/port_service.py",
    "services/operator/file_service.py",
    "services/operator/filesystem_service.py",
    "services/operator/intent_service.py",
])
def test_operator_services_use_build_tool_call_request(file_rel: str) -> None:
    """Every operator service file that dispatches an MCP tool call must
    import and invoke ``build_tool_call_request``. This catches a service
    being added that rolls its own JSON-RPC envelope off-list."""
    path = REPO_APP_ROOT / file_rel
    source = path.read_text(encoding="utf-8")
    assert "build_tool_call_request" in source, (
        f"{file_rel} does not reference build_tool_call_request; if this file "
        "no longer dispatches MCP tool calls, remove it from this parametrize "
        "list. Otherwise, route through the blessed constructor."
    )
