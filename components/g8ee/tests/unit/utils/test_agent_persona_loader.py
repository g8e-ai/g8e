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

"""Unit tests for agent_persona_loader module."""

import pytest

from app.utils.agent_persona_loader import get_agent_persona, get_tribunal_member, list_all_agents, AgentPersona
from pydantic import ValidationError

pytestmark = pytest.mark.unit


class TestGetAgentPersona:
    """Tests for get_agent_persona function."""

    def test_get_valid_agent_persona(self):
        """Test retrieving a valid agent persona."""
        persona = get_agent_persona("triage")
        assert persona.agent_id == "triage"
        assert persona.display_name == "Triage"
        # "scan-eye" reflects Triage's sharpened role as gatekeeper / posture reader.
        assert persona.icon == "scan-eye"
        assert persona.role == "classifier"
        assert persona.model_tier == "primary"
        assert persona.tools == []
        assert persona.identity
        assert persona.purpose
        assert isinstance(persona.autonomy, str) and persona.autonomy

    def test_get_tribunal_member_persona(self):
        """Test retrieving Tribunal member personas."""
        axiom = get_tribunal_member("axiom")
        assert axiom.agent_id == "axiom"
        assert axiom.display_name == "Axiom"
        # Temperature is advisory-only; runtime uses model default_temperature.
        assert axiom.temperature is None

        concord = get_tribunal_member("concord")
        assert concord.agent_id == "concord"
        assert concord.temperature is None

        variance = get_tribunal_member("variance")
        assert variance.agent_id == "variance"
        assert variance.temperature is None

    def test_get_invalid_agent_raises_keyerror(self):
        """Test that requesting an invalid agent ID raises KeyError."""
        with pytest.raises(KeyError) as exc_info:
            get_agent_persona("nonexistent_agent")
        assert "nonexistent_agent" in str(exc_info.value)
        assert "not found in agents.json" in str(exc_info.value)

    def test_get_system_prompt_with_full_persona(self):
        """Test get_system_prompt returns persona when defined."""
        persona = get_agent_persona("triage")
        system_prompt = persona.get_system_prompt()
        # The new Triage persona positions itself as the gatekeeper and must
        # emit the request_posture field for downstream dissent calibration.
        assert "You are Triage, the gatekeeper of g8e" in system_prompt
        assert "request_posture" in system_prompt
        assert persona.persona in system_prompt

    def test_get_system_prompt_with_todo_persona(self):
        """Test get_system_prompt falls back to identity/purpose for TODO personas."""
        persona = get_agent_persona("primary")
        system_prompt = persona.get_system_prompt()
        # primary has TODO persona, so should fall back to identity/purpose
        assert "<identity>" in system_prompt
        assert "<purpose>" in system_prompt
        assert persona.identity in system_prompt
        assert persona.purpose in system_prompt

    def test_temperature_handling_null(self):
        """Test temperature handling for agents with null temperature."""
        persona = get_agent_persona("triage")
        assert persona.temperature is None

    def test_tools_is_list(self):
        """Test that tools field is always a list."""
        triage = get_agent_persona("triage")
        assert isinstance(triage.tools, list)
        
        primary = get_agent_persona("primary")
        assert isinstance(primary.tools, list)
        assert len(primary.tools) > 0


class TestPipelineTemplateContract:
    """Tribunal members and Verifier personas are consumed by command_generator
    as str.format() templates. If anyone removes the placeholders during a
    persona rewrite, the formatted prompt silently loses the user's intent,
    os/shell context, or the candidate command — a hard-to-debug correctness
    regression. These tests pin the contract explicitly.
    """

    def test_tribunal_member_personas_accept_generation_kwargs(self):
        """All three Tribunal members must format cleanly with the exact kwargs
        command_generator._run_generation_pass supplies."""
        kwargs = dict(
            forbidden_patterns_message="FORBIDDEN",
            command_constraints_message="CONSTRAINTS",
            intent="list processes",
            os="linux",
            shell="bash",
            user_context="root (uid=0)",
            working_directory="/home/user",
            original_command="ps aux",
        )
        for member_id in ("axiom", "concord", "variance"):
            persona = get_tribunal_member(member_id)
            formatted = persona.persona.format(**kwargs)
            # Every substituted value must appear in the formatted prompt —
            # this catches silent placeholder drift.
            for needle in ("FORBIDDEN", "CONSTRAINTS", "list processes", "linux", "bash", "ps aux"):
                assert needle in formatted, f"{member_id} persona dropped '{needle}'"

    def test_verifier_persona_accepts_verifier_kwargs_and_enforces_ok_contract(self):
        """Verifier persona must format with the caller's kwargs AND carry the
        terse 'ok / corrected-command' output contract that the pipeline parses."""
        persona = get_agent_persona("verifier")
        formatted = persona.get_system_prompt().format(
            forbidden_patterns_message="FORBIDDEN",
            command_constraints_message="CONSTRAINTS",
            intent="list files",
            os="linux",
            user_context="root (uid=0)",
            candidate_command="ls -la",
        )
        for needle in ("FORBIDDEN", "CONSTRAINTS", "list files", "linux", "ls -la"):
            assert needle in formatted
        # The pipeline parses `response.text.strip().lower() == "ok"` — the
        # persona must tell the model that contract, or revisions will read as
        # verdict labels that the parser can't match.
        assert "`ok`" in formatted
        assert "corrected command" in formatted


class TestSharpenedTribunalPersonas:
    """Guard rails for the sharpened Axiom / Concord / Variance voices. Each
    member must have a distinct worldview so the Tribunal's disagreement is
    ideological, not just statistical."""

    def test_axiom_is_the_minimalist(self):
        axiom = get_tribunal_member("axiom")
        assert "Minimalist" in axiom.persona
        assert axiom.temperature is None

    def test_concord_is_the_archivist(self):
        concord = get_tribunal_member("concord")
        assert "Archivist" in concord.persona
        assert concord.temperature is None

    def test_variance_is_the_adversary(self):
        variance = get_tribunal_member("variance")
        assert "Adversary" in variance.persona
        assert variance.temperature is None
        # Variance is the only member that should reference the adversarial
        # request_posture signal — that coupling is part of the design.
        assert "adversarial" in variance.persona.lower()


class TestListAllAgents:
    """Tests for list_all_agents function."""

    def test_list_all_agents_returns_all_ids(self):
        """Test that list_all_agents returns all agent IDs."""
        agents = list_all_agents()
        assert isinstance(agents, list)
        assert len(agents) > 0
        assert "triage" in agents
        assert "primary" in agents
        assert "assistant" in agents
        assert "tribunal" in agents
        assert "verifier" in agents
        assert "title_generator" in agents
        assert "axiom" in agents
        assert "concord" in agents
        assert "variance" in agents
        assert "memory_generator" in agents
        assert "eval_judge" in agents
        assert "response_analyzer" in agents

    def test_list_all_agents_includes_sub_agents(self):
        """Test that list_all_agents includes response_analyzer sub-agents."""
        agents = list_all_agents()
        assert "response_analyzer_command_risk" in agents
        assert "response_analyzer_error" in agents
        assert "response_analyzer_file_risk" in agents


class TestAgentPersonaValidation:
    """Tests for Pydantic validation of AgentPersona."""

    def test_valid_agent_data_passes_validation(self):
        """Test that valid agent data passes Pydantic validation."""
        valid_data = {
            "id": "test_agent",
            "display_name": "Test Agent",
            "icon": "test",
            "description": "A test agent",
            "role": "tester",
            "model_tier": "primary",
            "temperature": 0.5,
            "tools": ["test_tool"],
            "identity": "Test identity",
            "purpose": "Test purpose",
            "autonomy": "fully_autonomous",
            "persona": "Test persona"
        }
        persona = AgentPersona.model_validate(valid_data)
        assert persona.agent_id == "test_agent"
        assert persona.display_name == "Test Agent"

    def test_missing_required_field_fails_validation(self):
        """Test that missing required fields fail Pydantic validation."""
        invalid_data = {
            "id": "test_agent",
            "display_name": "Test Agent",
            # Missing required fields: icon, description, role, model_tier, etc.
        }
        with pytest.raises(ValidationError):
            AgentPersona.model_validate(invalid_data)

    def test_autonomy_accepts_freeform_directive_prose(self):
        """Autonomy is free-form empowering directive prose, not an enum."""
        data = {
            "id": "test_agent",
            "display_name": "Test Agent",
            "icon": "test",
            "description": "A test agent",
            "role": "tester",
            "model_tier": "primary",
            "temperature": None,
            "tools": [],
            "identity": "Test identity",
            "purpose": "Test purpose",
            "autonomy": "You operate at the maximum level of agency this seat permits.",
            "persona": ""
        }
        persona = AgentPersona.model_validate(data)
        assert isinstance(persona.autonomy, str) and persona.autonomy

    def test_temperature_accepts_null(self):
        """Test that temperature accepts None/null."""
        data = {
            "id": "test_agent",
            "display_name": "Test Agent",
            "icon": "test",
            "description": "A test agent",
            "role": "tester",
            "model_tier": "primary",
            "temperature": None,
            "tools": [],
            "identity": "Test identity",
            "purpose": "Test purpose",
            "autonomy": "fully_autonomous",
            "persona": ""
        }
        persona = AgentPersona.model_validate(data)
        assert persona.temperature is None

    def test_temperature_accepts_float(self):
        """Test that temperature accepts float values."""
        data = {
            "id": "test_agent",
            "display_name": "Test Agent",
            "icon": "test",
            "description": "A test agent",
            "role": "tester",
            "model_tier": "primary",
            "temperature": 0.7,
            "tools": [],
            "identity": "Test identity",
            "purpose": "Test purpose",
            "autonomy": "fully_autonomous",
            "persona": ""
        }
        persona = AgentPersona.model_validate(data)
        assert persona.temperature == 0.7
