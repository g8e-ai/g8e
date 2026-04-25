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
    AuditorReason,
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
        expected = {"triage", "sage", "dash", "tribunal", "auditor", "scribe", "axiom", "concord", "variance", "pragma", "nemesis", "codex", "judge", "warden", "warden_command_risk", "warden_error", "warden_file_risk"}
        actual = set(_AGENTS["agent.metadata"].keys())
        assert actual == expected

class TestAuditorReason:
    """AuditorReason enum matches shared constants."""

    def test_auditor_reason_matches_shared_constants(self):
        json_vals = _AGENTS["tribunal.auditor_reason"]
        assert AuditorReason.OK.value == json_vals["ok"]
        assert AuditorReason.REVISED.value == json_vals["revised"]
        assert AuditorReason.EMPTY_RESPONSE.value == json_vals["empty_response"]
        assert AuditorReason.NO_VALID_REVISION.value == json_vals["no_valid_revision"]
        assert AuditorReason.AUDITOR_ERROR.value == json_vals["auditor_error"]


class TestAgentMetadataPersonaFields:
    """Verifies all agents have first-class persona fields."""

    REQUIRED_PERSONA_FIELDS = {"role", "model_tier", "tools", "identity", "purpose", "autonomy"}
    ALL_AGENT_KEYS = {"triage", "sage", "dash", "tribunal", "auditor", "scribe", "axiom", "concord", "variance", "pragma", "nemesis", "codex", "judge", "warden", "warden_command_risk", "warden_error", "warden_file_risk"}

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

    def test_model_tier_is_valid_value(self):
        """model_tier must be one of the three tiers resolve_model() understands.

        Pinning the valid set prevents typos like 'liter' or 'assistants' from
        silently slipping into agents.json. resolve_model() raises ValueError on
        unknown tiers, but agents.json values are declarative metadata that is
        not always passed through resolve_model() at runtime, so a typo would
        otherwise go undetected.
        """
        valid_tiers = {"primary", "assistant", "lite"}
        for key, agent in _AGENTS["agent.metadata"].items():
            tier = agent["model_tier"]
            assert tier in valid_tiers, (
                f"Agent '{key}' has invalid model_tier '{tier}'. "
                f"Must be one of {sorted(valid_tiers)}."
            )

    def test_model_tier_matches_runtime_routing(self):
        """Each agent's declared model_tier must match the tier its production
        code path actually requests from resolve_model().

        This catches drift between the persona declaration and the hardcoded
        runtime selection. If a runtime path changes its hardcoded tier (e.g.
        Tribunal members switch from 'lite' to 'assistant'), agents.json must
        be updated in lockstep so the declared persona reflects reality.

        Routing references:
        - triage:    chat_pipeline -> model_overrides.for_triage() -> lite
        - sage:      chat_pipeline COMPLEX -> 'primary'
        - dash:      chat_pipeline SIMPLE  -> 'assistant'
        - tribunal members (axiom/concord/variance/pragma/nemesis):
                     generator._resolve_model(tier='lite')
        - auditor:   generator._resolve_model(tier='primary')
        Other agents (scribe, codex, judge, warden*, tribunal coordinator)
        have their own paths or are coordinator metadata; they are checked
        only for tier validity, not lockstep alignment.
        """
        expected = {
            "triage": "lite",
            "sage": "primary",
            "dash": "assistant",
            "axiom": "lite",
            "concord": "lite",
            "variance": "lite",
            "pragma": "lite",
            "nemesis": "lite",
            "auditor": "primary",
        }
        metadata = _AGENTS["agent.metadata"]
        for agent_key, expected_tier in expected.items():
            actual_tier = metadata[agent_key]["model_tier"]
            assert actual_tier == expected_tier, (
                f"Agent '{agent_key}' declares model_tier='{actual_tier}' "
                f"but its runtime path requests tier='{expected_tier}'. "
                f"Update agents.json or the runtime hardcode in lockstep."
            )

    def test_tools_is_list(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["tools"], list), f"Agent '{key}' tools must be a list"

    def test_identity_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["identity"], str) and agent["identity"], f"Agent '{key}' identity must be a non-empty string"

    def test_purpose_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["purpose"], str) and agent["purpose"], f"Agent '{key}' purpose must be a non-empty string"

    def test_autonomy_is_nonempty_string(self):
        for key, agent in _AGENTS["agent.metadata"].items():
            assert isinstance(agent["autonomy"], str) and agent["autonomy"], f"Agent '{key}' autonomy must be a non-empty string"


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

    def test_auditor_reason_enum_matches(self):
        """auditor.json reason_enum must match AuditorReason."""
        model = _load_model_json("auditor.json")
        json_reasons = model["result"]["reason_enum"]["enum"]
        
        g8ee_reasons = [e.value for e in AuditorReason]
        assert set(json_reasons) == set(g8ee_reasons), (
            f"auditor.json reason_enum {json_reasons} does not match "
            f"AuditorReason {g8ee_reasons}"
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
