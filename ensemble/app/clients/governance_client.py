# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
GovernanceClient - Engine client for Gateway governance envelope submission and receipt verification.

This client provides a typed interface for Engine services to submit mutations through the
Gateway's fail-closed governance gate (POST /api/v1/governance/envelopes) and verify signed
ActionReceipts returned by the Gateway.

See: .local.dev/docs/plans/engine_gateway_secure_link.md
"""

from __future__ import annotations

import logging
import json
import binascii
from pathlib import Path
from typing import Any

import aiohttp
from nacl.signing import VerifyKey
from nacl.exceptions import BadSignatureError

from app.utils.envelope_builder import build_uap_envelope_json
from app.models.pubsub_messages import G8eMessage
from app.models.settings import GatewaySettings, TLSConfig
from app.services.infra.settings_service import SettingsService
from app.constants import AUTHORIZATION, GatewayAPIPaths
from app.constants.paths import PATHS
from app.errors import G8eError, NetworkError, ValidationError, ErrorCode, ErrorCategory
from app.utils.aiohttp_session import create_component_http_session

logger = logging.getLogger(__name__)


class GovernanceClient:
    """Client for submitting governance envelopes and verifying receipts.

    This client wraps the Gateway's POST /api/v1/governance/envelopes endpoint,
    which enforces L1/L2/L3 verification before executing mutations and returns
    a signed ActionReceipt as proof of execution.
    """

    def __init__(
        self,
        tls_config: TLSConfig | None = None,
        operator_session_id: str | None = None,
        gateway_settings: GatewaySettings | None = None,
        pki_dir: str | None = None,
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

        self._pki_dir = Path(pki_dir) if pki_dir else Path(PATHS["infra"]["pki_dir"])
        self._actuator_key_id: str | None = None
        self._actuator_pub_key: bytes | None = None
        self._actuator_key_loaded = False

    async def connect(self) -> bool:
        """Verify connectivity to the Gateway governance endpoint."""
        try:
            session = await self._get_http_session()
            async with session.get(f"{self._base_url}{GatewayAPIPaths.HEALTH}") as resp:
                if resp.status == 200:
                    logger.info("[GOVERNANCE-CLIENT] Connected to %s", self._base_url)
                    return True
                return False
        except Exception as e:
            logger.error("[GOVERNANCE-CLIENT] Connection failed: %s", e)
            return False

    async def fetch_state_root(self) -> str:
        """Fetch the current state Merkle root from the Gateway health endpoint.

        Per g8e protocol, applications must validate the state root before using it
        in envelope construction for replay defense and state binding.

        Returns:
            The state_merkle_root string from the Gateway, or empty string if unavailable

        Raises:
            NetworkError: If the HTTP request fails
        """
        try:
            session = await self._get_http_session()
            async with session.get(f"{self._base_url}{GatewayAPIPaths.HEALTH}") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    state_root = data.get("state_merkle_root", "")
                    if state_root:
                        logger.debug(
                            "[GOVERNANCE-CLIENT] Fetched state root: %s", state_root[:16] + "..."
                        )
                    return state_root
                logger.warning(
                    "[GOVERNANCE-CLIENT] Health endpoint returned status %s", resp.status
                )
                return ""
        except Exception as e:
            logger.error("[GOVERNANCE-CLIENT] Failed to fetch state root: %s", e)
            return ""

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

        Per g8e protocol, this method:
        - Fetches the current state root from Gateway if not provided
        - Builds a g8e-compliant envelope with transaction hash, nonce, and L3 proof
        - Submits to Gateway for L1/L2/L3 verification
        - Returns signed ActionReceipt

        Args:
            message: The G8eMessage to wrap in a governance envelope
            agent_ids: Optional list of Tribunal agent IDs for L2 metadata
            state_merkle_root: Current state Merkle root for replay protection (fetched if empty)

        Returns:
            The signed ActionReceipt from the Gateway

        Raises:
            NetworkError: If the HTTP request fails
            ValidationError: If the envelope is rejected by governance gates
        """
        # Fetch state root from Gateway if not provided (g8e compliance requirement)
        if not state_merkle_root:
            state_merkle_root = await self.fetch_state_root()

        # Build the envelope using envelope_builder with g8e compliance
        envelope_json = build_uap_envelope_json(
            message,
            agent_ids=agent_ids,
            state_merkle_root=state_merkle_root,
            client_cert_path=self._client_cert_path,
        )

        session = await self._get_http_session()
        url = f"{self._base_url}{GatewayAPIPaths.GOVERNANCE_ENVELOPES}"

        try:
            async with session.post(url, data=envelope_json) as resp:
                text = await resp.text()

                if resp.status == 403:
                    # Governance verification failed (L1/L2/L3 gates)
                    raise G8eError(
                        f"Governance verification failed: {text}",
                        code=ErrorCode.GOVERNANCE_REJECTED,
                        category=ErrorCategory.PERMISSION,
                        component="g8ee",
                    )
                if resp.status == 400:
                    # Malformed envelope
                    raise G8eError(
                        f"Invalid envelope: {text}",
                        code=ErrorCode.INVALID_INPUT,
                        category=ErrorCategory.VALIDATION,
                        component="g8ee",
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

    def _load_actuator_public_key(self) -> bool:
        """Load the actuator public key from the PKI directory.

        Reads ``actuator_pub.json`` (preferred) or ``actuator_pub.pem`` from the
        PKI directory. The file is written by the Gateway at startup via
        ``ExportActuatorPublicKey``.

        Returns:
            True if the key was loaded successfully, False otherwise.
        """
        if self._actuator_key_loaded:
            return self._actuator_pub_key is not None

        self._actuator_key_loaded = True

        json_path = self._pki_dir / "actuator_pub.json"
        pem_path = self._pki_dir / "actuator_pub.pem"

        if json_path.exists():
            try:
                data = json.loads(json_path.read_text())
                self._actuator_key_id = data.get("key_id", "")
                pub_hex = data.get("public_key", "")
                if pub_hex:
                    self._actuator_pub_key = binascii.unhexlify(pub_hex)
                    logger.info(
                        "[GOVERNANCE-CLIENT] Loaded actuator public key from %s (key_id=%s)",
                        json_path,
                        self._actuator_key_id[:16] if self._actuator_key_id else "unknown",
                    )
                    return True
            except Exception as e:
                logger.warning("[GOVERNANCE-CLIENT] Failed to read actuator_pub.json: %s", e)

        if pem_path.exists():
            try:
                pem_text = pem_path.read_text()
                lines = pem_text.strip().split("\n")
                b64_key = "".join(lines[1:-1])
                der_bytes = binascii.a2b_base64(b64_key)
                self._actuator_pub_key = der_bytes[-32:]
                logger.info("[GOVERNANCE-CLIENT] Loaded actuator public key from %s", pem_path)
                return True
            except Exception as e:
                logger.warning("[GOVERNANCE-CLIENT] Failed to parse actuator_pub.pem: %s", e)

        logger.warning(
            "[GOVERNANCE-CLIENT] Actuator public key not found in PKI dir %s", self._pki_dir
        )
        return False

    @staticmethod
    def _canonicalize_receipt(receipt: dict[str, Any]) -> bytes:
        """Produce deterministic JSON bytes for signature verification.

        Must match g8e's ``CanonicalizeActionReceipt`` in
        ``internal/services/governance/l5_actuator.go`` — same fields, same order.
        Go's ``json.Marshal`` preserves struct field order, so the Python side
        uses an insertion-ordered dict (Python 3.7+) without ``sort_keys``.
        """
        canonical = {
            "transaction_id": receipt.get("transaction_id", ""),
            "transaction_hash": receipt.get("transaction_hash", ""),
            "status": receipt.get("status", ""),
            "result_summary": receipt.get("result_summary", ""),
            "state_root_before": receipt.get("state_root_before", ""),
            "state_root_after": receipt.get("state_root_after", ""),
            "executed_at_unix_ms": receipt.get("executed_at_unix_ms", 0),
            "signer_key_id": receipt.get("signer_key_id", ""),
            "l2_status": receipt.get("l2_status", ""),
            "l3_status": receipt.get("l3_status", ""),
        }
        return json.dumps(canonical, separators=(",", ":")).encode("utf-8")

    def verify_receipt_signature(self, receipt: dict[str, Any]) -> bool:
        """Verify the Ed25519 signature of an ActionReceipt from the Gateway.

        The Gateway signs receipts with its Actuator private key during
        ``L5Actuator.Execute``. This method verifies the signature using the
        actuator public key distributed via the PKI directory
        (``actuator_pub.json`` / ``actuator_pub.pem``).

        Args:
            receipt: The ActionReceipt dictionary from the Gateway

        Returns:
            True if the signature is valid, False otherwise. Returns False if
            the actuator public key is not available or the receipt is missing
            required fields.
        """
        if not self._load_actuator_public_key():
            logger.warning(
                "[GOVERNANCE-CLIENT] Cannot verify receipt: actuator public key not available"
            )
            return False

        signature_hex = receipt.get("signature", "")
        if not signature_hex:
            logger.warning("[GOVERNANCE-CLIENT] Receipt has no signature field")
            return False

        signer_key_id = receipt.get("signer_key_id", "")
        if self._actuator_key_id and signer_key_id and signer_key_id != self._actuator_key_id:
            logger.warning(
                "[GOVERNANCE-CLIENT] Receipt signer_key_id %s does not match actuator key_id %s",
                signer_key_id,
                self._actuator_key_id,
            )
            return False

        try:
            sig_bytes = binascii.unhexlify(signature_hex)
            canonical_bytes = self._canonicalize_receipt(receipt)
            verify_key = VerifyKey(self._actuator_pub_key)
            verify_key.verify(canonical_bytes, sig_bytes)
            logger.info("[GOVERNANCE-CLIENT] Receipt signature verified successfully")
            return True
        except BadSignatureError:
            logger.warning("[GOVERNANCE-CLIENT] Receipt signature verification failed: bad signature")
            return False
        except Exception as e:
            logger.error("[GOVERNANCE-CLIENT] Receipt signature verification error: %s", e)
            return False

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
