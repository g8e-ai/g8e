# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for gateway posture observation.

Verifies that ``observe_gateway_posture`` queries the gateway health
endpoint and returns the observed posture, and that the CLI posture
observation logic never infers posture from the CLI argument alone for
governed arms.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.posture import observe_gateway_posture
from g8e_evals.schema import PostureObservation

pytestmark = pytest.mark.unit


class _StubResponse:
    def __init__(self, status_code: int, json_data: dict | None = None):
        self.status_code = status_code
        self._json = json_data or {}

    def json(self) -> dict:
        return self._json


class _StubAsyncClient:
    """Minimal async context manager that returns a canned GET response."""

    def __init__(self, response: _StubResponse):
        self._response = response

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        return False

    async def get(self, *args, **kwargs):
        return self._response


def _make_env() -> MagicMock:
    env = MagicMock()
    env.operator_url = "https://localhost:8443"
    env.auth_headers.return_value = {}
    env.make_async_client.return_value = _StubAsyncClient(
        _StubResponse(200, {"status": "ok", "posture": "consensus"})
    )
    return env


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_configured_posture():
    env = _make_env()
    posture = await observe_gateway_posture(env)
    assert posture == GovernancePosture.L2_CONSENSUS


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_none_on_http_error():
    env = _make_env()
    env.make_async_client.return_value = _StubAsyncClient(
        _StubResponse(503, {"error": "service initializing"})
    )
    posture = await observe_gateway_posture(env)
    assert posture is None


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_none_on_missing_field():
    env = _make_env()
    env.make_async_client.return_value = _StubAsyncClient(
        _StubResponse(200, {"status": "ok"})
    )
    posture = await observe_gateway_posture(env)
    assert posture is None


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_none_on_network_error():
    env = _make_env()

    class _ErrorClient:
        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def get(self, *args, **kwargs):
            raise httpx.ConnectError("connection refused")

    env.make_async_client.return_value = _ErrorClient()
    posture = await observe_gateway_posture(env)
    assert posture is None


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_doctrine_posture():
    env = _make_env()
    env.make_async_client.return_value = _StubAsyncClient(
        _StubResponse(200, {"status": "ok", "posture": "doctrine"})
    )
    posture = await observe_gateway_posture(env)
    assert posture == GovernancePosture.L1_DOCTRINE


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_notary_posture():
    env = _make_env()
    env.make_async_client.return_value = _StubAsyncClient(
        _StubResponse(200, {"status": "ok", "posture": "notary"})
    )
    posture = await observe_gateway_posture(env)
    assert posture == GovernancePosture.L3_NOTARY


@pytest.mark.asyncio
async def test_observe_gateway_posture_returns_none_on_unknown_posture():
    env = _make_env()
    env.make_async_client.return_value = _StubAsyncClient(
        _StubResponse(200, {"status": "ok", "posture": "ultra_secure"})
    )
    posture = await observe_gateway_posture(env)
    assert posture is None


def test_posture_observation_ungoverned_arm_uses_none():
    """Ungoverned arms record NONE as the observed posture without
    querying the gateway, because the task does not route through it."""
    obs = PostureObservation(
        requested_posture=GovernancePosture.NONE,
        observed_posture=GovernancePosture.NONE,
        observation_source="arm_definition",
        posture_match=True,
    )
    assert obs.observed_posture == GovernancePosture.NONE
    assert obs.posture_match is True


def test_posture_observation_governed_arm_records_observed_not_requested():
    """A governed arm records the independently observed posture from the
    gateway, not the requested posture from the CLI argument."""
    obs = PostureObservation(
        requested_posture=GovernancePosture.L2_CONSENSUS,
        observed_posture=GovernancePosture.L2_CONSENSUS,
        observation_source="gateway_health_endpoint",
        posture_match=True,
    )
    assert obs.observed_posture == GovernancePosture.L2_CONSENSUS
    assert obs.observation_source == "gateway_health_endpoint"


def test_posture_observation_mismatch_detected():
    """When the gateway posture differs from the requested posture,
    posture_match is False."""
    obs = PostureObservation(
        requested_posture=GovernancePosture.L2_CONSENSUS,
        observed_posture=GovernancePosture.L1_DOCTRINE,
        observation_source="gateway_health_endpoint",
        posture_match=False,
    )
    assert obs.posture_match is False


def test_posture_observation_unobserved_is_none():
    """When the gateway posture could not be observed, observed_posture
    is None and posture_match is None — never silently replaced with the
    requested posture."""
    obs = PostureObservation(
        requested_posture=GovernancePosture.L2_CONSENSUS,
        observed_posture=None,
        observation_source="gateway_health_endpoint",
        posture_match=None,
    )
    assert obs.observed_posture is None
    assert obs.posture_match is None
