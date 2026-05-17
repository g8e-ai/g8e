import json
import binascii
import nacl.signing
from typing import Dict, Any, Tuple

def canonicalize_receipt(receipt: Dict[str, Any]) -> bytes:
    """
    Produce deterministic byte representation for verification.
    Matches services/g8eo/internal/services/governance/warden.go:CanonicalizeActionReceipt
    """
    # Go's json.Marshal sorts keys alphabetically and uses compact encoding (no spaces)
    # Fields: transaction_id, transaction_hash, status, result_summary, state_root_before, 
    # state_root_after, executed_at_unix_ms, signer_key_id
    
    # We must ensure status is an int, not a string (if it was returned as a string by some API)
    status = receipt.get("status")
    if isinstance(status, str):
        # Map string status back to int if needed (e.g. from protojson output)
        # 0: UNSPECIFIED, 1: EXECUTING, 2: COMPLETED, 3: FAILED, etc.
        status_map = {
            "EXECUTION_STATUS_UNSPECIFIED": 0,
            "EXECUTION_STATUS_EXECUTING": 1,
            "EXECUTION_STATUS_COMPLETED": 2,
            "EXECUTION_STATUS_FAILED": 3,
            "EXECUTION_STATUS_CANCELLED": 4,
            "EXECUTION_STATUS_TIMEOUT": 5,
            "COMPLETED": 2, # Common alias
            "FAILED": 3,
        }
        status = status_map.get(status, 0)

    data = {
        "transaction_id": receipt.get("transaction_id"),
        "transaction_hash": receipt.get("transaction_hash"),
        "status": status,
        "result_summary": receipt.get("result_summary", ""),
        "state_root_before": receipt.get("state_root_before", ""),
        "state_root_after": receipt.get("state_root_after", ""),
        "executed_at_unix_ms": int(receipt.get("executed_at_unix_ms", 0)),
        "signer_key_id": receipt.get("signer_key_id"),
    }
    
    return json.dumps(data, sort_keys=True, separators=(',', ':')).encode('utf-8')

def verify_receipt_signature(receipt: Dict[str, Any], public_key_pem: str) -> bool:
    """Verify the Ed25519 signature of an ActionReceipt."""
    signature_hex = receipt.get("signature")
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
