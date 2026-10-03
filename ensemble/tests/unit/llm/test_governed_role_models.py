# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Roles served by governed inference take their model from the Inference Operator.

The Inference Operator rejects a model that differs from its binding for the
role (``ErrInferenceModelOverrideDenied``), so g8ee never sends a stored or
fallback model for a ``g8e`` role: it sends the Operator's ``role_bindings``.
"""

from __future__ import annotations

import base64
from unittest.mock import AsyncMock

import pytest

from app.constants import LLMProvider
from app.errors import ConfigurationError
from app.llm.governed_role_models import (
    GovernedRoleModelService,
    bind_governed_role_models,
    uses_governed_inference,
)
from app.llm.model_catalog import role_models_from_inventory
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eeUserSettings, LLMSettings
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    MODEL_ROLE_ASSISTANT,
    MODEL_ROLE_LITE,
    MODEL_ROLE_PRIMARY,
    InferenceRoleBinding,
    OllamaModelInventoryResult,
)

pytestmark = pytest.mark.unit

BOUND = {"primary": "gemma4:e4b", "assistant": "qwen3:1.7b", "lite": "qwen3.5:0.8b"}


def _inventory(bindings: dict[str, str]) -> OllamaModelInventoryResult:
    roles = {
        "primary": MODEL_ROLE_PRIMARY,
        "assistant": MODEL_ROLE_ASSISTANT,
        "lite": MODEL_ROLE_LITE,
    }
    return OllamaModelInventoryResult(
        status=EXECUTION_STATUS_COMPLETED,
        role_bindings=[
            InferenceRoleBinding(role=roles[role], served_model_tag=model)
            for role, model in bindings.items()
        ],
    )


def _operator_client(session_id: str, bindings: dict[str, str]) -> AsyncMock:
    client = AsyncMock()
    client.list.return_value = [
        {
            "status": "active",
            "operator_type": "remote",
            "operator_session_id": session_id,
            "runtime_config": {"inference_enabled": True},
        }
    ]
    client.dispatch.return_value = {
        "success": True,
        "result_payload": base64.b64encode(_inventory(bindings).SerializeToString()).decode(),
    }
    return client


def _context() -> G8eHttpContext:
    return G8eHttpContext(user_id="owner-1", web_session_id="ws-1")


def test_role_models_from_inventory_maps_each_binding():
    assert role_models_from_inventory(_inventory(BOUND)) == BOUND


def test_stored_models_for_g8e_roles_are_replaced_by_bindings():
    llm = LLMSettings(
        primary_provider=LLMProvider.G8E,
        assistant_provider=LLMProvider.G8E,
        lite_provider=LLMProvider.G8E,
        primary_model="gemma4:e4b",
        assistant_model="gemma4:e2b",
        lite_model="qwen3:0.6b",
    )
    bound = bind_governed_role_models(llm, BOUND)
    assert (bound.primary_model, bound.assistant_model, bound.lite_model) == (
        "gemma4:e4b",
        "qwen3:1.7b",
        "qwen3.5:0.8b",
    )
    assert bound.resolved_lite_model == "qwen3.5:0.8b"


def test_role_falling_back_to_g8e_gets_its_own_binding():
    """A Lite role with no provider falls back to Primary's g8e provider, but it
    is still dispatched as Lite, so it must carry the Lite binding."""
    llm = LLMSettings(primary_provider=LLMProvider.G8E, primary_model="stale")
    bound = bind_governed_role_models(llm, BOUND)
    for role in ("primary", "assistant", "lite"):
        provider, _, _, model = bound.resolve(role)
        assert provider == LLMProvider.G8E.value
        assert model == BOUND[role]


def test_direct_provider_roles_are_untouched():
    llm = LLMSettings(
        primary_provider=LLMProvider.G8E,
        assistant_provider=LLMProvider.OLLAMA,
        lite_provider=LLMProvider.OLLAMA,
        assistant_model="llama3.2:3b",
        lite_model="gemma4:e2b",
    )
    bound = bind_governed_role_models(llm, BOUND)
    assert bound.primary_model == "gemma4:e4b"
    assert bound.assistant_provider is LLMProvider.OLLAMA
    assert bound.assistant_model == "llama3.2:3b"
    assert bound.lite_model == "gemma4:e2b"


def test_g8e_role_without_binding_fails_closed():
    llm = LLMSettings(primary_provider=LLMProvider.G8E)
    with pytest.raises(ConfigurationError, match="assistant"):
        bind_governed_role_models(llm, {"primary": "gemma4:e4b", "lite": "qwen3.5:0.8b"})


def test_uses_governed_inference_follows_fallback_chain():
    assert uses_governed_inference(LLMSettings(primary_provider=LLMProvider.G8E))
    assert not uses_governed_inference(
        LLMSettings(primary_provider=LLMProvider.OLLAMA, primary_model="m")
    )


@pytest.mark.asyncio
async def test_bindings_are_requested_once_per_operator_session():
    client = _operator_client("inference-session-1", BOUND)
    service = GovernedRoleModelService(client)

    assert await service.role_models(_context()) == BOUND
    assert await service.role_models(_context()) == BOUND
    assert client.dispatch.await_count == 1

    client.list.return_value[0]["operator_session_id"] = "inference-session-2"
    await service.role_models(_context())
    assert client.dispatch.await_count == 2


@pytest.mark.asyncio
async def test_bind_skips_the_operator_when_no_role_uses_g8e():
    client = _operator_client("inference-session-1", BOUND)
    service = GovernedRoleModelService(client)
    settings = G8eeUserSettings(
        llm=LLMSettings(primary_provider=LLMProvider.OLLAMA, primary_model="llama3.2:3b")
    )

    assert await service.bind(settings, _context()) is settings
    client.list.assert_not_awaited()
    client.dispatch.assert_not_awaited()


@pytest.mark.asyncio
async def test_bind_applies_operator_bindings_to_user_settings():
    client = _operator_client("inference-session-1", BOUND)
    service = GovernedRoleModelService(client)
    settings = G8eeUserSettings(
        llm=LLMSettings(primary_provider=LLMProvider.G8E, lite_model="qwen3:0.6b")
    )

    bound = await service.bind(settings, _context())
    assert bound.llm.lite_model == "qwen3.5:0.8b"
    assert settings.llm.lite_model == "qwen3:0.6b"
