# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Per-role models for governed inference come from the Inference Operator.

The Inference Operator authorizes a non-campaign request only when its model
equals the Operator's configured model for the request's role, and rejects
anything else with ``inference: model override not permitted by role
authority``. The Operator reports those bindings as ``role_bindings`` in its
typed model inventory. g8ee reads them and sets every role that runs through
the ``g8e`` provider, including a role that inherits ``g8e`` through the
Lite -> Assistant -> Primary fallback, to its bound model. A stored or
fallback model is never sent for a ``g8e`` role.

Bindings are fixed for an Operator session (they come from its startup
configuration), so they are cached per Inference Operator session ID. Request
overrides, such as a campaign's registry-bound model, are applied after this
step and still take precedence.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, get_args

from app.constants import LLMProvider
from app.errors import ConfigurationError
from app.llm.model_catalog import (
    inference_operator_session_id,
    request_governed_inventory,
    role_models_from_inventory,
)
from app.models.http_context import G8eHttpContext
from app.models.internal_api import LLMRole
from app.models.settings import G8eeUserSettings, LLMSettings

if TYPE_CHECKING:
    from app.clients.gateway_operator_client import GatewayOperatorClient

ROLES: tuple[LLMRole, ...] = get_args(LLMRole)


def _effective_provider(llm: LLMSettings, role: LLMRole) -> str | None:
    provider, _, _, _ = llm.resolve(role)
    return provider


def uses_governed_inference(llm: LLMSettings) -> bool:
    """Whether any role, after the fallback chain, runs through the g8e provider."""
    return any(_effective_provider(llm, role) == LLMProvider.G8E.value for role in ROLES)


def bind_governed_role_models(llm: LLMSettings, role_models: dict[LLMRole, str]) -> LLMSettings:
    """Return llm with every g8e role pinned to the g8e provider and its bound model.

    A g8e role the Operator serves no model for fails closed: the Operator would
    reject every request for that role.
    """
    updates: dict[str, object] = {}
    for role in ROLES:
        if _effective_provider(llm, role) != LLMProvider.G8E.value:
            continue
        model = role_models.get(role)
        if not model:
            raise ConfigurationError(
                f"The Inference Operator serves no model for the {role} role; "
                f"start it with --inference-{role}-model or choose another provider for {role}"
            )
        updates[f"{role}_provider"] = LLMProvider.G8E
        updates[f"{role}_model"] = model
    return llm.model_copy(update=updates) if updates else llm


class GovernedRoleModelService:
    """Reads the Inference Operator's role bindings and applies them to settings."""

    def __init__(self, operator_client: GatewayOperatorClient) -> None:
        self._operator_client = operator_client
        self._cached_session_id: str | None = None
        self._cached_role_models: dict[LLMRole, str] = {}

    async def role_models(self, context: G8eHttpContext) -> dict[LLMRole, str]:
        session_id = await inference_operator_session_id(self._operator_client, context.user_id)
        if session_id != self._cached_session_id:
            result = await request_governed_inventory(self._operator_client, context, session_id)
            self._cached_role_models = role_models_from_inventory(result)
            self._cached_session_id = session_id
        return dict(self._cached_role_models)

    async def bind(self, settings: G8eeUserSettings, context: G8eHttpContext) -> G8eeUserSettings:
        """Return settings with g8e roles bound; settings is returned as-is when no role uses g8e."""
        if not uses_governed_inference(settings.llm):
            return settings
        role_models = await self.role_models(context)
        return settings.model_copy(
            update={"llm": bind_governed_role_models(settings.llm, role_models)}
        )
