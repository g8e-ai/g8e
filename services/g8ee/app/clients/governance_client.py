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
GovernanceClient - Engine client for Gateway governance envelope submission and receipt verification.

This client provides a typed interface for Engine services to submit mutations through the
Gateway's fail-closed governance gate (POST /api/governance/envelope) and verify signed
ActionReceipts returned by the Gateway.

See: .local.dev/docs/plans/engine_gateway_secure_link.md
"""

from __future__ import annotations

import logging
import json
from typing import Any

import aiohttp

from app.utils.envelope_builder import build_uap_envelope_json
from app.models.pubsub_messages import G8eMessage
from app.models.settings import GatewaySettings, TLSConfig
from app.services.infra.settings_service import SettingsService
from app.constants import AUTHORIZATION
from app.errors import NetworkError, ValidationError, ErrorCode
from app.utils.aiohttp_session import create_component_http_session

logger = logging.getLogger(__name__)


class GovernanceClient:
    """Client for submitting governance envelopes and verifying receipts.

    This client wraps the Gateway's POST /api/governance/envelope endpoint,
    which enforces L1/L2/L3 verification before executing mutations and returns
    a signed ActionReceipt as proof of execution.
    """

    def __init__(
        self,
        tls_config: TLSConfig | None = None,
        operator_session_id: str | None = None,
        gateway_settings: GatewaySettings | None = None,
    ) -> None:
        if gateway_settings is None:
            service = SettingsService()
            gateway_settings = GatewaySettings.from_bootstrap(service)

        self._base_url = gateway_settings.http_url

        if tls_config is not None:
            self._ca_cert_path = tls_config.ca_cert_path
            self._client_cert_path = tls_config.client_cert_path
            self._client_key_path = tls_config.client_key_path
        else:
            self._ca_cert_path = None
            self._client_cert_path = None
            self._client_key_path = None

        self._operator_session_id = operator_session_id
        self._session: aiohttp.ClientSession | None = None

    async def connect(self) -> bool:
        """Verify connectivity to the Gateway governance endpoint."""
        try:
            session = await self._get_http_session()
            async with session.get(f"{self._base_url}/health") as resp:
                if resp.status == 200:
                    logger.info("[GOVERNANCE-CLIENT] Connected to %s", self._base_url)
                    return True
                return False
        except Exception as e:
            logger.error("[GOVERNANCE-CLIENT] Connection failed: %s", e)
            return False

    async def _get_http_session(self) -> aiohttp.ClientSession:
        headers = {}
        if self._operator_session_id:
            headers[AUTHORIZATION] = f"Bearer {self._operator_session_id}"

        if not hasattr(self, "_session") or self._session is None:
            self._session = create_component_http_session(
                None,
                timeout=aiohttp.ClientTimeout(total=30),
                ca_cert_path=self._ca_cert_path,
                client_cert_path=self._client_cert_path,
                client_key_path=self._client_key_path,
                headers=headers,
            )
        return self._session

    async def submit_envelope(
        self,
        message: G8eMessage,
        *,
        agent_ids: list[str] | None = None,
        state_merkle_root: str = "",
    ) -> dict[str, Any]:
        """Submit a governance envelope to the Gateway for verification and execution.

        Args:
            message: The G8eMessage to wrap in a governance envelope
            agent_ids: Optional list of Tribunal agent IDs for L2 metadata
            state_merkle_root: Current state Merkle root for replay protection

        Returns:
            The signed ActionReceipt from the Gateway

        Raises:
            NetworkError: If the HTTP request fails
            ValidationError: If the envelope is rejected by governance gates
        """
        # Build the envelope using envelope_builder (no L2 signing - Gateway handles it)
        envelope_json = build_uap_envelope_json(
            message,
            agent_ids=agent_ids,
            state_merkle_root=state_merkle_root,
        )

        session = await self._get_http_session()
        url = f"{self._base_url}/api/governance/envelope"

        try:
            async with session.post(url, data=envelope_json) as resp:
                text = await resp.text()

                if resp.status == 403:
                    # Governance verification failed (L1/L2/L3 gates)
                    raise ValidationError(
                        f"Governance verification failed: {text}",
                        component="g8ee",
                        code=ErrorCode.GOVERNANCE_REJECTED,
                    )
                if resp.status == 400:
                    # Malformed envelope
                    raise ValidationError(
                        f"Invalid envelope: {text}",
                        component="g8ee",
                        code=ErrorCode.INVALID_INPUT,
                    )
                if resp.status == 503:
                    # Gateway not ready
                    raise NetworkError(
                        f"Gateway not ready: {text}",
                        component="g8ee",
                    )
                if resp.status >= 400:
                    raise NetworkError(
                        f"Gateway HTTP {resp.status}: {text}",
                        component="g8ee",
                    )

                # Success - parse the receipt
                receipt = json.loads(text)
                logger.info(
                    "[GOVERNANCE-CLIENT] Envelope submitted successfully, status=%s",
                    receipt.get("status", "unknown"),
                )
                return receipt

        except aiohttp.ClientError as e:
            raise NetworkError(
                f"Gateway request failed: {e}",
                component="g8ee",
                cause=e,
            ) from e

    def verify_receipt_signature(self, receipt: dict[str, Any]) -> bool:
        """Verify the ActionReceipt signature from the Gateway.

        This is a placeholder for future signature verification. The Gateway
        signs receipts with its Actuator private key; the Engine should verify
        the signature using the Gateway's public key to ensure receipt authenticity.

        Args:
            receipt: The ActionReceipt dictionary from the Gateway

        Returns:
            True if the signature is valid, False otherwise

        Note:
            Full signature verification requires the Gateway's public key to be
            distributed to Engine instances. This is deferred until PKI key distribution
            is implemented per the attested bootstrap plan (§6).
        """
        # TODO: Implement Ed25519 signature verification once Gateway public key
        # is available to Engine instances. See plan §6 (Attested Bootstrap).
        logger.warning(
            "[GOVERNANCE-CLIENT] Receipt signature verification not yet implemented - "
            "deferring until Gateway public key distribution is available"
        )
        return True

    async def update_governed_doc(
        self,
        collection: str,
        document_id: str,
        updates: dict[str, Any],
        event_type: str,
        *,
        case_id: str | None = None,
        investigation_id: str | None = None,
        task_id: str | None = None,
        web_session_id: str | None = None,
        user_id: str | None = None,
        operator_id: str | None = None,
        operator_session_id: str | None = None,
        merge: bool = True,
    ) -> dict[str, Any]:
        """Submit a governed document update via governance envelope.

        Args:
            collection: Target collection name
            document_id: Document ID to update
            updates: Dictionary of field updates
            event_type: EventType for the governance envelope
            case_id: Optional case ID for context
            investigation_id: Optional investigation ID for context
            task_id: Optional task ID for context
            web_session_id: Optional web session ID
            user_id: Optional user ID
            operator_id: Optional operator ID
            operator_session_id: Optional operator session ID
            merge: If True, use PATCH (merge); if False, use PUT (replace)

        Returns:
            The signed ActionReceipt from the Gateway

        Raises:
            NetworkError: If the HTTP request fails
            ValidationError: If the envelope is rejected by governance gates
        """
        from app.models.pubsub_messages import G8eMessage
        from app.models.command_request_payloads import DocumentUpdateRequestPayload

        payload = DocumentUpdateRequestPayload(
            collection=collection,
            document_id=document_id,
            updates=updates,
            merge=merge,
        )

        message = G8eMessage(
            id=document_id,
            source_component="g8ee",
            event_type=event_type,
            case_id=case_id,
            investigation_id=investigation_id,
            task_id=task_id,
            web_session_id=web_session_id,
            user_id=user_id,
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            payload=payload,
        )

        return await self.submit_envelope(message)

    async def delete_governed_doc(
        self,
        collection: str,
        document_id: str,
        event_type: str,
        *,
        case_id: str | None = None,
        investigation_id: str | None = None,
        task_id: str | None = None,
        web_session_id: str | None = None,
        user_id: str | None = None,
        operator_id: str | None = None,
        operator_session_id: str | None = None,
    ) -> dict[str, Any]:
        """Submit a governed document deletion via governance envelope.

        Args:
            collection: Target collection name
            document_id: Document ID to delete
            event_type: EventType for the governance envelope
            case_id: Optional case ID for context
            investigation_id: Optional investigation ID for context
            task_id: Optional task ID for context
            web_session_id: Optional web session ID
            user_id: Optional user ID
            operator_id: Optional operator ID
            operator_session_id: Optional operator session ID

        Returns:
            The signed ActionReceipt from the Gateway

        Raises:
            NetworkError: If the HTTP request fails
            ValidationError: If the envelope is rejected by governance gates
        """
        from app.models.pubsub_messages import G8eMessage
        from app.models.command_request_payloads import DocumentDeleteRequestPayload

        payload = DocumentDeleteRequestPayload(
            collection=collection,
            document_id=document_id,
        )

        message = G8eMessage(
            id=document_id,
            source_component="g8ee",
            event_type=event_type,
            case_id=case_id,
            investigation_id=investigation_id,
            task_id=task_id,
            web_session_id=web_session_id,
            user_id=user_id,
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            payload=payload,
        )

        return await self.submit_envelope(message)

    async def close(self) -> None:
        """Close the HTTP session."""
        try:
            session = self._session
        except AttributeError:
            session = None

        if session and not session.closed:
            await session.close()
