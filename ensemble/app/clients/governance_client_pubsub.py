# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
PubSubGovernanceClient - governance envelope submission using pubsub.

Replaces HTTP-based GovernanceClient with pubsub-based communication.
Envelope submissions are sent to g8eo via pubsub channels.
"""

import json
import logging
from typing import Any

from app.clients.pubsub_client import PubSubClient
from app.constants.channels import OperatorChannel
from app.errors import NetworkError, ValidationError
from app.models.settings import GatewaySettings
from app.services.infra.settings_service import SettingsService

logger = logging.getLogger(__name__)


class PubSubGovernanceClient:
    """Pubsub-based governance envelope submission client."""

    def __init__(
        self,
        pubsub_client: PubSubClient,
        operator_id: str,
        operator_session_id: str,
        gateway_settings: GatewaySettings | None = None,
    ) -> None:
        if gateway_settings is None:
            service = SettingsService()
            gateway_settings = GatewaySettings.from_bootstrap(service)

        self._pubsub_client = pubsub_client
        self._operator_id = operator_id
        self._operator_session_id = operator_session_id
        self._gateway_settings = gateway_settings

    async def _storage_request(
        self,
        operation: str,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        """Execute a governance request via pubsub."""
        try:
            response = await self._pubsub_client.publish_storage_request(
                operator_id=self._operator_id,
                operator_session_id=self._operator_session_id,
                storage_type=OperatorChannel.GOVERNANCE,
                operation=operation,
                payload=payload,
            )

            if response.get("success"):
                return response.get("data", {})
            error = response.get("error", "Unknown error")
            raise NetworkError(f"Governance operation failed: {error}", component="g8ee")
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"Governance request failed: {e}",
                component="g8ee",
                cause=e,
            ) from e

    async def submit_envelope(
        self,
        envelope: dict[str, Any],
    ) -> dict[str, Any]:
        """Submit a governance envelope to the Gateway via pubsub.

        Args:
            envelope: Governance envelope dictionary

        Returns:
            Signed ActionReceipt from the Gateway

        Raises:
            ValidationError: If envelope validation fails
            NetworkError: If submission fails
        """
        try:
            # Serialize the pre-built envelope dict to JSON for pubsub transport
            uap_envelope = json.dumps(envelope)

            payload = {
                "envelope": uap_envelope,
            }

            response = await self._storage_request("submit", payload)
            receipt = response.get("receipt")

            if not receipt:
                raise ValidationError(
                    "Gateway did not return a signed receipt",
                    component="g8ee",
                )

            return receipt
        except (ValidationError, NetworkError):
            raise
        except Exception as e:
            raise NetworkError(
                f"Envelope submission failed: {e}",
                component="g8ee",
                cause=e,
            ) from e
