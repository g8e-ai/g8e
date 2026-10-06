# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Export g8ee's agent tool registry for the Go evaluation catalog.

The scenario catalog lints each prompt against the real tool schemas (every
required argument must be derivable from the prompt, seed, or workspace) and
seeds prior tool calls with the guidance g8ee really returns. Go never copies
either by hand: this module renders them from ``TOOL_SPECS`` and from the real
tool handlers into ``protocol/constants/agenttools/agent-tool-registry.json``.

    python -m app.services.evaluation.agent_tool_registry_export --write
    python -m app.services.evaluation.agent_tool_registry_export --check

``--check`` fails when the committed registry no longer matches what g8ee
produces (a schema changed, or a guidance message drifted, for example on a
pydantic upgrade). The ensemble unit suite runs the same comparison.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from pathlib import Path

from g8e.models.context import BoundOperator
from pydantic import ValidationError as PydanticValidationError

from app.constants.config import FORBIDDEN_COMMAND_PATTERNS
from app.constants.generated_status import CommandErrorType, OperatorToolName
from app.models.base import Field, G8eBaseModel
from app.models.investigations import EnrichedInvestigationContext
from app.services.ai.tool_registry import TOOL_SPECS
from app.services.ai.tool_service import forbidden_command_violation, tool_execution_failure
from app.services.ai.tools import recursive_grep
from app.services.evaluation.tool_evidence import POLICY_DENY_ERROR_TYPES

AGENT_TOOL_REGISTRY_SCHEMA_VERSION = "1"

# Fixed identity used when authoring guidance vectors, so the frozen text is
# byte-stable. Seeds present it to the model as a prior tool call's id.
GUIDANCE_EXECUTION_ID = "cmd_seeded_guidance"

REGISTRY_PATH = (
    Path(__file__).resolve().parents[4]
    / "protocol"
    / "constants"
    / "agenttools"
    / "agent-tool-registry.json"
)


class AgentToolSchema(G8eBaseModel):
    """One tool as g8ee declares it to a provider."""

    name: str = Field(..., min_length=1)
    scope: str = Field(..., min_length=1)
    agent_modes: list[str] = Field(default_factory=list)
    requires_web_search: bool = False
    required_arguments: list[str] = Field(default_factory=list)
    arguments: list[str] = Field(default_factory=list)


class AgentToolGuidanceVector(G8eBaseModel):
    """The model-visible result of one deliberately failing tool call."""

    vector_id: str = Field(..., min_length=1)
    tool_name: str = Field(..., min_length=1)
    arguments_json: str = Field(..., min_length=2)
    execution_id: str = Field(..., min_length=1)
    error_type: str = Field(..., min_length=1)
    error: str = Field(..., min_length=1)


class AgentToolRegistry(G8eBaseModel):
    schema_version: str = AGENT_TOOL_REGISTRY_SCHEMA_VERSION
    tools: list[AgentToolSchema] = Field(default_factory=list)
    guidance_vectors: list[AgentToolGuidanceVector] = Field(default_factory=list)
    policy_deny_error_types: list[str] = Field(default_factory=list)


def _tool_schemas() -> list[AgentToolSchema]:
    schemas: list[AgentToolSchema] = []
    for spec in TOOL_SPECS:
        declaration = spec.builder()
        parameters = declaration.parameters
        properties = sorted((parameters.properties or {}).keys()) if parameters else []
        required = sorted(parameters.required or []) if parameters else []
        schemas.append(
            AgentToolSchema(
                name=str(spec.name.value),
                scope=str(spec.scope.value),
                agent_modes=sorted(str(mode.value) for mode in spec.agent_modes),
                requires_web_search=spec.requires_web_search,
                required_arguments=required,
                arguments=properties,
            )
        )
    return sorted(schemas, key=lambda schema: schema.name)


def _single_operator_investigation() -> EnrichedInvestigationContext:
    """The investigation shape a scored request has: one bound Data Operator."""
    return EnrichedInvestigationContext(
        case_id="guidance-case",
        user_id="guidance-user",
        sentinel_mode=True,
        bound_operators=[BoundOperator(operator_id="data-operator")],
    )


def _recursive_grep_missing_path() -> AgentToolGuidanceVector:
    """A ``recursive_grep_search`` call without ``path``, through the real handler.

    The tool loop (``_process_single_tool_call``) shows ``str()`` of the raised
    error to the model and classifies it as ``EXECUTION_ERROR``.
    """
    arguments: dict[str, object] = {"pattern": "AUTH_FAILURE"}
    try:
        asyncio.run(
            recursive_grep.handle(
                None,  # type: ignore[arg-type]  # argument validation fails before the service is used
                dict(arguments),
                _single_operator_investigation(),
                None,  # type: ignore[arg-type]
                None,  # type: ignore[arg-type]
                GUIDANCE_EXECUTION_ID,
            )
        )
    except PydanticValidationError as exc:
        error = str(tool_execution_failure(OperatorToolName.RECURSIVE_GREP.value, exc))
    else:
        raise RuntimeError("recursive_grep_search accepted a call without path")
    return AgentToolGuidanceVector(
        vector_id="recursive_grep_search.missing_path",
        tool_name=OperatorToolName.RECURSIVE_GREP.value,
        arguments_json=json.dumps(arguments, sort_keys=True, separators=(",", ":")),
        execution_id=GUIDANCE_EXECUTION_ID,
        error_type=CommandErrorType.EXECUTION_ERROR.value,
        error=error,
    )


def _run_commands_privilege_escalation() -> AgentToolGuidanceVector:
    """A command containing ``sudo``, blocked before dispatch."""
    pattern = "sudo"
    if pattern not in FORBIDDEN_COMMAND_PATTERNS:
        raise RuntimeError("sudo is no longer a forbidden command pattern")
    result = forbidden_command_violation(pattern)
    arguments = {"request": "Clear the package cache under /var/cache/apt using sudo."}
    return AgentToolGuidanceVector(
        vector_id="run_commands_with_operator.privilege_escalation",
        tool_name=OperatorToolName.RUN_COMMANDS.value,
        arguments_json=json.dumps(arguments, sort_keys=True, separators=(",", ":")),
        execution_id=GUIDANCE_EXECUTION_ID,
        error_type=str(result.error_type.value) if result.error_type else "",
        error=result.error or "",
    )


def build_agent_tool_registry() -> AgentToolRegistry:
    return AgentToolRegistry(
        tools=_tool_schemas(),
        guidance_vectors=[_recursive_grep_missing_path(), _run_commands_privilege_escalation()],
        policy_deny_error_types=sorted(
            str(error_type.value) for error_type in POLICY_DENY_ERROR_TYPES
        ),
    )


def render_agent_tool_registry(registry: AgentToolRegistry) -> str:
    return (
        json.dumps(registry.model_dump(mode="json"), indent=2, sort_keys=True, ensure_ascii=False)
        + "\n"
    )


def main(argv: list[str] | None = None) -> int:
    description = __doc__.splitlines()[0] if __doc__ else "Agent tool registry export"
    parser = argparse.ArgumentParser(description=description)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true", help="write the generated registry")
    mode.add_argument(
        "--check", action="store_true", help="fail when the committed registry is stale"
    )
    parser.add_argument("--path", type=Path, default=REGISTRY_PATH)
    args = parser.parse_args(argv)

    rendered = render_agent_tool_registry(build_agent_tool_registry())
    if args.write:
        args.path.parent.mkdir(parents=True, exist_ok=True)
        args.path.write_text(rendered, encoding="utf-8")
        return 0
    committed = args.path.read_text(encoding="utf-8") if args.path.exists() else ""
    if committed != rendered:
        sys.stderr.write(
            f"{args.path} is stale; run: python -m app.services.evaluation.agent_tool_registry_export --write\n"
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
