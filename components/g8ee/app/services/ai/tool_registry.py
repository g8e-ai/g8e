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

"""Single-source declarative registry for AI tools.

Each ``ToolSpec`` entry in ``TOOL_SPECS`` carries everything the platform
needs to know about a tool:

- ``name`` -- the ``OperatorToolName`` enum value
- ``scope`` -- ``UNIVERSAL`` (no bound operator required) or ``OPERATOR_GATED``
  (bound-operator auth required; also the Tribunal routing set)
- ``agent_modes`` -- the set of ``AgentMode`` values in which the tool is
  exposed to the LLM
- ``builder`` / ``handler`` -- direct callable references into the per-tool
  modules under ``app.services.ai.tools``. ``builder()`` returns the
  ``ToolDeclaration`` registered with the LLM; ``handler(svc, ...)``
  dispatches execution. No string-based lookup; typos fail at import time.
- ``requires_web_search`` -- conditional registration gate for the
  ``g8e_web_search`` tool

Adding a new tool means creating one module under ``app/services/ai/tools/``
exporting ``build()`` and ``handle()`` callables, then adding exactly one
``ToolSpec`` entry referencing them.
"""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from enum import Enum

import app.llm.llm_types as types
from app.constants.prompts import AgentMode
from app.constants.settings import ToolDisplayCategory
from app.constants.status import OperatorToolName
from app.models.tool_results import ToolResult
from app.services.ai.tools import (
    check_port,
    fetch_file_diff,
    fetch_file_history,
    file_create,
    file_read,
    file_update,
    file_write,
    get_command_constraints,
    grant_intent,
    list_files,
    query_investigation_context,
    revoke_intent,
    run_commands,
    search_web,
    ssh_inventory,
    stream_operator,
)


class ToolScope(str, Enum):
    """Classification that controls the bound-operator auth gate and Tribunal routing."""
    UNIVERSAL = "universal"
    OPERATOR_GATED = "operator_gated"


_ALL_MODES: frozenset[AgentMode] = frozenset(AgentMode)
_BOUND_MODES: frozenset[AgentMode] = frozenset({
    AgentMode.OPERATOR_BOUND,
    AgentMode.CLOUD_OPERATOR_BOUND,
})


ToolBuilder = Callable[[], types.ToolDeclaration]
ToolHandler = Callable[..., Awaitable[ToolResult]]


@dataclass(frozen=True)
class ToolSpec:
    name: OperatorToolName
    scope: ToolScope
    agent_modes: frozenset[AgentMode]
    builder: ToolBuilder
    handler: ToolHandler
    display_label: str
    display_icon: str
    display_category: ToolDisplayCategory
    requires_web_search: bool = False


# Order here is the order tools are advertised to the LLM within a ToolGroup.
TOOL_SPECS: tuple[ToolSpec, ...] = (
    ToolSpec(
        name=OperatorToolName.QUERY_INVESTIGATION_CONTEXT,
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder=query_investigation_context.build,
        handler=query_investigation_context.handle,
        display_label="Querying investigation",
        display_icon="database",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.GET_COMMAND_CONSTRAINTS,
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder=get_command_constraints.build,
        handler=get_command_constraints.handle,
        display_label="Checking constraints",
        display_icon="shield-check",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.SSH_INVENTORY,
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder=ssh_inventory.build,
        handler=ssh_inventory.handle,
        display_label="Listing SSH inventory",
        display_icon="server",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.STREAM_OPERATOR,
        # must be ToolScope.UNIVERSAL despite being executor-shaped — the auth gate 
        # rejects any OPERATOR_TOOLS member when no operator is bound, and 
        # stream_operator is the whole point of running unbound.
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder=stream_operator.build,
        handler=stream_operator.handle,
        display_label="Streaming operator",
        display_icon="ship",
        display_category=ToolDisplayCategory.EXECUTION,
    ),
    ToolSpec(
        name=OperatorToolName.G8E_SEARCH_WEB,
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder=search_web.build,
        handler=search_web.handle,
        requires_web_search=True,
        display_label="Searching the web",
        display_icon="search",
        display_category=ToolDisplayCategory.SEARCH,
    ),
    ToolSpec(
        name=OperatorToolName.RUN_COMMANDS,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=run_commands.build,
        handler=run_commands.handle,
        display_label="Executing command",
        display_icon="terminal",
        display_category=ToolDisplayCategory.EXECUTION,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_CREATE,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=file_create.build,
        handler=file_create.handle,
        display_label="Creating file",
        display_icon="file-plus",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_WRITE,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=file_write.build,
        handler=file_write.handle,
        display_label="Writing file",
        display_icon="file-edit",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_READ,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=file_read.build,
        handler=file_read.handle,
        display_label="Reading file",
        display_icon="file-text",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_UPDATE,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=file_update.build,
        handler=file_update.handle,
        display_label="Updating file",
        display_icon="file-edit",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.LIST_FILES,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=list_files.build,
        handler=list_files.handle,
        display_label="Listing directory",
        display_icon="folder",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FETCH_FILE_HISTORY,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=fetch_file_history.build,
        handler=fetch_file_history.handle,
        display_label="Fetching file history",
        display_icon="history",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FETCH_FILE_DIFF,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=fetch_file_diff.build,
        handler=fetch_file_diff.handle,
        display_label="Fetching file diff",
        display_icon="git-diff",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.GRANT_INTENT,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=grant_intent.build,
        handler=grant_intent.handle,
        display_label="Requesting permission",
        display_icon="shield",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.REVOKE_INTENT,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=revoke_intent.build,
        handler=revoke_intent.handle,
        display_label="Revoking permission",
        display_icon="shield-off",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.CHECK_PORT,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder=check_port.build,
        handler=check_port.handle,
        display_label="Checking port",
        display_icon="network",
        display_category=ToolDisplayCategory.NETWORK,
    ),
)


def _validate_specs(specs: tuple[ToolSpec, ...]) -> None:
    """Fail loudly at import time on spec-table mistakes."""
    seen: set[str] = set()
    for spec in specs:
        if spec.name.value in seen:
            raise RuntimeError(f"Duplicate ToolSpec for {spec.name.value}")
        seen.add(spec.name.value)
        if spec.scope is ToolScope.OPERATOR_GATED and not spec.agent_modes.issubset(_BOUND_MODES):
            raise RuntimeError(
                f"OPERATOR_GATED tool {spec.name.value} may only be exposed in bound modes; "
                f"got {sorted(m.value for m in spec.agent_modes)}"
            )
        if not spec.agent_modes:
            raise RuntimeError(f"ToolSpec {spec.name.value} declares no agent_modes")
        if not callable(spec.builder):
            raise RuntimeError(f"ToolSpec {spec.name.value} has non-callable builder")
        if not callable(spec.handler):
            raise RuntimeError(f"ToolSpec {spec.name.value} has non-callable handler")


_validate_specs(TOOL_SPECS)


OPERATOR_TOOLS: frozenset[str] = frozenset(
    spec.name.value for spec in TOOL_SPECS if spec.scope is ToolScope.OPERATOR_GATED
)
"""Tools requiring a bound operator. Consumed by the auth gate in
``AIToolService.execute_tool_call`` and the Tribunal routing in
``agent_tool_loop``. Derived from ``TOOL_SPECS``.
"""


AI_UNIVERSAL_TOOLS: frozenset[str] = frozenset(
    spec.name.value for spec in TOOL_SPECS if spec.scope is ToolScope.UNIVERSAL
)
"""Tools available without a bound operator. Derived from ``TOOL_SPECS``."""


_BY_NAME: dict[str, ToolSpec] = {spec.name.value: spec for spec in TOOL_SPECS}


def get_tool_spec(name: str | OperatorToolName) -> ToolSpec | None:
    """Look up a ``ToolSpec`` by tool name (enum or string)."""
    key = name.value if isinstance(name, OperatorToolName) else name
    return _BY_NAME.get(key)
