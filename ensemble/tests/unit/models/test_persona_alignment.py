# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Persona alignment invariants per docs/architecture/ai_agents.md.

Pins the architectural contract:
- Triage is a classifier only; it does NOT emit clarifying questions
- Dash and Sage are reasoning agents; they DO have interrogation protocols
- When an agent emits questions, tool execution is deferred (enforced by gate in agent.py)
"""

import pytest

from app.models.personas.dash import DashPersona
from app.models.personas.sage import SagePersona
from app.models.personas.triage import TriagePersona

pytestmark = [pytest.mark.unit]


class TestTriagePersonaAlignment:
    """Triage must be classifier-only with no interrogation capability."""

    def test_triage_identity_has_no_interrogation_protocol(self):
        """Triage's identity must not contain <interrogation_protocol> block."""
        persona = TriagePersona()
        assert "<interrogation_protocol>" not in persona.identity, (
            "Triage is a classifier only; interrogation is handled by reasoning agents. "
            "The identity should not contain <interrogation_protocol>."
        )

    def test_triage_output_contract_explicitly_forbids_questions(self):
        """Triage's output_contract must explicitly state it does not emit questions."""
        persona = TriagePersona()
        assert "NO QUESTIONS" in persona.output_contract, (
            "Triage output_contract must explicitly forbid question emission."
        )
        assert "classifier only" in persona.output_contract.lower(), (
            "Triage output_contract must state it is a classifier only."
        )


class TestReasoningAgentPersonaAlignment:
    """Dash and Sage must have interrogation protocols for clarifying questions."""

    def test_dash_identity_has_interrogation_protocol(self):
        """Dash's identity must contain <interrogation_protocol> block."""
        persona = DashPersona()
        assert "<interrogation_protocol>" in persona.identity, (
            "Dash owns interrogation for simple turns; identity must include protocol."
        )

    def test_sage_identity_has_interrogation_protocol(self):
        """Sage's identity must contain <interrogation_protocol> block."""
        persona = SagePersona()
        assert "<interrogation_protocol>" in persona.identity, (
            "Sage owns interrogation for complex turns; identity must include protocol."
        )

    def test_interrogation_protocol_format_is_binary_yes_no(self):
        """Both reasoning agents must specify strictly binary YES/NO questions."""
        dash_persona = DashPersona()
        sage_persona = SagePersona()

        dash_protocol = dash_persona._get_interrogation_protocol()
        sage_protocol = sage_persona._get_interrogation_protocol()

        for agent_name, protocol in [("Dash", dash_protocol), ("Sage", sage_protocol)]:
            assert "strictly binary" in protocol.lower(), (
                f"{agent_name} interrogation protocol must specify strictly binary questions."
            )
            assert "YES/NO" in protocol, (
                f"{agent_name} interrogation protocol must specify YES/NO format."
            )
            assert "<interrogation>" in protocol, (
                f"{agent_name} interrogation protocol must show <interrogation> block format."
            )
