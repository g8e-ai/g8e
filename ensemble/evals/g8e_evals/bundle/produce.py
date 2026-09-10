# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Eval bundle production: create an immutable bundle from a report directory.

Recursively inventories every artifact in a report directory, classifies
each by its normalized relative path and typed report contract, builds a
typed versioned ``BundleManifest`` and ``ChecksumRoot``, optionally signs
them with a dedicated Ed25519 eval-run signing identity, and writes the
bundle to a directory through a staging/atomic-finalize boundary. The
bundle is the immutable contract that offline verification checks
against.

Unknown files fail closed. Symlinks, absolute paths, traversal segments,
backslashes, null bytes, duplicate normalized paths, and source paths
outside the report root are rejected. The destination must be empty or
non-existent; stale files are never mixed with new content.
"""

from __future__ import annotations

import hashlib
import os
import shutil
from datetime import UTC, datetime
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
# Classified by basename for root-level files. Nested evidence files
# under ``evidence/`` are classified by directory prefix.
_ARTIFACT_TYPE_MAP: dict[str, tuple[ArtifactType, PrivacyClass]] = {
    evals_constants.MANIFEST_JSON: (ArtifactType.RUN_MANIFEST, PrivacyClass.PUBLIC),
    evals_constants.TASKS_JSONL: (ArtifactType.TASKS, PrivacyClass.PUBLIC),
    evals_constants.ATTEMPTS_JSONL: (ArtifactType.ATTEMPTS, PrivacyClass.PUBLIC),
    evals_constants.RECEIPTS_JSONL: (ArtifactType.RECEIPTS, PrivacyClass.PUBLIC),
    evals_constants.STAGES_JSONL: (ArtifactType.STAGES, PrivacyClass.PUBLIC),
    evals_constants.METRICS_JSONL: (ArtifactType.METRICS, PrivacyClass.PUBLIC),
    evals_constants.EVIDENCE_INDEX_JSONL: (ArtifactType.EVIDENCE_INDEX, PrivacyClass.INTERNAL),
    evals_constants.ANALYSIS_INPUT_JSON: (ArtifactType.ANALYSIS_INPUT, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_JSON: (ArtifactType.ANALYSIS_JSON, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_MD: (ArtifactType.ANALYSIS_MD, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_HTML: (ArtifactType.ANALYSIS_HTML, PrivacyClass.PUBLIC),
    evals_constants.ANALYSIS_TXT: (ArtifactType.ANALYSIS_TXT, PrivacyClass.PUBLIC),
    evals_constants.DIAGNOSTIC_RESULTS_JSONL: (ArtifactType.DIAGNOSTIC, PrivacyClass.INTERNAL),
    evals_constants.CAMPAIGN_MANIFEST_JSON: (ArtifactType.CAMPAIGN_MANIFEST, PrivacyClass.PUBLIC),
    evals_constants.CAMPAIGN_ASSIGNMENTS_JSONL: (ArtifactType.CAMPAIGN_ASSIGNMENTS, PrivacyClass.PUBLIC),
    evals_constants.CAMPAIGN_COHORTS_JSONL: (ArtifactType.CAMPAIGN_COHORTS, PrivacyClass.PUBLIC),
    evals_constants.CAMPAIGN_SCHEDULE_JSON: (ArtifactType.CAMPAIGN_SCHEDULE, PrivacyClass.PUBLIC),
    evals_constants.CAMPAIGN_RETRY_POLICY_JSON: (ArtifactType.CAMPAIGN_RETRY_POLICY, PrivacyClass.PUBLIC),
    evals_constants.CAMPAIGN_STATUS_JSON: (ArtifactType.CAMPAIGN_STATUS, PrivacyClass.INTERNAL),
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

# Directory prefix for nested encrypted evidence artifacts.
_EVIDENCE_DIR_PREFIX = "evidence/"

# Media type by file extension.
_MEDIA_TYPES = {
    ".json": "application/json",
    ".jsonl": "application/x-jsonlines",
    ".md": "text/markdown",
    ".html": "text/html",
    ".txt": "text/plain",
    ".enc": "application/octet-stream",
}


class BundleProductionError(ValueError):
    """Raised when bundle production encounters an invalid report directory."""


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _media_type(filename: str) -> str:
    ext = PurePosixPath(filename).suffix
    return _MEDIA_TYPES.get(ext, "application/octet-stream")


def _normalize_relative_path(file_path: Path, report_root: Path) -> str:
    """Compute and validate the normalized relative path from the report root.

    Rejects symlinks, absolute paths, backslashes, null bytes, traversal
    segments, and paths outside the report root. Returns the POSIX-style
    relative path.
    """
    # Resolve the report root to handle any symlinks in the path itself.
    resolved_root = report_root.resolve()

    # Check for symlinks in the file path itself.
    if file_path.is_symlink():
        raise BundleProductionError(f"symlink rejected in report: {file_path}")

    # Resolve the file path and check it is inside the report root.
    resolved_file = file_path.resolve()
    try:
        rel = resolved_file.relative_to(resolved_root)
    except ValueError as exc:
        raise BundleProductionError(
            f"file outside report root: {file_path} (resolved: {resolved_file})"
        ) from exc

    rel_posix = rel.as_posix()

    if not rel_posix:
        raise BundleProductionError(f"empty relative path: {file_path}")

    if "\x00" in rel_posix:
        raise BundleProductionError(f"null byte in path: {file_path}")

    if "\\" in rel_posix:
        raise BundleProductionError(f"backslash in path: {file_path}")

    parts = PurePosixPath(rel_posix).parts
    if ".." in parts or "." in parts:
        raise BundleProductionError(f"traversal or dot segment in path: {file_path}")

    return rel_posix


def _classify_artifact(rel_path: str) -> tuple[ArtifactType, PrivacyClass]:
    """Classify an artifact by its normalized relative path.

    Root-level files are classified by basename using ``_ARTIFACT_TYPE_MAP``
    or ``_OBSERVATION_FILES``. Files under ``evidence/`` are classified as
    ``EVIDENCE_ARTIFACT`` / ``RESTRICTED``. Unknown files fail closed.
    """
    # Nested evidence files under evidence/ directory.
    if rel_path.startswith(_EVIDENCE_DIR_PREFIX):
        return (ArtifactType.EVIDENCE_ARTIFACT, PrivacyClass.RESTRICTED)

    # Root-level files classified by basename.
    basename = PurePosixPath(rel_path).name
    if basename in _ARTIFACT_TYPE_MAP:
        return _ARTIFACT_TYPE_MAP[basename]
    if basename in _OBSERVATION_FILES:
        return (ArtifactType.OBSERVATION_FILE, PrivacyClass.PUBLIC)

    raise BundleProductionError(
        f"unknown file in report directory, not in any typed report contract: {rel_path}"
    )


def _count_jsonl_records(content: bytes) -> int:
    """Count non-empty lines in a JSONL file."""
    return sum(1 for line in content.splitlines() if line.strip())


def _load_evidence_encryption_map(report_dir: Path) -> dict[str, EvidenceEncryption]:
    """Load encryption metadata for nested evidence artifacts from evidence-index.jsonl.

    Returns a mapping from ``storage_location`` to ``EvidenceEncryption``.
    If the evidence index file does not exist or is empty, returns an empty
    mapping.
    """
    evidence_index_path = report_dir / evals_constants.EVIDENCE_INDEX_JSONL
    if not evidence_index_path.exists():
        return {}

    from g8e_evals.schema import EvidenceIndex

    result: dict[str, EvidenceEncryption] = {}
    for line in evidence_index_path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        record = EvidenceIndex.model_validate_json(line)
        if record.storage_location and record.encryption is not None:
            result[record.storage_location] = record.encryption
    return result


def _derive_run_id(report_dir: Path) -> str:
    """Derive the run ID from the run manifest in the report directory.

    Parses ``manifest.json`` and extracts the ``run_id`` field. The run
    manifest is not fully schema-validated at production time; the verifier
    performs full schema validation in layer 2. Raises
    ``BundleProductionError`` if the manifest is missing, invalid JSON, or
    lacks a ``run_id`` field.
    """
    import json as _json

    manifest_path = report_dir / evals_constants.MANIFEST_JSON
    if not manifest_path.exists():
        raise BundleProductionError(
            f"run manifest not found in report directory: {manifest_path}"
        )
    try:
        manifest_data = _json.loads(manifest_path.read_text())
    except Exception as exc:
        raise BundleProductionError(
            f"invalid run manifest JSON in report directory: {exc}"
        ) from exc
    run_id = manifest_data.get("run_id")
    if not run_id or not isinstance(run_id, str):
        raise BundleProductionError(
            "run manifest missing or invalid run_id field"
        )
    return run_id


def _derive_release_version(report_dir: Path) -> str:
    """Derive the release version from the analysis input in the report directory.

    Parses ``analysis-input.json`` and extracts the ``release_version``
    field. The analysis input is not fully schema-validated at production
    time; the verifier performs full schema validation in layer 2. Raises
    ``BundleProductionError`` if the analysis input is missing, invalid
    JSON, or lacks a ``release_version`` field.
    """
    import json as _json

    analysis_input_path = report_dir / evals_constants.ANALYSIS_INPUT_JSON
    if not analysis_input_path.exists():
        raise BundleProductionError(
            f"analysis input not found in report directory: {analysis_input_path}"
        )
    try:
        analysis_data = _json.loads(analysis_input_path.read_text())
    except Exception as exc:
        raise BundleProductionError(
            f"invalid analysis input JSON in report directory: {exc}"
        ) from exc
    release_version = analysis_data.get("release_version")
    if not release_version or not isinstance(release_version, str):
        raise BundleProductionError(
            "analysis input missing or invalid release_version field"
        )
    return release_version


def _build_artifact_entry(
    rel_path: str,
    content: bytes,
    encryption: EvidenceEncryption | None = None,
) -> BundleArtifactEntry:
    artifact_type, privacy_class = _classify_artifact(rel_path)
    record_count = _count_jsonl_records(content) if rel_path.endswith(".jsonl") else 0
    return BundleArtifactEntry(
        path=rel_path,
        media_type=_media_type(rel_path),
        privacy_class=privacy_class,
        sha256=_sha256(content),
        byte_length=len(content),
        artifact_type=artifact_type,
        record_count=record_count,
        encryption=encryption,
    )


def _inventory_report_dir(report_dir: Path) -> list[tuple[str, Path]]:
    """Recursively inventory all regular files in the report directory.

    Rejects symlinks (files and directories), absolute paths, backslashes,
    null bytes, traversal segments, duplicate normalized paths, and source
    paths outside the report root. Returns a sorted list of
    (normalized_relative_path, absolute_path) pairs.
    """
    seen: set[str] = set()
    entries: list[tuple[str, Path]] = []

    for file_path in sorted(report_dir.rglob("*")):
        if file_path.is_dir():
            # Reject symlink directories.
            if file_path.is_symlink():
                raise BundleProductionError(
                    f"symlink directory rejected in report: {file_path}"
                )
            continue

        if file_path.is_symlink():
            raise BundleProductionError(f"symlink rejected in report: {file_path}")

        if not file_path.is_file():
            raise BundleProductionError(f"non-regular file in report: {file_path}")

        rel_path = _normalize_relative_path(file_path, report_dir)

        if rel_path in seen:
            raise BundleProductionError(f"duplicate normalized path: {rel_path}")
        seen.add(rel_path)

        entries.append((rel_path, file_path))

    return entries


def produce_bundle(
    report_dir: Path,
    bundle_dir: Path,
    bundle_id: str,
    signing_key: EvalSigningKey | None = None,
    created_at: datetime | None = None,
    restricted_encryption: EvidenceEncryption | None = None,
    diagnostic: bool = False,
) -> BundleManifest:
    """Create an immutable bundle from a report directory.

    Recursively inventories every regular file in ``report_dir``, classifies
    each by its normalized relative path and typed report contract, builds a
    typed ``BundleManifest`` and ``ChecksumRoot``, signs them, and writes the
    bundle to ``bundle_dir`` through a staging/atomic-finalize boundary.
    Returns the manifest.

    A release bundle requires a signing key. If ``signing_key`` is None and
    ``diagnostic`` is False, production fails closed. If ``diagnostic`` is
    True, an unsigned bundle may be produced for local debugging only; such a
    bundle cannot pass complete verification or publication.

    The ``run_id`` and ``release_version`` are derived from the validated
    report records (run manifest and analysis input), not from caller-supplied
    values. Unknown files fail closed. Symlinks, absolute paths, traversal
    segments, backslashes, null bytes, duplicate normalized paths, and source
    paths outside the report root are rejected. The destination must be empty
    or non-existent; stale files are never mixed with new content.
    """
    report_dir = Path(report_dir)
    bundle_dir = Path(bundle_dir)
    if created_at is None:
        created_at = datetime.now(UTC)

    # R5.6: A release bundle requires a signing key. Unsigned bundles are
    # only permitted in explicitly named diagnostic mode.
    if signing_key is None and not diagnostic:
        raise BundleProductionError(
            "release bundle requires a signing key; pass diagnostic=True for "
            "unsigned local debugging bundles that cannot pass verification"
        )

    # R5.4: Reject a non-empty destination instead of mixing with stale files.
    if bundle_dir.exists() and any(bundle_dir.iterdir()):
        raise BundleProductionError(
            f"bundle destination is not empty: {bundle_dir}"
        )

    # R5.5: Derive run_id and release_version from validated report records.
    run_id = _derive_run_id(report_dir)
    release_version = _derive_release_version(report_dir)

    # R5.2: Recursively inventory the report directory.
    inventory = _inventory_report_dir(report_dir)

    # R5.7: Load per-artifact encryption metadata from evidence-index.jsonl.
    evidence_encryption_map = _load_evidence_encryption_map(report_dir)

    # Read each source file once and build artifact entries.
    entries: list[BundleArtifactEntry] = []
    file_contents: list[tuple[str, bytes]] = []
    for rel_path, file_path in inventory:
        content = file_path.read_bytes()
        file_contents.append((rel_path, content))

        # Determine encryption metadata for this artifact.
        encryption: EvidenceEncryption | None = None
        _artifact_type, privacy_class = _classify_artifact(rel_path)
        if privacy_class == PrivacyClass.RESTRICTED:
            # Use per-artifact encryption from the evidence index for nested
            # evidence files. Fall back to caller-supplied
            # restricted_encryption for other restricted artifacts.
            if rel_path in evidence_encryption_map:
                encryption = evidence_encryption_map[rel_path]
            elif restricted_encryption is not None:
                encryption = restricted_encryption
            else:
                raise BundleProductionError(
                    f"restricted artifact without encryption metadata: {rel_path}"
                )

        entry = _build_artifact_entry(rel_path, content, encryption)
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

    # R5.4: Write to a staging directory, then atomically rename to the
    # final bundle directory.
    staging_dir = bundle_dir.parent / f"{bundle_dir.name}.staging"
    if staging_dir.exists():
        shutil.rmtree(staging_dir)
    staging_dir.mkdir(parents=True)

    # Write all data files to the staging directory.
    for rel_path, content in file_contents:
        dest = staging_dir / rel_path
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(content)

    # Write manifest and checksum root.
    from g8e_evals.analysis.canonical import canonical_model_json
    (staging_dir / evals_constants.BUNDLE_MANIFEST_JSON).write_text(canonical_model_json(manifest))
    (staging_dir / evals_constants.CHECKSUM_ROOT_JSON).write_text(canonical_model_json(checksum_root))

    # Sign and write signature if a signing key is provided.
    if signing_key is not None:
        signature = sign_bundle(manifest, checksum_root, signing_key, created_at)
        (staging_dir / evals_constants.BUNDLE_SIGNATURE_JSON).write_text(canonical_model_json(signature))

    # R5.4: Atomically finalize by renaming the staging directory.
    os.replace(staging_dir, bundle_dir)

    return manifest


__all__ = [
    "BundleProductionError",
    "produce_bundle",
]
