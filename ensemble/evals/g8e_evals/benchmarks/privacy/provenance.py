# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Provenance models and validation for synthetic privacy eval suites.

Synthetic suites are generated from the g8e-evals codebase itself, not
imported from an external dataset.  The provenance manifest captures the
suite generation code hash, the output dataset hash, the row count, and
the license under which the suite is published.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

from pydantic import BaseModel


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


class SyntheticSuiteOutput(BaseModel):
    """The output dataset of a synthetic suite."""

    path: str
    rows: int
    sha256: str


class SyntheticSuiteProvenance(BaseModel):
    """Immutable provenance manifest for a synthetic eval suite.

    The manifest binds the suite to the code that generated it and the
    output dataset it produced.  Validation rejects incomplete or
    mismatched provenance before execution.
    """

    schema_version: int
    benchmark: str
    source: SyntheticSuiteSource
    output: SyntheticSuiteOutput


def load_provenance(path: Path) -> SyntheticSuiteProvenance:
    return SyntheticSuiteProvenance.model_validate_json(path.read_text())


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
