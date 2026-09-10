# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Publish a verified eval bundle into the Gateway observe projections.

The publication path runs the complete fail-closed bundle verifier, reads
the typed bundle manifest and canonical analysis, builds a typed
``ObserveProducerEvalPublicationRequest`` from the verified fields, and
transmits only public-safe artifacts to the Gateway's mTLS producer
endpoint. Restricted artifacts are never transmitted; the publication
request carries only ``PUBLIC`` artifacts in its download catalog.

The publisher does not recompute the canonical analysis. It reads the
analysis and run manifest that the bundle verifier already validated.
"""

from __future__ import annotations

import base64
import logging
from pathlib import Path
from typing import TYPE_CHECKING, Literal

from g8e.constants import API_PATHS, CLI_SESSION_ID_HEADER
from g8e.models.observe_api import (
    BundleArtifactEntryWire,
    BundleManifestWire,
    EvidenceEncryptionWire,
    EvalMetricSummary,
    EvalVerificationStatus,
    ExternalReferenceWire,
    LayerResultWire,
    ObserveProducerDownloadArtifactInput,
    ObserveProducerEvalPublicationRequest,
    ObserveProducerResponse,
    VerificationFailureWire,
    VerificationReportWire,
)

from g8e_evals import constants as evals_constants
from g8e_evals.analysis import CanonicalEvalAnalysis, canonical_model_json
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle.manifest import BundleManifest, PrivacyClass
from g8e_evals.bundle.signing import EvalTrustStore
from g8e_evals.bundle.verify import VerificationReport, verify_bundle
from g8e_evals.schema import EvidenceEncryptionAlgorithm, RunManifest, VerificationStatus

if TYPE_CHECKING:
    from g8e_evals.transport import AuthContext

logger = logging.getLogger(__name__)

# Schema versions mirrored from the Gateway constants. These are the
# wire schema versions the Gateway validates, not the evals-internal
# analysis schema versions.
_OBSERVE_PUBLICATION_SCHEMA_VERSION = "1.0.0"
_OBSERVE_EVENT_PAYLOAD_SCHEMA_VERSION = "1.0.0"
_VERIFICATION_REPORT_SCHEMA_VERSION = "1.0.0"
_BUNDLE_MANIFEST_SCHEMA_VERSION = "1.0.0"


class PublicationError(Exception):
    """Raised when bundle verification or publication fails."""


def _map_metric_verification_status(counts: dict[str, int]) -> EvalVerificationStatus:
    """Map per-metric verification status counts to a single wire status.

    The wire ``EvalVerificationStatus`` has three values:
    ``verified``, ``receipt_verification_not_applicable``, and
    ``projection_validated``. The canonical analysis counts observations
    by the evals-internal ``VerificationStatus`` enum. A metric whose
    observations are all verified maps to ``verified``. A metric whose
    observations are all not-applicable maps to
    ``receipt_verification_not_applicable``. Mixed or partial status
    maps to ``projection_validated`` (the bundle is verified, but
    individual receipt verification is not uniform).
    """
    verified_count = counts.get(VerificationStatus.VERIFIED.value, 0)
    not_applicable_count = counts.get(VerificationStatus.NOT_APPLICABLE.value, 0)
    total = sum(counts.values())
    if total == 0:
        return "projection_validated"
    if verified_count == total:
        return "verified"
    if not_applicable_count == total:
        return "receipt_verification_not_applicable"
    return "projection_validated"


def _build_metric_summaries(analysis: CanonicalEvalAnalysis) -> list[EvalMetricSummary]:
    """Build wire metric summaries from the canonical analysis results.

    Each ``MetricAnalysisResult`` maps to one ``EvalMetricSummary``. The
    eligible count and denominator come directly from the analysis. The
    verification status is derived from the per-metric observation status
    counts. Each summary carries ``model_cohort_id`` and ``arm_id`` from the
    analysis result so the Gateway persists cohort-and-arm-stratified
    metrics.
    """
    summaries: list[EvalMetricSummary] = []
    for result in analysis.metric_results:
        wire_status = _map_metric_verification_status(result.verification_status_counts)
        summaries.append(
            EvalMetricSummary(
                schema_version=_OBSERVE_EVENT_PAYLOAD_SCHEMA_VERSION,
                metric_id=result.metric_id,
                metric_version=result.metric_version,
                model_cohort_id=result.model_cohort_id,
                arm_id=result.arm_id,
                value=result.value,
                unit=result.unit,
                eligible=result.eligible_count,
                denominator=result.denominator,
                verification_status=wire_status,
            )
        )
    return summaries


def _build_verification_report_wire(report: VerificationReport) -> VerificationReportWire:
    """Build the wire verification report from the typed verifier report."""
    layers = [
        LayerResultWire(layer=lr.layer.value, passed=lr.passed, failure_count=lr.failure_count)
        for lr in report.layers
    ]
    failures = [
        VerificationFailureWire(
            layer=f.layer.value,
            code=f.code.value,
            record_id=f.record_id,
            message=f.message,
        )
        for f in report.failures
    ]
    return VerificationReportWire(
        schema_version=_VERIFICATION_REPORT_SCHEMA_VERSION,
        bundle_id=report.bundle_id,
        run_id=report.run_id,
        release_version=report.release_version,
        verified_at=report.verified_at,
        ok=report.ok,
        layers=layers,
        failures=failures,
    )


def _resolve_encryption_algorithm(algorithm: EvidenceEncryptionAlgorithm) -> Literal["aes-256-gcm"]:
    """Resolve the evals-internal encryption algorithm to the wire literal.

    The wire ``EvidenceEncryptionWire.algorithm`` accepts only
    ``Literal["aes-256-gcm"]``. The evals-internal
    ``EvidenceEncryptionAlgorithm`` enum currently has one member with
    the same value. This function validates the enum member and returns
    the protocol-owned literal so Pyright can verify type safety without
    casts or type ignores.
    """
    if algorithm is EvidenceEncryptionAlgorithm.AES_256_GCM:
        return "aes-256-gcm"
    raise PublicationError(f"Unsupported encryption algorithm: {algorithm}")


def _build_manifest_wire(manifest: BundleManifest) -> BundleManifestWire:
    """Build the wire bundle manifest from the typed bundle manifest."""
    artifacts = [
        BundleArtifactEntryWire(
            path=a.path,
            media_type=a.media_type,
            privacy_class=a.privacy_class.value,
            sha256=a.sha256,
            byte_length=a.byte_length,
            artifact_type=a.artifact_type.value,
            record_count=a.record_count,
            encryption=(
                EvidenceEncryptionWire(
                    algorithm=_resolve_encryption_algorithm(a.encryption.algorithm),
                    key_id=a.encryption.key_id,
                    aad_sha256=a.encryption.aad_sha256,
                    ciphertext_sha256=a.encryption.ciphertext_sha256,
                    ciphertext_byte_length=a.encryption.ciphertext_byte_length,
                )
                if a.encryption is not None
                else None
            ),
        )
        for a in manifest.artifacts
    ]
    external_refs = [
        ExternalReferenceWire(
            reference_id=ref.reference_id,
            content_sha256=ref.content_sha256,
            byte_length=ref.byte_length,
            media_type=ref.media_type,
            description=ref.description,
            source_uri=ref.source_uri,
        )
        for ref in manifest.external_references
    ]
    return BundleManifestWire(
        schema_version=_BUNDLE_MANIFEST_SCHEMA_VERSION,
        bundle_id=manifest.bundle_id,
        run_id=manifest.run_id,
        release_version=manifest.release_version,
        created_at=manifest.created_at,
        artifacts=artifacts,
        external_references=external_refs,
        manifest_content_sha256=manifest.manifest_content_sha256,
        checksum_root_sha256=manifest.checksum_root_sha256,
    )


def _build_downloads(
    bundle_dir: Path, manifest: BundleManifest, run_id: str,
) -> list[ObserveProducerDownloadArtifactInput]:
    """Build the download catalog from public artifacts only.

    Reads only ``PUBLIC`` artifacts from the bundle directory, base64-
    encodes their content, and constructs typed download entries.
    Restricted and internal artifacts are never transmitted; only public-
    safe artifacts appear in the download catalog.
    """
    downloads: list[ObserveProducerDownloadArtifactInput] = []
    for entry in manifest.artifacts:
        if entry.privacy_class != PrivacyClass.PUBLIC:
            continue
        artifact_path = bundle_dir / entry.path
        content_bytes = artifact_path.read_bytes()
        content_b64 = base64.b64encode(content_bytes).decode("ascii")
        # Derive a stable artifact ID from the public path. The Gateway
        # validates artifact_id as a rooted relative path (no absolute,
        # no "..", no backslash).
        artifact_id = entry.path
        filename = Path(entry.path).name
        downloads.append(
            ObserveProducerDownloadArtifactInput(
                artifact_id=artifact_id,
                filename=filename,
                media_type=entry.media_type,
                byte_size=entry.byte_length,
                sha256=entry.sha256,
                privacy_classification="public_safe",
                source_run_id=run_id,
                content=content_b64,
            )
        )
    return downloads


def _read_analysis_input(bundle_dir: Path) -> AnalysisInputRecord:
    """Read the analysis-input record from the bundle directory.

    The analysis-input record carries the campaign manifest, model cohorts,
    and campaign assignments needed to derive campaign dimensions for the
    publication request. Returns a record with empty campaign fields when
    the bundle has no analysis-input file (single-arm diagnostic runs).
    """
    analysis_input_path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
    if not analysis_input_path.exists():
        return AnalysisInputRecord(
            run_id="",
            release_version="",
            tasks=[],
            attempts=[],
            metric_observations=[],
        )
    return AnalysisInputRecord.model_validate_json(analysis_input_path.read_text())


def _derive_campaign_fields(
    analysis: CanonicalEvalAnalysis,
    analysis_input: AnalysisInputRecord,
) -> tuple[str, list[str], list[str], int]:
    """Derive campaign dimensions from the canonical analysis and analysis input.

    Returns (campaign_id, arm_ids, model_cohort_ids, assignment_count).
    For single-arm diagnostic runs without campaign records, campaign_id is
    empty, arm_ids is the single arm from the analysis, model_cohort_ids is
    empty, and assignment_count is zero.

    For campaign runs, the campaign_id and model_cohort_ids are derived from
    the verified campaign records in the analysis input. The arm_ids are
    derived from the canonical analysis. The assignment_count is derived from
    the analysis input summary. Rejects missing, duplicate, or cross-bound
    campaign strata: every metric result's model_cohort_id and arm_id must be
    in the declared sets.
    """
    arm_ids = list(analysis.arm_ids)
    if not arm_ids:
        raise PublicationError(
            "canonical analysis has no arm_ids; cannot build campaign-aware publication request"
        )

    campaign_manifest = analysis_input.campaign_manifest
    if campaign_manifest is None:
        # Single-arm diagnostic run: no campaign records.
        return "", arm_ids, [], 0

    campaign_id = campaign_manifest.campaign_id
    cohort_ids = sorted({c.cohort_id for c in analysis_input.model_cohorts})
    assignment_count = analysis.input_summary.assignment_count

    # Reject cross-bound strata: every metric result's model_cohort_id and
    # arm_id must be in the declared sets.
    declared_arms = set(arm_ids)
    declared_cohorts = set(cohort_ids)
    for result in analysis.metric_results:
        if result.arm_id not in declared_arms:
            raise PublicationError(
                f"metric {result.metric_id} arm_id {result.arm_id!r} not in declared arm_ids {arm_ids}"
            )
        if result.model_cohort_id and result.model_cohort_id not in declared_cohorts:
            raise PublicationError(
                f"metric {result.metric_id} model_cohort_id {result.model_cohort_id!r} "
                f"not in declared model_cohort_ids {cohort_ids}"
            )

    return campaign_id, arm_ids, cohort_ids, assignment_count


def build_publication_request(
    bundle_dir: Path,
    manifest: BundleManifest,
    analysis: CanonicalEvalAnalysis,
    run_manifest: RunManifest,
    report: VerificationReport,
    web_session_id: str | None,
    cli_session_id: str | None,
) -> ObserveProducerEvalPublicationRequest:
    """Build a typed campaign-aware publication request from verified bundle fields.

    Accepts the already-run verification report so the caller controls
    when verification runs. Reads the typed bundle manifest, canonical
    analysis, run manifest, and analysis-input record to populate the
    campaign-aware eval projection fields. Campaign dimensions (campaign_id,
    arm_ids, model_cohort_ids, assignment_count) are derived from the
    canonical analysis and the verified campaign records in the analysis
    input. Reads only ``PUBLIC`` artifacts from the bundle directory and
    base64-encodes their content for the download catalog. Restricted
    and internal artifacts are never included. Does not recompute the
    canonical analysis.
    """
    bundle_dir = Path(bundle_dir)
    analysis_input = _read_analysis_input(bundle_dir)
    campaign_id, arm_ids, model_cohort_ids, assignment_count = _derive_campaign_fields(
        analysis, analysis_input
    )
    metrics = _build_metric_summaries(analysis)
    downloads = _build_downloads(bundle_dir, manifest, analysis.run_id)
    return ObserveProducerEvalPublicationRequest(
        schema_version=_OBSERVE_PUBLICATION_SCHEMA_VERSION,
        bundle_id=manifest.bundle_id,
        run_id=manifest.run_id,
        release_version=manifest.release_version,
        suite_id=run_manifest.suite_id,
        suite_version=run_manifest.suite_version,
        campaign_id=campaign_id,
        arm_ids=arm_ids,
        model_cohort_ids=model_cohort_ids,
        assignment_count=assignment_count,
        receipt_count=analysis.input_summary.receipt_count,
        assigned_tasks=analysis.input_summary.task_count,
        terminal_attempts=analysis.input_summary.attempt_count,
        metrics=metrics,
        verification_report=_build_verification_report_wire(report),
        bundle_manifest=_build_manifest_wire(manifest),
        downloads=downloads,
        web_session_id=web_session_id,
        cli_session_id=cli_session_id,
    )


def _read_bundle_analysis(bundle_dir: Path) -> CanonicalEvalAnalysis:
    """Read the canonical analysis from the bundle directory."""
    analysis_path = bundle_dir / evals_constants.ANALYSIS_JSON
    return CanonicalEvalAnalysis.model_validate_json(analysis_path.read_text())


def _read_run_manifest(bundle_dir: Path) -> RunManifest:
    """Read the run manifest from the bundle directory."""
    manifest_path = bundle_dir / evals_constants.MANIFEST_JSON
    return RunManifest.model_validate_json(manifest_path.read_text())


def _read_bundle_manifest(bundle_dir: Path) -> BundleManifest:
    """Read the typed bundle manifest from the bundle directory."""
    manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
    return BundleManifest.model_validate_json(manifest_path.read_text())


async def publish_bundle(
    bundle_dir: Path,
    gateway_url: str,
    auth_context: AuthContext,
    web_session_id: str | None,
    cli_session_id: str | None,
    trust_store: EvalTrustStore | None = None,
) -> ObserveProducerResponse:
    """Verify and publish a bundle to the Gateway observe projections.

    Runs the complete fail-closed bundle verifier. If verification
    fails, publication is rejected with a ``PublicationError``. On
    success, reads the typed manifest, canonical analysis, and run
    manifest, builds the typed publication request, and transmits it to
    the Gateway's mTLS producer endpoint via the canonical transport.

    Only ``PUBLIC`` artifacts are transmitted. Restricted and internal
    artifacts are never included in the download catalog. The Gateway
    derives user_id from the mTLS peer certificate, never from the
    request body.
    """
    bundle_dir = Path(bundle_dir)

    # Run complete fail-closed verification before publication.
    report = verify_bundle(bundle_dir, trust_store=trust_store)
    if not report.ok:
        failures = [(f.layer.name, f.code.value, f.message) for f in report.failures]
        raise PublicationError(
            f"Bundle verification failed; publication rejected. Failures: {failures}"
        )

    manifest = _read_bundle_manifest(bundle_dir)
    analysis = _read_bundle_analysis(bundle_dir)
    run_manifest = _read_run_manifest(bundle_dir)

    # Build the publication request with the verified report.
    request = build_publication_request(
        bundle_dir, manifest, analysis, run_manifest, report, web_session_id, cli_session_id,
    )

    # Transmit via the canonical mTLS transport.
    path = API_PATHS["observe_producer_eval_publication"]
    url = f"{gateway_url.rstrip('/')}{path}"
    headers = auth_context.auth_headers()
    if cli_session_id:
        headers[CLI_SESSION_ID_HEADER] = cli_session_id

    body = canonical_model_json(request)
    async with auth_context.make_async_client() as client:
        resp = await client.post(url, content=body, headers=headers)
    if resp.status_code != 200:
        raise PublicationError(
            f"Gateway rejected publication: HTTP {resp.status_code}: {resp.text}"
        )
    return ObserveProducerResponse.model_validate_json(resp.text)


__all__ = [
    "PublicationError",
    "build_publication_request",
    "publish_bundle",
]
