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

Validation enforces executable-code provenance: the declared ``code_path``
is resolved under a trusted eval root with traversal protection, and its
SHA-256 digest is verified against ``code_sha256`` before any dataset
rows are read.  The provenance benchmark must match the loader's
``SUITE_ID`` so suite substitution is rejected.  All provenance models
use ``extra="forbid"`` and strict SHA-256 field patterns so unknown
fields and malformed digests are rejected at parse time.
"""

from __future__ import annotations

import hashlib
import re
from pathlib import Path

from pydantic import BaseModel, ConfigDict, field_validator

_SHA256_PATTERN = re.compile(r"^[0-9a-f]{64}$")


def _validate_sha256(v: str) -> str:
    if not v or not v.strip():
        raise ValueError("sha256 field must not be empty")
    if not _SHA256_PATTERN.match(v):
        raise ValueError(f"sha256 field must be a 64-character lowercase hex digest: {v}")
    return v


def _resolve_under_root(code_path: str, trusted_root: Path) -> Path:
    """Resolve ``code_path`` under ``trusted_root`` with traversal protection.

    Rejects absolute paths, path separators that escape the root, and
    any resolved path that does not stay within ``trusted_root``.  This
    prevents path traversal attacks via crafted ``code_path`` values.
    """
    if not code_path or not code_path.strip():
        raise ValueError("code_path must not be empty")
    if code_path.startswith("/"):
        raise ValueError(f"code_path must not be absolute: {code_path}")
    if "\x00" in code_path:
        raise ValueError("code_path must not contain null bytes")
    if ".." in code_path.split("/"):
        raise ValueError(f"code_path must not contain traversal sequences: {code_path}")
    resolved = (trusted_root / code_path).resolve()
    root_resolved = trusted_root.resolve()
    try:
        resolved.relative_to(root_resolved)
    except ValueError:
        raise ValueError(f"code_path resolves outside trusted root: {code_path}") from None
    return resolved


class SyntheticSuiteSource(BaseModel):
    """The origin of a synthetic suite.

    Synthetic suites are generated from the g8e-evals codebase.  The
    source records the repository URL, the revision that produced the
    suite, the license, and the SHA-256 of the generation code.
    """

    model_config = ConfigDict(extra="forbid")

    repository: str
    revision: str
    license_spdx: str
    code_path: str
    code_sha256: str

    @field_validator("repository", "revision", "license_spdx", "code_path")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("source field must not be empty")
        return v

    @field_validator("code_sha256")
    @classmethod
    def _validate_code_sha256(cls, v: str) -> str:
        return _validate_sha256(v)


class SyntheticSuiteOutput(BaseModel):
    """The output dataset of a synthetic suite."""

    model_config = ConfigDict(extra="forbid")

    path: str
    rows: int
    sha256: str

    @field_validator("path")
    @classmethod
    def _reject_empty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("output field must not be empty")
        return v

    @field_validator("sha256")
    @classmethod
    def _validate_output_sha256(cls, v: str) -> str:
        return _validate_sha256(v)

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

    model_config = ConfigDict(extra="forbid")

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


def validate_provenance(
    provenance: SyntheticSuiteProvenance,
    *,
    suite_id: str,
    trusted_root: Path,
) -> None:
    """Reject incomplete or mismatched provenance before execution.

    Checks that every required field is present and non-empty, that the
    provenance benchmark matches the loader's ``SUITE_ID``, and that the
    declared generation code path resolves under ``trusted_root`` with a
    matching SHA-256 digest.  Pydantic field validators handle
    empty-string, empty-list, SHA-256 pattern, and unknown-field
    rejection at parse time; this function is the explicit pre-execution
    gate that loaders call after loading the manifest and before
    validating the dataset.
    """
    if provenance.schema_version < 1:
        raise ValueError(f"provenance schema_version must be positive: {provenance.schema_version}")
    if not provenance.benchmark.strip():
        raise ValueError("provenance benchmark must not be empty")
    if provenance.benchmark != suite_id:
        raise ValueError(
            f"provenance benchmark mismatch: {provenance.benchmark} != {suite_id}"
        )
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
    _verify_code_digest(src.code_path, src.code_sha256, trusted_root)


def _verify_code_digest(code_path: str, expected_sha256: str, trusted_root: Path) -> None:
    """Resolve ``code_path`` under ``trusted_root`` and verify its SHA-256.

    Raises ``ValueError`` if the path escapes the trusted root, the file
    does not exist, or the computed digest does not match the expected
    value.  This is the executable-code provenance gate: no dataset rows
    are read until the generation code hash is verified.
    """
    resolved = _resolve_under_root(code_path, trusted_root)
    if not resolved.is_file():
        raise ValueError(f"provenance code_path does not exist: {code_path}")
    actual = hashlib.sha256(resolved.read_bytes()).hexdigest()
    if actual != expected_sha256:
        raise ValueError(
            f"provenance code_sha256 mismatch for {code_path}: "
            f"{actual} != {expected_sha256}"
        )


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
