# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Cache-safety and size-budget contracts for the real (unmocked) system prompt.

The static prefix (everything before the agent persona) must be byte-identical
across turns and across Sage and Dash, so provider prompt caches keep hitting.
Section budgets keep the prompt diet from regressing (plan W4).
"""

from pathlib import Path

import pytest

from app.constants import (
    InvestigationStatus,
    OperatorType,
    Priority,
    PromptFile,
    PromptSection,
    ReasoningAgent,
    Severity,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.constants.message_sender import MessageSender
from app.llm import prompts
from app.models.agent import OperatorContext
from app.models.agents.triage import TriageResult
from app.models.investigations import (
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    EnrichedInvestigationContext,
)
from app.models.memory import InvestigationMemory
from app.utils.agent_persona_loader import get_agent_persona

# Character budgets per static section. A section may only exceed its budget
# with a recorded justification next to its entry.
SECTION_BUDGET_CHARS: dict[str, int] = {
    # Safety carries the forbidden-operations table and the masking rules, so
    # it exceeds the 1,800 line from the plan; the static prefix budget below
    # is the binding constraint.
    PromptSection.SAFETY: 2_250,
    PromptSection.LOYALTY: 2_400,
    # Dissent is the single owner of the warning and denial protocol; the
    # rules removed from loyalty live here. Loyalty + dissent together stay
    # under the 3,100 combined budget.
    PromptSection.DISSENT: 1_500,
    PromptSection.CAPABILITIES: 1_900,
    PromptSection.EXECUTION: 1_700,
    PromptSection.TOOLS: 950,
    PromptSection.RESPONSE_CONSTRAINTS: 800,
}
_TOOLS_DIR = Path(prompts.__file__).resolve().parents[1] / "prompts_data" / "tools"
LOYALTY_PLUS_DISSENT_BUDGET_CHARS = 3_100
STATIC_PREFIX_BUDGET_CHARS = 10_000
SENTINEL_MODE_BUDGET_CHARS = 1_100
DASH_PERSONA_BUDGET_CHARS = 1_800
TOOL_DESCRIPTIONS_BUDGET_CHARS = 15_000


def _operator(hostname: str) -> OperatorContext:
    return OperatorContext(
        operator_id=f"op_{hostname}",
        os="linux",
        hostname=hostname,
        username="g8e",
        working_directory="/home/g8e",
        operator_type=OperatorType.REMOTE,
        is_container=True,
        container_runtime="docker",
        init_system="systemd",
    )


def _investigation(case_id: str, title: str) -> EnrichedInvestigationContext:
    message = ConversationHistoryMessage(
        sender=MessageSender.USER_CHAT,
        content=f"message for {case_id}",
        metadata=ConversationMessageMetadata(),
        prev_hash="0" * 64,
        entry_hash="0" * 64,
    )
    return EnrichedInvestigationContext(
        case_id=case_id,
        case_title=title,
        case_description=f"description {case_id}",
        user_id=f"user_{case_id}",
        sentinel_mode=True,
        status=InvestigationStatus.OPEN,
        priority=Priority.HIGH,
        severity=Severity.HIGH,
        conversation_history=[message],
    )


def _triage(posture: TriageRequestPosture, summary: str) -> TriageResult:
    return TriageResult(
        complexity=TriageComplexityClassification.SIMPLE,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.ACTION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary=summary,
        request_posture=posture,
    )


def _memory(case_id: str, summary: str) -> InvestigationMemory:
    return InvestigationMemory(
        case_id=case_id,
        investigation_id=f"inv_{case_id}",
        user_id=f"user_{case_id}",
        status=InvestigationStatus.CLOSED,
        case_title=f"title {case_id}",
        investigation_summary=summary,
        communication_preferences=f"prefs {case_id}",
    )


def _build(agent: ReasoningAgent, *, variant: int, operator_bound: bool = True) -> str:
    prompt, _ = prompts.build_modular_system_prompt(
        operator_bound=operator_bound,
        system_context=_operator(f"host-{variant}"),
        user_memories=[_memory(f"u{variant}", f"user summary {variant}")],
        case_memories=[_memory(f"c{variant}", f"case summary {variant}")],
        investigation=_investigation(f"case_{variant}", f"Title {variant}"),
        triage_result=_triage(
            TriageRequestPosture.ESCALATED if variant % 2 else TriageRequestPosture.NORMAL,
            f"intent {variant}",
        ),
        agent_name=agent,
    )
    return prompt


def _static_prefix(prompt: str, agent: ReasoningAgent) -> str:
    persona = get_agent_persona(agent.value).get_system_prompt()
    assert persona, f"{agent.value} persona is empty"
    return prompt[: prompt.index(persona)]


@pytest.mark.parametrize("operator_bound", [True, False])
@pytest.mark.parametrize("agent", [ReasoningAgent.SAGE, ReasoningAgent.DASH])
def test_static_prefix_is_byte_identical_across_dynamic_context(agent, operator_bound):
    first = _build(agent, variant=1, operator_bound=operator_bound)
    second = _build(agent, variant=2, operator_bound=operator_bound)

    assert first != second, "dynamic context must change the full prompt"
    assert _static_prefix(first, agent) == _static_prefix(second, agent)


@pytest.mark.parametrize("operator_bound", [True, False])
def test_sage_and_dash_share_the_static_prefix_up_to_the_persona(operator_bound):
    sage = _build(ReasoningAgent.SAGE, variant=1, operator_bound=operator_bound)
    dash = _build(ReasoningAgent.DASH, variant=1, operator_bound=operator_bound)

    assert _static_prefix(sage, ReasoningAgent.SAGE) == _static_prefix(dash, ReasoningAgent.DASH)


@pytest.mark.parametrize("agent", [ReasoningAgent.SAGE, ReasoningAgent.DASH])
def test_static_prefix_carries_no_per_turn_identifiers(agent):
    prefix = _static_prefix(_build(agent, variant=7), agent)

    for per_turn in ("host-7", "case_7", "op_host-7", "user_c7", "intent 7"):
        assert per_turn not in prefix


def test_static_sections_stay_within_budget():
    _, sizes = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=None,
        user_memories=[],
        case_memories=[],
        investigation=None,
        agent_name=ReasoningAgent.DASH,
    )

    for section, budget in SECTION_BUDGET_CHARS.items():
        assert sizes[section] <= budget, (
            f"{section} is {sizes[section]} chars, budget {budget}"
        )
    combined = sizes[PromptSection.LOYALTY] + sizes[PromptSection.DISSENT]
    assert combined <= LOYALTY_PLUS_DISSENT_BUDGET_CHARS
    assert sizes[PromptSection.AGENT_PERSONA] <= DASH_PERSONA_BUDGET_CHARS


def test_static_prefix_stays_within_budget():
    prompt = _build(ReasoningAgent.DASH, variant=1)
    prefix = _static_prefix(prompt, ReasoningAgent.DASH)

    assert len(prefix) <= STATIC_PREFIX_BUDGET_CHARS, (
        f"static prefix is {len(prefix)} chars, budget {STATIC_PREFIX_BUDGET_CHARS}"
    )


def test_sentinel_mode_section_stays_within_budget():
    text = prompts.load_prompt(PromptFile.SYSTEM_SENTINEL_MODE)

    assert len(text) <= SENTINEL_MODE_BUDGET_CHARS


def test_tool_descriptions_stay_within_budget():
    total = sum(len(path.read_text(encoding="utf-8")) for path in _TOOLS_DIR.glob("*.txt"))

    assert total <= TOOL_DESCRIPTIONS_BUDGET_CHARS, (
        f"tool descriptions total {total} chars, budget {TOOL_DESCRIPTIONS_BUDGET_CHARS}"
    )
