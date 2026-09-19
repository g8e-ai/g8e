# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software
# is released under the Apache License, Version 2.0.

"""Deterministic identity and payload helpers for observe producer projections.

These pure helpers derive agent identity, display metadata, and routing
targets from typed domain objects. They never accept caller-supplied display
metadata when the persona registry owns it, and they never embed email,
session, or host data in agent IDs.
"""

from __future__ import annotations

from app.models.personas import PERSONA_REGISTRY, AgentPersonaModel
from app.models.personas import get_persona

# Single schema version accepted at the gateway producer boundary. Mirrors the
# Go constant internal/constants/paths.go:ObserveEventPayloadSchemaVersion.
OBSERVE_PRODUCER_SCHEMA_VERSION = "1.0.0"


class UnknownPersonaError(ValueError):
    """Raised when a persona id is not present in PERSONA_REGISTRY."""


def resolve_persona(persona_id: str) -> AgentPersonaModel:
    """Return the typed persona model for a registered persona id.

    Raises UnknownPersonaError if the id is not a key in PERSONA_REGISTRY.
    """
    if persona_id not in PERSONA_REGISTRY:
        raise UnknownPersonaError(
            f"persona id {persona_id!r} is not registered in PERSONA_REGISTRY"
        )
    return get_persona(persona_id)


def build_agent_id(user_id: str, persona_id: str) -> str:
    """Build the deterministic agent identity ``{user_id}:{persona_id}``.

    The persona id is validated against PERSONA_REGISTRY so an unknown
    persona never produces a fabricated agent identity. No email, session,
    or host data is embedded in the identity.
    """
    resolve_persona(persona_id)
    return f"{user_id}:{persona_id}"


def persona_display_name(persona_id: str) -> str:
    """Return the disclosure-safe display name owned by the persona registry."""
    return resolve_persona(persona_id).display_name


def persona_role(persona_id: str) -> str:
    """Return the role owned by the persona registry."""
    return resolve_persona(persona_id).role


def routing_target(
    web_session_id: str | None, cli_session_id: str | None
) -> tuple[str | None, str | None]:
    """Return the (web_session_id, cli_session_id) routing target pair.

    Exactly one of the two reaches the gateway. If neither is set, the
    producer call site skips the projection push (targetless skip), mirroring
    the SSE targetless-skip contract. This helper does not enforce mutual
    exclusivity; the gateway boundary validates that exactly one is set.
    """
    return web_session_id, cli_session_id
