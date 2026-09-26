# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Pure helpers for constructing observe producer request payloads from
typed domain objects.

These helpers derive agent identity, display metadata, routing targets,
and run projections from the persona registry and authoritative domain
models. They return ``None`` when the persona cannot be resolved or the
routing target is targetless, so call sites can skip the projection push
without conditional branching.
"""

from __future__ import annotations

from app.constants import InvestigationStatus, ReasoningAgent
from app.models.internal_api import (
    ObserveProducerAgentStateRequest,
    ObserveProducerRunStateRequest,
)
from app.utils.time_ids.timestamp import now

from .identity import (
    OBSERVE_PRODUCER_SCHEMA_VERSION,
    UnknownPersonaError,
    build_agent_id,
    persona_display_name,
    persona_role,
)


def resolve_chat_persona_id(active_agent: ReasoningAgent | None) -> str | None:
    """Map a ``ReasoningAgent`` to its registered persona id.

    Returns ``None`` when ``active_agent`` is ``None`` or its value is not
    a key in ``PERSONA_REGISTRY``. The caller skips the agent projection
    push when this returns ``None``.
    """
    if active_agent is None:
        return None
    persona_id = active_agent.value if hasattr(active_agent, "value") else str(active_agent)
    try:
        from app.models.personas import PERSONA_REGISTRY

        if persona_id not in PERSONA_REGISTRY:
            return None
    except Exception:
        return None
    return persona_id


def build_agent_state_request(
    *,
    user_id: str,
    persona_id: str,
    status: str,
    run_id: str | None = None,
    model: str | None = None,
    web_session_id: str | None = None,
    cli_session_id: str | None = None,
) -> ObserveProducerAgentStateRequest | None:
    """Build a typed agent-state producer request from registry-owned metadata.

    Returns ``None`` when the persona id is not registered. Display name
    and role always come from the persona registry; caller-supplied values
    are never accepted. Returns ``None`` when both routing targets are
    ``None`` (targetless skip).
    """
    if not web_session_id and not cli_session_id:
        return None
    try:
        agent_id = build_agent_id(user_id, persona_id)
        display_name = persona_display_name(persona_id)
        role = persona_role(persona_id)
    except UnknownPersonaError:
        return None
    return ObserveProducerAgentStateRequest(
        schema_version=OBSERVE_PRODUCER_SCHEMA_VERSION,
        agent_id=agent_id,
        display_name=display_name,
        role=role,
        status=status,  # type: ignore[arg-type]
        run_id=run_id,
        task_id=None,
        model=model,
        observed_at=now(),
        web_session_id=web_session_id,
        cli_session_id=cli_session_id,
    )


def map_investigation_status_to_run_lifecycle(
    status: InvestigationStatus,
) -> str:
    """Map an authoritative ``InvestigationStatus`` to a ``RunLifecycleStatus``.

    Open/active work maps to ``running``. Closed/resolved maps to
    ``completed``. Escalated is not terminal and maps to ``running``.
    There is no explicit cancelled or failed investigation status in the
    domain model, so those mappings are handled by the caller when an
    explicit cancellation or failure is recorded.
    """
    if status in (InvestigationStatus.OPEN, InvestigationStatus.ESCALATED):
        return "running"
    if status in (InvestigationStatus.CLOSED, InvestigationStatus.RESOLVED):
        return "completed"
    return "running"


def build_investigation_run_state_request(
    *,
    run_id: str,
    display_name: str,
    status: str,
    user_id: str,
    web_session_id: str | None = None,
    cli_session_id: str | None = None,
    started_at=None,
    ended_at=None,
) -> ObserveProducerRunStateRequest | None:
    """Build a typed investigation run-state producer request.

    Returns ``None`` when both routing targets are ``None`` (targetless
    skip). Task counts are left at truthful zero defaults because no
    task document creation or lifecycle path is implemented in the
    current ensemble. The protocol (``protocol/models/task.json``)
    designates the ensemble as the authority for task documents submitted
    via GovernanceEnvelope, but no ensemble code creates task documents,
    emits ``APP_TASK_*`` events, or defines a task model or service. The
    ``tasks`` collection is read by ``CaseDataService.get_case_tasks`` but
    nothing writes to it. Wiring task projections requires a separate
    task lifecycle implementation and is recorded as unsupported here.
    The Gateway computes ``tasks_in_queue`` from these projection fields
    (``total_tasks - completed_tasks``), not from SSE event subtraction.
    """
    if not web_session_id and not cli_session_id:
        return None
    return ObserveProducerRunStateRequest(
        schema_version=OBSERVE_PRODUCER_SCHEMA_VERSION,
        run_id=run_id,
        run_kind="investigation",
        display_name=display_name,
        status=status,  # type: ignore[arg-type]
        active_task_id=None,
        completed_tasks=0,
        total_tasks=0,
        started_at=started_at,
        ended_at=ended_at,
        observed_at=now(),
        web_session_id=web_session_id,
        cli_session_id=cli_session_id,
    )
