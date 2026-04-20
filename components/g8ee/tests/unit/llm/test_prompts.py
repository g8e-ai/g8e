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

import pytest
from unittest.mock import patch

from app.constants import (
    CloudSubtype,
    EventType,
    OperatorType,
    PromptFile,
    PromptSection,
    Severity,
    Priority,
    InvestigationStatus,
)
from app.llm import prompts
from app.models.agent import OperatorContext
from app.models.investigations import EnrichedInvestigationContext, ConversationHistoryMessage, ConversationMessageMetadata
from app.models.memory import InvestigationMemory

@pytest.fixture
def mock_loader():
    with patch("app.llm.prompts.load_prompt") as mock_load, \
         patch("app.llm.prompts.load_mode_prompts") as mock_mode_load:
        mock_load.side_effect = lambda x: f"Content of {x}"
        mock_mode_load.return_value = {
            PromptSection.CAPABILITIES: "Capabilities prompt",
            PromptSection.EXECUTION: "Execution prompt",
            PromptSection.TOOLS: "Tools prompt",
        }
        yield mock_load, mock_mode_load

@pytest.fixture
def operator_context():
    return OperatorContext(
        operator_id="op_123",
        os="linux",
        hostname="test-host",
        username="g8e",
        working_directory="/home/g8e",
        operator_type=OperatorType.SYSTEM,
        is_container=True,
        container_runtime="docker",
        init_system="systemd"
    )

@pytest.fixture
def enriched_investigation():
    msg = ConversationHistoryMessage(
        sender=EventType.EVENT_SOURCE_USER_CHAT,
        content="Help me with my server",
        metadata=ConversationMessageMetadata()
    )
    return EnrichedInvestigationContext(
        case_id="case_123",
        case_title="Server issue",
        case_description="Server is slow",
        user_id="user_123",
        sentinel_mode=True,
        status=InvestigationStatus.OPEN,
        priority=Priority.HIGH,
        severity=Severity.HIGH,
        conversation_history=[msg]
    )

def test_build_investigation_context_section_empty():
    assert prompts.build_investigation_context_section(None) == ""

def test_build_investigation_context_section_full(enriched_investigation):
    section = prompts.build_investigation_context_section(enriched_investigation)
    assert "<investigation_context>" in section
    assert "Case: Server issue" in section
    assert "Description: Server is slow" in section
    assert "Conversation history is available via query_investigation_context." in section

def test_build_learned_context_section_empty():
    assert prompts.build_learned_context_section([], []) == ""

def test_build_learned_context_section_full():
    user_mem = [InvestigationMemory(
        case_id="c1", investigation_id="i1", user_id="u1", 
        status=InvestigationStatus.OPEN, case_title="T1",
        communication_preferences="Brief"
    )]
    case_mem = [InvestigationMemory(
        case_id="c2", investigation_id="i2", user_id="u2", 
        status=InvestigationStatus.CLOSED, case_title="T2",
        investigation_summary="Fixed CPU spike"
    )]
    section = prompts.build_learned_context_section(user_mem, case_mem)
    assert "<learned_context>" in section
    assert "- Communication: Brief" in section
    assert "- Previous investigation: Fixed CPU spike" in section

def test_build_modular_system_prompt_basic(mock_loader, operator_context, enriched_investigation):
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=operator_context,
        user_memories=[],
        case_memories=[],
        investigation=enriched_investigation
    )
    
    assert f"Content of {PromptFile.CORE_IDENTITY}" in prompt
    assert f"Content of {PromptFile.CORE_SAFETY}" in prompt
    # Loyal-friction doctrine: loyalty + dissent must load right after safety
    # for every Primary/Assistant system prompt. If these drop out, the agents
    # revert to sycophantic behavior.
    assert f"Content of {PromptFile.CORE_LOYALTY}" in prompt
    assert f"Content of {PromptFile.CORE_DISSENT}" in prompt
    assert "Capabilities prompt" in prompt
    assert "<system_context>" in prompt
    assert "Hostname: test-host" in prompt
    assert "Naming Conventions: Standard naming" in prompt
    assert "custom_field: Custom value" in prompt
    assert "<investigation_context>" in prompt


def test_build_modular_system_prompt_loyalty_and_dissent_order(mock_loader, operator_context):
    """The doctrine sections must appear after safety and before mode prompts
    so the agent reads them before any capability/tool guidance.
    """
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=operator_context,
        user_memories=[],
        case_memories=[],
        investigation=None,
    )
    safety_pos = prompt.index(f"Content of {PromptFile.CORE_SAFETY}")
    loyalty_pos = prompt.index(f"Content of {PromptFile.CORE_LOYALTY}")
    dissent_pos = prompt.index(f"Content of {PromptFile.CORE_DISSENT}")
    capabilities_pos = prompt.index("Capabilities prompt")

    assert safety_pos < loyalty_pos < dissent_pos < capabilities_pos, (
        "Expected order: safety → loyalty → dissent → capabilities"
    )


def test_build_triage_context_section_none_returns_empty():
    assert prompts.build_triage_context_section(None) == ""


def test_build_triage_context_section_renders_posture_and_intent():
    from app.constants import (
        TriageComplexityClassification,
        TriageConfidence,
        TriageIntentClassification,
        TriageRequestPosture,
    )
    from app.models.agents.triage import TriageResult

    result = TriageResult(
        complexity=TriageComplexityClassification.COMPLEX,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.ACTION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary="User wants to drop the users table",
        request_posture=TriageRequestPosture.ADVERSARIAL,
        posture_confidence=TriageConfidence.HIGH,
    )
    section = prompts.build_triage_context_section(result)
    assert "<triage_context>" in section
    assert "request_posture: adversarial" in section
    assert "intent_summary: User wants to drop the users table" in section
    assert section.endswith("</triage_context>\n")


def test_build_triage_context_section_includes_user_facing_intent_on_error():
    """When triage fails, intent_summary is included but contains only user-facing
    instructions, not internal error details. Internal error fields (error_code,
    error_class, error_message) are for logging only and never exposed to AI."""
    from app.constants import (
        TriageComplexityClassification,
        TriageConfidence,
        TriageIntentClassification,
        TriageRequestPosture,
    )
    from app.models.agents.triage import TriageResult

    result = TriageResult(
        complexity=TriageComplexityClassification.COMPLEX,
        complexity_confidence=TriageConfidence.LOW,
        intent=TriageIntentClassification.UNKNOWN,
        intent_confidence=TriageConfidence.LOW,
        intent_summary="Triage unavailable: classification failed. Escalating to full LLM for complexity classification. Check provider configuration and retry.",
        request_posture=TriageRequestPosture.NORMAL,
        posture_confidence=TriageConfidence.LOW,
        error_code="CLASSIFICATION_ERROR",
        error_class="Exception",
        error_message="Provider error details",
    )
    section = prompts.build_triage_context_section(result)
    assert "<triage_context>" in section
    assert "request_posture: normal" in section
    assert "intent_summary: Triage unavailable: classification failed." in section
    assert "CLASSIFICATION_ERROR" not in section
    assert "Provider error details" not in section
    assert section.endswith("</triage_context>\n")


def test_build_modular_system_prompt_injects_triage_context(mock_loader, operator_context):
    """When Triage supplies a posture, the modular prompt must include the
    <triage_context> block so the dissent protocol can calibrate on it."""
    from app.constants import (
        TriageComplexityClassification,
        TriageConfidence,
        TriageIntentClassification,
        TriageRequestPosture,
    )
    from app.models.agents.triage import TriageResult

    triage_result = TriageResult(
        complexity=TriageComplexityClassification.COMPLEX,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.ACTION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary="escalated request",
        request_posture=TriageRequestPosture.ESCALATED,
        posture_confidence=TriageConfidence.HIGH,
    )
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=operator_context,
        user_memories=[],
        case_memories=[],
        investigation=None,
        triage_result=triage_result,
    )
    assert "<triage_context>" in prompt
    assert "request_posture: escalated" in prompt

def test_build_modular_system_prompt_cloud_operator(mock_loader):
    cloud_context = OperatorContext(
        operator_id="op_cloud",
        operator_type=OperatorType.CLOUD,
        cloud_subtype="aws",
        granted_intents=["s3:ListBucket"],
        is_cloud_operator=True
    )
    
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=cloud_context,
        user_memories=[],
        case_memories=[],
        investigation=None
    )
    
    assert "Operator Type: Cloud Operator for AWS" in prompt
    assert "granted_intents: ['s3:ListBucket']" in prompt

def test_build_modular_system_prompt_g8ep_cloud_operator(mock_loader):
    g8ep_context = OperatorContext(
        operator_id="op_g8ep",
        operator_type=OperatorType.CLOUD,
        cloud_subtype=CloudSubtype.G8E_POD,
        is_cloud_operator=True
    )

    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=g8ep_context,
        user_memories=[],
        case_memories=[],
        investigation=None
    )

    assert "Operator Type: g8ep Cloud Operator - Direct system access via G8E_POD" in prompt
    assert "Least-privilege" not in prompt


def test_build_modular_system_prompt_cloud_operator_missing_subtype(mock_loader):
    no_subtype_context = OperatorContext(
        operator_id="op_no_subtype",
        operator_type=OperatorType.CLOUD,
        is_cloud_operator=True
    )

    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=no_subtype_context,
        user_memories=[],
        case_memories=[],
        investigation=None
    )

    assert "Operator Type: Cloud Operator - Least-privilege intent-based access" in prompt
    assert "AWS" not in prompt
    assert "G8E_POD" not in prompt


def test_build_modular_system_prompt_no_systemd(mock_loader):
    no_systemd_context = OperatorContext(
        operator_id="op_no_systemd",
        is_container=True,
        init_system="openrc"
    )
    
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=no_systemd_context,
        user_memories=[],
        case_memories=[],
        investigation=None
    )
    
    assert "WARNING: systemd is NOT available" in prompt

def test_build_modular_system_prompt_sentinel_mode(mock_loader, enriched_investigation):
    enriched_investigation.sentinel_mode = True
    prompt = prompts.build_modular_system_prompt(
        operator_bound=False,
        system_context=None,
        user_memories=[],
        case_memories=[],
        investigation=enriched_investigation
    )
    
    assert f"Content of {PromptFile.SYSTEM_SENTINEL_MODE}" in prompt

def test_build_modular_system_prompt_multi_operator(mock_loader):
    """Test that multiple operators are rendered with proper operator tags."""
    operator1 = OperatorContext(
        operator_id="op_123",
        os="linux",
        hostname="test-host-1",
        username="g8e",
        working_directory="/home/g8e",
        operator_type=OperatorType.SYSTEM,
        is_container=True,
        container_runtime="docker",
        init_system="systemd"
    )
    
    operator2 = OperatorContext(
        operator_id="op_456", 
        os="ubuntu",
        hostname="test-host-2",
        username="ubuntu",
        working_directory="/home/ubuntu",
        operator_type=OperatorType.SYSTEM,
        is_container=False,
        init_system="systemd"
    )
    
    operator3 = OperatorContext(
        operator_id="op_789",
        operator_type=OperatorType.CLOUD,
        cloud_subtype="aws",
        granted_intents=["s3:ListBucket", "ec2:DescribeInstances"],
        is_cloud_operator=True
    )
    
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=[operator1, operator2, operator3],
        user_memories=[],
        case_memories=[],
        investigation=None
    )
    
    # Should contain all three operators with proper tags
    assert "<system_context>" in prompt
    assert "<operator index=\"0\">" in prompt
    assert "<operator index=\"1\">" in prompt
    assert "<operator index=\"2\">" in prompt
    assert "</operator>" in prompt
    
    # Operator 1 details
    assert "Hostname: test-host-1" in prompt
    assert "OS: linux" in prompt
    assert "Container Environment: YES" in prompt
    
    # Operator 2 details  
    assert "Hostname: test-host-2" in prompt
    assert "OS: ubuntu" in prompt
    assert "User: ubuntu" in prompt
    # Should NOT have container environment for operator 2
    assert "Container Environment: YES" not in prompt.split("<operator index=\"1\">")[1].split("</operator>")[0]
    
    # Operator 3 details (cloud operator)
    assert "Operator Type: Cloud Operator for AWS" in prompt
    assert "granted_intents: ['s3:ListBucket', 'ec2:DescribeInstances']" in prompt

def test_build_modular_system_prompt_backward_compatibility(mock_loader, operator_context):
    """Test that single OperatorContext still works (backward compatibility)."""
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=operator_context,  # Single context, not list
        user_memories=[],
        case_memories=[],
        investigation=None
    )
    
    # Should work exactly like before - no operator tags for single operator
    assert "<system_context>" in prompt
    assert "<operator index=" not in prompt  # No operator tags for single
    assert "Hostname: test-host" in prompt
    assert "OS: linux" in prompt

def test_build_modular_system_prompt_mixed_cloud_operator_detection(mock_loader):
    """Test that is_cloud_operator is True when any operator is cloud-based."""
    system_operator = OperatorContext(
        operator_id="op_system",
        operator_type=OperatorType.SYSTEM,
        is_cloud_operator=False
    )
    
    cloud_operator = OperatorContext(
        operator_id="op_cloud",
        operator_type=OperatorType.CLOUD,
        is_cloud_operator=True
    )
    
    # Single system operator - should not be cloud mode
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=system_operator,
        user_memories=[],
        case_memories=[],
        investigation=None
    )
    # This test mainly ensures the function doesn't crash with mixed operators
    
    # Mixed operators - should detect cloud operator
    prompt = prompts.build_modular_system_prompt(
        operator_bound=True,
        system_context=[system_operator, cloud_operator],
        user_memories=[],
        case_memories=[],
        investigation=None
    )
    # Should contain both operators
    assert "<operator index=\"0\">" in prompt
    assert "<operator index=\"1\">" in prompt
    assert "Operator Type: Operator - Standard system access" in prompt
    assert "Operator Type: Cloud Operator" in prompt


# ---------------------------------------------------------------------------
# Operator-not-bound mode contract tests
#
# These tests deliberately do NOT mock load_mode_prompts. They assert that the
# real, rendered OPERATOR_NOT_BOUND prompt contains the anchor phrases that
# prevent the mode from contradicting itself (capabilities says "no tools",
# execution must acknowledge that before offering any command). Regression
# protection for the scenario_01_not_bound_command eval without depending on
# a probabilistic model judge.
# ---------------------------------------------------------------------------

@pytest.mark.parametrize("g8e_web_search_available", [True, False])
def test_operator_not_bound_prompt_requires_acknowledgement(g8e_web_search_available):
    prompt = prompts.build_modular_system_prompt(
        operator_bound=False,
        system_context=None,
        user_memories=[],
        case_memories=[],
        investigation=None,
        g8e_web_search_available=g8e_web_search_available,
    )

    # Capabilities half: agent must know it has no execution hands.
    assert "No Operator connected" in prompt
    assert "no hands on user systems" in prompt

    # Execution half: agent must acknowledge the not-bound state before
    # offering any command. This is the directive that keeps the mode
    # internally consistent and what scenario_01 validates.
    assert "No Operator is currently bound" in prompt
    assert "acknowledge the bind-state limitation before anything else" in prompt
    assert "Never skip the bind-state acknowledgement" in prompt


def test_operator_not_bound_prompt_does_not_contradict_capabilities():
    """Capabilities says no tools; execution must not contradict that by
    instructing the agent to just run a command."""
    prompt = prompts.build_modular_system_prompt(
        operator_bound=False,
        system_context=None,
        user_memories=[],
        case_memories=[],
        investigation=None,
        g8e_web_search_available=False,
    )

    # The failure mode we are guarding against: an execution-mode prompt that
    # tells the agent to "provide a command" without any acknowledgement.
    # The acknowledgement anchor must appear before any "command" guidance.
    ack_pos = prompt.find("No Operator is currently bound")
    assert ack_pos != -1, "acknowledgement directive missing from execution section"
