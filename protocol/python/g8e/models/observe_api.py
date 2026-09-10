# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed read models for the passkey-scoped observability read API
(/api/v1/observe). These are browser-session-authenticated, user-scoped,
read-only projections. See protocol/models/observe_api.json for the canonical
wire shapes.
"""

from typing import Literal

from .base import ConfigDict, G8eBaseModel, UTCDatetime, Field
from .events import ObservedMeasurement


AgentLifecycleStatus = Literal[
    "idle", "queued", "running", "waiting", "completed", "failed", "offline"
]

RunLifecycleStatus = Literal[
    "queued", "running", "waiting", "completed", "failed", "cancelled"
]

RunKind = Literal["investigation", "eval", "demo", "workflow"]

EvalVerificationStatus = Literal[
    "projection_validated", "receipt_verification_not_applicable", "verified"
]

SnapshotFreshness = Literal["observed", "stale", "unavailable"]

DownloadPrivacyClassification = Literal["public_safe", "restricted"]


class AgentStateProjection(G8eBaseModel):
    """Current lifecycle state of a single agent persona in the roster."""

    schema_version: str
    agent_id: str
    display_name: str
    role: str
    status: AgentLifecycleStatus
    run_id: str | None = None
    task_id: str | None = None
    model: str | None = None
    throughput: ObservedMeasurement | None = None
    freshness: SnapshotFreshness
    observed_at: UTCDatetime


class SuccessRateMeasurement(G8eBaseModel):
    """Aggregate success-rate measurement with metric ID, version, denominator,
    and verification status."""

    metric_id: str
    metric_version: str
    value: float
    unit: str
    denominator: int
    verification_status: EvalVerificationStatus


class OverviewCounters(G8eBaseModel):
    """Bounded overview counters for the dashboard header."""

    schema_version: str
    agents_running: int
    agents_running_freshness: SnapshotFreshness
    tasks_in_queue: int
    tasks_in_queue_freshness: SnapshotFreshness
    success_rate: SuccessRateMeasurement | None = None
    generated_at: UTCDatetime


class OverviewMeasurements(G8eBaseModel):
    """Resource and throughput measurements for the dashboard."""

    schema_version: str
    total_throughput: ObservedMeasurement | None = None
    cpu: ObservedMeasurement | None = None
    ram: ObservedMeasurement | None = None
    vram: ObservedMeasurement | None = None
    disk: ObservedMeasurement | None = None


class RunSummary(G8eBaseModel):
    """Paginated read-only run summary owned by the authenticated user."""

    schema_version: str
    run_id: str
    run_kind: RunKind
    display_name: str
    status: RunLifecycleStatus
    active_task_id: str | None = None
    completed_tasks: int
    total_tasks: int
    started_at: UTCDatetime | None = None
    ended_at: UTCDatetime | None = None
    has_receipts: bool
    evidence_count: int
    observed_at: UTCDatetime


class RunTask(G8eBaseModel):
    """A single task within a run detail projection."""

    schema_version: str
    task_id: str
    display_name: str | None = None
    status: RunLifecycleStatus
    owning_agent: str | None = None
    model: str | None = None
    started_at: UTCDatetime | None = None
    ended_at: UTCDatetime | None = None


class EvidenceSafeLink(G8eBaseModel):
    """An evidence-safe link in a run detail projection."""

    schema_version: str
    artifact_id: str
    label: str
    media_type: str


class RunDetail(G8eBaseModel):
    """Typed run detail projection with tasks and evidence-safe links."""

    schema_version: str
    run_id: str
    run_kind: RunKind
    display_name: str
    status: RunLifecycleStatus
    active_task_id: str | None = None
    completed_tasks: int
    total_tasks: int
    tasks: list[RunTask]
    evidence_safe_links: list[EvidenceSafeLink]
    started_at: UTCDatetime | None = None
    ended_at: UTCDatetime | None = None
    observed_at: UTCDatetime


class EvalSummary(G8eBaseModel):
    """Paginated eval run projection."""

    schema_version: str
    run_id: str
    suite_id: str
    suite_version: str
    arm_id: str
    status: RunLifecycleStatus
    verification_status: EvalVerificationStatus
    receipt_count: int
    metric_count: int
    published_projection_sha256: str | None = None
    completed_at: UTCDatetime | None = None
    observed_at: UTCDatetime


class EvalMetricSummary(G8eBaseModel):
    """A single registered metric in an eval detail projection."""

    schema_version: str
    metric_id: str
    metric_version: str
    value: float | None = None
    unit: str
    eligible: int
    denominator: int
    verification_status: EvalVerificationStatus
    recorded_at: UTCDatetime | None = None


class EvalDetail(G8eBaseModel):
    """Typed eval detail projection."""

    schema_version: str
    run_id: str
    suite_id: str
    suite_version: str
    arm_id: str
    model_id: str | None = None
    model_provider: str | None = None
    status: RunLifecycleStatus
    verification_status: EvalVerificationStatus
    receipt_count: int
    assigned_tasks: int
    terminal_attempts: int
    metrics: list[EvalMetricSummary]
    published_projection_sha256: str | None = None
    completed_at: UTCDatetime | None = None
    observed_at: UTCDatetime


class DownloadArtifact(G8eBaseModel):
    """An allowlisted generated public-safe artifact in the download catalog."""

    schema_version: str
    artifact_id: str
    filename: str
    media_type: str
    byte_size: int
    sha256: str
    privacy_classification: DownloadPrivacyClassification
    source_run_id: str | None = None
    download_url: str
    generated_at: UTCDatetime


class ObserveBootstrapSnapshot(G8eBaseModel):
    """One bounded initial snapshot for first paint."""

    schema_version: str
    agents: list[AgentStateProjection]
    active_run: RunSummary | None = None
    overview: OverviewCounters
    measurements: OverviewMeasurements
    recent_runs: list[RunSummary]
    latest_evals: list[EvalSummary]
    downloads: list[DownloadArtifact]
    generated_at: UTCDatetime


class ObserveProducerAgentStateRequest(G8eBaseModel):
    """Typed request body for POST /api/v1/observe/producer/agent-state.

    Carries the AgentStatusUpdatedPayload fields plus the SSE routing target
    (exactly one of web_session_id or cli_session_id). The gateway derives
    user_id from the mTLS peer certificate, never from the request body.
    Unknown fields are rejected at the protocol boundary.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    schema_version: str
    agent_id: str
    display_name: str
    role: str
    status: AgentLifecycleStatus
    run_id: str | None = None
    task_id: str | None = None
    model: str | None = None
    observed_at: UTCDatetime
    web_session_id: str | None = None
    cli_session_id: str | None = None


class ObserveProducerRunStateRequest(G8eBaseModel):
    """Typed request body for POST /api/v1/observe/producer/run-state.

    Carries the RunStatusUpdatedPayload fields plus the SSE routing target.
    The gateway derives user_id from the mTLS peer certificate, never from
    the request body. Unknown fields are rejected at the protocol boundary.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    schema_version: str
    run_id: str
    run_kind: RunKind
    display_name: str
    status: RunLifecycleStatus
    active_task_id: str | None = None
    completed_tasks: int
    total_tasks: int
    started_at: UTCDatetime | None = None
    ended_at: UTCDatetime | None = None
    observed_at: UTCDatetime
    web_session_id: str | None = None
    cli_session_id: str | None = None


class ObserveProducerResponse(G8eBaseModel):
    """Typed response body for the mTLS producer endpoints.

    Carries a single accepted flag indicating the gateway accepted and
    persisted the projection. Contains no record identifiers, ownership
    fields, or echo of the request payload.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    accepted: bool = False


class EvidenceEncryptionWire(G8eBaseModel):
    """Wire type mirroring the Python EvidenceEncryption model.

    Restricted artifacts carry encryption metadata; public artifacts do not.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    algorithm: Literal["aes-256-gcm"]
    key_id: str
    aad_sha256: str
    ciphertext_sha256: str
    ciphertext_byte_length: int


class BundleArtifactEntryWire(G8eBaseModel):
    """Wire type mirroring the Python BundleArtifactEntry model.

    Each entry enumerates a single file beneath the bundle root with its
    media type, privacy class, canonical SHA-256 hash, byte length,
    semantic record identity, record count, and optional encryption
    metadata for restricted artifacts.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    path: str
    media_type: str
    privacy_class: Literal["public", "internal", "restricted"]
    sha256: str
    byte_length: int
    artifact_type: str
    record_count: int = 0
    encryption: EvidenceEncryptionWire | None = None


class ExternalReferenceWire(G8eBaseModel):
    """Wire type mirroring the Python ExternalReference model.

    A content-addressed reference to an artifact outside the bundle.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    reference_id: str
    content_sha256: str
    byte_length: int
    media_type: str
    description: str = ""
    source_uri: str = ""


class BundleManifestWire(G8eBaseModel):
    """Wire type mirroring the Python BundleManifest model.

    The immutable, versioned bundle manifest enumerating every included
    artifact. The publication path reads this as a typed input from the
    verified bundle; it does not recompute canonical analysis.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    schema_version: str
    bundle_id: str
    run_id: str
    release_version: str
    created_at: UTCDatetime
    artifacts: list[BundleArtifactEntryWire]
    external_references: list[ExternalReferenceWire] = Field(default_factory=list)
    manifest_content_sha256: str = ""
    checksum_root_sha256: str = ""


class LayerResultWire(G8eBaseModel):
    """Wire type mirroring the Python LayerResult model.

    The result of one verification layer.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    layer: int
    passed: bool
    failure_count: int


class VerificationFailureWire(G8eBaseModel):
    """Wire type mirroring the Python VerificationFailure model.

    One typed verification failure with stable code, layer, and record
    identity.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    layer: int
    code: str
    record_id: str = ""
    message: str


class VerificationReportWire(G8eBaseModel):
    """Wire type mirroring the Python VerificationReport model.

    The typed deterministic verification report. ok is true only when
    every layer passed with zero failures. The publication path requires
    ok=true to publish with the verified status; a receipt-only or
    partial verification cannot produce it.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    schema_version: str
    bundle_id: str = ""
    run_id: str = ""
    release_version: str = ""
    verified_at: UTCDatetime
    ok: bool
    layers: list[LayerResultWire]
    failures: list[VerificationFailureWire] = Field(default_factory=list)


class ObserveProducerDownloadArtifactInput(G8eBaseModel):
    """One entry in the publication request's download catalog.

    Carries the artifact metadata and base64 content for a single
    public-safe artifact. Restricted artifacts are never transmitted;
    only public_safe artifacts appear in the download catalog.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    artifact_id: str
    filename: str
    media_type: str
    byte_size: int
    sha256: str
    privacy_classification: DownloadPrivacyClassification
    source_run_id: str
    content: str


class ObserveProducerEvalPublicationRequest(G8eBaseModel):
    """Typed request body for POST /api/v1/observe/producer/eval-publication.

    Carries the verified bundle manifest, the verification report, and
    the download catalog with base64 content for public-safe artifacts.
    The gateway derives user_id from the mTLS peer certificate, never
    from the request body. The request carries no user_id field; unknown
    identity fields are rejected.
    """

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    schema_version: str
    bundle_id: str
    run_id: str
    release_version: str
    verification_report: VerificationReportWire
    bundle_manifest: BundleManifestWire
    downloads: list[ObserveProducerDownloadArtifactInput]
    web_session_id: str | None = None
    cli_session_id: str | None = None
