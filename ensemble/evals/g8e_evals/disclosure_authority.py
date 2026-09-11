# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Frozen typed disclosure authority for D12 public data formats.

The ``DisclosureAuthority`` is the frozen, content-addressed policy that
governs which fields are published, which are restricted, and which
become tombstones. It defines the canonical public JSONL output, the
derived CSV output, the prohibited SQLite format, the proof index, and
the deterministic output inventory.

D12 decision: Publish disclosure-approved canonical JSONL, derived CSV,
and hashes or tombstones for restricted records. Do not publish SQLite
in the first release.

Field classification:

- ``PUBLIC``: The field value is published in canonical JSONL and
  derived CSV without redaction.
- ``RESTRICTED``: The field value is never published. A tombstone
  (SHA-256 hash and length) is emitted in its place so a reader can
  verify the restricted artifact exists without seeing its content.
- ``TOMBSTONE``: The field is always published as a tombstone by
  design (e.g. evidence hashes, artifact digests).

Output formats:

- ``CANONICAL_JSONL``: One JSON object per line, sorted keys, canonical
  serialization. The primary public data surface.
- ``DERIVED_CSV``: A flat CSV projection of public fields for
  spreadsheet consumption. Derived from the canonical JSONL.
- ``PROHIBITED_SQLITE``: SQLite is explicitly prohibited in the first
  release. A candidate containing a SQLite file fails disclosure
  validation.

The authority is frozen with ``extra="forbid"`` and content-addressed
via SHA-256 over canonical JSON (sorted keys, no extra whitespace). A
changed authority creates a new identity and invalidates dependent
publication.
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


DISCLOSURE_AUTHORITY_SCHEMA_VERSION = "1.0.0"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class FieldClassification(StrEnum):
    """How a field is treated in the public disclosure surface.

    ``PUBLIC``: The field value is published without redaction.
    ``RESTRICTED``: The field value is never published; a tombstone
    (hash and length) replaces it in the public output.
    ``TOMBSTONE``: The field is always published as a tombstone by
    design (e.g. evidence hashes, artifact digests).
    """

    PUBLIC = "public"
    RESTRICTED = "restricted"
    TOMBSTONE = "tombstone"


class OutputFormat(StrEnum):
    """The output format for a disclosure artifact.

    ``CANONICAL_JSONL``: One JSON object per line with sorted keys.
    ``DERIVED_CSV``: A flat CSV projection of public fields.
    ``PROHIBITED_SQLITE``: SQLite is explicitly prohibited in the first
    release. A candidate containing a SQLite file fails disclosure.
    """

    CANONICAL_JSONL = "canonical_jsonl"
    DERIVED_CSV = "derived_csv"
    PROHIBITED_SQLITE = "prohibited_sqlite"


class DisclosureFieldEntry(BaseModel):
    """One field in the disclosure authority with its classification.

    Binds a field name (as it appears in the canonical record) to its
    classification, the canonical JSONL column name, and whether it
    appears in the derived CSV. Restricted fields are replaced by a
    tombstone in the public output.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    field_name: str = Field(min_length=1, description="Field name in the canonical record.")
    classification: FieldClassification = Field(description="How the field is treated in disclosure.")
    public_column_name: str = Field(
        min_length=1,
        description="Column name in the canonical JSONL and derived CSV output.",
    )
    in_csv: bool = Field(
        default=True,
        description="Whether the field appears in the derived CSV output.",
    )
    tombstone_hash_field: str = Field(
        default="",
        description="When classification is RESTRICTED or TOMBSTONE, the name of the hash field in the tombstone. Empty for PUBLIC.",
    )

    @model_validator(mode="after")
    def _validate_consistency(self) -> Self:
        if self.classification in (FieldClassification.RESTRICTED, FieldClassification.TOMBSTONE):
            if not self.tombstone_hash_field:
                raise ValueError(
                    f"tombstone_hash_field must be non-empty when classification is "
                    f"{self.classification.value!r} for {self.field_name!r}"
                )
        elif self.tombstone_hash_field:
            raise ValueError(
                f"tombstone_hash_field must be empty when classification is "
                f"{self.classification.value!r} for {self.field_name!r}"
            )
        return self


class DisclosureOutputEntry(BaseModel):
    """One output artifact in the disclosure output inventory.

    Binds an output file name to its format, whether it is required,
    and a description. The output inventory is deterministic: the same
    authority always produces the same set of output files.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    file_name: str = Field(min_length=1, description="Output file name.")
    output_format: OutputFormat = Field(description="Format of the output file.")
    required: bool = Field(description="Whether the file must be present in a disclosure-compliant candidate.")
    description: str = Field(min_length=1, description="What the output file contains.")

    @model_validator(mode="after")
    def _validate_prohibited_sqlite(self) -> Self:
        if self.output_format == OutputFormat.PROHIBITED_SQLITE and self.required:
            raise ValueError(
                f"PROHIBITED_SQLITE output {self.file_name!r} must not be required"
            )
        return self


class DisclosureAuthority(BaseModel):
    """Frozen typed disclosure authority for D12 public data formats.

    Defines field classification (public, restricted, tombstone),
    output formats (canonical JSONL, derived CSV, prohibited SQLite),
    proof indexing, and the deterministic output inventory. The
    authority is frozen and content-addressed so any mutation creates a
    new identity and invalidates dependent publication.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(
        default=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        min_length=1,
        description="Schema version of the disclosure authority.",
    )
    authority_id: str = Field(min_length=1, description="Unique authority identifier.")
    authority_version: str = Field(min_length=1, description="Authority version.")
    fields: list[DisclosureFieldEntry] = Field(
        min_length=1,
        description="Field classification entries, one per field name.",
    )
    outputs: list[DisclosureOutputEntry] = Field(
        min_length=1,
        description="Output inventory entries, one per output file.",
    )
    proof_index_description: str = Field(
        min_length=1,
        description="Description of the proof index: what it contains and how it is built.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of this authority.",
    )

    @model_validator(mode="after")
    def _validate_authority(self) -> Self:
        seen_fields: set[str] = set()
        for entry in self.fields:
            if entry.field_name in seen_fields:
                raise ValueError(f"duplicate field_name in authority: {entry.field_name!r}")
            seen_fields.add(entry.field_name)
        seen_columns: set[str] = set()
        for entry in self.fields:
            if entry.public_column_name in seen_columns:
                raise ValueError(
                    f"duplicate public_column_name in authority: {entry.public_column_name!r}"
                )
            seen_columns.add(entry.public_column_name)
        seen_outputs: set[str] = set()
        for entry in self.outputs:
            if entry.file_name in seen_outputs:
                raise ValueError(f"duplicate output file_name in authority: {entry.file_name!r}")
            seen_outputs.add(entry.file_name)
        expected = compute_disclosure_authority_hash(
            schema_version=self.schema_version,
            authority_id=self.authority_id,
            authority_version=self.authority_version,
            fields=[
                {
                    "field_name": e.field_name,
                    "classification": e.classification.value,
                    "public_column_name": e.public_column_name,
                    "in_csv": e.in_csv,
                    "tombstone_hash_field": e.tombstone_hash_field,
                }
                for e in self.fields
            ],
            outputs=[
                {
                    "file_name": e.file_name,
                    "output_format": e.output_format.value,
                    "required": e.required,
                    "description": e.description,
                }
                for e in self.outputs
            ],
            proof_index_description=self.proof_index_description,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"disclosure authority content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self

    def public_fields(self) -> list[DisclosureFieldEntry]:
        """Return sorted list of field entries with PUBLIC classification."""
        return sorted(
            (e for e in self.fields if e.classification == FieldClassification.PUBLIC),
            key=lambda e: e.field_name,
        )

    def restricted_fields(self) -> list[DisclosureFieldEntry]:
        """Return sorted list of field entries with RESTRICTED classification."""
        return sorted(
            (e for e in self.fields if e.classification == FieldClassification.RESTRICTED),
            key=lambda e: e.field_name,
        )

    def tombstone_fields(self) -> list[DisclosureFieldEntry]:
        """Return sorted list of field entries with TOMBSTONE classification."""
        return sorted(
            (e for e in self.fields if e.classification == FieldClassification.TOMBSTONE),
            key=lambda e: e.field_name,
        )

    def required_outputs(self) -> list[DisclosureOutputEntry]:
        """Return sorted list of output entries that are required."""
        return sorted(
            (e for e in self.outputs if e.required),
            key=lambda e: e.file_name,
        )

    def prohibited_outputs(self) -> list[DisclosureOutputEntry]:
        """Return sorted list of output entries with PROHIBITED_SQLITE format."""
        return sorted(
            (e for e in self.outputs if e.output_format == OutputFormat.PROHIBITED_SQLITE),
            key=lambda e: e.file_name,
        )

    def get_field(self, field_name: str) -> DisclosureFieldEntry | None:
        """Return the field entry for a field name, or None if not in the authority."""
        for entry in self.fields:
            if entry.field_name == field_name:
                return entry
        return None

    def get_output(self, file_name: str) -> DisclosureOutputEntry | None:
        """Return the output entry for a file name, or None if not in the authority."""
        for entry in self.outputs:
            if entry.file_name == file_name:
                return entry
        return None


def compute_disclosure_authority_hash(
    *,
    schema_version: str,
    authority_id: str,
    authority_version: str,
    fields: list[dict],
    outputs: list[dict],
    proof_index_description: str,
) -> str:
    """Compute the content hash for a disclosure authority without constructing the full model."""
    payload = json.dumps(
        {
            "schema_version": schema_version,
            "authority_id": authority_id,
            "authority_version": authority_version,
            "fields": sorted(fields, key=lambda f: f["field_name"]),
            "outputs": sorted(outputs, key=lambda o: o["file_name"]),
            "proof_index_description": proof_index_description,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "DISCLOSURE_AUTHORITY_SCHEMA_VERSION",
    "DisclosureAuthority",
    "DisclosureFieldEntry",
    "DisclosureOutputEntry",
    "FieldClassification",
    "OutputFormat",
    "compute_disclosure_authority_hash",
]
