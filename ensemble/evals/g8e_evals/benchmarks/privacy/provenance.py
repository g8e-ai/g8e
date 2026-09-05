# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Provenance models and validation for synthetic eval suites.

Synthetic suites are generated from the g8e-evals codebase itself, not
imported from an external dataset.  The provenance manifest captures the
suite generation code hash, the output dataset hash, the row count, the
license, the data partition, and the domain strata under which the suite
is published.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

from pydantic import BaseModel, field_validator


class SyntheticSuiteSource(BaseModel):
    """The origin of a synthetic suite.

    Synthetic suites are generated from the g8e-evals codebase.  The
    source records the repository URL, the revision that produced the
    suite, the license, and the SHA-256 of the generation code.
    """

    repository: str
    revision: str
    license_spdx: str
    code_path: str
    code_sha256: str

    @field_validator("repository", "revision", "license_spdx", "code_path", "code_sha256")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("source field must not be empty")
        return v


class SyntheticSuiteOutput(BaseModel):
    """The output dataset of a synthetic suite."""

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


class SyntheticSuiteProvenance(BaseModel):
    """Immutable provenance manifest for a synthetic eval suite.

    The manifest binds the suite to the code that generated it and the
    output dataset it produced.  The ``partition`` field identifies the
    data split (development, validation, holdout) and the ``domain_strata``
    field lists the domain categories the dataset covers.  Validation
    rejects incomplete or mismatched provenance before execution.
    """

    schema_version: int
    benchmark: str
    source: SyntheticSuiteSource
    output: SyntheticSuiteOutput
    partition: str
    domain_strata: list[str]

    @field_validator("benchmark", "partition")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("provenance field must not be empty")
        return v

    @field_validator("domain_strata")
    @classmethod
    def _reject_empty_list(cls, v: list[str]) -> list[str]:
        if not v:
            raise ValueError("domain_strata must not be empty")
        for item in v:
            if not item or not item.strip():
                raise ValueError("domain_strata entries must not be empty")
        return v


def load_provenance(path: Path) -> SyntheticSuiteProvenance:
    return SyntheticSuiteProvenance.model_validate_json(path.read_text())


def validate_provenance(provenance: SyntheticSuiteProvenance) -> None:
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
    src = provenance.source
    if not all([src.repository, src.revision, src.license_spdx, src.code_path, src.code_sha256]):
        raise ValueError("provenance source is incomplete")
    out = provenance.output
    if not all([out.path, out.sha256]) or out.rows <= 0:
        raise ValueError("provenance output is incomplete")


def validate_dataset(path: Path, provenance: SyntheticSuiteProvenance) -> None:
    """Validate that the dataset file matches its provenance manifest.

    Checks the output path name, the SHA-256 content hash, and the row
    count.  Raises ``ValueError`` on any mismatch.
    """
    content = path.read_bytes()
    digest = hashlib.sha256(content).hexdigest()
    if provenance.output.path != path.name:
        raise ValueError(f"dataset output path mismatch: {provenance.output.path} != {path.name}")
    if digest != provenance.output.sha256:
        raise ValueError(f"dataset SHA-256 mismatch: {digest} != {provenance.output.sha256}")
    if len(content.splitlines()) != provenance.output.rows:
        raise ValueError(
            f"dataset row count mismatch: {len(content.splitlines())} != {provenance.output.rows}"
        )
