# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical inventory of LLM call sites reachable from POST /api/v1/chat."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

ModelRole = Literal["primary", "assistant", "lite"] | None


@dataclass(frozen=True, slots=True)
class ChatModelCallSite:
    """One production model invocation site in the chat pipeline."""

    call_site: str
    module: str
    agent_role: str
    model_role: ModelRole
    provider_method: str


CHAT_MODEL_CALL_SITES: tuple[ChatModelCallSite, ...] = (
    ChatModelCallSite(
        call_site="triage",
        module="app.services.ai.triage",
        agent_role="triage",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="active_agent_primary",
        module="app.services.ai.agent",
        agent_role="sage",
        model_role="primary",
        provider_method="generate_content_stream_scored_role",
    ),
    ChatModelCallSite(
        call_site="active_agent_assistant",
        module="app.services.ai.agent",
        agent_role="dash",
        model_role="assistant",
        provider_method="generate_content_stream_scored_role",
    ),
    ChatModelCallSite(
        call_site="active_agent_lite",
        module="app.services.ai.agent",
        agent_role="dash",
        model_role="lite",
        provider_method="generate_content_stream_scored_role",
    ),
    ChatModelCallSite(
        call_site="title_generation",
        module="app.services.ai.title_generator",
        agent_role="scribe",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="memory_codex",
        module="app.services.ai.memory_generation_service",
        agent_role="codex",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="tribunal_generation",
        module="app.services.ai.tribunal.stages.generation",
        agent_role="tribunal",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="tribunal_auditor",
        module="app.services.ai.auditor_service",
        agent_role="auditor",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="warden_command_risk",
        module="app.services.ai.response_analyzer",
        agent_role="warden_command_risk",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="warden_error_analysis",
        module="app.services.ai.response_analyzer",
        agent_role="warden_error_analysis",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="warden_file_operation",
        module="app.services.ai.response_analyzer",
        agent_role="warden_file_operation",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
    ChatModelCallSite(
        call_site="eval_judge",
        module="app.services.ai.eval_judge",
        agent_role="judge",
        model_role="lite",
        provider_method="generate_content_lite",
    ),
)

CALL_SITE_BY_NAME = {site.call_site: site for site in CHAT_MODEL_CALL_SITES}
