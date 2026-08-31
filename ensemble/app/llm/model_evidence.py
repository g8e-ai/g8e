# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import base64
import hashlib
import json
from dataclasses import asdict, is_dataclass
from enum import Enum
from typing import Any

from pydantic import BaseModel

from app.models.model_telemetry import ModelBoundaryPrivacyAttestation


_MODEL_BOUNDARY_SCANNER_VERSION = "sentinel-regex@1.0.0"


def _json_value(value: Any) -> Any:
    if isinstance(value, BaseModel):
        return value.model_dump(mode="json")
    if is_dataclass(value) and not isinstance(value, type):
        return asdict(value)
    if isinstance(value, bytes):
        return {"base64": base64.b64encode(value).decode()}
    if isinstance(value, Enum):
        return value.value
    raise TypeError(f"Unsupported model evidence value: {type(value).__name__}")


def _canonical_model_boundary(value: Any) -> str:
    return json.dumps(
        value,
        default=_json_value,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )


def model_boundary_hash(value: Any) -> str:
    return hashlib.sha256(_canonical_model_boundary(value).encode()).hexdigest()


def model_boundary_privacy_attestation(value: Any) -> ModelBoundaryPrivacyAttestation:
    from app.security.sentinel_scrubber import inspect_sensitive_text

    canonical = _canonical_model_boundary(value)
    raw_sensitive_occurrences, raw_sensitive_types = inspect_sensitive_text(canonical)
    return ModelBoundaryPrivacyAttestation(
        scanner_version=_MODEL_BOUNDARY_SCANNER_VERSION,
        input_artifact_hash=hashlib.sha256(canonical.encode()).hexdigest(),
        raw_sensitive_occurrences=raw_sensitive_occurrences,
        raw_sensitive_types=raw_sensitive_types,
    )


def recorded_model_boundary_hash(provider: object, fallback: str) -> str:
    digest = getattr(provider, "input_artifact_hash", "")
    return digest if isinstance(digest, str) and digest else fallback


def recorded_model_boundary_privacy(
    provider: object,
) -> ModelBoundaryPrivacyAttestation | None:
    attestation = getattr(provider, "model_boundary_privacy", None)
    return attestation if isinstance(attestation, ModelBoundaryPrivacyAttestation) else None
