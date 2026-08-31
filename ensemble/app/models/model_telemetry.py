# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from pydantic import Field

from app.models.base import G8eBaseModel


class ModelBoundaryPrivacyAttestation(G8eBaseModel):
    scanner_version: str
    input_artifact_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    raw_sensitive_occurrences: int = Field(ge=0)
    raw_sensitive_types: list[str] = Field(default_factory=list)


class ModelCallTelemetry(G8eBaseModel):
    agent_role: str
    provider: str
    model: str
    monotonic_start: float
    monotonic_end: float
    input_tokens: int = 0
    output_tokens: int = 0
    thinking_tokens: int = 0
    cache_tokens: int = 0
    total_tokens: int = 0
    usage_reported: bool = False
    finish_reason: str | None = None
    retry_count: int = 0
    succeeded: bool = True
    error_type: str | None = None
    input_artifact_hash: str = ""
    output_artifact_hash: str = ""
    model_boundary_privacy: ModelBoundaryPrivacyAttestation | None = None
