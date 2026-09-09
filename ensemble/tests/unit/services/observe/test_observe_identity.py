# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software
# is released under the Apache License, Version 2.0.

"""Unit tests for observe producer identity helpers.

Verifies deterministic agent identity for every registered persona, two
users with the same persona producing distinct IDs, unknown persona
rejection, and that no email/session data appears in agent IDs.
"""

from __future__ import annotations

import pytest

from app.models.personas import PERSONA_REGISTRY
from app.services.observe.identity import (
    OBSERVE_PRODUCER_SCHEMA_VERSION,
    UnknownPersonaError,
    build_agent_id,
    persona_display_name,
    persona_role,
    resolve_persona,
    routing_target,
)


pytestmark = pytest.mark.unit


def test_build_agent_id_combines_user_id_and_persona_id_for_every_registered_persona():
    for persona_id in PERSONA_REGISTRY:
        agent_id = build_agent_id("user-1", persona_id)
        assert agent_id == f"user-1:{persona_id}"


def test_build_agent_id_distinct_for_two_users_with_same_persona():
    a = build_agent_id("user-a", "triage")
    b = build_agent_id("user-b", "triage")
    assert a != b
    assert a == "user-a:triage"
    assert b == "user-b:triage"


def test_build_agent_id_rejects_unknown_persona():
    with pytest.raises(UnknownPersonaError):
        build_agent_id("user-1", "nonexistent-persona")


def test_resolve_persona_returns_typed_model_for_registered_persona():
    persona = resolve_persona("triage")
    assert persona.id == "triage"
    assert persona.display_name
    assert persona.role


def test_resolve_persona_raises_for_unknown_persona():
    with pytest.raises(UnknownPersonaError):
        resolve_persona("nonexistent-persona")


def test_persona_display_name_returns_registry_owned_value():
    assert persona_display_name("triage") == resolve_persona("triage").display_name


def test_persona_role_returns_registry_owned_value():
    assert persona_role("triage") == resolve_persona("triage").role


def test_build_agent_id_contains_no_email_or_session_data():
    agent_id = build_agent_id("user-1", "triage")
    assert "@" not in agent_id
    assert "session" not in agent_id
    assert "web" not in agent_id
    assert "cli" not in agent_id


def test_routing_target_returns_provided_web_and_cli_ids():
    web, cli = routing_target("web-1", None)
    assert web == "web-1"
    assert cli is None

    web, cli = routing_target(None, "cli-1")
    assert web is None
    assert cli == "cli-1"


def test_routing_target_returns_none_pair_when_targetless():
    web, cli = routing_target(None, None)
    assert web is None
    assert cli is None


def test_observe_producer_schema_version_matches_gateway_constant():
    assert OBSERVE_PRODUCER_SCHEMA_VERSION == "1.0.0"
