# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Complete fail-closed offline eval bundle verification (P2-03).

The verifier reads a bundle directory and an externally supplied trust
store. It requires no originating service, runtime directory, network
access, or in-bundle trust policy. Verification proceeds through eleven
ordered layers, each producing typed failures with centralized stable
failure codes, protected-record identity, and a deterministic sorted
order. A cancelled verification raises ``VerificationCancelled`` and does
not emit a valid report.

The verification report is a frozen, extra-forbid typed model. Callers
inspect ``ok``, ``layers``, and ``failures``; they never parse error prose.
"""

from __future__ import annotations

import hashlib
import json
import threading
from datetime import UTC, datetime
from enum import IntEnum, StrEnum
from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, render_cli, render_html, render_markdown
from g8e_evals.analysis.engine import compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle.canonical import (
    compute_checksum_root_hash,
    compute_manifest_hash,
)
from g8e_evals.bundle.manifest import (
    BundleManifest,
    ChecksumRoot,
    PrivacyClass,
)
from g8e_evals.bundle.signing import (
    BundleSignature,
    BundleSignatureVerificationResult,
    EvalTrustStore,
    TrustStatus,
    verify_bundle_signature,
)
from g8e_evals.bundle.validation import (
    BundleManifestValidationError,
    BundlePathError,
    validate_bundle_manifest,
    validate_bundle_path,
)

VERIFICATION_REPORT_SCHEMA_VERSION = "1.0.0"

# Limits for the rooted-inventory layer. These are conservative bounds
# that catch accidental or malicious over-production without rejecting
# legitimate eval bundles. P2-04 may tighten or parameterize these.
MAX_BUNDLE_FILE_COUNT = 10_000
MAX_BUNDLE_PER_FILE_BYTES = 500 * 1024 * 1024  # 500 MiB
MAX_BUNDLE_TOTAL_BYTES = 5 * 1024 * 1024 * 1024  # 5 GiB

# Bundle metadata files that are not listed as data artifacts in the
# manifest. They are the verification contract, not the data being verified.
_BUNDLE_METADATA_FILES = frozenset({
    evals_constants.BUNDLE_MANIFEST_JSON,
    evals_constants.CHECKSUM_ROOT_JSON,
    evals_constants.BUNDLE_SIGNATURE_JSON,
})


class VerificationLayer(IntEnum):
    """Ordered verification layers. The integer value defines the sort order."""

    ROOTED_INVENTORY = 1
    SCHEMAS_CANONICAL = 2
    FILE_HASHES = 3
    SIGNATURES_TRUST = 4
    RECORD_BINDINGS = 5
    ENVELOPE_RECEIPT = 6
    CHAIN_LINKS = 7
    METRIC_PRODUCERS = 8
    ANALYSIS_REPRODUCTION = 9
    RENDERER_EQUALITY = 10
    PRIVACY_SEPARATION = 11


class VerificationFailureCode(StrEnum):
    """Centralized stable failure codes for the verification report."""

    # Layer 1: Rooted inventory and limits
    ORPHAN_FILE = "orphan_file"
    MISSING_FILE = "missing_file"
    PATH_NOT_ROOTED = "path_not_rooted"
    PATH_TRAVERSAL = "path_traversal"
    SYMLINK_REJECTED = "symlink_rejected"
    DUPLICATE_PATH = "duplicate_path"
    FILE_COUNT_LIMIT = "file_count_limit"
    PER_FILE_BYTE_LIMIT = "per_file_byte_limit"
    TOTAL_BYTE_LIMIT = "total_byte_limit"

    # Layer 2: Schemas and canonical bytes
    MANIFEST_SCHEMA_INVALID = "manifest_schema_invalid"
    CHECKSUM_SCHEMA_INVALID = "checksum_schema_invalid"
    SIGNATURE_SCHEMA_INVALID = "signature_schema_invalid"
    MANIFEST_SELF_HASH_MISMATCH = "manifest_self_hash_mismatch"
    CHECKSUM_SELF_HASH_MISMATCH = "checksum_self_hash_mismatch"
    CHECKSUM_MANIFEST_MISMATCH = "checksum_manifest_mismatch"

    # Layer 3: File hashes and references
    FILE_HASH_MISMATCH = "file_hash_mismatch"
    BYTE_LENGTH_MISMATCH = "byte_length_mismatch"
    EXTERNAL_REF_INVALID = "external_ref_invalid"

    # Layer 4: Signatures and trust
    SIGNATURE_MISSING = "signature_missing"
    TRUST_STORE_MISSING = "trust_store_missing"
    SIGNATURE_UNKNOWN_KEY = "signature_unknown_key"
    SIGNATURE_REVOKED = "signature_revoked"
    SIGNATURE_EXPIRED = "signature_expired"
    SIGNATURE_WRONG_SCOPE = "signature_wrong_scope"
    SIGNATURE_WRONG_ALGORITHM = "signature_wrong_algorithm"
    SIGNATURE_MALFORMED = "signature_malformed"
    SIGNATURE_SUBSTITUTED = "signature_substituted"

    # Layer 5: Record bindings
    RUN_ID_MISMATCH = "run_id_mismatch"
    UNKNOWN_TASK = "unknown_task"
    UNKNOWN_ATTEMPT = "unknown_attempt"
    CROSS_RUN_RECORD = "cross_run_record"
    WRONG_TASK_BINDING = "wrong_task_binding"
    DUPLICATE_IDENTITY = "duplicate_identity"

    # Layer 6: Envelope/receipt correlation
    RECEIPT_UNKNOWN_ATTEMPT = "receipt_unknown_attempt"
    RECEIPT_RUN_MISMATCH = "receipt_run_mismatch"
    ENVELOPE_UNCORRELATED = "envelope_uncorrelated"

    # Layer 7: Chain links
    STAGE_UNKNOWN_ATTEMPT = "stage_unknown_attempt"
    STAGE_RUN_MISMATCH = "stage_run_mismatch"
    STAGE_TASK_MISMATCH = "stage_task_mismatch"
    PERSISTENCE_ATTESTATION_INVALID = "persistence_attestation_invalid"
    COMMITMENT_ATTESTATION_INVALID = "commitment_attestation_invalid"
    AUDIT_LINK_INVALID = "audit_link_invalid"

    # Layer 8: Metric producers
    METRIC_OBSERVATION_INVALID = "metric_observation_invalid"
    METRIC_UNREGISTERED = "metric_unregistered"

    # Layer 9: Analysis reproduction
    ANALYSIS_INPUT_SCHEMA_INVALID = "analysis_input_schema_invalid"
    ANALYSIS_REPRODUCTION_FAILED = "analysis_reproduction_failed"
    ANALYSIS_REPRODUCTION_MISMATCH = "analysis_reproduction_mismatch"

    # Layer 10: Renderer equality
    RENDERER_BYTE_MISMATCH = "renderer_byte_mismatch"

    # Layer 11: Privacy separation
    RESTRICTED_WITHOUT_ENCRYPTION = "restricted_without_encryption"
    RESTRICTED_PLAINTEXT_EXPOSED = "restricted_plaintext_exposed"


class VerificationCancelled(Exception):
    """Raised when verification is cancelled. Does not emit a valid report."""


class VerificationFailure(BaseModel):
    """One typed verification failure with stable code, layer, and record identity."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    layer: VerificationLayer = Field(description="Verification layer that produced the failure.")
    code: VerificationFailureCode = Field(description="Centralized stable failure code.")
    record_id: str = Field(default="", description="Protected-record identity, empty if not record-specific.")
    message: str = Field(description="Human-readable detail. Not for parsing; the code is the stable contract.")


class LayerResult(BaseModel):
    """Result of one verification layer."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    layer: VerificationLayer = Field(description="Verification layer.")
    passed: bool = Field(description="True when the layer produced zero failures.")
    failure_count: int = Field(ge=0, description="Number of failures produced by this layer.")


class VerificationReport(BaseModel):
    """Typed deterministic verification report.

    ``ok`` is ``True`` only when every layer passed with zero failures.
    ``failures`` is sorted by layer order, then by record ID, then by code.
    Callers inspect ``ok``, ``layers``, and ``failures``; they never parse
    error prose.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(default=VERIFICATION_REPORT_SCHEMA_VERSION)
    bundle_id: str = Field(default="", description="Bundle identity from the manifest, empty if unreadable.")
    run_id: str = Field(default="", description="Run identity from the manifest, empty if unreadable.")
    release_version: str = Field(default="", description="Release version from the manifest, empty if unreadable.")
    verified_at: datetime = Field(description="Verification timestamp.")
    ok: bool = Field(description="True only when every layer passed with zero failures.")
    layers: list[LayerResult] = Field(description="Per-layer results in verification order.")
    failures: list[VerificationFailure] = Field(
        default_factory=list,
        description="All failures sorted by layer, record ID, and code.",
    )


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _read_json_file(path: Path) -> bytes | None:
    """Read raw bytes of a JSON file, returning None if the file does not exist."""
    if not path.exists():
        return None
    return path.read_bytes()


def _sort_failures(failures: list[VerificationFailure]) -> list[VerificationFailure]:
    """Sort failures by layer order, then record ID, then code."""
    return sorted(failures, key=lambda f: (int(f.layer), f.record_id, f.code))


def _check_cancel(cancel_event: threading.Event | None) -> None:
    if cancel_event is not None and cancel_event.is_set():
        raise VerificationCancelled("verification cancelled before next layer")


# ---------------------------------------------------------------------------
# Layer 1: Rooted inventory and limits
# ---------------------------------------------------------------------------


def _verify_rooted_inventory(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    # Validate manifest paths and check for duplicates.
    seen_paths: set[str] = set()
    for entry in manifest.artifacts:
        try:
            normalized = validate_bundle_path(entry.path)
        except BundlePathError as exc:
            code = VerificationFailureCode.PATH_TRAVERSAL if "traversal" in str(exc) else VerificationFailureCode.PATH_NOT_ROOTED
            failures.append(VerificationFailure(
                layer=VerificationLayer.ROOTED_INVENTORY,
                code=code,
                record_id=entry.path,
                message=str(exc),
            ))
            continue
        if normalized in seen_paths:
            failures.append(VerificationFailure(
                layer=VerificationLayer.ROOTED_INVENTORY,
                code=VerificationFailureCode.DUPLICATE_PATH,
                record_id=normalized,
                message=f"duplicate normalized path: {normalized}",
            ))
        seen_paths.add(normalized)

    # Check for orphan files (files not in manifest, excluding metadata).
    listed = seen_paths
    for file_path in sorted(bundle_root.rglob("*")):
        if file_path.is_dir():
            continue
        rel = file_path.relative_to(bundle_root).as_posix()
        if rel in _BUNDLE_METADATA_FILES:
            continue
        if file_path.is_symlink():
            failures.append(VerificationFailure(
                layer=VerificationLayer.ROOTED_INVENTORY,
                code=VerificationFailureCode.SYMLINK_REJECTED,
                record_id=rel,
                message=f"symlink rejected in bundle: {rel}",
            ))
            continue
        if rel not in listed:
            failures.append(VerificationFailure(
                layer=VerificationLayer.ROOTED_INVENTORY,
                code=VerificationFailureCode.ORPHAN_FILE,
                record_id=rel,
                message=f"orphan file not listed in manifest: {rel}",
            ))

    # Check for missing files (manifest entries that don't exist).
    for entry in manifest.artifacts:
        normalized = validate_bundle_path(entry.path)
        file_path = bundle_root / normalized
        if not file_path.exists():
            failures.append(VerificationFailure(
                layer=VerificationLayer.ROOTED_INVENTORY,
                code=VerificationFailureCode.MISSING_FILE,
                record_id=normalized,
                message=f"missing file listed in manifest: {normalized}",
            ))
        elif file_path.is_symlink():
            failures.append(VerificationFailure(
                layer=VerificationLayer.ROOTED_INVENTORY,
                code=VerificationFailureCode.SYMLINK_REJECTED,
                record_id=normalized,
                message=f"symlink rejected in bundle: {normalized}",
            ))

    # Check limits.
    all_files = [f for f in bundle_root.rglob("*") if f.is_file() and not f.is_symlink()]
    if len(all_files) > MAX_BUNDLE_FILE_COUNT:
        failures.append(VerificationFailure(
            layer=VerificationLayer.ROOTED_INVENTORY,
            code=VerificationFailureCode.FILE_COUNT_LIMIT,
            record_id="",
            message=f"file count {len(all_files)} exceeds limit {MAX_BUNDLE_FILE_COUNT}",
        ))

    total_bytes = 0
    for entry in manifest.artifacts:
        normalized = validate_bundle_path(entry.path)
        file_path = bundle_root / normalized
        if file_path.exists() and file_path.is_file():
            size = file_path.stat().st_size
            total_bytes += size
            if size > MAX_BUNDLE_PER_FILE_BYTES:
                failures.append(VerificationFailure(
                    layer=VerificationLayer.ROOTED_INVENTORY,
                    code=VerificationFailureCode.PER_FILE_BYTE_LIMIT,
                    record_id=normalized,
                    message=f"file {normalized} size {size} exceeds limit {MAX_BUNDLE_PER_FILE_BYTES}",
                ))

    if total_bytes > MAX_BUNDLE_TOTAL_BYTES:
        failures.append(VerificationFailure(
            layer=VerificationLayer.ROOTED_INVENTORY,
            code=VerificationFailureCode.TOTAL_BYTE_LIMIT,
            record_id="",
            message=f"total bytes {total_bytes} exceeds limit {MAX_BUNDLE_TOTAL_BYTES}",
        ))

    return failures


# ---------------------------------------------------------------------------
# Layer 2: Schemas and canonical bytes
# ---------------------------------------------------------------------------


def _verify_schemas_canonical(
    bundle_root: Path,
    manifest: BundleManifest | None,
    checksum_root: ChecksumRoot | None,
    manifest_raw: bytes | None,
    checksum_raw: bytes | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        if manifest_raw is not None:
            # File exists but deserialization failed.
            failures.append(VerificationFailure(
                layer=VerificationLayer.SCHEMAS_CANONICAL,
                code=VerificationFailureCode.MANIFEST_SCHEMA_INVALID,
                record_id="",
                message="bundle-manifest.json failed schema validation",
            ))
        else:
            failures.append(VerificationFailure(
                layer=VerificationLayer.SCHEMAS_CANONICAL,
                code=VerificationFailureCode.MANIFEST_SCHEMA_INVALID,
                record_id="",
                message="bundle-manifest.json not found",
            ))
        return failures

    # Validate manifest structure (paths, duplicates, restricted encryption).
    try:
        validate_bundle_manifest(manifest)
    except BundleManifestValidationError as exc:
        failures.append(VerificationFailure(
            layer=VerificationLayer.SCHEMAS_CANONICAL,
            code=VerificationFailureCode.MANIFEST_SCHEMA_INVALID,
            record_id="",
            message=str(exc),
        ))

    # Verify manifest self-hash.
    if manifest.manifest_content_sha256:
        expected = compute_manifest_hash(manifest)
        if manifest.manifest_content_sha256 != expected:
            failures.append(VerificationFailure(
                layer=VerificationLayer.SCHEMAS_CANONICAL,
                code=VerificationFailureCode.MANIFEST_SELF_HASH_MISMATCH,
                record_id="",
                message=f"manifest self-hash mismatch: stored={manifest.manifest_content_sha256} computed={expected}",
            ))

    if checksum_root is None:
        if checksum_raw is not None:
            failures.append(VerificationFailure(
                layer=VerificationLayer.SCHEMAS_CANONICAL,
                code=VerificationFailureCode.CHECKSUM_SCHEMA_INVALID,
                record_id="",
                message="checksum-root.json failed schema validation",
            ))
        else:
            failures.append(VerificationFailure(
                layer=VerificationLayer.SCHEMAS_CANONICAL,
                code=VerificationFailureCode.CHECKSUM_SCHEMA_INVALID,
                record_id="",
                message="checksum-root.json not found",
            ))
        return failures

    # Verify checksum root self-hash.
    if checksum_root.checksum_root_sha256:
        expected = compute_checksum_root_hash(checksum_root)
        if checksum_root.checksum_root_sha256 != expected:
            failures.append(VerificationFailure(
                layer=VerificationLayer.SCHEMAS_CANONICAL,
                code=VerificationFailureCode.CHECKSUM_SELF_HASH_MISMATCH,
                record_id="",
                message=f"checksum root self-hash mismatch: stored={checksum_root.checksum_root_sha256} computed={expected}",
            ))

    # Verify checksum root entries match manifest entries.
    manifest_map = {validate_bundle_path(e.path): e.sha256 for e in manifest.artifacts}
    checksum_map = {validate_bundle_path(e.path): e.sha256 for e in checksum_root.entries}

    if set(manifest_map.keys()) != set(checksum_map.keys()):
        failures.append(VerificationFailure(
            layer=VerificationLayer.SCHEMAS_CANONICAL,
            code=VerificationFailureCode.CHECKSUM_MANIFEST_MISMATCH,
            record_id="",
            message="checksum root paths do not match manifest paths",
        ))
    else:
        for path in sorted(manifest_map.keys()):
            if manifest_map[path] != checksum_map[path]:
                failures.append(VerificationFailure(
                    layer=VerificationLayer.SCHEMAS_CANONICAL,
                    code=VerificationFailureCode.CHECKSUM_MANIFEST_MISMATCH,
                    record_id=path,
                    message=f"checksum root hash mismatch for {path}: manifest={manifest_map[path]} checksum={checksum_map[path]}",
                ))

    return failures


# ---------------------------------------------------------------------------
# Layer 3: File hashes and references
# ---------------------------------------------------------------------------


def _verify_file_hashes(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    for entry in manifest.artifacts:
        normalized = validate_bundle_path(entry.path)
        file_path = bundle_root / normalized
        if not file_path.exists() or not file_path.is_file():
            continue  # Missing files are reported in layer 1.

        content = file_path.read_bytes()
        actual_hash = _sha256(content)
        if actual_hash != entry.sha256:
            failures.append(VerificationFailure(
                layer=VerificationLayer.FILE_HASHES,
                code=VerificationFailureCode.FILE_HASH_MISMATCH,
                record_id=normalized,
                message=f"hash mismatch for {normalized}: manifest={entry.sha256} actual={actual_hash}",
            ))

        if len(content) != entry.byte_length:
            failures.append(VerificationFailure(
                layer=VerificationLayer.FILE_HASHES,
                code=VerificationFailureCode.BYTE_LENGTH_MISMATCH,
                record_id=normalized,
                message=f"byte length mismatch for {normalized}: manifest={entry.byte_length} actual={len(content)}",
            ))

    # Verify external references are content-addressed.
    for ref in manifest.external_references:
        if not ref.content_sha256 or len(ref.content_sha256) != 64:
            failures.append(VerificationFailure(
                layer=VerificationLayer.FILE_HASHES,
                code=VerificationFailureCode.EXTERNAL_REF_INVALID,
                record_id=ref.reference_id,
                message=f"external reference has invalid content_sha256: {ref.reference_id}",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 4: Manifest/checksum signatures and assessed trust
# ---------------------------------------------------------------------------


def _verify_signatures_trust(
    bundle_signature: BundleSignature | None,
    signature_raw: bytes | None,
    manifest: BundleManifest | None,
    checksum_root: ChecksumRoot | None,
    trust_store: EvalTrustStore | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if signature_raw is None:
        failures.append(VerificationFailure(
            layer=VerificationLayer.SIGNATURES_TRUST,
            code=VerificationFailureCode.SIGNATURE_MISSING,
            record_id="",
            message="bundle-signature.json not found",
        ))
        return failures

    if bundle_signature is None:
        failures.append(VerificationFailure(
            layer=VerificationLayer.SIGNATURES_TRUST,
            code=VerificationFailureCode.SIGNATURE_SCHEMA_INVALID,
            record_id="",
            message="bundle-signature.json failed schema validation",
        ))
        return failures

    if trust_store is None:
        failures.append(VerificationFailure(
            layer=VerificationLayer.SIGNATURES_TRUST,
            code=VerificationFailureCode.TRUST_STORE_MISSING,
            record_id="",
            message="no trust store supplied; the verifier never trusts in-bundle keys",
        ))
        return failures

    if manifest is None or checksum_root is None:
        failures.append(VerificationFailure(
            layer=VerificationLayer.SIGNATURES_TRUST,
            code=VerificationFailureCode.SIGNATURE_MALFORMED,
            record_id="",
            message="cannot verify signatures without manifest and checksum root",
        ))
        return failures

    result: BundleSignatureVerificationResult = verify_bundle_signature(
        bundle_signature, manifest, checksum_root, trust_store,
    )

    for status, label in [
        (result.manifest_status, "manifest"),
        (result.checksum_root_status, "checksum_root"),
    ]:
        if status != TrustStatus.TRUSTED:
            code_map = {
                TrustStatus.UNKNOWN: VerificationFailureCode.SIGNATURE_UNKNOWN_KEY,
                TrustStatus.REVOKED: VerificationFailureCode.SIGNATURE_REVOKED,
                TrustStatus.EXPIRED: VerificationFailureCode.SIGNATURE_EXPIRED,
                TrustStatus.WRONG_SCOPE: VerificationFailureCode.SIGNATURE_WRONG_SCOPE,
                TrustStatus.WRONG_ALGORITHM: VerificationFailureCode.SIGNATURE_WRONG_ALGORITHM,
                TrustStatus.MALFORMED: VerificationFailureCode.SIGNATURE_MALFORMED,
                TrustStatus.SUBSTITUTED: VerificationFailureCode.SIGNATURE_SUBSTITUTED,
            }
            code = code_map.get(status, VerificationFailureCode.SIGNATURE_MALFORMED)
            failures.append(VerificationFailure(
                layer=VerificationLayer.SIGNATURES_TRUST,
                code=code,
                record_id=label,
                message=f"{label} signature status: {status.value}",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 5: Run/task/attempt bindings
# ---------------------------------------------------------------------------


def _read_jsonl[T: BaseModel](bundle_root: Path, filename: str, model_cls: type[T]) -> list[T]:
    """Read a JSONL file and deserialize each line as a typed model."""
    path = bundle_root / filename
    if not path.exists():
        return []
    records: list[T] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            records.append(model_cls.model_validate_json(line))
        except ValidationError:
            continue
    return records


def _verify_record_bindings(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    # Read the run manifest for run_id.
    run_manifest_path = bundle_root / evals_constants.MANIFEST_JSON
    run_id = manifest.run_id
    if run_manifest_path.exists():
        try:
            run_manifest = json.loads(run_manifest_path.read_text())
            file_run_id = run_manifest.get("run_id", "")
            if file_run_id and file_run_id != run_id:
                failures.append(VerificationFailure(
                    layer=VerificationLayer.RECORD_BINDINGS,
                    code=VerificationFailureCode.RUN_ID_MISMATCH,
                    record_id="run_manifest",
                    message=f"run_id mismatch: manifest={run_id} run_manifest={file_run_id}",
                ))
        except (json.JSONDecodeError, ValueError):
            failures.append(VerificationFailure(
                layer=VerificationLayer.RECORD_BINDINGS,
                code=VerificationFailureCode.RUN_ID_MISMATCH,
                record_id="run_manifest",
                message="run manifest is not valid JSON",
            ))

    # Read tasks and attempts.
    from g8e_evals.schema import AttemptRecord, TaskDefinition
    tasks = _read_jsonl(bundle_root, evals_constants.TASKS_JSONL, TaskDefinition)
    attempts = _read_jsonl(bundle_root, evals_constants.ATTEMPTS_JSONL, AttemptRecord)

    task_ids = [t.task_id for t in tasks]
    if len(task_ids) != len(set(task_ids)):
        failures.append(VerificationFailure(
            layer=VerificationLayer.RECORD_BINDINGS,
            code=VerificationFailureCode.DUPLICATE_IDENTITY,
            record_id="tasks",
            message="duplicate task ID",
        ))
    task_by_id = {t.task_id: t for t in tasks}

    attempt_ids = [a.attempt_id for a in attempts]
    if len(attempt_ids) != len(set(attempt_ids)):
        failures.append(VerificationFailure(
            layer=VerificationLayer.RECORD_BINDINGS,
            code=VerificationFailureCode.DUPLICATE_IDENTITY,
            record_id="attempts",
            message="duplicate attempt ID",
        ))
    attempt_by_id = {a.attempt_id: a for a in attempts}

    for attempt in attempts:
        if attempt.run_id != run_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.RECORD_BINDINGS,
                code=VerificationFailureCode.RUN_ID_MISMATCH,
                record_id=attempt.attempt_id,
                message=f"attempt {attempt.attempt_id} run_id mismatch: {attempt.run_id} != {run_id}",
            ))
        if attempt.task_id not in task_by_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.RECORD_BINDINGS,
                code=VerificationFailureCode.UNKNOWN_TASK,
                record_id=attempt.attempt_id,
                message=f"attempt {attempt.attempt_id} references unknown task {attempt.task_id}",
            ))

    # Verify receipts reference valid attempts.
    from g8e_evals.schema import ReceiptObservation
    receipts = _read_jsonl(bundle_root, evals_constants.RECEIPTS_JSONL, ReceiptObservation)
    for receipt in receipts:
        if receipt.attempt_id not in attempt_by_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.RECORD_BINDINGS,
                code=VerificationFailureCode.UNKNOWN_ATTEMPT,
                record_id=receipt.receipt_id,
                message=f"receipt {receipt.receipt_id} references unknown attempt {receipt.attempt_id}",
            ))
        if receipt.run_id != run_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.RECORD_BINDINGS,
                code=VerificationFailureCode.CROSS_RUN_RECORD,
                record_id=receipt.receipt_id,
                message=f"receipt {receipt.receipt_id} run_id mismatch: {receipt.run_id} != {run_id}",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 6: Envelope/receipt correlation
# ---------------------------------------------------------------------------


def _verify_envelope_receipt(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    from g8e_evals.schema import ReceiptObservation
    receipts = _read_jsonl(bundle_root, evals_constants.RECEIPTS_JSONL, ReceiptObservation)

    # Read attempts for correlation.
    from g8e_evals.schema import AttemptRecord
    attempts = _read_jsonl(bundle_root, evals_constants.ATTEMPTS_JSONL, AttemptRecord)
    attempt_by_id = {a.attempt_id: a for a in attempts}

    for receipt in receipts:
        if receipt.attempt_id not in attempt_by_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.ENVELOPE_RECEIPT,
                code=VerificationFailureCode.RECEIPT_UNKNOWN_ATTEMPT,
                record_id=receipt.receipt_id,
                message=f"receipt {receipt.receipt_id} references unknown attempt {receipt.attempt_id}",
            ))
        attempt = attempt_by_id.get(receipt.attempt_id)
        if attempt and receipt.run_id != attempt.run_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.ENVELOPE_RECEIPT,
                code=VerificationFailureCode.RECEIPT_RUN_MISMATCH,
                record_id=receipt.receipt_id,
                message=f"receipt {receipt.receipt_id} run_id {receipt.run_id} != attempt {attempt.attempt_id} run_id {attempt.run_id}",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 7: Chain links (stages, persistence, commitment, audit)
# ---------------------------------------------------------------------------


def _verify_chain_links(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    from g8e_evals.schema import AttemptRecord, StageObservation
    attempts = _read_jsonl(bundle_root, evals_constants.ATTEMPTS_JSONL, AttemptRecord)
    attempt_by_id = {a.attempt_id: a for a in attempts}
    stages = _read_jsonl(bundle_root, evals_constants.STAGES_JSONL, StageObservation)

    stage_ids: set[str] = set()
    for stage in stages:
        if stage.stage_id in stage_ids:
            failures.append(VerificationFailure(
                layer=VerificationLayer.CHAIN_LINKS,
                code=VerificationFailureCode.DUPLICATE_IDENTITY,
                record_id=stage.stage_id,
                message=f"duplicate stage ID: {stage.stage_id}",
            ))
        stage_ids.add(stage.stage_id)

        if stage.attempt_id not in attempt_by_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.CHAIN_LINKS,
                code=VerificationFailureCode.STAGE_UNKNOWN_ATTEMPT,
                record_id=stage.stage_id,
                message=f"stage {stage.stage_id} references unknown attempt {stage.attempt_id}",
            ))
            continue

        attempt = attempt_by_id[stage.attempt_id]
        if stage.run_id != attempt.run_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.CHAIN_LINKS,
                code=VerificationFailureCode.STAGE_RUN_MISMATCH,
                record_id=stage.stage_id,
                message=f"stage {stage.stage_id} run_id {stage.run_id} != attempt run_id {attempt.run_id}",
            ))
        if stage.task_id != attempt.task_id:
            failures.append(VerificationFailure(
                layer=VerificationLayer.CHAIN_LINKS,
                code=VerificationFailureCode.STAGE_TASK_MISMATCH,
                record_id=stage.stage_id,
                message=f"stage {stage.stage_id} task_id {stage.task_id} != attempt task_id {attempt.task_id}",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 8: Metric producers
# ---------------------------------------------------------------------------


def _verify_metric_producers(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
    from g8e_evals.schema import MetricObservation
    metrics = _read_jsonl(bundle_root, evals_constants.METRICS_JSONL, MetricObservation)

    for metric in metrics:
        try:
            DEFAULT_METRIC_REGISTRY.validate(metric)
        except Exception as exc:
            failures.append(VerificationFailure(
                layer=VerificationLayer.METRIC_PRODUCERS,
                code=VerificationFailureCode.METRIC_OBSERVATION_INVALID,
                record_id=f"{metric.metric_id}:{metric.attempt_id}",
                message=f"metric observation invalid: {exc}",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 9: Canonical analysis reproduction
# ---------------------------------------------------------------------------


def _verify_analysis_reproduction(
    bundle_root: Path,
    manifest: BundleManifest | None,
) -> tuple[list[VerificationFailure], object | None]:
    """Reproduce canonical analysis from analysis-input.json and compare.

    Returns the failures and the reproduced analysis (or None on failure).
    """
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures, None

    analysis_input_path = bundle_root / evals_constants.ANALYSIS_INPUT_JSON
    analysis_json_path = bundle_root / evals_constants.ANALYSIS_JSON

    if not analysis_input_path.exists():
        failures.append(VerificationFailure(
            layer=VerificationLayer.ANALYSIS_REPRODUCTION,
            code=VerificationFailureCode.ANALYSIS_INPUT_SCHEMA_INVALID,
            record_id="",
            message="analysis-input.json not found",
        ))
        return failures, None

    try:
        analysis_input = AnalysisInputRecord.model_validate_json(analysis_input_path.read_text())
    except ValidationError as exc:
        failures.append(VerificationFailure(
            layer=VerificationLayer.ANALYSIS_REPRODUCTION,
            code=VerificationFailureCode.ANALYSIS_INPUT_SCHEMA_INVALID,
            record_id="",
            message=f"analysis-input.json schema invalid: {exc}",
        ))
        return failures, None

    try:
        reproduced = compute_canonical_analysis_from_record(analysis_input)
    except Exception as exc:
        failures.append(VerificationFailure(
            layer=VerificationLayer.ANALYSIS_REPRODUCTION,
            code=VerificationFailureCode.ANALYSIS_REPRODUCTION_FAILED,
            record_id="",
            message=f"analysis reproduction failed: {exc}",
        ))
        return failures, None

    if not analysis_json_path.exists():
        failures.append(VerificationFailure(
            layer=VerificationLayer.ANALYSIS_REPRODUCTION,
            code=VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH,
            record_id="",
            message="analysis.json not found for comparison",
        ))
        return failures, reproduced

    stored = analysis_json_path.read_text()
    reproduced_json = canonical_model_json(reproduced)
    if stored != reproduced_json:
        failures.append(VerificationFailure(
            layer=VerificationLayer.ANALYSIS_REPRODUCTION,
            code=VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH,
            record_id="",
            message="reproduced analysis does not match stored analysis.json",
        ))

    return failures, reproduced


# ---------------------------------------------------------------------------
# Layer 10: Renderer byte equality
# ---------------------------------------------------------------------------


def _verify_renderer_equality(
    bundle_root: Path,
    reproduced_analysis: object | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if reproduced_analysis is None:
        return failures

    from g8e_evals.analysis.canonical import CanonicalEvalAnalysis

    analysis = reproduced_analysis if isinstance(reproduced_analysis, CanonicalEvalAnalysis) else None
    if analysis is None:
        return failures

    renderers = [
        (evals_constants.ANALYSIS_MD, render_markdown),
        (evals_constants.ANALYSIS_HTML, render_html),
        (evals_constants.ANALYSIS_TXT, render_cli),
    ]

    for filename, renderer in renderers:
        path = bundle_root / filename
        if not path.exists():
            failures.append(VerificationFailure(
                layer=VerificationLayer.RENDERER_EQUALITY,
                code=VerificationFailureCode.RENDERER_BYTE_MISMATCH,
                record_id=filename,
                message=f"{filename} not found for comparison",
            ))
            continue

        stored = path.read_text()
        reproduced_str = renderer(analysis)
        if stored != reproduced_str:
            failures.append(VerificationFailure(
                layer=VerificationLayer.RENDERER_EQUALITY,
                code=VerificationFailureCode.RENDERER_BYTE_MISMATCH,
                record_id=filename,
                message=f"reproduced {filename} does not match stored bytes",
            ))

    return failures


# ---------------------------------------------------------------------------
# Layer 11: Public/restricted separation
# ---------------------------------------------------------------------------


def _verify_privacy_separation(
    manifest: BundleManifest | None,
) -> list[VerificationFailure]:
    failures: list[VerificationFailure] = []

    if manifest is None:
        return failures

    for entry in manifest.artifacts:
        if entry.privacy_class == PrivacyClass.RESTRICTED:
            if entry.encryption is None:
                failures.append(VerificationFailure(
                    layer=VerificationLayer.PRIVACY_SEPARATION,
                    code=VerificationFailureCode.RESTRICTED_WITHOUT_ENCRYPTION,
                    record_id=entry.path,
                    message=f"restricted artifact without encryption metadata: {entry.path}",
                ))
        if entry.privacy_class == PrivacyClass.PUBLIC:
            if entry.encryption is not None:
                failures.append(VerificationFailure(
                    layer=VerificationLayer.PRIVACY_SEPARATION,
                    code=VerificationFailureCode.RESTRICTED_PLAINTEXT_EXPOSED,
                    record_id=entry.path,
                    message=f"public artifact carries encryption metadata (should be restricted): {entry.path}",
                ))

    return failures


# ---------------------------------------------------------------------------
# Main verification function
# ---------------------------------------------------------------------------


def verify_bundle(
    bundle_root: Path,
    trust_store: EvalTrustStore | None = None,
    cancel_event: threading.Event | None = None,
) -> VerificationReport:
    """Run complete fail-closed offline verification of a bundle directory.

    Reads the bundle directory and the externally supplied trust store.
    Requires no originating service, runtime directory, network access, or
    in-bundle trust policy. Verification proceeds through eleven ordered
    layers. A cancelled verification raises ``VerificationCancelled`` and
    does not emit a valid report.
    """
    bundle_root = Path(bundle_root)

    # Read raw bytes of metadata files.
    manifest_raw = _read_json_file(bundle_root / evals_constants.BUNDLE_MANIFEST_JSON)
    checksum_raw = _read_json_file(bundle_root / evals_constants.CHECKSUM_ROOT_JSON)
    signature_raw = _read_json_file(bundle_root / evals_constants.BUNDLE_SIGNATURE_JSON)

    # Deserialize metadata files (best-effort; failures recorded in layer 2).
    manifest: BundleManifest | None = None
    if manifest_raw is not None:
        try:
            manifest = BundleManifest.model_validate_json(manifest_raw)
        except ValidationError:
            manifest = None

    checksum_root: ChecksumRoot | None = None
    if checksum_raw is not None:
        try:
            checksum_root = ChecksumRoot.model_validate_json(checksum_raw)
        except ValidationError:
            checksum_root = None

    bundle_signature: BundleSignature | None = None
    if signature_raw is not None:
        try:
            bundle_signature = BundleSignature.model_validate_json(signature_raw)
        except ValidationError:
            bundle_signature = None

    all_failures: list[VerificationFailure] = []

    # Layer 1: Rooted inventory and limits
    _check_cancel(cancel_event)
    all_failures += _verify_rooted_inventory(bundle_root, manifest)

    # Layer 2: Schemas and canonical bytes
    _check_cancel(cancel_event)
    all_failures += _verify_schemas_canonical(
        bundle_root, manifest, checksum_root, manifest_raw, checksum_raw,
    )

    # Layer 3: File hashes and references
    _check_cancel(cancel_event)
    all_failures += _verify_file_hashes(bundle_root, manifest)

    # Layer 4: Manifest/checksum signatures and assessed trust
    _check_cancel(cancel_event)
    all_failures += _verify_signatures_trust(
        bundle_signature, signature_raw, manifest, checksum_root, trust_store,
    )

    # Layer 5: Run/task/attempt bindings
    _check_cancel(cancel_event)
    all_failures += _verify_record_bindings(bundle_root, manifest)

    # Layer 6: Envelope/receipt correlation
    _check_cancel(cancel_event)
    all_failures += _verify_envelope_receipt(bundle_root, manifest)

    # Layer 7: Chain links
    _check_cancel(cancel_event)
    all_failures += _verify_chain_links(bundle_root, manifest)

    # Layer 8: Metric producers
    _check_cancel(cancel_event)
    all_failures += _verify_metric_producers(bundle_root, manifest)

    # Layer 9: Canonical analysis reproduction
    _check_cancel(cancel_event)
    reproduction_failures, reproduced_analysis = _verify_analysis_reproduction(
        bundle_root, manifest,
    )
    all_failures += reproduction_failures

    # Layer 10: Renderer byte equality
    _check_cancel(cancel_event)
    all_failures += _verify_renderer_equality(bundle_root, reproduced_analysis)

    # Layer 11: Public/restricted separation
    _check_cancel(cancel_event)
    all_failures += _verify_privacy_separation(manifest)

    # Build per-layer results.
    layer_results: list[LayerResult] = []
    for layer in VerificationLayer:
        count = sum(1 for f in all_failures if f.layer == layer)
        layer_results.append(LayerResult(layer=layer, passed=count == 0, failure_count=count))

    sorted_failures = _sort_failures(all_failures)

    bundle_id = manifest.bundle_id if manifest else ""
    run_id = manifest.run_id if manifest else ""
    release_version = manifest.release_version if manifest else ""

    return VerificationReport(
        bundle_id=bundle_id,
        run_id=run_id,
        release_version=release_version,
        verified_at=datetime.now(UTC),
        ok=len(sorted_failures) == 0,
        layers=layer_results,
        failures=sorted_failures,
    )


__all__ = [
    "VERIFICATION_REPORT_SCHEMA_VERSION",
    "LayerResult",
    "VerificationCancelled",
    "VerificationFailure",
    "VerificationFailureCode",
    "VerificationLayer",
    "VerificationReport",
    "verify_bundle",
]
