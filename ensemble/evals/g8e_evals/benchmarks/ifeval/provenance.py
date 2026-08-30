# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
from pathlib import Path

from pydantic import BaseModel


class DatasetSource(BaseModel):
    url: str
    revision: str
    license_spdx: str
    license_url: str
    sha256: str


class DatasetTransformation(BaseModel):
    description: str
    code_path: str
    code_sha256: str
    fixture_path: str
    fixture_sha256: str


class DatasetOutput(BaseModel):
    path: str
    rows: int
    sha256: str


class DatasetProvenance(BaseModel):
    schema_version: int
    benchmark: str
    source: DatasetSource
    selected_keys: list[int]
    transformation: DatasetTransformation
    output: DatasetOutput


def load_provenance(path: Path) -> DatasetProvenance:
    return DatasetProvenance.model_validate_json(path.read_text())


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
