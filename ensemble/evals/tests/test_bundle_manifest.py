# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the immutable eval bundle manifest (P2-01).

Verifies the typed, versioned bundle manifest model, public/restricted
artifact class separation, path rooting and safety, deterministic
canonicalization, and roundtrip/order-independence/duplicate/orphan/
missing-file/unknown-field/path-traversal rejection.
"""

from __future__ import annotations

import hashlib
import json
from datetime import UTC, datetime
from pathlib import Path

import pytest
from pydantic import ValidationError

pytestmark = pytest.mark.integration

from g8e_evals.bundle import (
    BUNDLE_MANIFEST_SCHEMA_VERSION,
    ArtifactType,
    BundleArtifactEntry,
    BundleManifest,
    BundleManifestValidationError,
    BundlePathError,
    ChecksumEntry,
    ChecksumRoot,
    ExternalReference,
    PrivacyClass,
    canonical_checksum_root_bytes,
    canonical_manifest_bytes,
    compute_checksum_root_hash,
    compute_manifest_hash,
    validate_bundle_manifest,
    validate_bundle_path,
    validate_no_orphans,
    validate_no_missing_files,
)
from g8e_evals.schema import EvidenceEncryption, EvidenceEncryptionAlgorithm


_TS = datetime(2026, 1, 1, tzinfo=UTC)


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


def _restricted_entry(
    path: str = "evidence/raw-prompt.enc",
    content: bytes = b"encrypted",
) -> BundleArtifactEntry:
    return BundleArtifactEntry(
        path=path,
        media_type="application/octet-stream",
        privacy_class=PrivacyClass.RESTRICTED,
        sha256=_sha256(content),
        byte_length=len(content),
        artifact_type=ArtifactType.EVIDENCE_INDEX,
        record_count=0,
        encryption=EvidenceEncryption(
            algorithm=EvidenceEncryptionAlgorithm.AES_256_GCM,
            key_id="key-1",
            aad_sha256=_sha256(b"aad"),
            ciphertext_sha256=_sha256(b"ciphertext"),
            ciphertext_byte_length=100,
        ),
    )


def _minimal_manifest(artifacts: list[BundleArtifactEntry] | None = None) -> BundleManifest:
    return BundleManifest(
        bundle_id="bundle-1",
        run_id="run-1",
        release_version="v2.1.8",
        created_at=_TS,
        artifacts=artifacts or [_public_entry()],
    )


# ---------------------------------------------------------------------------
# BundleArtifactEntry model tests
# ---------------------------------------------------------------------------


class TestBundleArtifactEntry:
    """The artifact entry is a frozen, extra-forbid typed model."""

    def test_frozen_model_rejects_mutation(self) -> None:
        entry = _public_entry()
        with pytest.raises(ValidationError):
            entry.path = "other.json"

    def test_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleArtifactEntry.model_validate({
                "path": "manifest.json",
                "media_type": "application/json",
                "privacy_class": "public",
                "sha256": _sha256(b"x"),
                "byte_length": 1,
                "artifact_type": "run_manifest",
                "record_count": 1,
                "unknown_field": "bad",
            })

    def test_invalid_sha256_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleArtifactEntry(
                path="manifest.json",
                media_type="application/json",
                privacy_class=PrivacyClass.PUBLIC,
                sha256="not-a-hash",
                byte_length=1,
                artifact_type=ArtifactType.RUN_MANIFEST,
                record_count=1,
            )

    def test_negative_byte_length_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleArtifactEntry(
                path="manifest.json",
                media_type="application/json",
                privacy_class=PrivacyClass.PUBLIC,
                sha256=_sha256(b"x"),
                byte_length=-1,
                artifact_type=ArtifactType.RUN_MANIFEST,
                record_count=1,
            )

    def test_negative_record_count_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleArtifactEntry(
                path="manifest.json",
                media_type="application/json",
                privacy_class=PrivacyClass.PUBLIC,
                sha256=_sha256(b"x"),
                byte_length=1,
                artifact_type=ArtifactType.RUN_MANIFEST,
                record_count=-1,
            )

    def test_empty_path_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleArtifactEntry(
                path="",
                media_type="application/json",
                privacy_class=PrivacyClass.PUBLIC,
                sha256=_sha256(b"x"),
                byte_length=1,
                artifact_type=ArtifactType.RUN_MANIFEST,
                record_count=1,
            )

    def test_empty_media_type_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleArtifactEntry(
                path="manifest.json",
                media_type="",
                privacy_class=PrivacyClass.PUBLIC,
                sha256=_sha256(b"x"),
                byte_length=1,
                artifact_type=ArtifactType.RUN_MANIFEST,
                record_count=1,
            )

    def test_encryption_optional_for_public_artifacts(self) -> None:
        entry = _public_entry()
        assert entry.encryption is None

    def test_encryption_stored_for_restricted_artifacts(self) -> None:
        entry = _restricted_entry()
        assert entry.encryption is not None
        assert entry.encryption.algorithm == EvidenceEncryptionAlgorithm.AES_256_GCM


# ---------------------------------------------------------------------------
# BundleManifest model tests
# ---------------------------------------------------------------------------


class TestBundleManifest:
    """The manifest is a frozen, versioned, extra-forbid typed model."""

    def test_frozen_model_rejects_mutation(self) -> None:
        manifest = _minimal_manifest()
        with pytest.raises(ValidationError):
            manifest.bundle_id = "other"

    def test_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleManifest.model_validate({
                "bundle_id": "bundle-1",
                "run_id": "run-1",
                "release_version": "v2.1.8",
                "created_at": _TS.isoformat(),
                "artifacts": [json.loads(_public_entry().model_dump_json())],
                "unknown_field": "bad",
            })

    def test_schema_version_defaults_to_current(self) -> None:
        manifest = _minimal_manifest()
        assert manifest.schema_version == BUNDLE_MANIFEST_SCHEMA_VERSION

    def test_empty_bundle_id_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleManifest(
                bundle_id="",
                run_id="run-1",
                release_version="v2.1.8",
                created_at=_TS,
                artifacts=[_public_entry()],
            )

    def test_empty_run_id_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleManifest(
                bundle_id="bundle-1",
                run_id="",
                release_version="v2.1.8",
                created_at=_TS,
                artifacts=[_public_entry()],
            )

    def test_empty_release_version_rejected(self) -> None:
        with pytest.raises(ValidationError):
            BundleManifest(
                bundle_id="bundle-1",
                run_id="run-1",
                release_version="",
                created_at=_TS,
                artifacts=[_public_entry()],
            )

    def test_external_references_default_to_empty(self) -> None:
        manifest = _minimal_manifest()
        assert manifest.external_references == []

    def test_manifest_with_external_references(self) -> None:
        ref = ExternalReference(
            reference_id="dataset-ifeval-subset",
            content_sha256=_sha256(b"dataset"),
            byte_length=1000,
            media_type="application/json",
            description="Pinned IFEval subset dataset",
        )
        manifest = BundleManifest(
            bundle_id="bundle-1",
            run_id="run-1",
            release_version="v2.1.8",
            created_at=_TS,
            artifacts=[_public_entry()],
            external_references=[ref],
        )
        assert len(manifest.external_references) == 1
        assert manifest.external_references[0].reference_id == "dataset-ifeval-subset"


# ---------------------------------------------------------------------------
# ExternalReference model tests
# ---------------------------------------------------------------------------


class TestExternalReference:
    """External references are content-addressed references outside the bundle."""

    def test_frozen_model_rejects_mutation(self) -> None:
        ref = ExternalReference(
            reference_id="ref-1",
            content_sha256=_sha256(b"x"),
            byte_length=1,
            media_type="application/json",
        )
        with pytest.raises(ValidationError):
            ref.reference_id = "other"

    def test_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            ExternalReference.model_validate({
                "reference_id": "ref-1",
                "content_sha256": _sha256(b"x"),
                "byte_length": 1,
                "media_type": "application/json",
                "unknown_field": "bad",
            })

    def test_invalid_sha256_rejected(self) -> None:
        with pytest.raises(ValidationError):
            ExternalReference(
                reference_id="ref-1",
                content_sha256="bad",
                byte_length=1,
                media_type="application/json",
            )

    def test_empty_reference_id_rejected(self) -> None:
        with pytest.raises(ValidationError):
            ExternalReference(
                reference_id="",
                content_sha256=_sha256(b"x"),
                byte_length=1,
                media_type="application/json",
            )


# ---------------------------------------------------------------------------
# ArtifactType enum tests
# ---------------------------------------------------------------------------


class TestArtifactType:
    """The artifact type enum covers every semantic record identity."""

    def test_all_required_types_exist(self) -> None:
        required = {
            "run_manifest", "preregistration_manifest", "tasks", "fixtures",
            "attempts", "posture_observations", "state_observations",
            "envelopes", "receipts", "persistence_attestations",
            "commitment_attestations", "stages", "metrics", "audit_links",
            "evidence_index", "analysis_input", "analysis_json",
            "analysis_md", "analysis_html", "analysis_txt", "checksum",
            "bridge_records", "trust_references", "external_references",
            "observation_file", "diagnostic",
        }
        actual = {t.value for t in ArtifactType}
        missing = required - actual
        assert not missing, f"missing artifact types: {missing}"


# ---------------------------------------------------------------------------
# ChecksumRoot and ChecksumEntry tests
# ---------------------------------------------------------------------------


class TestChecksumRoot:
    """The checksum root is a deterministic file listing all paths and hashes."""

    def test_checksum_entry_frozen(self) -> None:
        entry = ChecksumEntry(path="manifest.json", sha256=_sha256(b"x"))
        with pytest.raises(ValidationError):
            entry.path = "other"

    def test_checksum_entry_extra_rejected(self) -> None:
        with pytest.raises(ValidationError):
            ChecksumEntry.model_validate({
                "path": "manifest.json",
                "sha256": _sha256(b"x"),
                "bad": "field",
            })

    def test_checksum_root_frozen(self) -> None:
        root = ChecksumRoot(entries=[ChecksumEntry(path="a", sha256=_sha256(b"a"))])
        with pytest.raises(ValidationError):
            root.entries = []

    def test_checksum_root_extra_rejected(self) -> None:
        with pytest.raises(ValidationError):
            ChecksumRoot.model_validate({
                "entries": [{"path": "a", "sha256": _sha256(b"a")}],
                "unknown": "field",
            })


# ---------------------------------------------------------------------------
# Path validation tests (Step 3)
# ---------------------------------------------------------------------------


class TestPathValidation:
    """Every path is rooted beneath the bundle root and rejects unsafe paths."""

    def test_valid_relative_path_accepted(self) -> None:
        assert validate_bundle_path("manifest.json") == "manifest.json"

    def test_valid_nested_relative_path_accepted(self) -> None:
        assert validate_bundle_path("evidence/raw-prompt.enc") == "evidence/raw-prompt.enc"

    def test_absolute_path_rejected(self) -> None:
        with pytest.raises(BundlePathError, match="absolute"):
            validate_bundle_path("/etc/passwd")

    def test_traversal_rejected(self) -> None:
        with pytest.raises(BundlePathError, match="traversal"):
            validate_bundle_path("../secret")

    def test_traversal_in_middle_rejected(self) -> None:
        with pytest.raises(BundlePathError, match="traversal"):
            validate_bundle_path("a/../../b")

    def test_empty_path_rejected(self) -> None:
        with pytest.raises(BundlePathError, match="empty"):
            validate_bundle_path("")

    def test_backslash_rejected(self) -> None:
        with pytest.raises(BundlePathError, match="backslash"):
            validate_bundle_path("evidence\\raw.enc")

    def test_null_byte_rejected(self) -> None:
        with pytest.raises(BundlePathError, match="null"):
            validate_bundle_path("manifest\x00.json")

    def test_leading_dot_slash_normalized(self) -> None:
        assert validate_bundle_path("./manifest.json") == "manifest.json"

    def test_duplicate_normalized_paths_rejected_in_manifest(self) -> None:
        """Two paths that normalize to the same path are duplicates."""
        entry_a = _public_entry(path="manifest.json")
        entry_b = _public_entry(path="./manifest.json")
        manifest = BundleManifest(
            bundle_id="b",
            run_id="r",
            release_version="v",
            created_at=_TS,
            artifacts=[entry_a, entry_b],
        )
        with pytest.raises(BundleManifestValidationError, match="duplicate"):
            validate_bundle_manifest(manifest)


# ---------------------------------------------------------------------------
# Manifest validation tests (Step 3)
# ---------------------------------------------------------------------------


class TestManifestValidation:
    """The manifest validates paths, rejects duplicates, and enforces types."""

    def test_valid_manifest_accepted(self) -> None:
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST),
            _public_entry(path="tasks.jsonl", artifact_type=ArtifactType.TASKS, content=b'{"task":1}\n'),
            _public_entry(path="analysis.json", artifact_type=ArtifactType.ANALYSIS_JSON, content=b'{"analysis":1}'),
        ])
        validate_bundle_manifest(manifest)

    def test_duplicate_path_rejected(self) -> None:
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json"),
            _public_entry(path="manifest.json"),
        ])
        with pytest.raises(BundleManifestValidationError, match="duplicate"):
            validate_bundle_manifest(manifest)

    def test_absolute_path_in_manifest_rejected(self) -> None:
        entry = BundleArtifactEntry(
            path="/etc/passwd",
            media_type="application/json",
            privacy_class=PrivacyClass.PUBLIC,
            sha256=_sha256(b"x"),
            byte_length=1,
            artifact_type=ArtifactType.RUN_MANIFEST,
            record_count=1,
        )
        manifest = BundleManifest(
            bundle_id="b", run_id="r", release_version="v",
            created_at=_TS, artifacts=[entry],
        )
        with pytest.raises(BundleManifestValidationError, match=r"absolute|path"):
            validate_bundle_manifest(manifest)

    def test_traversal_path_in_manifest_rejected(self) -> None:
        entry = BundleArtifactEntry(
            path="../secret",
            media_type="application/json",
            privacy_class=PrivacyClass.PUBLIC,
            sha256=_sha256(b"x"),
            byte_length=1,
            artifact_type=ArtifactType.RUN_MANIFEST,
            record_count=1,
        )
        manifest = BundleManifest(
            bundle_id="b", run_id="r", release_version="v",
            created_at=_TS, artifacts=[entry],
        )
        with pytest.raises(BundleManifestValidationError, match=r"traversal|path"):
            validate_bundle_manifest(manifest)

    def test_restricted_artifact_without_encryption_rejected(self) -> None:
        """Restricted artifacts must carry encryption metadata."""
        entry = BundleArtifactEntry(
            path="evidence/raw.enc",
            media_type="application/octet-stream",
            privacy_class=PrivacyClass.RESTRICTED,
            sha256=_sha256(b"x"),
            byte_length=1,
            artifact_type=ArtifactType.EVIDENCE_INDEX,
            record_count=0,
            encryption=None,
        )
        manifest = BundleManifest(
            bundle_id="b", run_id="r", release_version="v",
            created_at=_TS, artifacts=[entry],
        )
        with pytest.raises(BundleManifestValidationError, match=r"encryption|restricted"):
            validate_bundle_manifest(manifest)

    def test_restricted_artifact_with_encryption_accepted(self) -> None:
        entry = _restricted_entry()
        manifest = _minimal_manifest([entry])
        validate_bundle_manifest(manifest)

    def test_empty_artifacts_list_rejected(self) -> None:
        """A bundle with no artifacts is invalid at construction time."""
        with pytest.raises(Exception, match="at least 1"):
            BundleManifest(
                bundle_id="b", run_id="r", release_version="v",
                created_at=_TS, artifacts=[],
            )


# ---------------------------------------------------------------------------
# Public/restricted separation tests (Step 2)
# ---------------------------------------------------------------------------


class TestPublicRestrictedSeparation:
    """Public verification cannot require restricted plaintext or decryption keys."""

    def test_public_artifacts_have_public_privacy_class(self) -> None:
        entry = _public_entry()
        assert entry.privacy_class == PrivacyClass.PUBLIC
        assert entry.encryption is None

    def test_restricted_artifacts_have_restricted_privacy_class(self) -> None:
        entry = _restricted_entry()
        assert entry.privacy_class == PrivacyClass.RESTRICTED
        assert entry.encryption is not None

    def test_restricted_artifact_referenced_by_ciphertext_metadata_only(self) -> None:
        """Restricted artifacts expose ciphertext hashes, not plaintext."""
        entry = _restricted_entry()
        assert entry.encryption is not None
        assert entry.encryption.ciphertext_sha256 is not None
        assert entry.encryption.ciphertext_byte_length > 0
        assert entry.encryption.key_id is not None
        assert entry.encryption.algorithm is not None

    def test_public_manifest_does_not_require_decryption_keys(self) -> None:
        """A manifest with only public artifacts has no encryption metadata."""
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST),
            _public_entry(path="analysis.json", artifact_type=ArtifactType.ANALYSIS_JSON, content=b"{}"),
        ])
        validate_bundle_manifest(manifest)
        for entry in manifest.artifacts:
            assert entry.privacy_class == PrivacyClass.PUBLIC
            assert entry.encryption is None

    def test_mixed_manifest_separates_public_and_restricted(self) -> None:
        public = _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST)
        restricted = _restricted_entry()
        manifest = _minimal_manifest([public, restricted])
        validate_bundle_manifest(manifest)
        public_entries = [e for e in manifest.artifacts if e.privacy_class == PrivacyClass.PUBLIC]
        restricted_entries = [e for e in manifest.artifacts if e.privacy_class == PrivacyClass.RESTRICTED]
        assert len(public_entries) == 1
        assert len(restricted_entries) == 1
        assert all(e.encryption is None for e in public_entries)
        assert all(e.encryption is not None for e in restricted_entries)


# ---------------------------------------------------------------------------
# Deterministic canonicalization tests (Step 4)
# ---------------------------------------------------------------------------


class TestCanonicalization:
    """Manifest and checksum root canonicalization is deterministic."""

    def test_roundtrip_manifest_serialization(self) -> None:
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST),
            _public_entry(path="tasks.jsonl", artifact_type=ArtifactType.TASKS, content=b'{"t":1}\n'),
        ])
        canonical = canonical_manifest_bytes(manifest)
        restored = BundleManifest.model_validate_json(canonical)
        assert restored == manifest

    def test_order_independence_manifest_bytes(self) -> None:
        """Different artifact input order produces identical canonical bytes."""
        entry_a = _public_entry(path="a.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b"a")
        entry_b = _public_entry(path="b.json", artifact_type=ArtifactType.TASKS, content=b"b")
        manifest_ab = _minimal_manifest([entry_a, entry_b])
        manifest_ba = _minimal_manifest([entry_b, entry_a])
        assert canonical_manifest_bytes(manifest_ab) == canonical_manifest_bytes(manifest_ba)

    def test_manifest_hash_deterministic(self) -> None:
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST),
            _public_entry(path="tasks.jsonl", artifact_type=ArtifactType.TASKS, content=b'{"t":1}\n'),
        ])
        hash1 = compute_manifest_hash(manifest)
        hash2 = compute_manifest_hash(manifest)
        assert hash1 == hash2
        assert len(hash1) == 64

    def test_manifest_hash_order_independent(self) -> None:
        entry_a = _public_entry(path="a.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b"a")
        entry_b = _public_entry(path="b.json", artifact_type=ArtifactType.TASKS, content=b"b")
        manifest_ab = _minimal_manifest([entry_a, entry_b])
        manifest_ba = _minimal_manifest([entry_b, entry_a])
        assert compute_manifest_hash(manifest_ab) == compute_manifest_hash(manifest_ba)

    def test_manifest_hash_changes_on_content_change(self) -> None:
        entry_a = _public_entry(path="manifest.json", content=b'{"v":1}')
        entry_b = _public_entry(path="manifest.json", content=b'{"v":2}')
        manifest_a = _minimal_manifest([entry_a])
        manifest_b = _minimal_manifest([entry_b])
        assert compute_manifest_hash(manifest_a) != compute_manifest_hash(manifest_b)

    def test_checksum_root_roundtrip(self) -> None:
        root = ChecksumRoot(entries=[
            ChecksumEntry(path="manifest.json", sha256=_sha256(b"m")),
            ChecksumEntry(path="tasks.jsonl", sha256=_sha256(b"t")),
        ])
        canonical = canonical_checksum_root_bytes(root)
        restored = ChecksumRoot.model_validate_json(canonical)
        assert restored == root

    def test_checksum_root_order_independent(self) -> None:
        root_ab = ChecksumRoot(entries=[
            ChecksumEntry(path="a.json", sha256=_sha256(b"a")),
            ChecksumEntry(path="b.json", sha256=_sha256(b"b")),
        ])
        root_ba = ChecksumRoot(entries=[
            ChecksumEntry(path="b.json", sha256=_sha256(b"b")),
            ChecksumEntry(path="a.json", sha256=_sha256(b"a")),
        ])
        assert canonical_checksum_root_bytes(root_ab) == canonical_checksum_root_bytes(root_ba)

    def test_checksum_root_hash_deterministic(self) -> None:
        root = ChecksumRoot(entries=[
            ChecksumEntry(path="manifest.json", sha256=_sha256(b"m")),
        ])
        assert compute_checksum_root_hash(root) == compute_checksum_root_hash(root)
        assert len(compute_checksum_root_hash(root)) == 64

    def test_checksum_root_hash_order_independent(self) -> None:
        root_ab = ChecksumRoot(entries=[
            ChecksumEntry(path="a.json", sha256=_sha256(b"a")),
            ChecksumEntry(path="b.json", sha256=_sha256(b"b")),
        ])
        root_ba = ChecksumRoot(entries=[
            ChecksumEntry(path="b.json", sha256=_sha256(b"b")),
            ChecksumEntry(path="a.json", sha256=_sha256(b"a")),
        ])
        assert compute_checksum_root_hash(root_ab) == compute_checksum_root_hash(root_ba)

    def test_manifest_hash_excludes_hash_field(self) -> None:
        """The manifest hash does not include the manifest_content_sha256 field."""
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST),
        ])
        hash1 = compute_manifest_hash(manifest)
        manifest_with_hash = manifest.model_copy(update={"manifest_content_sha256": hash1})
        hash2 = compute_manifest_hash(manifest_with_hash)
        assert hash1 == hash2


# ---------------------------------------------------------------------------
# Orphan and missing-file tests (Step 4)
# ---------------------------------------------------------------------------


class TestOrphanAndMissingFile:
    """Orphan files and missing files are detected."""

    def test_orphan_file_detected(self, tmp_path: Path) -> None:
        """A file in the bundle directory not listed in the manifest is an orphan."""
        (tmp_path / "manifest.json").write_text('{"v":1}')
        (tmp_path / "orphan.txt").write_text("orphan")
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b'{"v":1}'),
        ])
        with pytest.raises(BundleManifestValidationError, match="orphan"):
            validate_no_orphans(manifest, tmp_path)

    def test_no_orphans_when_all_files_listed(self, tmp_path: Path) -> None:
        (tmp_path / "manifest.json").write_text('{"v":1}')
        (tmp_path / "tasks.jsonl").write_text('{"t":1}\n')
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b'{"v":1}'),
            _public_entry(path="tasks.jsonl", artifact_type=ArtifactType.TASKS, content=b'{"t":1}\n'),
        ])
        validate_no_orphans(manifest, tmp_path)

    def test_missing_file_detected(self, tmp_path: Path) -> None:
        """A file listed in the manifest that does not exist is a missing file."""
        (tmp_path / "manifest.json").write_text('{"v":1}')
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b'{"v":1}'),
            _public_entry(path="missing.json", artifact_type=ArtifactType.ANALYSIS_JSON, content=b"{}"),
        ])
        with pytest.raises(BundleManifestValidationError, match="missing"):
            validate_no_missing_files(manifest, tmp_path)

    def test_no_missing_files_when_all_exist(self, tmp_path: Path) -> None:
        (tmp_path / "manifest.json").write_text('{"v":1}')
        (tmp_path / "tasks.jsonl").write_text('{"t":1}\n')
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b'{"v":1}'),
            _public_entry(path="tasks.jsonl", artifact_type=ArtifactType.TASKS, content=b'{"t":1}\n'),
        ])
        validate_no_missing_files(manifest, tmp_path)

    def test_hash_mismatch_detected(self, tmp_path: Path) -> None:
        """A file whose content hash does not match the manifest is rejected."""
        (tmp_path / "manifest.json").write_text('{"different":true}')
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b'{"v":1}'),
        ])
        with pytest.raises(BundleManifestValidationError, match="hash"):
            validate_no_missing_files(manifest, tmp_path)

    def test_symlink_rejected(self, tmp_path: Path) -> None:
        """Symlinks in the bundle directory are rejected."""
        (tmp_path / "manifest.json").write_text('{"v":1}')
        target = tmp_path / "manifest.json"
        link = tmp_path / "link.json"
        link.symlink_to(target)
        manifest = _minimal_manifest([
            _public_entry(path="manifest.json", artifact_type=ArtifactType.RUN_MANIFEST, content=b'{"v":1}'),
            _public_entry(path="link.json", artifact_type=ArtifactType.ANALYSIS_JSON, content=b'{"v":1}'),
        ])
        with pytest.raises(BundleManifestValidationError, match="symlink"):
            validate_no_missing_files(manifest, tmp_path)


# ---------------------------------------------------------------------------
# Unknown-field rejection tests (Step 4)
# ---------------------------------------------------------------------------


class TestUnknownFieldRejection:
    """Unknown fields in manifest JSON are rejected on deserialization."""

    def test_unknown_field_in_manifest_rejected(self) -> None:
        manifest = _minimal_manifest()
        data = json.loads(manifest.model_dump_json())
        data["unknown_field"] = "bad"
        with pytest.raises(ValidationError):
            BundleManifest.model_validate(data)

    def test_unknown_field_in_artifact_entry_rejected(self) -> None:
        entry = _public_entry()
        data = json.loads(entry.model_dump_json())
        data["unknown_field"] = "bad"
        with pytest.raises(ValidationError):
            BundleArtifactEntry.model_validate(data)

    def test_unknown_field_in_checksum_entry_rejected(self) -> None:
        entry = ChecksumEntry(path="a", sha256=_sha256(b"a"))
        data = json.loads(entry.model_dump_json())
        data["unknown_field"] = "bad"
        with pytest.raises(ValidationError):
            ChecksumEntry.model_validate(data)
