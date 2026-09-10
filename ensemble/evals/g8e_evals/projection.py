# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed safe public projection for campaign results.

Public projections contain only allowlisted fields: campaign identity,
variant identity, task identity, metric identity, numerator,
denominator, rate, unit, verification status, and evidence link. No raw
prompts, outputs, keys, credentials, private endpoints, machine-specific
paths, evidence-key metadata, or fields outside the explicit schema
allowlist cross the projection boundary.

The projection function fails closed on any prohibited field.
"""

from __future__ import annotations

import math
from pathlib import PurePosixPath

from pydantic import BaseModel, ConfigDict, Field, ValidationError


PROJECTION_SCHEMA_VERSION = "1.0.0"

_ALLOWED_FIELDS = frozenset({
    "campaign_id",
    "campaign_revision",
    "variant_id",
    "task_id",
    "metric_id",
    "numerator",
    "denominator",
    "rate",
    "unit",
    "verification_status",
    "evidence_link",
})

_PROHIBITED_FIELD_PATTERNS = frozenset({
    "raw_prompt",
    "raw_output",
    "api_key",
    "private_endpoint",
    "machine_path",
    "evidence_key",
    "credentials",
    "private_key",
    "secret",
    "password",
    "token",
    "session_id",
    "user_id",
    "chain_of_thought",
    "trail",
    "encrypted_artifact_location",
    "private_download_location",
})


class PublicProjectionError(ValueError):
    """Raised when a record cannot be safely projected to public."""


class PublicProjection(BaseModel):
    """Typed safe public projection of a campaign metric result.

    Contains only allowlisted fields. No raw prompts, outputs, keys,
    credentials, private endpoints, machine paths, or evidence-key
    metadata. The model is frozen with ``extra="forbid"``.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    variant_id: str = Field(min_length=1, description="Model variant ID.")
    task_id: str = Field(min_length=1, description="Task ID.")
    metric_id: str = Field(min_length=1, description="Metric ID.")
    numerator: int = Field(ge=0, description="Count of positive outcomes.")
    denominator: int = Field(ge=0, description="Total count of measured outcomes.")
    rate: float = Field(ge=0.0, le=1.0, description="Numerator divided by denominator.")
    unit: str = Field(min_length=1, description="Metric unit.")
    verification_status: str = Field(min_length=1, description="Verification status.")
    evidence_link: str = Field(min_length=1, description="Relative path to the public proof artifact.")


def _check_traversal(path: str) -> bool:
    """Return True if the path contains traversal sequences."""
    if not path:
        return False
    normalized = str(PurePosixPath(path))
    parts = PurePosixPath(normalized).parts
    return ".." in parts or normalized.startswith("..")


def project_to_public(record: dict) -> PublicProjection:
    """Project a private campaign record to a safe public projection.

    Fails closed on any prohibited field, unknown field, non-finite
    value, or path traversal in the evidence link. Returns a frozen
    ``PublicProjection`` with only allowlisted fields.
    """
    if not isinstance(record, dict):
        raise PublicProjectionError(f"record must be a dict, got {type(record).__name__}")

    for key in record:
        if key not in _ALLOWED_FIELDS:
            if key in _PROHIBITED_FIELD_PATTERNS:
                raise PublicProjectionError(
                    f"prohibited field in public projection: {key!r}"
                )
            raise PublicProjectionError(
                f"unknown field not in allowlist: {key!r}"
            )

    rate = record.get("rate")
    if rate is not None and not math.isfinite(rate):
        raise PublicProjectionError(f"non-finite rate value: {rate!r}")

    evidence_link = record.get("evidence_link", "")
    if _check_traversal(evidence_link):
        raise PublicProjectionError(
            f"traversal in evidence_link: {evidence_link!r}"
        )

    try:
        return PublicProjection(
            campaign_id=record["campaign_id"],
            campaign_revision=record["campaign_revision"],
            variant_id=record["variant_id"],
            task_id=record["task_id"],
            metric_id=record["metric_id"],
            numerator=record["numerator"],
            denominator=record["denominator"],
            rate=record["rate"],
            unit=record["unit"],
            verification_status=record["verification_status"],
            evidence_link=record["evidence_link"],
        )
    except KeyError as e:
        raise PublicProjectionError(f"missing required field: {e}") from e
    except ValidationError as e:
        raise PublicProjectionError(f"validation failed: {e}") from e


__all__ = [
    "PROJECTION_SCHEMA_VERSION",
    "PublicProjection",
    "PublicProjectionError",
    "project_to_public",
]
