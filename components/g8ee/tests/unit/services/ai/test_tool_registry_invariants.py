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

"""Guards against tool-registry drift.

These tests protect against the class of bug where an ``OperatorToolName`` enum
value (or ``OPERATOR_TOOLS`` frozenset entry) exists without a corresponding
``ToolDeclaration`` registered in ``AIToolService``. That mismatch previously
misled documentation and approval-gate reasoning; ``_assert_tool_registry_invariants``
is the enforcement point and these tests cover it directly.
"""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants.status import OperatorToolName
from app.services.ai.tool_registry import (
    AI_UNIVERSAL_TOOLS,
    OPERATOR_TOOLS,
)
from app.errors import ConfigurationError
from app.services.ai.grounding.web_search_provider import WebSearchProvider
from app.services.ai.tool_service import AIToolService
from app.services.investigation.investigation_service import InvestigationService
from app.services.operator.command_service import OperatorCommandService

pytestmark = [pytest.mark.unit]


def _build_tool_service(web_search_provider: WebSearchProvider | None = None) -> AIToolService:
    return AIToolService(
        operator_command_service=MagicMock(spec=OperatorCommandService),
        investigation_service=AsyncMock(spec=InvestigationService),
        web_search_provider=web_search_provider,
    )


# Enum values that are intentionally NOT yet registered as ToolSpecs.
#
# These names exist on the g8eo wire (handlers, payload models, audit events)
# and in the ``OperatorToolName`` enum, but their AI-facing surface has been
# deleted and is pending restoration. The full-coverage invariant
# ``test_every_operator_tool_name_has_a_spec`` uses this set as its only
# allowlist — every entry here is a TODO for the restoration PR.
#
# **Do not add to this set without explicit, documented reason.** Removing
# an entry requires adding a ``ToolSpec`` in ``tool_registry.TOOL_SPECS``
# and the corresponding ``_build_*`` / ``_handle_*`` methods on
# ``AIToolService``. See ``docs/architecture/ai_agents.md``.
from app.constants.tool_registry_pending import PENDING_RESTORATION as _PENDING_RESTORATION


def test_pending_restoration_tools_are_not_in_operator_tools():
    """Pending-restoration names must not leak into OPERATOR_TOOLS.

    ``OPERATOR_TOOLS`` is consumed by ``execute_tool_call`` (bound-operator auth
    gate) and ``agent_tool_loop`` (Tribunal routing). Listing a pending name
    there without a ``ToolSpec`` would create a dead approval-gate entry that
    ``_assert_tool_registry_invariants`` already catches at startup — this test
    pins the contract and fails loudly if the invariant drifts.
    """
    assert _PENDING_RESTORATION.isdisjoint(OPERATOR_TOOLS)


def test_every_operator_tool_name_has_a_spec_or_is_pending():
    """Every ``OperatorToolName`` value MUST map to a ``ToolSpec`` or be allowlisted.

    This is the cross-surface invariant that would have prevented the original
    "dead tool" drift (four enum values, wire payloads, and g8eo handlers
    shipped with no g8ee ``ToolSpec`` to dispatch them). If a new enum value
    is added without a spec, this test fails and forces the author to either
    (a) wire the spec now or (b) add an explicit allowlist entry with a
    documented restoration owner.
    """
    from app.services.ai.tool_registry import TOOL_SPECS

    spec_names = {spec.name.value for spec in TOOL_SPECS}
    enum_names = {member.value for member in OperatorToolName}

    uncovered = enum_names - spec_names - _PENDING_RESTORATION
    assert not uncovered, (
        f"OperatorToolName values missing a ToolSpec and not in "
        f"_PENDING_RESTORATION: {sorted(uncovered)}. Either add a ToolSpec in "
        f"tool_registry.TOOL_SPECS or explicitly add the name to "
        f"_PENDING_RESTORATION with a restoration owner."
    )


def test_pending_restoration_allowlist_does_not_shadow_active_specs():
    """An entry in ``_PENDING_RESTORATION`` must genuinely have no ``ToolSpec``.

    Prevents an accidental allowlist entry from masking a real ToolSpec and
    hiding classification mistakes. The allowlist is strictly for *not-yet-
    shipped* tools; once restored, the entry must be removed.
    """
    from app.services.ai.tool_registry import TOOL_SPECS

    spec_names = {spec.name.value for spec in TOOL_SPECS}
    stale = _PENDING_RESTORATION & spec_names
    assert not stale, (
        f"_PENDING_RESTORATION contains names that DO have a ToolSpec: "
        f"{sorted(stale)}. Remove them from _PENDING_RESTORATION — they are "
        f"no longer pending."
    )


def test_operator_tools_and_universal_tools_are_disjoint():
    assert OPERATOR_TOOLS.isdisjoint(AI_UNIVERSAL_TOOLS)


def test_tool_service_init_registers_all_operator_tools():
    """Every OPERATOR_TOOLS entry must be registered as a ToolDeclaration."""
    svc = _build_tool_service(web_search_provider=MagicMock(spec=WebSearchProvider))
    declared = {
        name.value if isinstance(name, OperatorToolName) else str(name)
        for name in svc._tool_declarations.keys()
    }
    assert OPERATOR_TOOLS.issubset(declared)


def test_tool_service_init_registers_required_universal_tools():
    """Universal tools (except g8e_web_search, which is conditional) must always be registered."""
    svc = _build_tool_service(web_search_provider=None)
    declared = {
        name.value if isinstance(name, OperatorToolName) else str(name)
        for name in svc._tool_declarations.keys()
    }
    required = AI_UNIVERSAL_TOOLS - {OperatorToolName.G8E_SEARCH_WEB.value}
    assert required.issubset(declared)


def test_assertion_fails_when_operator_tools_contains_unregistered_name(monkeypatch):
    """If OPERATOR_TOOLS grows a name with no declaration, startup must fail loudly."""
    import app.services.ai.tool_service as tool_service_mod

    augmented = OPERATOR_TOOLS | {"phantom_tool_that_does_not_exist"}
    monkeypatch.setattr(tool_service_mod, "OPERATOR_TOOLS", augmented)

    with pytest.raises(ConfigurationError, match="phantom_tool_that_does_not_exist"):
        _build_tool_service(web_search_provider=None)


def test_assertion_fails_when_declaration_is_unclassified(monkeypatch):
    """A registered declaration not in OPERATOR_TOOLS or AI_UNIVERSAL_TOOLS must fail startup.

    This prevents silently shipping a tool whose approval-gate classification was
    forgotten (the precise failure mode that would bypass the bound-operator check
    in ``execute_tool_call``).
    """
    import app.services.ai.tool_service as tool_service_mod

    # Shrink OPERATOR_TOOLS so RUN_COMMANDS becomes unclassified.
    shrunken = OPERATOR_TOOLS - {OperatorToolName.RUN_COMMANDS.value}
    monkeypatch.setattr(tool_service_mod, "OPERATOR_TOOLS", shrunken)

    with pytest.raises(ConfigurationError, match="not classified"):
        _build_tool_service(web_search_provider=None)


def test_every_spec_has_non_empty_display_metadata():
    """Every ``ToolSpec`` must declare ``display_label``, ``display_icon``, and ``display_category``.

    Display metadata previously lived in a parallel ``_TOOL_DISPLAY_METADATA``
    dict in ``agent_tool_loop.py`` that had to be hand-kept in sync with
    ``TOOL_SPECS``. After folding onto ``ToolSpec``, this test guards against
    anyone re-introducing the drift by shipping a spec without a label/icon.
    """
    from app.services.ai.tool_registry import TOOL_SPECS

    for spec in TOOL_SPECS:
        assert spec.display_label, (
            f"ToolSpec {spec.name.value} has empty display_label"
        )
        assert spec.display_icon, (
            f"ToolSpec {spec.name.value} has empty display_icon"
        )
        assert spec.display_category, (
            f"ToolSpec {spec.name.value} has empty display_category"
        )


def test_tool_display_metadata_uses_tool_spec():
    """``tool_display_metadata`` must read from ``ToolSpec``, not a duplicated dict.

    Verifies the fold from ``_TOOL_DISPLAY_METADATA`` onto ``ToolSpec`` is
    wired end-to-end: the public lookup function returns the exact values
    declared on the spec for a known tool, and returns the documented
    fallback for an unknown one.
    """
    from app.services.ai.agent_tool_loop import tool_display_metadata
    from app.services.ai.tool_registry import get_tool_spec

    spec = get_tool_spec(OperatorToolName.RUN_COMMANDS)
    assert spec is not None
    label, icon, detail, category = tool_display_metadata(
        OperatorToolName.RUN_COMMANDS.value, "uname -a"
    )
    assert (label, icon, category) == (
        spec.display_label, spec.display_icon, spec.display_category,
    )
    assert detail == "uname -a"

    # Unknown tool names fall back to the generic display.
    label, icon, detail, category = tool_display_metadata("not_a_real_tool", "x")
    assert (label, icon) == ("Processing", "sync")
    assert detail == "x"


def test_every_spec_has_builder_and_handler_method():
    """Every ``ToolSpec`` must reference real ``_build_*`` / ``_handle_*`` methods on ``AIToolService``.

    Guards against typos in ``builder_attr`` / ``handler_attr`` that would otherwise surface
    only at service-construction time.
    """
    from app.services.ai.tool_registry import TOOL_SPECS

    for spec in TOOL_SPECS:
        assert hasattr(AIToolService, spec.builder_attr), (
            f"ToolSpec for {spec.name.value} references missing builder {spec.builder_attr}"
        )
        assert hasattr(AIToolService, spec.handler_attr), (
            f"ToolSpec for {spec.name.value} references missing handler {spec.handler_attr}"
        )


def test_operator_gated_specs_only_exposed_in_bound_modes():
    """OPERATOR_GATED tools must never be advertised in OPERATOR_NOT_BOUND.

    Exposing an operator-gated tool without a bound operator would let the LLM call it,
    only to have ``execute_tool_call`` reject it at dispatch time — a wasted turn and
    a confusing failure surface. The ``ToolScope.OPERATOR_GATED`` guard in
    ``tool_registry._validate_specs`` enforces this at import time; this test pins
    the contract for future readers.
    """
    from app.constants.prompts import AgentMode
    from app.services.ai.tool_registry import TOOL_SPECS, ToolScope

    for spec in TOOL_SPECS:
        if spec.scope is ToolScope.OPERATOR_GATED:
            assert AgentMode.OPERATOR_NOT_BOUND not in spec.agent_modes, (
                f"OPERATOR_GATED tool {spec.name.value} must not be exposed in OPERATOR_NOT_BOUND"
            )


def test_get_tools_partitions_by_agent_mode():
    """``get_tools`` must derive its tool set purely from ``TOOL_SPECS.agent_modes``."""
    from app.constants.prompts import AgentMode
    from app.services.ai.tool_registry import TOOL_SPECS, ToolScope

    svc = _build_tool_service(web_search_provider=MagicMock(spec=WebSearchProvider))

    not_bound = svc.get_tools(AgentMode.OPERATOR_NOT_BOUND, model_to_use=None)
    bound = svc.get_tools(AgentMode.OPERATOR_BOUND, model_to_use=None)

    not_bound_names = {t.name for group in not_bound for t in group.tools}
    bound_names = {t.name for group in bound for t in group.tools}

    # OPERATOR_NOT_BOUND exposes only UNIVERSAL tools.
    expected_not_bound = {
        spec.name.value for spec in TOOL_SPECS if spec.scope is ToolScope.UNIVERSAL
    }
    assert not_bound_names == expected_not_bound

    # OPERATOR_BOUND exposes everything.
    expected_bound = {spec.name.value for spec in TOOL_SPECS}
    assert bound_names == expected_bound
