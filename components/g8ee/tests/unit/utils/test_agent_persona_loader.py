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

        concord = get_tribunal_member("concord")
        assert concord.agent_id == "concord"

        variance = get_tribunal_member("variance")
        assert variance.agent_id == "variance"

        pragma = get_tribunal_member("pragma")
        assert pragma.agent_id == "pragma"

        nemesis = get_tribunal_member("nemesis")
        assert nemesis.agent_id == "nemesis"

    def test_get_invalid_agent_raises_keyerror(self):
        """Test that requesting an invalid agent ID raises KeyError."""
        with pytest.raises(KeyError) as exc_info:
            get_agent_persona("nonexistent_agent")
        assert "nonexistent_agent" in str(exc_info.value)
        assert "not found in agents.json" in str(exc_info.value)

    def test_get_system_prompt_constructs_from_fields(self):
        """Test get_system_prompt constructs prompt from individual fields."""
        persona = get_agent_persona("triage")
        system_prompt = persona.get_system_prompt()
        assert "<role>" in system_prompt
        assert "<identity>" in system_prompt
        assert "<purpose>" in system_prompt
        assert "<autonomy>" in system_prompt
        assert persona.role in system_prompt
        assert persona.identity in system_prompt
        assert persona.purpose in system_prompt
        assert persona.autonomy in system_prompt

    def test_get_system_prompt_constructs_from_fields_stub(self):
        """Test get_system_prompt constructs prompt from individual fields for stub."""
        stub = AgentPersona.model_validate({
            "id": "stub",
            "display_name": "Stub",
            "icon": "x",
            "description": "stub",
            "role": "stub",
            "model_tier": "primary",
            "tools": [],
            "identity": "I am a stub.",
            "purpose": "To be replaced.",
            "autonomy": "none",
        })
        system_prompt = stub.get_system_prompt()
        assert "<role>" in system_prompt
        assert "<identity>" in system_prompt
        assert "<purpose>" in system_prompt
        assert "<autonomy>" in system_prompt
        assert stub.identity in system_prompt
        assert stub.purpose in system_prompt

    def test_sage_system_prompt_constructs_from_fields(self):
        """Sage's system prompt is constructed from individual fields."""
        sage = get_agent_persona("sage")
        prompt = sage.get_system_prompt()
        assert "<role>" in prompt
        assert "<identity>" in prompt
        assert "<purpose>" in prompt
        assert "<autonomy>" in prompt
        assert sage.identity in prompt

    def test_dash_system_prompt_is_fast_path_and_shares_sage_toolset(self):
        """Dash is the fast-path voice but keeps Sage's full tool set."""
        dash = get_agent_persona("dash")
        sage = get_agent_persona("sage")
        prompt = dash.get_system_prompt()
        assert "<role>" in prompt
        assert "<identity>" in prompt
        assert dash.identity in prompt
        assert set(dash.tools) == set(sage.tools)

    def test_tools_is_list(self):
        """Test that tools field is always a list."""
        triage = get_agent_persona("triage")
        assert isinstance(triage.tools, list)
        
        sage = get_agent_persona("sage")
        assert isinstance(sage.tools, list)
        assert len(sage.tools) > 0


class TestPipelineTemplateContract:
    """Tribunal members and Verifier personas are consumed by command_generator
    as str.format() templates. If anyone removes the placeholders during a
    persona rewrite, the formatted prompt silently loses the user's intent,
    os/shell context, or the candidate command — a hard-to-debug correctness
    regression. These tests pin the contract explicitly.
    """

    def test_tribunal_member_system_prompts_are_pure_voice(self):
        """Tribunal member system prompts must not carry scaffolding placeholders —
        scaffolding lives in TRIBUNAL_PROMPT_TEMPLATE in command_generator."""
        for member_id in ("axiom", "concord", "variance", "pragma", "nemesis"):
            prompt_text = get_tribunal_member(member_id).get_system_prompt()
            for placeholder in (
                "{forbidden_patterns_message}",
                "{command_constraints_message}",
                "{request}",
                "{guidelines}",
                "{operator_context}",
                "{os}",
                "{shell}",
                "{user_context}",
                "{working_directory}",
            ):
                assert placeholder not in prompt_text, (
                    f"{member_id} system prompt still carries scaffolding placeholder {placeholder}"
                )

    def test_tribunal_prompt_template_renders_with_member_voice(self):
        """The shared TRIBUNAL_PROMPT_TEMPLATE must render cleanly using the
        kwargs command_generator._run_generation_pass supplies."""
        from app.services.ai.command_generator import TRIBUNAL_PROMPT_TEMPLATE

        kwargs = dict(
            forbidden_patterns_message="FORBIDDEN",
            command_constraints_message="CONSTRAINTS",
            request="list processes",
            guidelines="",
            os="linux",
            shell="bash",
            user_context="root (uid=0)",
            working_directory="/home/user",
            operator_context="Hostname: host1\nOS: linux",
        )
        for member_id in ("axiom", "concord", "variance", "pragma", "nemesis"):
            formatted = TRIBUNAL_PROMPT_TEMPLATE.format(**kwargs)
            for needle in ("FORBIDDEN", "CONSTRAINTS", "list processes", "linux", "bash"):
                assert needle in formatted, f"{member_id}: template dropped '{needle}'"

    def test_verifier_template_renders_and_enforces_ok_contract(self):
        """TRIBUNAL_VERIFIER_TEMPLATE must carry the terse
        'ok / corrected-command' output contract that the pipeline parses."""
        from app.services.ai.command_generator import TRIBUNAL_VERIFIER_TEMPLATE

        formatted = TRIBUNAL_VERIFIER_TEMPLATE.format(
            forbidden_patterns_message="FORBIDDEN",
            command_constraints_message="CONSTRAINTS",
            request="list files",
            guidelines="",
            os="linux",
            shell="bash",
            working_directory="/home/user",
            user_context="root (uid=0)",
            operator_context="Hostname: host1\nOS: linux",
            verifier_context="<candidate_command>\nls -la\n</candidate_command>",
        )
        for needle in ("FORBIDDEN", "CONSTRAINTS", "list files", "linux", "ls -la"):
            assert needle in formatted
        # The pipeline parses JSON with status "ok" or "revised"
        assert "\"ok\"" in formatted
        assert "\"revised\"" in formatted


class TestSharpenedTribunalPersonas:
    """Guard rails for the sharpened Axiom / Concord / Variance / Pragma / Nemesis voices. Each
    member must have a distinct worldview so the Tribunal's disagreement is
    ideological, not just statistical."""

    def test_axiom_is_the_minimalist(self):
        axiom = get_tribunal_member("axiom")
        assert "The Composer" in axiom.description

    def test_concord_is_the_guardian(self):
        concord = get_tribunal_member("concord")
        assert "The Guardian" in concord.description

    def test_variance_is_the_exhaustive(self):
        variance = get_tribunal_member("variance")
        assert "The Exhaustive" in variance.description

    def test_pragma_is_the_conventional(self):
        pragma = get_tribunal_member("pragma")
        assert "The Conventional" in pragma.description

    def test_nemesis_is_the_adversary(self):
        nemesis = get_tribunal_member("nemesis")
        assert "The Adversary" in nemesis.description
        # Nemesis is the only member that should reference the adversarial
        # request_posture signal — that coupling is part of the design.
        assert "adversarial" in nemesis.identity.lower()


class TestListAllAgents:
    """Tests for list_all_agents function."""

    def test_list_all_agents_returns_all_ids(self):
        """Test that list_all_agents returns all agent IDs."""
        agents = list_all_agents()
        assert isinstance(agents, list)
        assert len(agents) > 0
        assert "triage" in agents
        assert "sage" in agents
        assert "dash" in agents
        assert "tribunal" in agents
        assert "auditor" in agents
        assert "scribe" in agents
        assert "axiom" in agents
        assert "concord" in agents
        assert "variance" in agents
        assert "pragma" in agents
        assert "nemesis" in agents
        assert "codex" in agents
        assert "judge" in agents
        assert "warden" in agents

    def test_list_all_agents_includes_sub_agents(self):
        """Test that list_all_agents includes warden sub-agents."""
        agents = list_all_agents()
        assert "warden_command_risk" in agents
        assert "warden_error" in agents
        assert "warden_file_risk" in agents


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
            "tools": ["test_tool"],
            "identity": "Test identity",
            "purpose": "Test purpose",
            "autonomy": "fully_autonomous"
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
            "tools": [],
            "identity": "Test identity",
            "purpose": "Test purpose",
            "autonomy": "You operate at the maximum level of agency this seat permits."
        }
        persona = AgentPersona.model_validate(data)
        assert isinstance(persona.autonomy, str) and persona.autonomy

