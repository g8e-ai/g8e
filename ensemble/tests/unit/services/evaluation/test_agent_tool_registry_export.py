# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Drift tests for the generated agent tool registry.

The Go scenario catalog lints prompts against these schemas and seeds prior
tool calls with these guidance vectors, so the committed registry must equal
what g8ee produces today.
"""

from __future__ import annotations

import json

import pytest

from app.constants import CommandErrorType
from app.models.tool_results import CommandExecutionResult
from app.services.evaluation.agent_tool_registry_export import (
    REGISTRY_PATH,
    build_agent_tool_registry,
    main,
    render_agent_tool_registry,
)
from app.services.evaluation.tool_evidence import (
    POLICY_DENY_ERROR_TYPES,
    _policy_outcome_from_result,
)

pytestmark = [pytest.mark.unit]


def test_committed_registry_matches_what_g8ee_generates_today():
    assert REGISTRY_PATH.read_text(encoding="utf-8") == render_agent_tool_registry(
        build_agent_tool_registry()
    ), "agent-tool-registry.json is stale: run `make agent-tool-registry`"


def test_registry_declares_required_arguments_from_the_real_tool_schemas():
    tools = {tool.name: tool for tool in build_agent_tool_registry().tools}

    assert tools["recursive_grep_search"].required_arguments == ["path", "pattern", "target_operators"]
    assert tools["file_read_on_operator"].required_arguments == [
        "file_path",
        "justification",
        "target_operators",
    ]
    assert tools["query_investigation_context"].required_arguments == ["data_type"]
    assert tools["run_commands_with_operator"].required_arguments == ["request"]


def test_guidance_vectors_carry_the_text_the_model_is_shown():
    vectors = {vector.vector_id: vector for vector in build_agent_tool_registry().guidance_vectors}

    grep = vectors["recursive_grep_search.missing_path"]
    assert json.loads(grep.arguments_json) == {"pattern": "AUTH_FAILURE"}
    assert "Tool execution failed for recursive_grep_search" in grep.error
    assert "path\n  Field required" in grep.error

    escalation = vectors["run_commands_with_operator.privilege_escalation"]
    assert escalation.error.startswith("SECURITY VIOLATION: Command contains forbidden pattern 'sudo'.")
    assert escalation.error_type == "security.violation"


def test_registry_exports_the_error_types_g8ee_records_as_a_deny_decision():
    exported = set(build_agent_tool_registry().policy_deny_error_types)

    assert exported == {error_type.value for error_type in POLICY_DENY_ERROR_TYPES}
    assert "security.violation" in exported
    assert {"approval.denied", "user.denied"}.isdisjoint(exported)


@pytest.mark.parametrize("error_type", sorted(CommandErrorType, key=lambda value: value.value))
def test_policy_outcome_is_deny_exactly_for_the_exported_error_types(error_type):
    result = CommandExecutionResult(success=False, error_type=error_type)

    expected = "deny" if error_type in POLICY_DENY_ERROR_TYPES else "refused"
    assert _policy_outcome_from_result(result) == expected


def test_check_mode_fails_when_the_committed_registry_is_stale(tmp_path):
    stale = tmp_path / "agent-tool-registry.json"
    stale.write_text("{}\n", encoding="utf-8")

    assert main(["--check", "--path", str(stale)]) == 1
    assert main(["--write", "--path", str(stale)]) == 0
    assert main(["--check", "--path", str(stale)]) == 0
