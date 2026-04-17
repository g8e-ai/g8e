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
- ``builder_attr`` / ``handler_attr`` -- method names on ``AIToolService`` that
  build the declaration and dispatch execution
- ``requires_web_search`` -- conditional registration gate for the
  ``g8e_web_search`` tool

Consumers that previously duplicated classification data
(``OPERATOR_TOOLS`` / ``AI_UNIVERSAL_TOOLS`` frozensets, ``get_tools``
partition branches, ``__init__`` registration blocks, ``_build_tool_handlers``
dispatch table) now all derive from this one tuple. Adding a new tool means
adding exactly one ``ToolSpec`` plus the corresponding ``_build_*`` /
``_handle_*`` methods.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from app.constants.prompts import AgentMode
from app.constants.settings import ToolDisplayCategory
from app.constants.status import OperatorToolName


class ToolScope(str, Enum):
    """Classification that controls the bound-operator auth gate and Tribunal routing."""
    UNIVERSAL = "universal"
    OPERATOR_GATED = "operator_gated"


_ALL_MODES: frozenset[AgentMode] = frozenset(AgentMode)
_BOUND_MODES: frozenset[AgentMode] = frozenset({
    AgentMode.OPERATOR_BOUND,
    AgentMode.CLOUD_OPERATOR_BOUND,
})


@dataclass(frozen=True)
class ToolSpec:
    name: OperatorToolName
    scope: ToolScope
    agent_modes: frozenset[AgentMode]
    builder_attr: str
    handler_attr: str
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
        builder_attr="_build_query_investigation_context_tool",
        handler_attr="_handle_query_investigation_context",
        display_label="Querying investigation",
        display_icon="database",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.GET_COMMAND_CONSTRAINTS,
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder_attr="_build_get_command_constraints_tool",
        handler_attr="_handle_get_command_constraints",
        display_label="Checking constraints",
        display_icon="shield-check",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.G8E_SEARCH_WEB,
        scope=ToolScope.UNIVERSAL,
        agent_modes=_ALL_MODES,
        builder_attr="_build_search_web_tool",
        handler_attr="_handle_search_web",
        requires_web_search=True,
        display_label="Searching the web",
        display_icon="search",
        display_category=ToolDisplayCategory.SEARCH,
    ),
    ToolSpec(
        name=OperatorToolName.RUN_COMMANDS,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_run_operator_commands_tool",
        handler_attr="_handle_run_commands",
        display_label="Executing command",
        display_icon="terminal",
        display_category=ToolDisplayCategory.EXECUTION,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_CREATE,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_file_create_tool",
        handler_attr="_handle_file_create",
        display_label="Creating file",
        display_icon="file-plus",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_WRITE,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_file_write_tool",
        handler_attr="_handle_file_write",
        display_label="Writing file",
        display_icon="file-edit",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_READ,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_file_read_tool",
        handler_attr="_handle_file_read",
        display_label="Reading file",
        display_icon="file-text",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FILE_UPDATE,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_file_update_tool",
        handler_attr="_handle_file_update",
        display_label="Updating file",
        display_icon="file-edit",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.LIST_FILES,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_list_directory_tool",
        handler_attr="_handle_list_files",
        display_label="Listing directory",
        display_icon="folder",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FETCH_FILE_HISTORY,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_fetch_file_history_tool",
        handler_attr="_handle_fetch_file_history",
        display_label="Fetching file history",
        display_icon="history",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.FETCH_FILE_DIFF,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_fetch_file_diff_tool",
        handler_attr="_handle_fetch_file_diff",
        display_label="Fetching file diff",
        display_icon="git-diff",
        display_category=ToolDisplayCategory.FILE,
    ),
    ToolSpec(
        name=OperatorToolName.GRANT_INTENT,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_grant_intent_permission_tool",
        handler_attr="_handle_grant_intent",
        display_label="Requesting permission",
        display_icon="shield",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.REVOKE_INTENT,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_revoke_intent_permission_tool",
        handler_attr="_handle_revoke_intent",
        display_label="Revoking permission",
        display_icon="shield-off",
        display_category=ToolDisplayCategory.GENERAL,
    ),
    ToolSpec(
        name=OperatorToolName.CHECK_PORT,
        scope=ToolScope.OPERATOR_GATED,
        agent_modes=_BOUND_MODES,
        builder_attr="_build_port_check_tool",
        handler_attr="_handle_port_check",
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
