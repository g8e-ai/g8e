# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Provenance models and validation for the IFEval subset dataset.

The IFEval subset is imported from the upstream google-research
instruction-following-eval dataset.  The provenance manifest captures the
upstream source URL, revision, license, transformation code hash, the
output dataset hash, the row count, the data partition, and the domain
strata.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

from pydantic import BaseModel, field_validator


class DatasetSource(BaseModel):
    url: str
    revision: str
    license_spdx: str
    license_url: str
    sha256: str

    @field_validator("url", "revision", "license_spdx", "license_url", "sha256")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("source field must not be empty")
        return v


class DatasetTransformation(BaseModel):
    description: str
    code_path: str
    code_sha256: str
    fixture_path: str
    fixture_sha256: str

    @field_validator("description", "code_path", "code_sha256", "fixture_path", "fixture_sha256")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("transformation field must not be empty")
        return v


class DatasetOutput(BaseModel):
    path: str
    rows: int
    sha256: str

    @field_validator("path", "sha256")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("output field must not be empty")
        return v

    @field_validator("rows")
    @classmethod
    def _reject_non_positive(cls, v: int) -> int:
        if v <= 0:
            raise ValueError("output rows must be positive")
        return v


class DatasetProvenance(BaseModel):
    schema_version: int
    benchmark: str
    source: DatasetSource
    selected_keys: list[int]
    transformation: DatasetTransformation
    output: DatasetOutput
    partition: str
    domain_strata: list[str]

    @field_validator("benchmark", "partition")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("provenance field must not be empty")
        return v

    @field_validator("selected_keys")
    @classmethod
    def _reject_empty_list(cls, v: list[int]) -> list[int]:
        if not v:
            raise ValueError("selected_keys must not be empty")
        return v

    @field_validator("domain_strata")
    @classmethod
    def _reject_empty_strata(cls, v: list[str]) -> list[str]:
        if not v:
            raise ValueError("domain_strata must not be empty")
        for item in v:
            if not item or not item.strip():
                raise ValueError("domain_strata entries must not be empty")
        return v


def load_provenance(path: Path) -> DatasetProvenance:
    return DatasetProvenance.model_validate_json(path.read_text())


def validate_provenance(provenance: DatasetProvenance) -> None:
    """Reject incomplete provenance before execution.

    Checks that every required field is present and non-empty.  Pydantic
    field validators handle empty-string and empty-list rejection at parse
    time; this function is the explicit pre-execution gate that loaders
    call after loading the manifest and before validating the dataset.
    """
    if provenance.schema_version < 1:
        raise ValueError(f"provenance schema_version must be positive: {provenance.schema_version}")
    if not provenance.benchmark.strip():
        raise ValueError("provenance benchmark must not be empty")
    if not provenance.partition.strip():
        raise ValueError("provenance partition must not be empty")
    if not provenance.domain_strata:
        raise ValueError("provenance domain_strata must not be empty")
    if not provenance.selected_keys:
        raise ValueError("provenance selected_keys must not be empty")
    src = provenance.source
    if not all([src.url, src.revision, src.license_spdx, src.license_url, src.sha256]):
        raise ValueError("provenance source is incomplete")
    tr = provenance.transformation
    if not all([tr.description, tr.code_path, tr.code_sha256, tr.fixture_path, tr.fixture_sha256]):
        raise ValueError("provenance transformation is incomplete")
    out = provenance.output
    if not all([out.path, out.sha256]) or out.rows <= 0:
        raise ValueError("provenance output is incomplete")


def validate_dataset(path: Path, provenance: DatasetProvenance) -> None:
    content = path.read_bytes()
    digest = hashlib.sha256(content).hexdigest()
    if provenance.benchmark != "ifeval_subset":
        raise ValueError(f"unexpected benchmark provenance: {provenance.benchmark}")
    if provenance.output.path != path.name:
        raise ValueError(f"IFEval output path mismatch: {provenance.output.path}")
    if digest != provenance.output.sha256:
        raise ValueError(f"IFEval subset SHA-256 mismatch: {digest}")
    if len(content.splitlines()) != provenance.output.rows:
        raise ValueError("IFEval subset row count mismatch")
