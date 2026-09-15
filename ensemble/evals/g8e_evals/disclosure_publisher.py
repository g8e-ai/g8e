# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""D12-mandated public disclosure output builder for v5 publication.

The disclosure publisher consumes a frozen ``DisclosureAuthority`` and a
v5 candidate directory, classifies every field per the authority, and
emits the D12-mandated public data surface:

- ``disclosure-public.jsonl``: canonical JSONL with PUBLIC field values
  and tombstones (SHA-256 hash + byte length) for RESTRICTED and TOMBSTONE
  fields. One JSON object per line, sorted keys.
- ``disclosure-derived.csv``: a flat CSV projection of PUBLIC fields only.
- ``disclosure-tombstones.jsonl``: tombstone records (hash and length) for
  every RESTRICTED and TOMBSTONE field, emitted when the authority
  classifies any field as RESTRICTED.
- ``disclosure-proof-index.json``: proof index mapping public records to
  their source evidence artifacts in the candidate directory.
- ``disclosure-output-inventory.json``: deterministic output inventory
  listing all produced disclosure files with their SHA-256 hashes.

The publisher reads the v5 candidate's ``campaign-projections.jsonl`` as
the canonical input record set. Each projection row is classified field by
field against the authority. A field not present in the authority is
rejected so a publication cannot silently introduce an unclassified field.

The publisher is deterministic: the same authority and candidate always
produce the same disclosure outputs. The output inventory is content
addressed so a changed output set creates a new inventory identity.

D12 decision: Publish disclosure-approved canonical JSONL, derived CSV,
and hashes or tombstones for restricted records. Do not publish SQLite
in the first release.
"""

from __future__ import annotations

import csv
import hashlib
import io
from collections.abc import Sequence
from pathlib import Path
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, TypeAdapter, model_validator

from g8e_evals.constants import (
    CAMPAIGN_PROJECTIONS_JSONL,
)
from g8e_evals.disclosure_authority import (
    DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
    DisclosureAuthority,
    DisclosureFieldEntry,
    DisclosureOutputEntry,
    FieldClassification,
    OutputFormat,
    OutputRole,
    compute_disclosure_authority_hash,
)
from g8e_evals.publication import CampaignProjectionRow
from g8e_evals.serialization import (
    canonical_json,
    canonical_model_list,
    read_jsonl_models,
)


# Default disclosure output file names. These match the D12-mandated roles.
DISCLOSURE_PUBLIC_JSONL = "disclosure-public.jsonl"
DISCLOSURE_DERIVED_CSV = "disclosure-derived.csv"
DISCLOSURE_TOMBSTONES_JSONL = "disclosure-tombstones.jsonl"
DISCLOSURE_PROOF_INDEX_JSON = "disclosure-proof-index.json"
DISCLOSURE_OUTPUT_INVENTORY_JSON = "disclosure-output-inventory.json"
DISCLOSURE_PROHIBITED_SQLITE = "evidence.sqlite"

DISCLOSURE_OUTPUT_SCHEMA_VERSION = "1.0.0"


def _sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _sha256_str(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class DisclosurePublishError(ValueError):
    """Raised when disclosure outputs cannot be built or validated."""


class TombstoneRecord(BaseModel):
    """Tombstone for one restricted or tombstone-classified field value.

    A tombstone proves the restricted artifact exists without revealing
    its content. The ``sha256`` is the SHA-256 of the original field value's
    canonical JSON encoding; ``byte_length`` is the length of that encoding
    in bytes. A reader can verify the restricted artifact exists by
    recomputing the hash from the original value without seeing it.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    record_index: int = Field(ge=0, description="Index of the source record in the input JSONL.")
    field_name: str = Field(min_length=1, description="Field name in the canonical record.")
    public_column_name: str = Field(min_length=1, description="Column name in the public output.")
    sha256: str = Field(min_length=64, max_length=64, description="SHA-256 of the original value's canonical JSON encoding.")
    byte_length: int = Field(ge=0, description="Byte length of the original value's canonical JSON encoding.")


class ProofIndexEntry(BaseModel):
    """One entry in the proof index mapping a public record to its source.

    The ``record_index`` is the 0-based index of the record in the input
    JSONL. The ``source_file`` is the candidate-relative path of the source
    artifact. The ``source_sha256`` is the SHA-256 of the source file so a
    reader can verify the proof index binds to the exact source artifact.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    record_index: int = Field(ge=0, description="Index of the source record in the input JSONL.")
    source_file: str = Field(min_length=1, description="Candidate-relative path of the source artifact.")
    source_sha256: str = Field(min_length=64, max_length=64, description="SHA-256 of the source file.")


class OutputInventoryEntry(BaseModel):
    """One entry in the deterministic output inventory."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    file_name: str = Field(min_length=1, description="Output file name.")
    sha256: str = Field(min_length=64, max_length=64, description="SHA-256 of the output file content.")
    byte_length: int = Field(ge=0, description="Byte length of the output file.")


class OutputInventory(BaseModel):
    """Deterministic output inventory listing all produced disclosure files.

    The inventory is content addressed via SHA-256 over canonical JSON so a
    changed output set creates a new inventory identity. The
    ``disclosure_authority_hash`` binds the inventory to the exact
    authority that produced it.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(min_length=1, description="Output inventory schema version.")
    disclosure_authority_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the disclosure authority.")
    outputs: list[OutputInventoryEntry] = Field(min_length=1, description="Sorted output inventory entries.")
    content_hash: str = Field(min_length=64, max_length=64, description="SHA-256 over canonical JSON of this inventory.")

    @model_validator(mode="after")
    def _validate_inventory(self) -> Self:
        seen: set[str] = set()
        for entry in self.outputs:
            if entry.file_name in seen:
                raise ValueError(f"duplicate output file_name in inventory: {entry.file_name!r}")
            seen.add(entry.file_name)
        expected = compute_output_inventory_hash(
            schema_version=self.schema_version,
            disclosure_authority_hash=self.disclosure_authority_hash,
            outputs=self.outputs,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"output inventory content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self


class ProofIndex(BaseModel):
    """Proof index mapping public records to their source evidence artifacts.

    The ``disclosure_authority_hash`` binds the proof index to the exact
    authority that produced it. The ``entries`` list maps each public
    record (by 0-based index) to its source artifact path and SHA-256.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(min_length=1, description="Proof index schema version.")
    disclosure_authority_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the disclosure authority.")
    proof_index_description: str = Field(min_length=1, description="Description of the proof index.")
    entries: list[ProofIndexEntry] = Field(min_length=1, description="Proof index entries, one per public record.")


def compute_output_inventory_hash(
    *,
    schema_version: str,
    disclosure_authority_hash: str,
    outputs: Sequence[OutputInventoryEntry],
) -> str:
    """Compute the content hash for an output inventory without constructing the full model."""
    payload = canonical_json(
        {
            "schema_version": schema_version,
            "disclosure_authority_hash": disclosure_authority_hash,
            "outputs": canonical_model_list(sorted(outputs, key=lambda o: o.file_name)),
        }
    )
    return _sha256_str(payload)


def _classify_field(
    authority: DisclosureAuthority,
    field_name: str,
) -> DisclosureFieldEntry:
    entry = authority.get_field(field_name)
    if entry is None:
        raise DisclosurePublishError(
            f"field {field_name!r} not present in disclosure authority"
        )
    return entry


def _build_tombstone(value: object) -> tuple[str, int]:
    """Return (sha256, byte_length) for a field value's canonical JSON encoding."""
    encoded = canonical_json(value).encode()
    return _sha256_bytes(encoded), len(encoded)


def build_disclosure_outputs(
    *,
    candidate_dir: Path,
    authority: DisclosureAuthority,
    output_dir: Path,
) -> OutputInventory:
    """Build D12-mandated disclosure outputs from a v5 candidate directory.

    Reads ``campaign-projections.jsonl`` from the candidate directory,
    classifies every field per the frozen ``DisclosureAuthority``, and
    writes the canonical JSONL, derived CSV, tombstones, proof index, and
    output inventory to ``output_dir``. Returns the typed
    ``OutputInventory``.

    Raises ``DisclosurePublishError`` when a field in the input records is
    not present in the authority, when the candidate directory is missing
    the input JSONL, or when a SQLite file is present in the candidate
    (D12 prohibition).
    """
    if not candidate_dir.is_dir():
        raise DisclosurePublishError(f"candidate directory does not exist: {candidate_dir}")
    if output_dir.exists():
        raise DisclosurePublishError(f"output directory already exists: {output_dir}")

    _reject_sqlite_in_candidate(candidate_dir)

    projections_path = candidate_dir / CAMPAIGN_PROJECTIONS_JSONL
    if not projections_path.is_file():
        raise DisclosurePublishError(
            f"missing {CAMPAIGN_PROJECTIONS_JSONL} in candidate directory"
        )
    source_sha256 = _sha256_bytes(projections_path.read_bytes())
    records = read_jsonl_models(projections_path, CampaignProjectionRow)

    public_fields = authority.public_fields()
    restricted_fields = authority.restricted_fields()
    tombstone_fields = authority.tombstone_fields()

    output_dir.mkdir(parents=True)

    # open map: column keys are the authority's public_column_name values,
    # determined at runtime by the authority's field classifications.
    public_rows: list[dict[str, str | int | float | dict[str, str | int]]] = []
    tombstone_records: list[TombstoneRecord] = []
    proof_entries: list[ProofIndexEntry] = []

    for index, record in enumerate(records):
        record_dict = record.model_dump(mode="json")
        public_row: dict[str, str | int | float | dict[str, str | int]] = {}
        for field_name, value in sorted(record_dict.items()):
            entry = _classify_field(authority, field_name)
            if entry.classification == FieldClassification.PUBLIC:
                public_row[entry.public_column_name] = value
            else:
                sha, length = _build_tombstone(value)
                tombstone_records.append(TombstoneRecord(
                    record_index=index,
                    field_name=field_name,
                    public_column_name=entry.public_column_name,
                    sha256=sha,
                    byte_length=length,
                ))
                if entry.classification == FieldClassification.TOMBSTONE:
                    public_row[entry.public_column_name] = {"sha256": sha, "byte_length": length}
        public_rows.append(public_row)
        proof_entries.append(ProofIndexEntry(
            record_index=index,
            source_file=CAMPAIGN_PROJECTIONS_JSONL,
            source_sha256=source_sha256,
        ))

    _write_public_jsonl(output_dir / DISCLOSURE_PUBLIC_JSONL, public_rows)
    _write_derived_csv(output_dir / DISCLOSURE_DERIVED_CSV, public_rows, public_fields)
    if restricted_fields or tombstone_fields:
        _write_tombstones_jsonl(output_dir / DISCLOSURE_TOMBSTONES_JSONL, tombstone_records)
    _write_proof_index(output_dir / DISCLOSURE_PROOF_INDEX_JSON, proof_entries, authority)

    inventory = _build_output_inventory(output_dir, authority)
    _write_output_inventory(output_dir / DISCLOSURE_OUTPUT_INVENTORY_JSON, inventory)
    return inventory


def _reject_sqlite_in_candidate(candidate_dir: Path) -> None:
    sqlite_suffixes = (".sqlite", ".db", ".sqlite3")
    for path in candidate_dir.rglob("*"):
        if path.is_file() and path.name.endswith(sqlite_suffixes):
            rel = path.relative_to(candidate_dir).as_posix()
            raise DisclosurePublishError(
                f"D12 prohibition: SQLite file in candidate: {rel}"
            )


def _write_public_jsonl(
    path: Path,
    rows: list[dict[str, str | int | float | dict[str, str | int]]],
) -> None:
    lines = [canonical_json(row) for row in rows]
    path.write_text("".join(line + "\n" for line in lines))


def _write_derived_csv(
    path: Path,
    rows: list[dict[str, str | int | float | dict[str, str | int]]],
    public_fields: list[DisclosureFieldEntry],
) -> None:
    csv_fields = [e for e in public_fields if e.in_csv]
    header = [e.public_column_name for e in csv_fields]
    buffer = io.StringIO()
    writer = csv.writer(buffer, lineterminator="\n")
    writer.writerow(header)
    for row in rows:
        writer.writerow([row.get(e.public_column_name, "") for e in csv_fields])
    path.write_text(buffer.getvalue())


def _write_tombstones_jsonl(path: Path, records: list[TombstoneRecord]) -> None:
    path.write_text("".join(r.model_dump_json() + "\n" for r in records))


def _write_proof_index(path: Path, entries: list[ProofIndexEntry], authority: DisclosureAuthority) -> None:
    proof_index = ProofIndex(
        schema_version=DISCLOSURE_OUTPUT_SCHEMA_VERSION,
        disclosure_authority_hash=authority.content_hash,
        proof_index_description=authority.proof_index_description,
        entries=entries,
    )
    path.write_text(canonical_json(proof_index.model_dump(mode="json", by_alias=True)) + "\n")


def _build_output_inventory(output_dir: Path, authority: DisclosureAuthority) -> OutputInventory:
    entries: list[OutputInventoryEntry] = []
    for path in sorted(output_dir.rglob("*")):
        if path.is_file():
            content = path.read_bytes()
            entries.append(OutputInventoryEntry(
                file_name=path.relative_to(output_dir).as_posix(),
                sha256=_sha256_bytes(content),
                byte_length=len(content),
            ))
    content_hash = compute_output_inventory_hash(
        schema_version=DISCLOSURE_OUTPUT_SCHEMA_VERSION,
        disclosure_authority_hash=authority.content_hash,
        outputs=entries,
    )
    return OutputInventory(
        schema_version=DISCLOSURE_OUTPUT_SCHEMA_VERSION,
        disclosure_authority_hash=authority.content_hash,
        outputs=entries,
        content_hash=content_hash,
    )


def _write_output_inventory(path: Path, inventory: OutputInventory) -> None:
    path.write_text(inventory.model_dump_json() + "\n")


class DisclosureValidationResult(BaseModel):
    """Typed result of disclosure output validation.

    ``ok`` is True only when every check passes. ``failures`` is a sorted
    list of typed failure messages.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool = Field(description="True when all disclosure validation checks pass.")
    checked_layers: list[str] = Field(default_factory=list, description="Sorted validation layer names.")
    failures: list[str] = Field(default_factory=list, description="Sorted failure messages.")


def validate_disclosure_outputs(
    *,
    output_dir: Path,
    authority: DisclosureAuthority,
) -> DisclosureValidationResult:
    """Validate disclosure outputs against the frozen authority.

    Checks that every required output file exists, that the output
    inventory recomputes, that the proof index binds to the authority,
    that no SQLite file is present, and that the canonical JSONL contains
    only authority-classified fields. Returns a typed
    ``DisclosureValidationResult``.
    """
    failures: list[str] = []
    checked_layers: list[str] = []

    checked_layers.append("required_outputs")
    required_outputs = authority.required_outputs()
    for entry in required_outputs:
        path = output_dir / entry.file_name
        if not path.exists():
            failures.append(f"missing required disclosure output: {entry.file_name}")
        elif path.is_symlink():
            failures.append(f"symlink rejected: {entry.file_name}")
        elif not path.is_file():
            failures.append(f"not a regular file: {entry.file_name}")

    checked_layers.append("prohibited_sqlite")
    sqlite_outputs = authority.prohibited_outputs()
    for entry in sqlite_outputs:
        path = output_dir / entry.file_name
        if path.exists():
            failures.append(f"D12 prohibition: SQLite file present: {entry.file_name}")
    sqlite_suffixes = (".sqlite", ".db", ".sqlite3")
    for path in output_dir.rglob("*"):
        if path.is_file() and path.name.endswith(sqlite_suffixes):
            rel = path.relative_to(output_dir).as_posix()
            failures.append(f"D12 prohibition: SQLite file in output: {rel}")

    checked_layers.append("output_inventory")
    inv_path = output_dir / DISCLOSURE_OUTPUT_INVENTORY_JSON
    if inv_path.is_file() and not inv_path.is_symlink():
        try:
            inventory = OutputInventory.model_validate_json(inv_path.read_text())
            if inventory.disclosure_authority_hash != authority.content_hash:
                failures.append("output inventory disclosure_authority_hash mismatch")
            for entry in inventory.outputs:
                path = output_dir / entry.file_name
                if not path.is_file():
                    failures.append(f"output inventory references missing file: {entry.file_name}")
                else:
                    actual_sha = _sha256_bytes(path.read_bytes())
                    if actual_sha != entry.sha256:
                        failures.append(f"output inventory sha256 drift: {entry.file_name}")
        except Exception as e:
            failures.append(f"output inventory validation failed: {e}")

    checked_layers.append("proof_index")
    proof_path = output_dir / DISCLOSURE_PROOF_INDEX_JSON
    if proof_path.is_file() and not proof_path.is_symlink():
        try:
            proof = ProofIndex.model_validate_json(proof_path.read_text())
            if proof.disclosure_authority_hash != authority.content_hash:
                failures.append("proof index disclosure_authority_hash mismatch")
        except Exception as e:
            failures.append(f"proof index validation failed: {e}")

    checked_layers.append("public_jsonl_fields")
    public_path = output_dir / DISCLOSURE_PUBLIC_JSONL
    if public_path.is_file() and not public_path.is_symlink():
        try:
            authority_columns = {e.public_column_name for e in authority.fields}
            # open map: column keys are the authority's public_column_name values,
            # determined at runtime by the authority's field classifications.
            row_adapter: TypeAdapter[dict[str, object]] = TypeAdapter(dict[str, object])
            for line in public_path.read_text().splitlines():
                line = line.strip()
                if not line:
                    continue
                row = row_adapter.validate_json(line)
                for key in row:
                    if key not in authority_columns:
                        failures.append(f"unclassified field in public JSONL: {key!r}")
        except Exception as e:
            failures.append(f"public JSONL field validation failed: {e}")

    checked_layers.append("tombstones")
    has_restricted = any(
        e.classification == FieldClassification.RESTRICTED for e in authority.fields
    )
    if has_restricted:
        tomb_path = output_dir / DISCLOSURE_TOMBSTONES_JSONL
        if not tomb_path.exists():
            failures.append(f"missing required tombstones file: {DISCLOSURE_TOMBSTONES_JSONL}")

    ok = len(failures) == 0
    return DisclosureValidationResult(
        ok=ok,
        checked_layers=sorted(set(checked_layers)),
        failures=sorted(failures),
    )


def _default_public_field(field_name: str) -> DisclosureFieldEntry:
    """Construct a PUBLIC field entry for a projection row field name."""
    return DisclosureFieldEntry(
        field_name=field_name,
        classification=FieldClassification.PUBLIC,
        public_column_name=field_name,
        in_csv=True,
        tombstone_hash_field="",
    )


def _default_output_entries(*, include_tombstones: bool) -> list[DisclosureOutputEntry]:
    """Construct the D12-mandated output inventory entries."""
    outputs = [
        DisclosureOutputEntry(
            file_name=DISCLOSURE_PUBLIC_JSONL,
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.PUBLIC_JSONL,
            required=True,
            description="Canonical public JSONL with PUBLIC field values and tombstones for restricted fields.",
        ),
        DisclosureOutputEntry(
            file_name=DISCLOSURE_DERIVED_CSV,
            output_format=OutputFormat.DERIVED_CSV,
            output_role=OutputRole.DERIVED_CSV,
            required=True,
            description="Flat CSV projection of public fields for spreadsheet consumption.",
        ),
    ]
    if include_tombstones:
        outputs.append(DisclosureOutputEntry(
            file_name=DISCLOSURE_TOMBSTONES_JSONL,
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.TOMBSTONES,
            required=True,
            description="Tombstone records (SHA-256 hash and byte length) for restricted fields.",
        ))
    outputs.extend([
        DisclosureOutputEntry(
            file_name=DISCLOSURE_PROOF_INDEX_JSON,
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.PROOF_INDEX,
            required=True,
            description="Proof index mapping public records to their source evidence artifacts.",
        ),
        DisclosureOutputEntry(
            file_name=DISCLOSURE_OUTPUT_INVENTORY_JSON,
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.OUTPUT_INVENTORY,
            required=True,
            description="Deterministic output inventory listing all produced disclosure files with SHA-256 hashes.",
        ),
        DisclosureOutputEntry(
            file_name=DISCLOSURE_PROHIBITED_SQLITE,
            output_format=OutputFormat.PROHIBITED_SQLITE,
            output_role=OutputRole.PROHIBITED_SQLITE,
            required=False,
            description="SQLite is explicitly prohibited in the first release per D12.",
        ),
    ])
    return outputs


def build_default_disclosure_authority() -> DisclosureAuthority:
    """Build the default D12 disclosure authority for v5 campaign projections.

    The default authority classifies every field of
    ``CampaignProjectionRow`` as PUBLIC, with no RESTRICTED fields. The
    field universe is derived from ``CampaignProjectionRow.model_fields``
    (typed introspection) so it cannot drift from the projection row
    model. This is the baseline authority for the first release where all
    projection fields are already public-safe. A campaign with restricted
    evidence fields must define a campaign-specific authority that
    classifies those fields as RESTRICTED.
    """
    fields = [_default_public_field(name) for name in CampaignProjectionRow.model_fields]
    outputs = _default_output_entries(include_tombstones=False)
    proof_index_description = (
        "Maps each public projection record to its source "
        "campaign-projections.jsonl artifact by record index and source SHA-256."
    )
    content_hash = compute_disclosure_authority_hash(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id="default-v5-projection",
        authority_version="1",
        fields=fields,
        outputs=outputs,
        proof_index_description=proof_index_description,
    )
    return DisclosureAuthority(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id="default-v5-projection",
        authority_version="1",
        fields=fields,
        outputs=outputs,
        proof_index_description=proof_index_description,
        content_hash=content_hash,
    )


def build_restricted_disclosure_authority() -> DisclosureAuthority:
    """Build a D12 disclosure authority with RESTRICTED fields for testing.

    This authority classifies ``evidence_link`` as RESTRICTED so the
    disclosure publisher emits tombstones for it. The field universe is
    derived from ``CampaignProjectionRow.model_fields`` with an explicit
    override for ``evidence_link``. It includes the required TOMBSTONES
    output role. Used by the disclosure dress rehearsal mutation tests to
    verify tombstone generation.
    """
    # RestrictedFieldOverride: field_name -> (classification, public_column_name, in_csv, tombstone_hash_field)
    overrides: dict[str, tuple[FieldClassification, str, bool, str]] = {
        "evidence_link": (
            FieldClassification.RESTRICTED,
            "evidence_link",
            False,
            "evidence_link_hash",
        ),
    }
    fields: list[DisclosureFieldEntry] = []
    for name in CampaignProjectionRow.model_fields:
        if name in overrides:
            classification, public_column_name, in_csv, tombstone_hash_field = overrides[name]
            fields.append(DisclosureFieldEntry(
                field_name=name,
                classification=classification,
                public_column_name=public_column_name,
                in_csv=in_csv,
                tombstone_hash_field=tombstone_hash_field,
            ))
        else:
            fields.append(_default_public_field(name))
    outputs = _default_output_entries(include_tombstones=True)
    proof_index_description = (
        "Maps each public projection record to its source "
        "campaign-projections.jsonl artifact by record index and source "
        "SHA-256. Restricted evidence_link fields are replaced with tombstones."
    )
    content_hash = compute_disclosure_authority_hash(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id="restricted-v5-projection",
        authority_version="1",
        fields=fields,
        outputs=outputs,
        proof_index_description=proof_index_description,
    )
    return DisclosureAuthority(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id="restricted-v5-projection",
        authority_version="1",
        fields=fields,
        outputs=outputs,
        proof_index_description=proof_index_description,
        content_hash=content_hash,
    )


__all__ = [
    "DISCLOSURE_DERIVED_CSV",
    "DISCLOSURE_OUTPUT_INVENTORY_JSON",
    "DISCLOSURE_OUTPUT_SCHEMA_VERSION",
    "DISCLOSURE_PROHIBITED_SQLITE",
    "DISCLOSURE_PROOF_INDEX_JSON",
    "DISCLOSURE_PUBLIC_JSONL",
    "DISCLOSURE_TOMBSTONES_JSONL",
    "DisclosurePublishError",
    "DisclosureValidationResult",
    "OutputInventory",
    "OutputInventoryEntry",
    "ProofIndex",
    "ProofIndexEntry",
    "TombstoneRecord",
    "build_default_disclosure_authority",
    "build_disclosure_outputs",
    "build_restricted_disclosure_authority",
    "compute_output_inventory_hash",
    "validate_disclosure_outputs",
]
