# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed wire models for the public spectator feed protocol. These mirror
the Go models in internal/models/public_feed.go and the canonical protocol
JSON in protocol/models/public_feed.json.

The Gateway outbound publisher signs and sends append-only batches to a
hosted mirror's authenticated ingest endpoint. Public browsers connect to
the mirror's anonymous read-only endpoints (bootstrap, cursor-paginated
history, replayable SSE, content-addressed proof downloads). The private
Gateway receives no public connection.
"""

from typing import Any, Literal

from .base import ConfigDict, G8eBaseModel, UTCDatetime, Field

# ---------------------------------------------------------------------------
# Closed-vocabulary enum types
# ---------------------------------------------------------------------------

PublicFeedRecordType = Literal["projection", "event", "proof_manifest", "key_revocation"]

PublicFeedOutboxStatus = Literal["pending", "sent", "acknowledged", "failed"]

PublicFeedIngestRejectionReason = Literal[
    "signature_invalid",
    "sequence_out_of_order",
    "hash_chain_mismatch",
    "duplicate_sequence",
    "oversized_batch",
    "revoked_key",
    "unknown_key",
]

PublicFeedProofClassification = Literal["public_safe"]

CampaignFreshness = Literal[
    "active",
    "delayed",
    "stale",
    "intentionally_stopped",
    "safety_stopped",
    "source_offline",
]


# ---------------------------------------------------------------------------
# Batch and record models
# ---------------------------------------------------------------------------


class PublicFeedRecord(G8eBaseModel):
    """One record in a signed batch. A record is a serialized projection or
    event payload with its SHA-256 content hash."""

    sequence: int
    record_type: PublicFeedRecordType
    record_hash: str
    record_bytes: str


class PublicFeedBatch(G8eBaseModel):
    """A signed append-only batch binding source, protocol version, schema
    version, monotonic sequence range, prior-batch hash, record hashes,
    generated time, content hash, signing key ID, and Ed25519 signature."""

    protocol_version: str
    schema_version: str
    source_id: str
    first_sequence: int
    last_sequence: int
    previous_batch_hash: str
    record_hashes: list[str]
    generated_at: UTCDatetime
    content_hash: str
    signing_key_id: str
    signature: str
    records: list[PublicFeedRecord]


class PublicFeedSnapshot(G8eBaseModel):
    """A snapshot binding the high-water sequence (the last accepted
    sequence) and the feed-chain hash (the content hash of the last
    accepted batch)."""

    protocol_version: str
    source_id: str
    high_water_sequence: int
    feed_chain_hash: str
    batch_count: int
    generated_at: UTCDatetime
    freshness: CampaignFreshness


class PublicProofCatalogSummary(G8eBaseModel):
    """Summary of the public proof catalog for bootstrap responses."""

    artifact_count: int
    total_byte_size: int
    last_generated_at: UTCDatetime | None = None


class PublicFeedBootstrap(G8eBaseModel):
    """One bounded initial snapshot for public first paint. Contains the
    feed snapshot, current source freshness, recent projections, and proof
    catalog summary."""

    protocol_version: str
    snapshot: PublicFeedSnapshot
    source_freshness: CampaignFreshness
    recent_projections: list[dict[str, Any]]
    proof_catalog_summary: PublicProofCatalogSummary
    generated_at: UTCDatetime


class PublicFeedCursorPage(G8eBaseModel):
    """Cursor-paginated page of public records. Uses stable cursors and
    bounded page sizes (1-100, default 20)."""

    protocol_version: str
    items: list[dict[str, Any]]
    cursor: str | None = None
    has_more: bool
    limit: int


# ---------------------------------------------------------------------------
# Ingest request/response models
# ---------------------------------------------------------------------------


class PublicIngestRequest(G8eBaseModel):
    """Authenticated ingest request from the Gateway outbound publisher to
    the hosted mirror. Carries the signed batch."""

    batch: PublicFeedBatch


class PublicIngestResponse(G8eBaseModel):
    """Authenticated ingest response from the mirror. Carries the accepted
    flag, the updated high-water sequence, and the updated feed-chain hash.
    A rejected batch carries a typed rejection reason."""

    accepted: bool
    high_water_sequence: int | None = None
    feed_chain_hash: str | None = None
    rejection_reason: PublicFeedIngestRejectionReason | None = None


# ---------------------------------------------------------------------------
# Key revocation model
# ---------------------------------------------------------------------------


class PublicKeyRevocationRecord(G8eBaseModel):
    """A key revocation record published as a signed batch record before the
    old key is deactivated."""

    revoked_key_id: str
    revoked_at: UTCDatetime
    new_key_id: str
    revocation_signature: str


# ---------------------------------------------------------------------------
# Proof package models
# ---------------------------------------------------------------------------


class PublicProofCatalogEntry(G8eBaseModel):
    """One entry in the public proof catalog. Each artifact is served under
    an immutable content-addressed identity derived from its SHA-256."""

    artifact_id: str
    filename: str
    media_type: str
    byte_size: int
    sha256: str
    classification: PublicFeedProofClassification
    campaign_id: str
    source_run_id: str | None = None
    generated_at: UTCDatetime
    verification_command: str
    immutable_url: str


class PublicProofManifest(G8eBaseModel):
    """Root manifest for a public proof package. Binds the proof root
    SHA-256, campaign identity, verified index generation hash, artifact
    count, artifact catalog, verifier instructions, and generation time."""

    schema_version: str
    proof_root_sha256: str
    campaign_id: str
    campaign_revision: str
    verified_index_generation_hash: str
    verification_ok: bool
    artifact_count: int
    artifacts: list[PublicProofCatalogEntry]
    verifier_instructions: str
    generated_at: UTCDatetime
    signing_key_id: str
    signature: str


class PublicProofCatalog(G8eBaseModel):
    """The full public proof catalog listing every available proof package
    artifact. Served as a bounded anonymous read."""

    schema_version: str
    entries: list[PublicProofCatalogEntry]
    generated_at: UTCDatetime


# ---------------------------------------------------------------------------
# Export configuration and outbox models
# ---------------------------------------------------------------------------


class PublicExportConfig(G8eBaseModel):
    """Owner-only export configuration for the outbound publisher. Carries
    the allowlisted HTTPS origin, signing key ID, pseudonymous source ID,
    batch size limit, and retry policy. The private key is stored in the
    Gateway runtime tree, not in this configuration."""

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    enabled: bool
    mirror_origin: str
    source_id: str
    signing_key_id: str
    batch_max_records: int
    batch_max_bytes: int
    retry_max_attempts: int
    retry_initial_backoff_seconds: int
    retry_max_backoff_seconds: int
    ack_confirmation_window_seconds: int


class PublicOutboxEntry(G8eBaseModel):
    """One entry in the durable ordered outbox. The outbound publisher
    writes each batch to the outbox before network transmission."""

    sequence: int
    batch_hash: str
    batch_bytes: str
    status: PublicFeedOutboxStatus
    attempts: int
    last_attempt_at: UTCDatetime | None = None
    acknowledged_at: UTCDatetime | None = None
    created_at: UTCDatetime
