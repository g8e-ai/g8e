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

import json
import binascii
import nacl.signing

from g8e_evals.models import ActionReceipt


def canonicalize_receipt(receipt: ActionReceipt) -> bytes:
    """
    Produce deterministic byte representation for verification.
    Matches services/g8eo/internal/services/governance/warden.go:CanonicalizeActionReceipt
    """
    data = {
        "transaction_id": receipt.transaction_id,
        "transaction_hash": receipt.transaction_hash,
        "status": _status_to_int(receipt.status.value),
        "result_summary": receipt.result_summary,
        "state_root_before": receipt.state_root_before,
        "state_root_after": receipt.state_root_after,
        "executed_at_unix_ms": receipt.executed_at_unix_ms,
        "signer_key_id": receipt.signer_key_id,
    }
    return json.dumps(data, sort_keys=True, separators=(',', ':')).encode('utf-8')


def _status_to_int(status: str) -> int:
    """Convert ExecutionStatus enum value to its protobuf integer ordinal."""
    status_map = {
        "EXECUTION_STATUS_UNSPECIFIED": 0,
        "EXECUTION_STATUS_EXECUTING": 1,
        "EXECUTION_STATUS_COMPLETED": 2,
        "EXECUTION_STATUS_FAILED": 3,
        "EXECUTION_STATUS_CANCELLED": 4,
        "EXECUTION_STATUS_TIMEOUT": 5,
    }
    return status_map.get(status, 0)

def verify_receipt_signature(receipt: ActionReceipt, public_key_pem: str) -> bool:
    """Verify the Ed25519 signature of an ActionReceipt."""
    signature_hex = receipt.signature
    if not signature_hex:
        return False
        
    try:
        sig_bytes = binascii.unhexlify(signature_hex)
        canonical_bytes = canonicalize_receipt(receipt)
        
        # public_key_pem is expected to be PEM-encoded Ed25519 public key
        # For simplicity in Phase 1, if it starts with 'ed25519:' we treat it as hex or b64
        # But the plan says PEM under .g8e/pki.
        
        # Parse PEM (basic implementation for Ed25519)
        # Note: In a real environment, use a library like cryptography to parse PEM.
        # Here we'll handle the raw hex or base64 if it's not PEM for easier testing.
        
        if "-----BEGIN PUBLIC KEY-----" in public_key_pem:
            # Strip PEM headers and footers
            lines = public_key_pem.strip().split("\n")
            b64_key = "".join(lines[1:-1])
            # Ed25519 SPKI is 44 bytes (prefix + 32 bytes key)
            der_bytes = binascii.a2b_base64(b64_key)
            # The last 32 bytes of the SPKI DER for Ed25519 is the raw key
            raw_pub_key = der_bytes[-32:]
        else:
            # Assume raw hex for now if not PEM
            raw_pub_key = binascii.unhexlify(public_key_pem)
            
        verify_key = nacl.signing.VerifyKey(raw_pub_key)
        verify_key.verify(canonical_bytes, sig_bytes)
        return True
    except Exception:
        return False
