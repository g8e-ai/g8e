# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Protocol-owned parsing and signature verification for ActionReceipt."""

from __future__ import annotations

import base64
import binascii
import hashlib
import json
import struct
from collections.abc import Mapping

import nacl.exceptions
import nacl.signing
from google.protobuf import json_format
from nacl.bindings import crypto_sign_PUBLICKEYBYTES

from g8e.operator.v1.operator_pb2 import ActionReceipt, ReceiptPersistenceAttestation

ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")
PUBLIC_KEY_PEM_BEGIN = "-----BEGIN PUBLIC KEY-----"
PUBLIC_KEY_PEM_END = "-----END PUBLIC KEY-----"


def parse_action_receipt(data: Mapping[str, object]) -> ActionReceipt:
    """Parse a protojson object into the canonical generated message type."""
    receipt = ActionReceipt()
    json_format.ParseDict(dict(data), receipt, ignore_unknown_fields=False)
    return receipt


def action_receipt_to_dict(receipt: ActionReceipt) -> dict[str, object]:
    """Serialize an ActionReceipt with canonical proto field names and enum names."""
    return json_format.MessageToDict(
        receipt,
        preserving_proto_field_name=True,
        always_print_fields_with_no_presence=True,
    )


def _deterministic_stage_evidence_hash(receipt: ActionReceipt) -> str:
    if not receipt.deterministic_stage_evidence:
        return ""
    payload = bytearray()
    for stage in receipt.deterministic_stage_evidence:
        encoded = stage.SerializeToString(deterministic=True)
        payload.extend(struct.pack(">Q", len(encoded)))
        payload.extend(encoded)
    return hashlib.sha256(payload).hexdigest()


def canonicalize_action_receipt(receipt: ActionReceipt) -> bytes:
    """Produce the exact bytes signed by Go CanonicalizeActionReceipt."""
    data = {
        "transaction_id": receipt.transaction_id,
        "transaction_hash": receipt.transaction_hash,
        "status": receipt.status,
        "result_summary": receipt.result_summary,
        "state_root_before": receipt.state_root_before,
        "state_root_after": receipt.state_root_after,
        "executed_at_unix_ms": receipt.executed_at_unix_ms,
        "signer_key_id": receipt.signer_key_id,
        "l2_status": receipt.l2_status,
        "l3_status": receipt.l3_status,
    }
    stage_evidence_hash = _deterministic_stage_evidence_hash(receipt)
    if stage_evidence_hash:
        data["deterministic_stage_evidence_hash"] = stage_evidence_hash
    payload = json.dumps(data, ensure_ascii=False, separators=(",", ":"))
    payload = (
        payload.replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("&", "\\u0026")
        .replace("\u2028", "\\u2028")
        .replace("\u2029", "\\u2029")
    )
    return payload.encode()


def _canonical_string(value: str) -> bytes:
    encoded = value.encode()
    return struct.pack(">I", len(encoded)) + encoded


def _signature_digest(signatures: list[str]) -> str:
    values = sorted(signature for signature in signatures if signature)
    if not values:
        return ""
    payload = struct.pack(">I", len(values))
    payload += b"".join(_canonical_string(value) for value in values)
    return hashlib.sha256(payload).hexdigest()


def canonicalize_receipt_persistence_attestation(
    attestation: ReceiptPersistenceAttestation,
) -> bytes:
    return b"".join(
        (
            _canonical_string(attestation.transaction_id),
            _canonical_string(attestation.receipt_signature_digest),
            struct.pack(">Q", attestation.persisted_at_unix_ms),
            _canonical_string(attestation.audit_record_id),
            _canonical_string(attestation.signer_key_id),
        )
    )


def decode_ed25519_public_key(public_key: str | bytes) -> bytes:
    """Decode an Ed25519 public key from raw bytes, hexadecimal, or SPKI PEM."""
    if isinstance(public_key, bytes):
        raw_key = public_key
    else:
        value = public_key.strip()
        if value.startswith(PUBLIC_KEY_PEM_BEGIN):
            lines = [
                line
                for line in value.splitlines()
                if line not in {PUBLIC_KEY_PEM_BEGIN, PUBLIC_KEY_PEM_END}
            ]
            der_key = base64.b64decode("".join(lines), validate=True)
            if not der_key.startswith(ED25519_SPKI_PREFIX):
                raise ValueError("public key is not an Ed25519 SPKI key")
            raw_key = der_key[len(ED25519_SPKI_PREFIX) :]
        else:
            raw_key = bytes.fromhex(value)
    if len(raw_key) != crypto_sign_PUBLICKEYBYTES:
        raise ValueError("Ed25519 public key must be 32 bytes")
    return raw_key


def verify_action_receipt_signature(
    receipt: ActionReceipt,
    public_key: str | bytes,
) -> bool:
    """Verify an ActionReceipt signature and fail closed on malformed input."""
    try:
        signature = binascii.unhexlify(receipt.signature)
        verify_key = nacl.signing.VerifyKey(decode_ed25519_public_key(public_key))
        verify_key.verify(canonicalize_action_receipt(receipt), signature)
    except (ValueError, binascii.Error, nacl.exceptions.BadSignatureError):
        return False
    return True


def verify_receipt_persistence_attestation(
    receipt: ActionReceipt,
    public_key: str | bytes,
) -> bool:
    try:
        if not receipt.HasField("final_persistence_attestation"):
            return False
        attestation = receipt.final_persistence_attestation
        if (
            not receipt.transaction_id
            or attestation.transaction_id != receipt.transaction_id
            or attestation.audit_record_id != receipt.transaction_id
            or not receipt.signer_key_id
            or attestation.signer_key_id != receipt.signer_key_id
            or attestation.persisted_at_unix_ms <= 0
            or attestation.receipt_signature_digest != _signature_digest([receipt.signature])
        ):
            return False
        signature = binascii.unhexlify(attestation.signature)
        verify_key = nacl.signing.VerifyKey(decode_ed25519_public_key(public_key))
        verify_key.verify(canonicalize_receipt_persistence_attestation(attestation), signature)
    except (
        ValueError,
        OverflowError,
        struct.error,
        binascii.Error,
        nacl.exceptions.BadSignatureError,
    ):
        return False
    return True
