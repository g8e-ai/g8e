# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the eval-run signing identity and assessed trust (P2-02).

Verifies the dedicated Ed25519 eval-run signing identity, signing of the
canonical manifest root and checksum root, protocol-owned public-key
metadata, external assessed-trust input, and fail-closed verification for
unknown, revoked, expired, wrong-scope, wrong-algorithm, malformed,
duplicate, and substituted keys and signatures.

The verifier never trusts a key merely because the bundle contains it.
Trust originates exclusively from the externally supplied ``EvalTrustStore``.
"""

from __future__ import annotations

import hashlib
from datetime import UTC, datetime, timedelta

import pytest
from pydantic import ValidationError

pytestmark = pytest.mark.unit

from g8e_evals.bundle import (
    ArtifactType,
    BundleArtifactEntry,
    BundleManifest,
    ChecksumEntry,
    ChecksumRoot,
    PrivacyClass,
    canonical_checksum_root_bytes,
    canonical_manifest_bytes,
    compute_checksum_root_hash,
    compute_manifest_hash,
)
from g8e_evals.bundle.signing import (
    EVAL_SIGNING_ALGORITHM,
    EVAL_TRUST_SCOPE,
    BundleSignature,
    BundleSignatureVerificationResult,
    EvalSignature,
    EvalSigningKey,
    EvalTrustStore,
    EvalTrustedKey,
    TrustStatus,
    sign_bundle,
    verify_bundle_signature,
)


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _public_entry(
    path: str = "manifest.json",
    artifact_type: ArtifactType = ArtifactType.RUN_MANIFEST,
    content: bytes = b'{"test": true}',
) -> BundleArtifactEntry:
    return BundleArtifactEntry(
        path=path,
        media_type="application/json",
        privacy_class=PrivacyClass.PUBLIC,
        sha256=_sha256(content),
        byte_length=len(content),
        artifact_type=artifact_type,
        record_count=1,
    )


def _minimal_manifest(artifacts: list[BundleArtifactEntry] | None = None) -> BundleManifest:
    return BundleManifest(
        bundle_id="bundle-1",
        run_id="run-1",
        release_version="v2.1.8",
        created_at=_TS,
        artifacts=artifacts or [_public_entry()],
    )


def _minimal_checksum_root(manifest: BundleManifest) -> ChecksumRoot:
    return ChecksumRoot(
        entries=[
            ChecksumEntry(path=e.path, sha256=e.sha256)
            for e in manifest.artifacts
        ],
    )


def _trusted_key(signing_key: EvalSigningKey) -> EvalTrustedKey:
    return EvalTrustedKey(
        key_id=signing_key.key_id,
        public_key_hex=signing_key.public_key_hex,
        algorithm=EVAL_SIGNING_ALGORITHM,
        scope=EVAL_TRUST_SCOPE,
        valid_from=_TS,
        valid_until=_TS_PLUS,
        revoked=False,
        source="protocol-owned-test-metadata",
    )


# ---------------------------------------------------------------------------
# EvalSigningKey tests (Step 1)
# ---------------------------------------------------------------------------


class TestEvalSigningKey:
    """The dedicated Ed25519 eval-run signing identity is not an actuator receipt."""

    def test_generate_produces_distinct_keys(self) -> None:
        a = EvalSigningKey.generate()
        b = EvalSigningKey.generate()
        assert a.key_id != b.key_id
        assert a.public_key_hex != b.public_key_hex

    def test_key_id_is_hex_of_public_key(self) -> None:
        signing_key = EvalSigningKey.generate()
        assert signing_key.key_id == signing_key.public_key_hex
        assert len(signing_key.key_id) == 64
        int(signing_key.key_id, 16)  # valid hex

    def test_from_seed_is_deterministic(self) -> None:
        seed = b"\x01" * 32
        a = EvalSigningKey.from_seed(seed)
        b = EvalSigningKey.from_seed(seed)
        assert a.key_id == b.key_id
        assert a.public_key_hex == b.public_key_hex

    def test_sign_returns_hex_signature(self) -> None:
        signing_key = EvalSigningKey.generate()
        signature = signing_key.sign(b"message")
        assert isinstance(signature, str)
        int(signature, 16)  # valid hex
        assert len(signature) == 128  # Ed25519 signature is 64 bytes

    def test_sign_is_deterministic_for_same_key_and_message(self) -> None:
        signing_key = EvalSigningKey.from_seed(b"\x02" * 32)
        sig_a = signing_key.sign(b"message")
        sig_b = signing_key.sign(b"message")
        assert sig_a == sig_b

    def test_sign_differs_for_different_messages(self) -> None:
        signing_key = EvalSigningKey.generate()
        assert signing_key.sign(b"a") != signing_key.sign(b"b")

    def test_seed_must_be_32_bytes(self) -> None:
        with pytest.raises(ValueError, match="seed"):
            EvalSigningKey.from_seed(b"\x01" * 31)


# ---------------------------------------------------------------------------
# EvalSignature model tests (Step 2)
# ---------------------------------------------------------------------------


class TestEvalSignature:
    """Each signature binds algorithm, key ID, bundle identity, and signed digest."""

    def test_frozen_model_rejects_mutation(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        digest = compute_manifest_hash(manifest)
        signature = EvalSignature(
            algorithm=EVAL_SIGNING_ALGORITHM,
            key_id=signing_key.key_id,
            bundle_id="bundle-1",
            run_id="run-1",
            release_version="v2.1.8",
            signed_digest=digest,
            signature=signing_key.sign(canonical_manifest_bytes(manifest)),
            signed_at=_TS,
        )
        with pytest.raises(ValidationError):
            signature.key_id = "other"

    def test_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalSignature.model_validate({
                "algorithm": EVAL_SIGNING_ALGORITHM,
                "key_id": "k" * 64,
                "bundle_id": "bundle-1",
                "run_id": "run-1",
                "release_version": "v2.1.8",
                "signed_digest": "d" * 64,
                "signature": "s" * 128,
                "signed_at": _TS.isoformat(),
                "unknown_field": "bad",
            })

    def test_invalid_signature_hex_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalSignature(
                algorithm=EVAL_SIGNING_ALGORITHM,
                key_id="k" * 64,
                bundle_id="bundle-1",
                run_id="run-1",
                release_version="v2.1.8",
                signed_digest="d" * 64,
                signature="not-hex",
                signed_at=_TS,
            )

    def test_invalid_signed_digest_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalSignature(
                algorithm=EVAL_SIGNING_ALGORITHM,
                key_id="k" * 64,
                bundle_id="bundle-1",
                run_id="run-1",
                release_version="v2.1.8",
                signed_digest="not-a-hash",
                signature="s" * 128,
                signed_at=_TS,
            )

    def test_empty_key_id_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalSignature(
                algorithm=EVAL_SIGNING_ALGORITHM,
                key_id="",
                bundle_id="bundle-1",
                run_id="run-1",
                release_version="v2.1.8",
                signed_digest="d" * 64,
                signature="s" * 128,
                signed_at=_TS,
            )

    def test_wrong_algorithm_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalSignature(
                algorithm="rsa-2048",
                key_id="k" * 64,
                bundle_id="bundle-1",
                run_id="run-1",
                release_version="v2.1.8",
                signed_digest="d" * 64,
                signature="s" * 128,
                signed_at=_TS,
            )


# ---------------------------------------------------------------------------
# BundleSignature model tests (Step 2)
# ---------------------------------------------------------------------------


class TestBundleSignature:
    """The bundle signature carries manifest and checksum-root signatures."""

    def test_frozen_model_rejects_mutation(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        with pytest.raises(ValidationError):
            bundle_sig.manifest_signature = bundle_sig.checksum_root_signature  # type: ignore[misc]

    def test_extra_field_rejected(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        data = bundle_sig.model_dump(mode="json")
        data["unknown_field"] = "bad"
        with pytest.raises(ValidationError):
            BundleSignature.model_validate(data)


# ---------------------------------------------------------------------------
# sign_bundle tests (Step 2)
# ---------------------------------------------------------------------------


class TestSignBundle:
    """sign_bundle signs the canonical manifest root and checksum root."""

    def test_signs_manifest_and_checksum_root(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)

        assert bundle_sig.manifest_signature.key_id == signing_key.key_id
        assert bundle_sig.checksum_root_signature.key_id == signing_key.key_id
        assert bundle_sig.manifest_signature.algorithm == EVAL_SIGNING_ALGORITHM
        assert bundle_sig.checksum_root_signature.algorithm == EVAL_SIGNING_ALGORITHM

    def test_manifest_signed_digest_matches_canonical_hash(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)

        assert bundle_sig.manifest_signature.signed_digest == compute_manifest_hash(manifest)
        assert bundle_sig.checksum_root_signature.signed_digest == compute_checksum_root_hash(checksum_root)

    def test_signature_verifies_against_signing_key(self) -> None:
        """The produced signature is a valid Ed25519 signature over canonical bytes."""
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)

        assert signing_key.verify(
            canonical_manifest_bytes(manifest),
            bundle_sig.manifest_signature.signature,
        )
        assert signing_key.verify(
            canonical_checksum_root_bytes(checksum_root),
            bundle_sig.checksum_root_signature.signature,
        )

    def test_signature_binds_bundle_identity(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)

        assert bundle_sig.manifest_signature.bundle_id == manifest.bundle_id
        assert bundle_sig.manifest_signature.run_id == manifest.run_id
        assert bundle_sig.manifest_signature.release_version == manifest.release_version
        assert bundle_sig.checksum_root_signature.bundle_id == manifest.bundle_id
        assert bundle_sig.checksum_root_signature.run_id == manifest.run_id
        assert bundle_sig.checksum_root_signature.release_version == manifest.release_version

    def test_different_manifests_produce_different_signatures(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest_a = _minimal_manifest([_public_entry(path="a.json", content=b"a")])
        manifest_b = _minimal_manifest([_public_entry(path="a.json", content=b"b")])
        checksum_a = _minimal_checksum_root(manifest_a)
        checksum_b = _minimal_checksum_root(manifest_b)
        sig_a = sign_bundle(manifest_a, checksum_a, signing_key, _TS)
        sig_b = sign_bundle(manifest_b, checksum_b, signing_key, _TS)
        assert sig_a.manifest_signature.signature != sig_b.manifest_signature.signature
        assert sig_a.manifest_signature.signed_digest != sig_b.manifest_signature.signed_digest

    def test_different_keys_produce_different_signatures(self) -> None:
        key_a = EvalSigningKey.generate()
        key_b = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        sig_a = sign_bundle(manifest, checksum_root, key_a, _TS)
        sig_b = sign_bundle(manifest, checksum_root, key_b, _TS)
        assert sig_a.manifest_signature.key_id != sig_b.manifest_signature.key_id
        assert sig_a.manifest_signature.signature != sig_b.manifest_signature.signature


# ---------------------------------------------------------------------------
# EvalTrustedKey tests (Step 3)
# ---------------------------------------------------------------------------


class TestEvalTrustedKey:
    """Protocol-owned public-key metadata is a frozen typed model."""

    def test_frozen_model_rejects_mutation(self) -> None:
        signing_key = EvalSigningKey.generate()
        trusted = _trusted_key(signing_key)
        with pytest.raises(ValidationError):
            trusted.revoked = True  # type: ignore[misc]

    def test_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalTrustedKey.model_validate({
                "key_id": "k" * 64,
                "public_key_hex": "p" * 64,
                "algorithm": EVAL_SIGNING_ALGORITHM,
                "scope": EVAL_TRUST_SCOPE,
                "valid_from": _TS.isoformat(),
                "valid_until": _TS_PLUS.isoformat(),
                "revoked": False,
                "source": "test",
                "unknown_field": "bad",
            })

    def test_invalid_public_key_hex_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalTrustedKey(
                key_id="k" * 64,
                public_key_hex="not-hex",
                algorithm=EVAL_SIGNING_ALGORITHM,
                scope=EVAL_TRUST_SCOPE,
                valid_from=_TS,
                valid_until=_TS_PLUS,
                revoked=False,
                source="test",
            )

    def test_wrong_algorithm_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalTrustedKey(
                key_id="k" * 64,
                public_key_hex="p" * 64,
                algorithm="rsa-2048",
                scope=EVAL_TRUST_SCOPE,
                valid_from=_TS,
                valid_until=_TS_PLUS,
                revoked=False,
                source="test",
            )

    def test_empty_source_rejected(self) -> None:
        with pytest.raises(ValidationError):
            EvalTrustedKey(
                key_id="k" * 64,
                public_key_hex="p" * 64,
                algorithm=EVAL_SIGNING_ALGORITHM,
                scope=EVAL_TRUST_SCOPE,
                valid_from=_TS,
                valid_until=_TS_PLUS,
                revoked=False,
                source="",
            )


# ---------------------------------------------------------------------------
# EvalTrustStore tests (Step 3)
# ---------------------------------------------------------------------------


class TestEvalTrustStore:
    """The trust store is the external assessed-trust input, not the bundle."""

    def test_empty_trust_store_has_no_keys(self) -> None:
        store = EvalTrustStore(keys=[])
        assert store.lookup("k" * 64) is None

    def test_lookup_finds_key_by_id(self) -> None:
        signing_key = EvalSigningKey.generate()
        trusted = _trusted_key(signing_key)
        store = EvalTrustStore(keys=[trusted])
        assert store.lookup(signing_key.key_id) is trusted

    def test_duplicate_key_ids_rejected(self) -> None:
        signing_key = EvalSigningKey.generate()
        trusted = _trusted_key(signing_key)
        with pytest.raises(ValueError, match="duplicate"):
            EvalTrustStore(keys=[trusted, trusted])

    def test_trust_store_is_frozen(self) -> None:
        signing_key = EvalSigningKey.generate()
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])
        with pytest.raises(ValidationError):
            store.keys = []  # type: ignore[misc]


# ---------------------------------------------------------------------------
# verify_bundle_signature tests (Step 4)
# ---------------------------------------------------------------------------


class TestVerifyBundleSignature:
    """Verification fails closed for every trust and signature failure mode."""

    def test_valid_signature_with_trusted_key_verifies(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.TRUSTED
        assert result.checksum_root_status == TrustStatus.TRUSTED
        assert result.ok is True

    def test_unknown_key_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        other_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        # Trust store contains a different key, not the signing key.
        store = EvalTrustStore(keys=[_trusted_key(other_key)])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.UNKNOWN
        assert result.checksum_root_status == TrustStatus.UNKNOWN
        assert result.ok is False

    def test_revoked_key_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        trusted = _trusted_key(signing_key).model_copy(update={"revoked": True})
        store = EvalTrustStore(keys=[trusted])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.REVOKED
        assert result.checksum_root_status == TrustStatus.REVOKED
        assert result.ok is False

    def test_expired_key_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        # valid_until is before the signature's signed_at.
        expired = _trusted_key(signing_key).model_copy(
            update={"valid_until": _TS - timedelta(seconds=1)},
        )
        store = EvalTrustStore(keys=[expired])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.EXPIRED
        assert result.checksum_root_status == TrustStatus.EXPIRED
        assert result.ok is False

    def test_not_yet_valid_key_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        # valid_from is after the signature's signed_at.
        not_yet = _trusted_key(signing_key).model_copy(
            update={"valid_from": _TS + timedelta(days=1)},
        )
        store = EvalTrustStore(keys=[not_yet])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.EXPIRED
        assert result.checksum_root_status == TrustStatus.EXPIRED
        assert result.ok is False

    def test_wrong_scope_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        wrong_scope = _trusted_key(signing_key).model_copy(
            update={"scope": "other-scope"},
        )
        store = EvalTrustStore(keys=[wrong_scope])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.WRONG_SCOPE
        assert result.checksum_root_status == TrustStatus.WRONG_SCOPE
        assert result.ok is False

    def test_wrong_algorithm_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        # The trust store key declares a different algorithm than the signature.
        wrong_algo = _trusted_key(signing_key).model_copy(
            update={"algorithm": "ed448"},
        )
        store = EvalTrustStore(keys=[wrong_algo])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.WRONG_ALGORITHM
        assert result.checksum_root_status == TrustStatus.WRONG_ALGORITHM
        assert result.ok is False

    def test_substituted_public_key_fails_closed(self) -> None:
        """The signature verifies against the signing key, but the trust store
        carries a different public key for the same key_id. Verification must
        fail because the signature does not verify against the trusted key."""
        signing_key = EvalSigningKey.generate()
        impostor = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        substituted = _trusted_key(signing_key).model_copy(
            update={"public_key_hex": impostor.public_key_hex},
        )
        store = EvalTrustStore(keys=[substituted])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.SUBSTITUTED
        assert result.checksum_root_status == TrustStatus.SUBSTITUTED
        assert result.ok is False

    def test_malformed_signature_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        malformed = bundle_sig.model_copy(
            update={
                "manifest_signature": bundle_sig.manifest_signature.model_copy(
                    update={"signature": "ff" * 64},
                ),
            },
        )
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(malformed, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.MALFORMED
        assert result.checksum_root_status == TrustStatus.TRUSTED
        assert result.ok is False

    def test_signed_digest_mismatch_fails_closed(self) -> None:
        """The signature is valid but the signed_digest does not match the
        recomputed canonical hash. This detects a manifest swapped after signing."""
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        # Swap the manifest for a different one with the same identity fields.
        swapped_manifest = _minimal_manifest(
            [_public_entry(path="a.json", content=b"different")],
        )
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(bundle_sig, swapped_manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.SUBSTITUTED
        assert result.ok is False

    def test_bundle_identity_mismatch_fails_closed(self) -> None:
        """The signature binds bundle_id/run_id/release_version; a mismatch fails."""
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        mismatched = bundle_sig.model_copy(
            update={
                "manifest_signature": bundle_sig.manifest_signature.model_copy(
                    update={"bundle_id": "other-bundle"},
                ),
            },
        )
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(mismatched, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.SUBSTITUTED
        assert result.ok is False

    def test_run_id_mismatch_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        mismatched = bundle_sig.model_copy(
            update={
                "manifest_signature": bundle_sig.manifest_signature.model_copy(
                    update={"run_id": "other-run"},
                ),
            },
        )
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(mismatched, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.SUBSTITUTED
        assert result.ok is False

    def test_release_version_mismatch_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        mismatched = bundle_sig.model_copy(
            update={
                "manifest_signature": bundle_sig.manifest_signature.model_copy(
                    update={"release_version": "v9.9.9"},
                ),
            },
        )
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(mismatched, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.SUBSTITUTED
        assert result.ok is False

    def test_checksum_root_digest_mismatch_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        swapped_checksum = ChecksumRoot(
            entries=[ChecksumEntry(path="other.json", sha256=_sha256(b"other"))],
        )
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(bundle_sig, manifest, swapped_checksum, store)

        assert result.checksum_root_status == TrustStatus.SUBSTITUTED
        assert result.ok is False

    def test_empty_trust_store_fails_closed(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        store = EvalTrustStore(keys=[])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.manifest_status == TrustStatus.UNKNOWN
        assert result.ok is False

    def test_verification_result_is_frozen(self) -> None:
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        store = EvalTrustStore(keys=[_trusted_key(signing_key)])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        with pytest.raises(ValidationError):
            result.manifest_status = TrustStatus.UNKNOWN  # type: ignore[misc]

    def test_verification_result_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleSignatureVerificationResult.model_validate({
                "manifest_status": "trusted",
                "checksum_root_status": "trusted",
                "ok": True,
                "unknown_field": "bad",
            })


# ---------------------------------------------------------------------------
# In-bundle key is never trusted (Step 3 invariant)
# ---------------------------------------------------------------------------


class TestInBundleKeyNeverTrusted:
    """A key embedded in the bundle is never trusted by the verifier."""

    def test_signature_with_key_not_in_trust_store_fails_even_if_valid(self) -> None:
        """Even if the signature is cryptographically valid, verification fails
        when the key is not in the external trust store. The verifier never
        trusts a key merely because the bundle contains it."""
        signing_key = EvalSigningKey.generate()
        manifest = _minimal_manifest()
        checksum_root = _minimal_checksum_root(manifest)
        bundle_sig = sign_bundle(manifest, checksum_root, signing_key, _TS)
        # Empty trust store: no key is trusted.
        store = EvalTrustStore(keys=[])

        result = verify_bundle_signature(bundle_sig, manifest, checksum_root, store)

        assert result.ok is False
        assert result.manifest_status == TrustStatus.UNKNOWN
