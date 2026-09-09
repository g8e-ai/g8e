# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Eval bundle production: create an immutable bundle from a report directory.

Reads every artifact in a report directory, builds a typed versioned
``BundleManifest`` and ``ChecksumRoot``, optionally signs them with a
dedicated Ed25519 eval-run signing identity, and writes the bundle to a
directory. The bundle is the immutable contract that offline verification
checks against.
"""

from __future__ import annotations

import hashlib
import shutil
from datetime import datetime
from pathlib import Path, PurePosixPath

from g8e_evals import constants as evals_constants
from g8e_evals.bundle.canonical import (
    compute_checksum_root_hash,
    compute_manifest_hash,
)
from g8e_evals.bundle.manifest import (
    BUNDLE_MANIFEST_SCHEMA_VERSION,
    ArtifactType,
    BundleArtifactEntry,
    BundleManifest,
    ChecksumEntry,
    ChecksumRoot,
    PrivacyClass,
)
from g8e_evals.bundle.signing import EvalSigningKey, sign_bundle
from g8e_evals.schema import EvidenceEncryption


# Mapping from report-directory filenames to (ArtifactType, PrivacyClass).
# Files not in this mapping are included as OBSERVATION_FILE/PUBLIC by
# default, or DIAGNOSTIC/INTERNAL if they are the diagnostic results file.
_ARTIFACT_TYPE_MAP: dict[str, tuple[ArtifactType, PrivacyClass]] = {
    evals_constants.MANIFEST_JSON: (ArtifactType.RUN_MANIFEST, PrivacyClass.PUBLIC),
    evals_constants.TASKS_JSONL: (ArtifactType.TASKS, PrivacyClass.PUBLIC),
    evals_constants.ATTEMPTS_JSONL: (ArtifactType.ATTEMPTS, PrivacyClass.PUBLIC),
    evals_constants.RECEIPTS_JSONL: (ArtifactType.RECEIPTS, PrivacyClass.PUBLIC),
    evals_constants.STAGES_JSONL: (ArtifactType.STAGES, PrivacyClass.PUBLIC),
    evals_constants.METRICS_JSONL: (ArtifactType.METRICS, PrivacyClass.PUBLIC),
    evals_constants.EVIDENCE_INDEX_JSONL: (ArtifactType.EVIDENCE_INDEX, PrivacyClass.RESTRICTED),
    evals_constants.ANALYSIS_INPUT_JSON: (ArtifactType.ANALYSIS_INPUT, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_JSON: (ArtifactType.ANALYSIS_JSON, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_MD: (ArtifactType.ANALYSIS_MD, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_HTML: (ArtifactType.ANALYSIS_HTML, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_TXT: (ArtifactType.ANALYSIS_TXT, PrivacyClass.PUBLIC),
    evals_constants.DIAGNOSTIC_RESULTS_JSONL: (ArtifactType.DIAGNOSTIC, PrivacyClass.INTERNAL),
}

# Observation JSONL files that map to OBSERVATION_FILE/PUBLIC.
_OBSERVATION_FILES = {
    evals_constants.FINAL_STATE_OBSERVATIONS_JSONL,
    evals_constants.STATE_OBSERVATIONS_JSONL,
    evals_constants.REHYDRATION_OBSERVATIONS_JSONL,
    evals_constants.SECRET_DETECTION_OBSERVATIONS_JSONL,
    evals_constants.UNAUTHORIZED_MUTATION_OBSERVATIONS_JSONL,
    evals_constants.TOKEN_STORE_PERSISTENCE_OBSERVATIONS_JSONL,
    evals_constants.TOKEN_TTL_EXPIRY_OBSERVATIONS_JSONL,
    evals_constants.TOKEN_PERSISTENCE_FAILURE_OBSERVATIONS_JSONL,
    evals_constants.EXFILTRATION_ATTEMPT_OBSERVATIONS_JSONL,
    evals_constants.ARTIFACT_LEAKAGE_OBSERVATIONS_JSONL,
    evals_constants.REPLAY_ATTEMPT_OBSERVATIONS_JSONL,
    evals_constants.SIGNED_FIELD_TAMPERING_OBSERVATIONS_JSONL,
    evals_constants.PAYLOAD_TAMPERING_OBSERVATIONS_JSONL,
    evals_constants.STALE_STATE_ROOT_OBSERVATIONS_JSONL,
    evals_constants.IDENTITY_MISMATCH_OBSERVATIONS_JSONL,
    evals_constants.NONCE_EXPIRATION_OBSERVATIONS_JSONL,
    evals_constants.SIGNER_DEFECT_OBSERVATIONS_JSONL,
    evals_constants.L3_PROOF_TRANSPLANT_OBSERVATIONS_JSONL,
    evals_constants.REVOKED_CREDENTIAL_OBSERVATIONS_JSONL,
    evals_constants.EVIDENCE_PRESERVATION_OBSERVATIONS_JSONL,
    evals_constants.POLICY_ATTACK_OBSERVATIONS_JSONL,
    evals_constants.TOOL_SEQUENCE_OBSERVATIONS_JSONL,
    evals_constants.FACTUAL_QA_OBSERVATIONS_JSONL,
    evals_constants.CITATION_BACKED_OBSERVATIONS_JSONL,
    evals_constants.PARTIAL_MILESTONE_OBSERVATIONS_JSONL,
    evals_constants.RELIABILITY_OBSERVATIONS_JSONL,
    evals_constants.ECONOMICS_PERFORMANCE_OBSERVATIONS_JSONL,
}

# Media type by file extension.
_MEDIA_TYPES = {
    ".json": "application/json",
    ".jsonl": "application/x-jsonlines",
    ".md": "text/markdown",
    ".html": "text/html",
    ".txt": "text/plain",
}


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _media_type(filename: str) -> str:
    ext = PurePosixPath(filename).suffix
    return _MEDIA_TYPES.get(ext, "application/octet-stream")


def _artifact_type_and_privacy(filename: str) -> tuple[ArtifactType, PrivacyClass]:
    if filename in _ARTIFACT_TYPE_MAP:
        return _ARTIFACT_TYPE_MAP[filename]
    if filename in _OBSERVATION_FILES:
        return (ArtifactType.OBSERVATION_FILE, PrivacyClass.PUBLIC)
    return (ArtifactType.DIAGNOSTIC, PrivacyClass.INTERNAL)


def _count_jsonl_records(content: bytes) -> int:
    """Count non-empty lines in a JSONL file."""
    return sum(1 for line in content.splitlines() if line.strip())


def _build_artifact_entry(
    filename: str,
    content: bytes,
    encryption: EvidenceEncryption | None = None,
) -> BundleArtifactEntry:
    artifact_type, privacy_class = _artifact_type_and_privacy(filename)
    record_count = _count_jsonl_records(content) if filename.endswith(".jsonl") else 0
    return BundleArtifactEntry(
        path=filename,
        media_type=_media_type(filename),
        privacy_class=privacy_class,
        sha256=_sha256(content),
        byte_length=len(content),
        artifact_type=artifact_type,
        record_count=record_count,
        encryption=encryption,
    )


def produce_bundle(
    report_dir: Path,
    bundle_dir: Path,
    bundle_id: str,
    run_id: str,
    release_version: str,
    signing_key: EvalSigningKey | None = None,
    created_at: datetime | None = None,
    restricted_encryption: EvidenceEncryption | None = None,
) -> BundleManifest:
    """Create an immutable bundle from a report directory.

    Reads every regular file in ``report_dir``, builds a typed
    ``BundleManifest`` and ``ChecksumRoot``, optionally signs them, and
    writes the bundle to ``bundle_dir``. Returns the manifest.
    """
    report_dir = Path(report_dir)
    bundle_dir = Path(bundle_dir)
    if created_at is None:
        from datetime import UTC, datetime
        created_at = datetime.now(UTC)

    bundle_dir.mkdir(parents=True, exist_ok=True)

    # Collect all regular files in the report directory (no subdirectories).
    files: list[Path] = sorted(
        f for f in report_dir.iterdir() if f.is_file() and not f.is_symlink()
    )

    # Build artifact entries.
    entries: list[BundleArtifactEntry] = []
    for file_path in files:
        content = file_path.read_bytes()
        filename = file_path.name
        encryption = restricted_encryption if _artifact_type_and_privacy(filename)[1] == PrivacyClass.RESTRICTED else None
        entry = _build_artifact_entry(filename, content, encryption)
        entries.append(entry)

    # Build manifest with empty self-hash fields.
    manifest = BundleManifest(
        schema_version=BUNDLE_MANIFEST_SCHEMA_VERSION,
        bundle_id=bundle_id,
        run_id=run_id,
        release_version=release_version,
        created_at=created_at,
        artifacts=entries,
        external_references=[],
        manifest_content_sha256="",
        checksum_root_sha256="",
    )

    # Build checksum root with empty self-hash, compute hash, then set it.
    checksum_entries = [
        ChecksumEntry(path=e.path, sha256=e.sha256) for e in entries
    ]
    checksum_root = ChecksumRoot(
        schema_version=BUNDLE_MANIFEST_SCHEMA_VERSION,
        entries=checksum_entries,
        checksum_root_sha256="",
    )
    checksum_hash = compute_checksum_root_hash(checksum_root)
    checksum_root = checksum_root.model_copy(update={"checksum_root_sha256": checksum_hash})

    # Set checksum root hash on manifest BEFORE computing manifest hash,
    # because canonical_manifest_bytes only excludes manifest_content_sha256,
    # not checksum_root_sha256. The manifest hash must cover the checksum root hash.
    manifest = manifest.model_copy(update={"checksum_root_sha256": checksum_hash})
    manifest_hash = compute_manifest_hash(manifest)
    manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})

    # Write all files to the bundle directory.
    for file_path in files:
        shutil.copy2(file_path, bundle_dir / file_path.name)

    # Write manifest and checksum root.
    from g8e_evals.analysis.canonical import canonical_model_json
    (bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON).write_text(canonical_model_json(manifest))
    (bundle_dir / evals_constants.CHECKSUM_ROOT_JSON).write_text(canonical_model_json(checksum_root))

    # Sign and write signature if a signing key is provided.
    if signing_key is not None:
        signature = sign_bundle(manifest, checksum_root, signing_key, created_at)
        (bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON).write_text(canonical_model_json(signature))

    return manifest


__all__ = [
    "produce_bundle",
]
