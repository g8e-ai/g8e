# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Dedicated Ed25519 eval-run signing identity and assessed external trust.

The eval bundle signing identity is dedicated to eval bundles. It is not an
actuator receipt identity and is never represented as an ``ActionReceipt``.
A bundle signature binds the signature algorithm, key ID, bundle ID, run ID,
release version, and signed digest to the canonical manifest root and
checksum root bytes.

The verifier never trusts a key merely because the bundle contains it.
Trust originates exclusively from the externally supplied ``EvalTrustStore``,
which carries protocol-owned public-key metadata and assessed-trust state.
Verification fails closed for unknown, revoked, expired, wrong-scope,
wrong-algorithm, malformed, duplicate, and substituted keys and signatures.
"""

from __future__ import annotations

import binascii
from datetime import datetime
from enum import StrEnum

import nacl.exceptions
import nacl.signing
from nacl.bindings import crypto_sign_SEEDBYTES
from pydantic import BaseModel, ConfigDict, Field, field_validator

from g8e_evals.bundle.canonical import (
    canonical_checksum_root_bytes,
    canonical_manifest_bytes,
    compute_checksum_root_hash,
    compute_manifest_hash,
)
from g8e_evals.bundle.manifest import BundleManifest, ChecksumRoot


EVAL_SIGNING_ALGORITHM = "ed25519"
EVAL_TRUST_SCOPE = "eval-bundle"
ACTUATOR_TRUST_SCOPE = "actuator-evidence"


class TrustStatus(StrEnum):
    """Outcome of assessing a bundle signature against the external trust store.

    ``TRUSTED`` means the key is known, not revoked, within its validity
    window, in scope, of the correct algorithm, and the signature verifies
    against the canonical bytes and bound identity. Every other value is a
    fail-closed disposition: the signature is not accepted.
    """

    TRUSTED = "trusted"
    UNKNOWN = "unknown"
    REVOKED = "revoked"
    EXPIRED = "expired"
    WRONG_SCOPE = "wrong_scope"
    WRONG_ALGORITHM = "wrong_algorithm"
    MALFORMED = "malformed"
    SUBSTITUTED = "substituted"


class EvalSigningKey:
    """Dedicated Ed25519 signing identity for eval bundles.

    This is not an actuator receipt identity and is never represented as an
    ``ActionReceipt``. The key ID is the hexadecimal encoding of the raw
    Ed25519 public key, matching the convention used by the platform receipt
    verifier. Signatures are hexadecimal-encoded Ed25519 signatures over the
    canonical bytes of the signed object.
    """

    def __init__(self, signing_key: nacl.signing.SigningKey) -> None:
        self._signing_key = signing_key

    @classmethod
    def generate(cls) -> EvalSigningKey:
        """Generate a new random Ed25519 signing identity."""
        return cls(nacl.signing.SigningKey.generate())

    @classmethod
    def from_seed(cls, seed: bytes) -> EvalSigningKey:
        """Construct a signing identity from a 32-byte seed.

        Deterministic for the same seed, which is useful for tests and
        reproducible fixtures. The seed must be exactly 32 bytes.
        """
        if len(seed) != crypto_sign_SEEDBYTES:
            raise ValueError(
                f"Ed25519 seed must be {crypto_sign_SEEDBYTES} bytes, got {len(seed)}",
            )
        return cls(nacl.signing.SigningKey(seed))

    @property
    def key_id(self) -> str:
        """Hexadecimal encoding of the raw public key."""
        return self._signing_key.verify_key.encode().hex()

    @property
    def public_key_hex(self) -> str:
        """Hexadecimal encoding of the raw public key."""
        return self.key_id

    def sign(self, message: bytes) -> str:
        """Sign ``message`` and return a hexadecimal-encoded signature."""
        return binascii.hexlify(self._signing_key.sign(message).signature).decode()

    def verify(self, message: bytes, signature_hex: str) -> bool:
        """Verify a hexadecimal-encoded signature over ``message``."""
        try:
            verify_key = self._signing_key.verify_key
            signature = binascii.unhexlify(signature_hex)
            verify_key.verify(message, signature)
        except (ValueError, binascii.Error, nacl.exceptions.BadSignatureError):
            return False
        return True


class EvalSignature(BaseModel):
    """One signature over a canonical root (manifest or checksum root).

    Binds the signature algorithm, key ID, bundle ID, run ID, release
    version, SHA-256 signed digest of the canonical bytes, the
    hexadecimal-encoded Ed25519 signature, and the signing timestamp. The
    signed digest and bound identity fields let the verifier detect a
    manifest or root swapped after signing without recomputing the signature
    alone.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    algorithm: str = Field(
        min_length=1,
        description="Signature algorithm. Must match the trusted key's algorithm.",
    )
    key_id: str = Field(
        min_length=1,
        pattern=r"^[0-9a-f]{64}$",
        description="Hex Ed25519 public key identifying the signing identity.",
    )
    bundle_id: str = Field(min_length=1, description="Bundle identity bound by the signature.")
    run_id: str = Field(min_length=1, description="Run identity bound by the signature.")
    release_version: str = Field(
        min_length=1,
        description="Release version bound by the signature.",
    )
    signed_digest: str = Field(
        pattern=r"^[0-9a-f]{64}$",
        description="SHA-256 over the canonical bytes of the signed object.",
    )
    signature: str = Field(
        pattern=r"^[0-9a-f]{128}$",
        description="Hexadecimal-encoded Ed25519 signature over the canonical bytes.",
    )
    signed_at: datetime = Field(description="Signature timestamp.")

    @field_validator("algorithm")
    @classmethod
    def _validate_algorithm(cls, value: str) -> str:
        if value != EVAL_SIGNING_ALGORITHM:
            raise ValueError(f"unsupported signature algorithm: {value}")
        return value


class BundleSignature(BaseModel):
    """Signatures over the canonical manifest root and checksum root."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    manifest_signature: EvalSignature = Field(
        description="Signature over the canonical manifest root bytes.",
    )
    checksum_root_signature: EvalSignature = Field(
        description="Signature over the canonical checksum root bytes.",
    )


class EvalTrustedKey(BaseModel):
    """Protocol-owned public-key metadata for one trusted eval signing identity.

    The verifier never trusts a key merely because the bundle contains it.
    Each trusted key declares its algorithm, scope, validity window,
    revocation state, and provenance source. A signature is accepted only
    when its key ID resolves to a trusted key that is not revoked, within
    its validity window, in scope, of the correct algorithm, and the
    signature verifies against the canonical bytes and bound identity.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    key_id: str = Field(
        min_length=1,
        pattern=r"^[0-9a-f]{64}$",
        description="Hex Ed25519 public key identifying the trusted signing identity.",
    )
    public_key_hex: str = Field(
        min_length=1,
        pattern=r"^[0-9a-f]{64}$",
        description="Hex raw Ed25519 public key used to verify signatures.",
    )
    algorithm: str = Field(
        min_length=1,
        description="Signature algorithm this key is trusted for.",
    )
    scope: str = Field(
        min_length=1,
        description="Trust scope this key is trusted for (e.g. eval-bundle).",
    )
    valid_from: datetime = Field(description="Validity window start (inclusive).")
    valid_until: datetime = Field(description="Validity window end (inclusive).")
    revoked: bool = Field(
        default=False,
        description="True if the key has been revoked and must no longer be trusted.",
    )
    source: str = Field(
        min_length=1,
        description="Provenance of the trust assertion (protocol-owned metadata).",
    )

    @field_validator("algorithm")
    @classmethod
    def _validate_algorithm(cls, value: str) -> str:
        if value != EVAL_SIGNING_ALGORITHM:
            raise ValueError(f"unsupported trusted-key algorithm: {value}")
        return value


class EvalTrustStore(BaseModel):
    """External assessed-trust input for eval bundle signature verification.

    The trust store is supplied by the verifier out of band from
    protocol-owned public-key metadata. It is never read from inside the
    bundle. A signature is accepted only when its key ID resolves to a
    trusted key in this store.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    keys: list[EvalTrustedKey] = Field(
        default_factory=list,
        description="Trusted eval signing identities, keyed by key_id.",
    )

    @field_validator("keys")
    @classmethod
    def _reject_duplicate_key_ids(cls, keys: list[EvalTrustedKey]) -> list[EvalTrustedKey]:
        seen: set[str] = set()
        for key in keys:
            if key.key_id in seen:
                raise ValueError(f"duplicate trusted key_id: {key.key_id}")
            seen.add(key.key_id)
        return keys

    def lookup(self, key_id: str) -> EvalTrustedKey | None:
        """Return the trusted key for ``key_id`` or ``None`` if not present."""
        for key in self.keys:
            if key.key_id == key_id:
                return key
        return None


class BundleSignatureVerificationResult(BaseModel):
    """Typed result of verifying a bundle signature against the trust store.

    ``manifest_status`` and ``checksum_root_status`` are the per-root
    ``TrustStatus`` dispositions. ``ok`` is ``True`` only when both roots are
    ``TRUSTED``. Every other combination is fail-closed.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    manifest_status: TrustStatus = Field(description="Trust disposition of the manifest signature.")
    checksum_root_status: TrustStatus = Field(
        description="Trust disposition of the checksum-root signature.",
    )
    ok: bool = Field(description="True only when both signatures are TRUSTED.")


def sign_bundle(
    manifest: BundleManifest,
    checksum_root: ChecksumRoot,
    signing_key: EvalSigningKey,
    signed_at: datetime,
) -> BundleSignature:
    """Sign the canonical manifest root and checksum root with ``signing_key``.

    Each signature binds the algorithm, key ID, bundle ID, run ID, release
    version, SHA-256 signed digest of the canonical bytes, the
    hexadecimal-encoded Ed25519 signature, and the signing timestamp.
    """
    manifest_bytes = canonical_manifest_bytes(manifest)
    checksum_bytes = canonical_checksum_root_bytes(checksum_root)
    manifest_digest = compute_manifest_hash(manifest)
    checksum_digest = compute_checksum_root_hash(checksum_root)

    manifest_signature = EvalSignature(
        algorithm=EVAL_SIGNING_ALGORITHM,
        key_id=signing_key.key_id,
        bundle_id=manifest.bundle_id,
        run_id=manifest.run_id,
        release_version=manifest.release_version,
        signed_digest=manifest_digest,
        signature=signing_key.sign(manifest_bytes),
        signed_at=signed_at,
    )
    checksum_root_signature = EvalSignature(
        algorithm=EVAL_SIGNING_ALGORITHM,
        key_id=signing_key.key_id,
        bundle_id=manifest.bundle_id,
        run_id=manifest.run_id,
        release_version=manifest.release_version,
        signed_digest=checksum_digest,
        signature=signing_key.sign(checksum_bytes),
        signed_at=signed_at,
    )
    return BundleSignature(
        manifest_signature=manifest_signature,
        checksum_root_signature=checksum_root_signature,
    )


def _verify_one_signature(
    signature: EvalSignature,
    canonical_bytes: bytes,
    expected_digest: str,
    expected_bundle_id: str,
    expected_run_id: str,
    expected_release_version: str,
    trust_store: EvalTrustStore,
) -> TrustStatus:
    """Assess one signature against the trust store. Fail-closed."""
    trusted = trust_store.lookup(signature.key_id)
    if trusted is None:
        return TrustStatus.UNKNOWN
    if trusted.revoked:
        return TrustStatus.REVOKED
    if not (trusted.valid_from <= signature.signed_at <= trusted.valid_until):
        return TrustStatus.EXPIRED
    if trusted.scope != EVAL_TRUST_SCOPE:
        return TrustStatus.WRONG_SCOPE
    if trusted.algorithm != signature.algorithm:
        return TrustStatus.WRONG_ALGORITHM
    if trusted.public_key_hex != trusted.key_id:
        return TrustStatus.SUBSTITUTED
    if (
        signature.bundle_id != expected_bundle_id
        or signature.run_id != expected_run_id
        or signature.release_version != expected_release_version
        or signature.signed_digest != expected_digest
    ):
        return TrustStatus.SUBSTITUTED
    try:
        verify_key = nacl.signing.VerifyKey(bytes.fromhex(trusted.public_key_hex))
        verify_key.verify(canonical_bytes, bytes.fromhex(signature.signature))
    except (ValueError, binascii.Error, nacl.exceptions.BadSignatureError):
        return TrustStatus.MALFORMED
    return TrustStatus.TRUSTED


def verify_bundle_signature(
    bundle_signature: BundleSignature,
    manifest: BundleManifest,
    checksum_root: ChecksumRoot,
    trust_store: EvalTrustStore,
) -> BundleSignatureVerificationResult:
    """Verify both signatures against the external trust store. Fail-closed.

    The verifier never trusts a key merely because the bundle contains it.
    Trust originates exclusively from ``trust_store``. Each signature is
    assessed for unknown, revoked, expired, wrong-scope, wrong-algorithm,
    substituted, and malformed dispositions. The result is ``ok`` only when
    both roots are ``TRUSTED``.
    """
    manifest_status = _verify_one_signature(
        signature=bundle_signature.manifest_signature,
        canonical_bytes=canonical_manifest_bytes(manifest),
        expected_digest=compute_manifest_hash(manifest),
        expected_bundle_id=manifest.bundle_id,
        expected_run_id=manifest.run_id,
        expected_release_version=manifest.release_version,
        trust_store=trust_store,
    )
    checksum_root_status = _verify_one_signature(
        signature=bundle_signature.checksum_root_signature,
        canonical_bytes=canonical_checksum_root_bytes(checksum_root),
        expected_digest=compute_checksum_root_hash(checksum_root),
        expected_bundle_id=manifest.bundle_id,
        expected_run_id=manifest.run_id,
        expected_release_version=manifest.release_version,
        trust_store=trust_store,
    )
    return BundleSignatureVerificationResult(
        manifest_status=manifest_status,
        checksum_root_status=checksum_root_status,
        ok=manifest_status == TrustStatus.TRUSTED
        and checksum_root_status == TrustStatus.TRUSTED,
    )


__all__ = [
    "ACTUATOR_TRUST_SCOPE",
    "EVAL_SIGNING_ALGORITHM",
    "EVAL_TRUST_SCOPE",
    "BundleSignature",
    "BundleSignatureVerificationResult",
    "EvalSignature",
    "EvalSigningKey",
    "EvalTrustStore",
    "EvalTrustedKey",
    "TrustStatus",
    "sign_bundle",
    "verify_bundle_signature",
]
