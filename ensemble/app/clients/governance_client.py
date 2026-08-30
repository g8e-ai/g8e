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

import asyncio
import base64
import hashlib
import logging
import json
import binascii
import secrets
from datetime import datetime, timedelta, UTC
from pathlib import Path
from typing import Any

import aiohttp
from nacl.signing import VerifyKey
from nacl.exceptions import BadSignatureError

from app.constants.action_type_mappings import map_event_type_to_action_type
from app.models.pubsub_messages import G8eMessage
from app.models.settings import GatewaySettings, TLSConfig
from app.services.infra.settings_service import SettingsService
from app.constants import AUTHORIZATION, GatewayAPIPaths
from app.constants.config import G8EE_COMPONENT
from app.constants.paths import PATHS
from app.errors import G8eError, NetworkError, ValidationError, ErrorCode, ErrorCategory
from app.utils.aiohttp_session import create_component_http_session
from g8e.models.governance import (
    GovernanceEnvelope,
    GovernanceL2,
    GovernanceL3,
    GovernanceL3Proof,
    GovernanceMetadata,
    compute_transaction_hash,
)

logger = logging.getLogger(__name__)


# Mapping from internal g8ee payload types to canonical g8e protocol payload types
PAYLOAD_TYPE_MAPPING = {
    "command": "CommandRequested",
    "command_cancel": "CommandCancelRequested",
    "file_edit": "FileEditRequested",
    "fs_list": "FsListRequested",
    "fs_grep": "FsGrepRequested",
    "fs_read": "FsReadRequested",
    "fetch_logs": "FetchLogsRequested",
    "fetch_history": "FetchHistoryRequested",
    "fetch_file_history": "FetchFileHistoryRequested",
    "fetch_file_diff": "FetchFileDiffRequested",
    "restore_file": "RestoreFileRequested",
    "check_port": "CheckPortRequested",
    "heartbeat": "HeartbeatRequested",
    "document_update": "DocumentUpdateRequested",
    "document_delete": "DocumentDeleteRequested",
    "direct_command_audit": "DirectCommandAuditRequested",
}


# Mapping from internal source_component strings to proto Component enum value
# names. The Go gateway decodes GovernanceEnvelope.source_component as the
# g8e.common.v1.Component proto enum via protojson, which expects enum value
# names (e.g. "COMPONENT_AGENT"), not the internal lowercase strings the
# ensemble uses for routing and validation (e.g. "g8ee", "client").
#
# The gateway component ("g8eo-gateway") is not a valid source for governed
# outbound mutations from the ensemble and is intentionally absent. Unknown
# or empty values raise a typed ValidationError rather than silently
# defaulting to COMPONENT_AGENT — a misclassified identity could attribute a
# governed action to the wrong component and bypass identity binding.
_COMPONENT_TO_PROTO_ENUM = {
    "g8ee": "COMPONENT_AGENT",
    "client": "COMPONENT_CLIENT",
    "g8eo": "COMPONENT_G8EO",
}


def _source_component_to_proto_enum(internal: str) -> str:
    """Translate an internal source_component string to a proto Component enum value name.

    Fail-closed: raises ValidationError for unknown or empty values. A
    misclassified source component could attribute a governed action to the
    wrong component and silently bypass transport-to-envelope identity binding.
    """
    if not internal:
        raise ValidationError(
            "source_component is required for governance envelope construction",
            component="g8ee",
        )
    try:
        return _COMPONENT_TO_PROTO_ENUM[internal]
    except KeyError as exc:
        raise ValidationError(
            f"unknown source_component {internal!r}: cannot map to proto Component enum",
            component="g8ee",
        ) from exc


def map_to_canonical_payload_type(internal_payload_type: str) -> str:
    """Map internal g8ee payload type to canonical g8e protocol payload type.

    Per g8e protocol specification, applications must use canonical typed payload
    identifiers for envelope construction. This function translates internal
    payload naming to the canonical protocol schema.

    Args:
        internal_payload_type: Internal payload type (e.g., "command", "file_edit")

    Returns:
        Canonical g8e protocol payload type (e.g., "CommandRequested")
    """
    return PAYLOAD_TYPE_MAPPING.get(internal_payload_type, internal_payload_type)


def generate_nonce() -> str:
    """Generate a cryptographically secure random nonce for replay defense.

    Returns:
        Hexadecimal string (32 bytes = 64 hex characters)
    """
    return secrets.token_hex(32)


def get_certificate_fingerprint(cert_path: str | None) -> str:
    """Compute SHA256 fingerprint of mTLS client certificate.

    Args:
        cert_path: Path to client certificate file

    Returns:
        Hexadecimal SHA256 fingerprint string, or empty string if cert not found
    """
    if not cert_path or not Path(cert_path).exists():
        return ""

    try:
        cert_bytes = Path(cert_path).read_bytes()
        return hashlib.sha256(cert_bytes).hexdigest()
    except Exception as e:
        logger.warning("Failed to compute certificate fingerprint: %s", e)
        return ""


def build_governance_envelope(
    message: G8eMessage,
    *,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    client_cert_path: str | None = None,
) -> GovernanceEnvelope:
    """Build a g8e-compliant GovernanceEnvelope with structured intent data.

    Constructs a ``g8e.models.governance.GovernanceEnvelope`` per the g8e
    protocol specification:
    - Uses canonical JSON wire format (protojson-compatible)
    - Generates deterministic transaction hash (SHA256 of canonical fields)
    - Includes nonce for replay defense
    - Includes L3 notary proof (mTLS certificate fingerprint)
    - Uses typed payload identifiers per protocol schema

    Args:
        message: The G8eMessage to wrap in a governance envelope
        agent_ids: Optional list of Tribunal agent IDs for L2 metadata
        state_merkle_root: Current state Merkle root for replay protection
        client_cert_path: Path to mTLS client certificate for L3 proof

    Returns:
        A g8e-compliant GovernanceEnvelope with transaction hash and governance metadata
    """
    if message.payload is None:
        raise ValueError("G8eMessage.payload is required to build GovernanceEnvelope")

    # Serialize the protobuf payload to bytes for the envelope's payload field
    proto_payload = message.payload.to_protobuf()
    payload_bytes = proto_payload.SerializeToString()
    payload_dict = message.payload.model_dump(mode="json")

    action_type = map_event_type_to_action_type(message.event_type)

    now_utc = datetime.now(UTC)
    expires_at = now_utc + timedelta(minutes=5)

    # Generate nonce for replay defense
    nonce = generate_nonce()

    # Compute L3 notary proof (mTLS certificate fingerprint)
    cert_fingerprint = get_certificate_fingerprint(client_cert_path)

    # Bind identity: the human user who authorized the action (requestor) and
    # the app acting on their behalf (acting_app). Both are included in the
    # canonical transaction hash so they are cryptographically tamper-evident
    # and verified by the gateway's identity binding check. The acting app is
    # always the ensemble (g8ee) for envelopes built here.
    requestor_user_id = message.user_id or ""
    acting_app_id = G8EE_COMPONENT

    payload_b64 = base64.b64encode(payload_bytes).decode("ascii") if payload_bytes else ""
    transaction_hash = compute_transaction_hash(
        action_type=action_type,
        target_resource="localhost",
        payload=payload_b64,
        state_merkle_root=state_merkle_root,
        nonce=nonce,
        expires_at=expires_at.isoformat(),
        intent_data=payload_dict,
        requestor_user_id=requestor_user_id,
        acting_app_id=acting_app_id,
    )

    # L2 Metadata - consensus_set_id only; votes/signatures handled by Gateway
    l2 = GovernanceL2()
    if agent_ids:
        l2.consensus_set_id = agent_ids[0]

    # L3 Metadata - notary proof (mTLS certificate fingerprint)
    l3 = GovernanceL3()
    if cert_fingerprint:
        l3.proof = GovernanceL3Proof(mtls_cert_fingerprint=cert_fingerprint)

    return GovernanceEnvelope(
        protocol_version="1.0",
        id=transaction_hash,
        timestamp=message.timestamp or now_utc,
        expires_at=expires_at,
        source_component=_source_component_to_proto_enum(message.source_component),
        event_type=message.event_type,
        action_type=action_type,
        target_resource="localhost",
        operator_id=message.operator_id or "",
        operator_session_id=message.operator_session_id or "",
        web_session_id=message.web_session_id or "",
        cli_session_id=message.cli_session_id or "",
        state_merkle_root=state_merkle_root,
        nonce=nonce,
        transaction_hash=transaction_hash,
        intent_data=payload_dict,
        case_id=message.case_id,
        investigation_id=message.investigation_id,
        task_id=message.task_id,
        payload=payload_b64,
        requestor_user_id=requestor_user_id or None,
        acting_app_id=acting_app_id,
        governance=GovernanceMetadata(l2=l2, l3=l3),
    )


def build_governance_envelope_json(
    message: G8eMessage,
    *,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    client_cert_path: str | None = None,
) -> str:
    """Build a g8e-compliant GovernanceEnvelope and return it as a JSON string.

    Args:
        message: The G8eMessage to wrap in a governance envelope
        agent_ids: Optional list of Tribunal agent IDs for L2 metadata
        state_merkle_root: Current state Merkle root for replay protection
        client_cert_path: Path to mTLS client certificate for L3 proof

    Returns:
        Canonical JSON string representation of the envelope
    """
    envelope = build_governance_envelope(
        message,
        agent_ids=agent_ids,
        state_merkle_root=state_merkle_root,
        client_cert_path=client_cert_path,
    )
    return envelope.model_dump_json(exclude_none=True)


class GovernanceClient:
    """Client for submitting governance envelopes and verifying receipts.

    This client wraps the Gateway's POST /api/v1/governance/envelopes endpoint,
    which enforces L1/L2/L3 verification before executing mutations and returns
    a signed ActionReceipt as proof of execution.
    """

    # Maximum number of TX_STATE_MISMATCH retries before giving up. The state
    # root may change between fetch and submit when concurrent envelopes are
    # in flight; retrying with a fresh state root resolves the race.
    _STATE_ROOT_MAX_RETRIES = 3

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
        self._submission_lock = asyncio.Lock()

        self._pki_dir = Path(pki_dir) if pki_dir else Path(PATHS["infra"]["pki_dir"])
        self._actuator_key_id: str | None = None
        self._actuator_pub_key: bytes | None = None
        self._actuator_key_loaded = False

        # Cached operator transport identity parsed from the client cert's
        # SPIFFE URI SAN. Resolved lazily on the first submit_envelope call
        # that needs it. Format: spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>
        self._operator_identity_cache: tuple[str | None, str | None] | None = None

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

    def _read_cert_spiffe_uri(self) -> str | None:
        """Read the SPIFFE URI SAN from the client certificate.

        Returns the first URI SAN found in the cert at
        ``self._client_cert_path``, or None when no cert is configured or
        the cert has no URI SAN.
        """
        if not self._client_cert_path:
            return None
        try:
            from cryptography import x509

            with open(self._client_cert_path, "rb") as f:
                cert = x509.load_pem_x509_certificate(f.read())
            for ext in cert.extensions:
                if isinstance(ext.value, x509.SubjectAlternativeName):
                    for name in ext.value:
                        if isinstance(name, x509.UniformResourceIdentifier):
                            return name.value
        except Exception as e:
            logger.warning("[GOVERNANCE-CLIENT] Failed to read cert SPIFFE URI: %s", e)
        return None

    def _resolve_operator_identity_from_cert(self) -> tuple[str | None, str | None]:
        """Resolve (operator_id, operator_session_id) from the client cert's SPIFFE URI SAN.

        The operator mTLS cert carries a SPIFFE URI of the form
        ``spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>``.
        The gateway's ``verifyEnvelopeIdentityBinding`` checks that both
        ``operator_id`` and ``operator_session_id`` match the cert's SPIFFE
        URI suffix. This method parses them from the cert so every
        submitted envelope is bound to the operator transport identity.

        The result is cached after the first successful parse. Returns
        ``(None, None)`` when no client cert is configured or the cert has
        no operator SPIFFE URI SAN.
        """
        if self._operator_identity_cache is not None:
            return self._operator_identity_cache

        spiffe_uri = self._read_cert_spiffe_uri()
        if not spiffe_uri:
            self._operator_identity_cache = (None, None)
            return self._operator_identity_cache

        # Format: spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>
        parts = spiffe_uri.split("/")
        if (
            len(parts) >= 7
            and parts[0] == "spiffe:"
            and parts[2] == "g8e.local"
            and parts[3] == "operator"
        ):
            operator_id = parts[5]
            operator_session_id = parts[6]
            self._operator_identity_cache = (operator_id, operator_session_id)
            logger.info(
                "[GOVERNANCE-CLIENT] Resolved operator identity from cert "
                "(operator_id=%s, session=%s...)",
                operator_id,
                operator_session_id[:8] if operator_session_id else "unknown",
            )
        else:
            logger.warning(
                "[GOVERNANCE-CLIENT] Cert SPIFFE URI is not an operator identity: %s",
                spiffe_uri,
            )
            self._operator_identity_cache = (None, None)

        return self._operator_identity_cache

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

        The L4 warden compares the envelope's state_merkle_root to the gateway's
        current state root. When concurrent envelopes are in flight, the state
        root may change between fetch and submit, producing TX_STATE_MISMATCH.
        This method retries up to _STATE_ROOT_MAX_RETRIES times by re-fetching
        the state root and rebuilding the envelope. The caller-provided
        state_merkle_root is honored on the first attempt but not on retries
        (the caller's value is stale by definition on retry).

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
        # Inject the operator transport identity (operator_id +
        # operator_session_id) when the message omits either field. The
        # gateway's verifyEnvelopeIdentityBinding rejects mutation actions
        # (DOCUMENT_UPDATE, DOCUMENT_DELETE, FILE_EDIT) with
        # ErrIdentityBindingFailed when both are empty. The identity is
        # resolved from the operator mTLS cert's SPIFFE URI SAN, which
        # guarantees the stamped values match the transport identity the
        # gateway verifies against. Resolution is lazy (first submission)
        # and cached so the cert is read at most once.
        if not message.operator_id or not message.operator_session_id:
            cert_op_id, cert_op_session = self._resolve_operator_identity_from_cert()
            updates: dict[str, str | None] = {}
            if not message.operator_id and cert_op_id:
                updates["operator_id"] = cert_op_id
            if not message.operator_session_id and cert_op_session:
                updates["operator_session_id"] = cert_op_session
            if updates:
                message = message.model_copy(update=updates)
            elif not message.operator_session_id and self._operator_session_id:
                # Fall back to the constructor-provided session ID when the
                # cert has no SPIFFE URI SAN (e.g. app cert fallback path).
                message = message.model_copy(
                    update={"operator_session_id": self._operator_session_id}
                )

        async with self._submission_lock:
            # Retry loop: re-fetch the state root on TX_STATE_MISMATCH. The state
            # root may change between fetch and submit when concurrent envelopes
            # are in flight (e.g. case creation + investigation creation + chat
            # message append all submitting in quick succession).
            current_root = state_merkle_root
            for attempt in range(self._STATE_ROOT_MAX_RETRIES + 1):
                if not current_root:
                    current_root = await self.fetch_state_root()

                envelope_json = build_governance_envelope_json(
                    message,
                    agent_ids=agent_ids,
                    state_merkle_root=current_root,
                    client_cert_path=self._client_cert_path,
                )

                session = await self._get_http_session()
                url = f"{self._base_url}{GatewayAPIPaths.GOVERNANCE_ENVELOPES}"

                try:
                    async with session.post(url, data=envelope_json) as resp:
                        text = await resp.text()

                        if resp.status == 403:
                            # Governance verification failed (L1/L2/L3 gates).
                            # TX_STATE_MISMATCH is returned as a 403 with the
                            # error string in the body. Retry by re-fetching the
                            # state root.
                            if (
                                "TX_STATE_MISMATCH" in text
                                and attempt < self._STATE_ROOT_MAX_RETRIES
                            ):
                                logger.warning(
                                    "[GOVERNANCE-CLIENT] TX_STATE_MISMATCH on attempt %d/%d, re-fetching state root",
                                    attempt + 1,
                                    self._STATE_ROOT_MAX_RETRIES + 1,
                                )
                                current_root = ""  # force re-fetch on next iteration
                                continue
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

            # Exhausted retries — the final attempt's 403 is raised inside the loop.
            # This line is unreachable but satisfies the type checker.
            raise G8eError(
                "Governance verification failed: TX_STATE_MISMATCH after retries",
                code=ErrorCode.GOVERNANCE_REJECTED,
                category=ErrorCategory.PERMISSION,
                component="g8ee",
            )

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
            source_component=G8EE_COMPONENT,
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
            source_component=G8EE_COMPONENT,
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
