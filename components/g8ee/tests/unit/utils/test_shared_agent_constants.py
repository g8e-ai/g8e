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

"""
Contract test: g8ee Agent-related enums must exactly match the canonical values 
in shared/constants/agents.json and shared/models/agents/*.json.
"""

import json
import pytest
from pathlib import Path

from app.constants.shared import _AGENTS
from app.constants import (
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    CommandGenerationOutcome,
    TribunalMember,
    VerifierReason,
    TRIBUNAL_MEMBER_TEMPERATURES,
)

pytestmark = pytest.mark.unit

_SHARED_MODELS_DIR = Path(__file__).parent.parent.parent.parent.parent.parent / "shared" / "models" / "agents"

def _load_model_json(filename: str) -> dict:
    """Load a shared model JSON file."""
    path = _SHARED_MODELS_DIR / filename
    with open(path, "r") as f:
        return json.load(f)

class TestAgentConstantsMatchSharedJSON:
    """Verifies that Triage enums match shared/constants/agents.json."""

    def test_triage_complexity_matches(self):
        json_vals = _AGENTS["triage.complexity"]
        assert TriageComplexityClassification.SIMPLE.value == json_vals["simple"]
        assert TriageComplexityClassification.COMPLEX.value == json_vals["complex"]

    def test_triage_confidence_matches(self):
        json_vals = _AGENTS["triage.confidence"]
        assert TriageConfidence.HIGH.value == json_vals["high"]
        assert TriageConfidence.LOW.value == json_vals["low"]

    def test_triage_intent_matches(self):
        json_vals = _AGENTS["triage.intent"]
        assert TriageIntentClassification.INFORMATION.value == json_vals["information"]
        assert TriageIntentClassification.ACTION.value == json_vals["action"]
        assert TriageIntentClassification.UNKNOWN.value == json_vals["unknown"]

    def test_metadata_completeness(self):
        """Verify all expected agents have metadata in the shared constants."""
        expected = {"triage", "primary", "assistant", "tribunal", "verifier", "title_generator", "axiom", "concord", "variance"}
        actual = set(_AGENTS["agent.metadata"].keys())
        assert actual == expected

    def test_tribunal_temperatures_sourced_from_shared(self):
        """Verify TRIBUNAL_MEMBER_TEMPERATURES is sourced from shared constants."""
        shared_temps = _AGENTS["tribunal.temperatures"]
        # Verify values match shared constants
        assert TRIBUNAL_MEMBER_TEMPERATURES[TribunalMember.AXIOM] == shared_temps["axiom"]
        assert TRIBUNAL_MEMBER_TEMPERATURES[TribunalMember.CONCORD] == shared_temps["concord"]
        assert TRIBUNAL_MEMBER_TEMPERATURES[TribunalMember.VARIANCE] == shared_temps["variance"]
        # Verify the dict is properly typed
        assert isinstance(TRIBUNAL_MEMBER_TEMPERATURES, dict)
        assert all(isinstance(v, float) for v in TRIBUNAL_MEMBER_TEMPERATURES.values())


class TestAgentMetadataPersonaFields:
    """Verifies all agents have first-class persona fields."""

    REQUIRED_PERSONA_FIELDS = {"role", "model_tier", "temperature", "tools", "identity", "purpose", "autonomy"}
    VALID_AUTONOMY_VALUES = {"fully_autonomous", "human_approved"}
    ALL_AGENT_KEYS = {"triage", "primary", "assistant", "tribunal", "verifier", "title_generator", "axiom", "concord", "variance"}

    def test_all_agents_have_persona_fields(self):
        metadata = _AGENTS["agent.metadata"]
        for agent_key in self.ALL_AGENT_KEYS:
            agent = metadata[agent_key]
            missing = self.REQUIRED_PERSONA_FIELDS - set(agent.keys())
            assert not missing, f"Agent '{agent_key}' missing persona fields: {missing}"

    def test_role_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["role"], str) and agent["role"], f"Agent '{key}' role must be a non-empty string"

    def test_model_tier_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["model_tier"], str) and agent["model_tier"], f"Agent '{key}' model_tier must be a non-empty string"

    def test_temperature_is_null_or_number(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            temp = agent["temperature"]
            assert temp is None or isinstance(temp, (int, float)), f"Agent '{key}' temperature must be null or numeric, got {type(temp)}"

    def test_tools_is_list(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["tools"], list), f"Agent '{key}' tools must be a list"

    def test_identity_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["identity"], str) and agent["identity"], f"Agent '{key}' identity must be a non-empty string"

    def test_purpose_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["purpose"], str) and agent["purpose"], f"Agent '{key}' purpose must be a non-empty string"

    def test_autonomy_is_valid_value(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert agent["autonomy"] in self.VALID_AUTONOMY_VALUES, f"Agent '{key}' autonomy must be one of {self.VALID_AUTONOMY_VALUES}, got '{agent['autonomy']}'"


class TestSharedModelJSONEnumsMatchG8ee:
    """Verifies that shared model JSON enum values match g8ee enum values."""

    def test_tribunal_outcome_enum_matches(self):
        """tribunal.json outcome enum must match CommandGenerationOutcome."""
        model = _load_model_json("tribunal.json")
        json_outcomes = model["voting_result"]["outcome"]["enum"]
        
        g8ee_outcomes = [e.value for e in CommandGenerationOutcome]
        assert set(json_outcomes) == set(g8ee_outcomes), (
            f"tribunal.json outcome enum {json_outcomes} does not match "
            f"CommandGenerationOutcome {g8ee_outcomes}"
        )

    def test_tribunal_member_enum_matches(self):
        """tribunal.json member enum must match TribunalMember."""
        model = _load_model_json("tribunal.json")
        json_members = model["voting_result"]["candidates"]["items"]["properties"]["member"]["enum"]
        
        g8ee_members = [e.value for e in TribunalMember]
        assert set(json_members) == set(g8ee_members), (
            f"tribunal.json member enum {json_members} does not match "
            f"TribunalMember {g8ee_members}"
        )

    def test_verifier_reason_enum_matches(self):
        """verifier.json reason_enum must match VerifierReason."""
        model = _load_model_json("verifier.json")
        json_reasons = model["result"]["reason_enum"]["enum"]
        
        g8ee_reasons = [e.value for e in VerifierReason]
        assert set(json_reasons) == set(g8ee_reasons), (
            f"verifier.json reason_enum {json_reasons} does not match "
            f"VerifierReason {g8ee_reasons}"
        )

    def test_triage_complexity_enum_matches(self):
        """triage.json complexity enum must match TriageComplexityClassification."""
        model = _load_model_json("triage.json")
        json_complexity = model["result"]["complexity"]["enum"]
        
        g8ee_complexity = [e.value for e in TriageComplexityClassification]
        assert set(json_complexity) == set(g8ee_complexity), (
            f"triage.json complexity enum {json_complexity} does not match "
            f"TriageComplexityClassification {g8ee_complexity}"
        )

    def test_triage_complexity_confidence_enum_matches(self):
        """triage.json complexity_confidence enum must match TriageConfidence."""
        model = _load_model_json("triage.json")
        json_confidence = model["result"]["complexity_confidence"]["enum"]
        
        g8ee_confidence = [e.value for e in TriageConfidence]
        assert set(json_confidence) == set(g8ee_confidence), (
            f"triage.json complexity_confidence enum {json_confidence} does not match "
            f"TriageConfidence {g8ee_confidence}"
        )

    def test_triage_intent_enum_matches(self):
        """triage.json intent enum must match TriageIntentClassification."""
        model = _load_model_json("triage.json")
        json_intent = model["result"]["intent"]["enum"]
        
        g8ee_intent = [e.value for e in TriageIntentClassification]
        assert set(json_intent) == set(g8ee_intent), (
            f"triage.json intent enum {json_intent} does not match "
            f"TriageIntentClassification {g8ee_intent}"
        )

    def test_triage_intent_confidence_enum_matches(self):
        """triage.json intent_confidence enum must match TriageConfidence."""
        model = _load_model_json("triage.json")
        json_confidence = model["result"]["intent_confidence"]["enum"]
        
        g8ee_confidence = [e.value for e in TriageConfidence]
        assert set(json_confidence) == set(g8ee_confidence), (
            f"triage.json intent_confidence enum {json_confidence} does not match "
            f"TriageConfidence {g8ee_confidence}"
        )
