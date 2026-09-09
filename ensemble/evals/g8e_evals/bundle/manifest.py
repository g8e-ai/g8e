# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed, versioned immutable eval bundle manifest models.

The bundle manifest enumerates every included artifact path, media type,
privacy class, canonical hash, byte length, and semantic record identity.
It is the immutable contract that offline verification checks against the
actual bundle directory contents.

Public verification cannot require restricted plaintext or decryption keys.
Restricted artifacts are referenced by authenticated ciphertext and content
metadata without exposing secrets.
"""

from __future__ import annotations

from datetime import datetime
from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.schema import EvidenceEncryption


BUNDLE_MANIFEST_SCHEMA_VERSION = "1.0.0"


class PrivacyClass(StrEnum):
    """Privacy classification of a bundle artifact.

    ``PUBLIC`` artifacts are verifiable without restricted plaintext or
    decryption keys. ``INTERNAL`` artifacts are not published externally
    but do not require decryption. ``RESTRICTED`` artifacts are encrypted
    and referenced by authenticated ciphertext and content metadata.
    """

    PUBLIC = "public"
    INTERNAL = "internal"
    RESTRICTED = "restricted"


class ArtifactType(StrEnum):
    """Semantic record identity of a bundle artifact.

    Every artifact type that can appear in an eval bundle has exactly one
    enum value. Unknown types are rejected at deserialization.
    """

    RUN_MANIFEST = "run_manifest"
    PREREGISTRATION_MANIFEST = "preregistration_manifest"
    TASKS = "tasks"
    FIXTURES = "fixtures"
    ATTEMPTS = "attempts"
    POSTURE_OBSERVATIONS = "posture_observations"
    STATE_OBSERVATIONS = "state_observations"
    ENVELOPES = "envelopes"
    RECEIPTS = "receipts"
    PERSISTENCE_ATTESTATIONS = "persistence_attestations"
    COMMITMENT_ATTESTATIONS = "commitment_attestations"
    STAGES = "stages"
    METRICS = "metrics"
    AUDIT_LINKS = "audit_links"
    EVIDENCE_INDEX = "evidence_index"
    ANALYSIS_INPUT = "analysis_input"
    ANALYSIS_JSON = "analysis_json"
    ANALYSIS_MD = "analysis_md"
    ANALYSIS_HTML = "analysis_html"
    ANALYSIS_TXT = "analysis_txt"
    CHECKSUM = "checksum"
    BRIDGE_RECORDS = "bridge_records"
    TRUST_REFERENCES = "trust_references"
    EXTERNAL_REFERENCES = "external_references"
    OBSERVATION_FILE = "observation_file"
    DIAGNOSTIC = "diagnostic"


class BundleArtifactEntry(BaseModel):
    """One artifact entry in the bundle manifest.

    Enumerates a single file beneath the bundle root with its media type,
    privacy class, canonical SHA-256 hash, byte length, semantic record
    identity, record count (for JSONL files), and optional encryption
    metadata for restricted artifacts.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    path: str = Field(min_length=1, description="Rooted relative path beneath the bundle root.")
    media_type: str = Field(min_length=1, description="IANA media type of the artifact content.")
    privacy_class: PrivacyClass = Field(description="Public, internal, or restricted.")
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$", description="SHA-256 of the artifact content.")
    byte_length: int = Field(ge=0, description="Byte length of the artifact content.")
    artifact_type: ArtifactType = Field(description="Semantic record identity.")
    record_count: int = Field(default=0, ge=0, description="Number of records for JSONL files.")
    encryption: EvidenceEncryption | None = Field(
        default=None,
        description="Encryption metadata for restricted artifacts.",
    )


class ExternalReference(BaseModel):
    """A content-addressed reference to an artifact outside the bundle.

    External references are content-addressed by SHA-256 hash. They are
    not included in the bundle directory but are declared in the manifest
    so verification can check them if obtained out of band.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    reference_id: str = Field(min_length=1, description="Stable identifier for the external reference.")
    content_sha256: str = Field(pattern=r"^[0-9a-f]{64}$", description="SHA-256 of the external content.")
    byte_length: int = Field(ge=0, description="Byte length of the external content.")
    media_type: str = Field(min_length=1, description="IANA media type of the external content.")
    description: str = Field(default="", description="Human-readable description of the reference.")
    source_uri: str = Field(default="", description="Optional source URI for the external content.")


class BundleManifest(BaseModel):
    """Immutable, versioned bundle manifest enumerating every included artifact.

    The manifest is the immutable contract between the bundle producer and
    the offline verifier. It lists every artifact path, media type, privacy
    class, canonical hash, byte length, semantic record identity, and
    content-addressed external references. The ``manifest_content_sha256``
    field is computed over the canonical manifest bytes with that field set
    to empty string, enabling self-verification without a chicken-and-egg
    dependency.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(default=BUNDLE_MANIFEST_SCHEMA_VERSION)
    bundle_id: str = Field(min_length=1, description="Unique bundle identity.")
    run_id: str = Field(min_length=1, description="Run identity from the run manifest.")
    release_version: str = Field(min_length=1, description="Release version.")
    created_at: datetime = Field(description="Bundle creation timestamp.")

    artifacts: list[BundleArtifactEntry] = Field(
        min_length=1,
        description="Every included artifact, sorted by path in canonical form.",
    )

    external_references: list[ExternalReference] = Field(
        default_factory=list,
        description="Content-addressed references to artifacts outside the bundle.",
    )

    manifest_content_sha256: str = Field(
        default="",
        description="SHA-256 over canonical manifest bytes with this field empty.",
    )
    checksum_root_sha256: str = Field(
        default="",
        description="SHA-256 over the canonical checksum root bytes.",
    )


class ChecksumEntry(BaseModel):
    """One path and hash pair in the checksum root."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    path: str = Field(min_length=1, description="Rooted relative path beneath the bundle root.")
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$", description="SHA-256 of the artifact content.")


class ChecksumRoot(BaseModel):
    """Deterministic checksum file listing every artifact path and its hash.

    The checksum root is a separate file from the manifest. It lists every
    artifact path and SHA-256 hash, sorted by path. The
    ``checksum_root_sha256`` field is computed over the canonical checksum
    root bytes with that field set to empty string.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(default=BUNDLE_MANIFEST_SCHEMA_VERSION)
    entries: list[ChecksumEntry] = Field(
        min_length=1,
        description="Every path and hash pair, sorted by path in canonical form.",
    )

    checksum_root_sha256: str = Field(
        default="",
        description="SHA-256 over canonical checksum root bytes with this field empty.",
    )
