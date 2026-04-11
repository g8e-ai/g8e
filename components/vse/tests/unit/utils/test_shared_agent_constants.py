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
Contract test: VSE Agent-related enums must exactly match the canonical values 
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


class TestSharedModelJSONEnumsMatchVSE:
    """Verifies that shared model JSON enum values match VSE enum values."""

    def test_tribunal_outcome_enum_matches(self):
        """tribunal.json outcome enum must match CommandGenerationOutcome."""
        model = _load_model_json("tribunal.json")
        json_outcomes = model["voting_result"]["outcome"]["enum"]
        
        vse_outcomes = [e.value for e in CommandGenerationOutcome]
        assert set(json_outcomes) == set(vse_outcomes), (
            f"tribunal.json outcome enum {json_outcomes} does not match "
            f"CommandGenerationOutcome {vse_outcomes}"
        )

    def test_tribunal_member_enum_matches(self):
        """tribunal.json member enum must match TribunalMember."""
        model = _load_model_json("tribunal.json")
        json_members = model["voting_result"]["candidates"]["items"]["properties"]["member"]["enum"]
        
        vse_members = [e.value for e in TribunalMember]
        assert set(json_members) == set(vse_members), (
            f"tribunal.json member enum {json_members} does not match "
            f"TribunalMember {vse_members}"
        )

    def test_verifier_reason_enum_matches(self):
        """verifier.json reason_enum must match VerifierReason."""
        model = _load_model_json("verifier.json")
        json_reasons = model["result"]["reason_enum"]["enum"]
        
        vse_reasons = [e.value for e in VerifierReason]
        assert set(json_reasons) == set(vse_reasons), (
            f"verifier.json reason_enum {json_reasons} does not match "
            f"VerifierReason {vse_reasons}"
        )

    def test_triage_complexity_enum_matches(self):
        """triage.json complexity enum must match TriageComplexityClassification."""
        model = _load_model_json("triage.json")
        json_complexity = model["result"]["complexity"]["enum"]
        
        vse_complexity = [e.value for e in TriageComplexityClassification]
        assert set(json_complexity) == set(vse_complexity), (
            f"triage.json complexity enum {json_complexity} does not match "
            f"TriageComplexityClassification {vse_complexity}"
        )

    def test_triage_complexity_confidence_enum_matches(self):
        """triage.json complexity_confidence enum must match TriageConfidence."""
        model = _load_model_json("triage.json")
        json_confidence = model["result"]["complexity_confidence"]["enum"]
        
        vse_confidence = [e.value for e in TriageConfidence]
        assert set(json_confidence) == set(vse_confidence), (
            f"triage.json complexity_confidence enum {json_confidence} does not match "
            f"TriageConfidence {vse_confidence}"
        )

    def test_triage_intent_enum_matches(self):
        """triage.json intent enum must match TriageIntentClassification."""
        model = _load_model_json("triage.json")
        json_intent = model["result"]["intent"]["enum"]
        
        vse_intent = [e.value for e in TriageIntentClassification]
        assert set(json_intent) == set(vse_intent), (
            f"triage.json intent enum {json_intent} does not match "
            f"TriageIntentClassification {vse_intent}"
        )

    def test_triage_intent_confidence_enum_matches(self):
        """triage.json intent_confidence enum must match TriageConfidence."""
        model = _load_model_json("triage.json")
        json_confidence = model["result"]["intent_confidence"]["enum"]
        
        vse_confidence = [e.value for e in TriageConfidence]
        assert set(json_confidence) == set(vse_confidence), (
            f"triage.json intent_confidence enum {json_confidence} does not match "
            f"TriageConfidence {vse_confidence}"
        )
