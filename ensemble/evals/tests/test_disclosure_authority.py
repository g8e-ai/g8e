# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the D12 disclosure authority model.

Verifies that ``DisclosureAuthority`` is frozen with ``extra="forbid"``,
validates content hashes, rejects duplicate fields and outputs, enforces
field classification consistency, prohibits SQLite in the first release,
and provides deterministic query helpers.
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from g8e_evals.disclosure_authority import (
    DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
    DisclosureAuthority,
    DisclosureFieldEntry,
    DisclosureOutputEntry,
    FieldClassification,
    OutputFormat,
    compute_disclosure_authority_hash,
)


_HASH = "a" * 64


def _make_fields() -> list[dict]:
    return [
        {
            "field_name": "variant_id",
            "classification": "public",
            "public_column_name": "variant_id",
            "in_csv": True,
            "tombstone_hash_field": "",
        },
        {
            "field_name": "task_id",
            "classification": "public",
            "public_column_name": "task_id",
            "in_csv": True,
            "tombstone_hash_field": "",
        },
        {
            "field_name": "raw_prompt",
            "classification": "restricted",
            "public_column_name": "raw_prompt_tombstone",
            "in_csv": False,
            "tombstone_hash_field": "raw_prompt_sha256",
        },
        {
            "field_name": "model_output",
            "classification": "restricted",
            "public_column_name": "model_output_tombstone",
            "in_csv": False,
            "tombstone_hash_field": "model_output_sha256",
        },
        {
            "field_name": "evidence_hash",
            "classification": "tombstone",
            "public_column_name": "evidence_hash",
            "in_csv": True,
            "tombstone_hash_field": "evidence_hash",
        },
    ]


def _make_outputs() -> list[dict]:
    return [
        {
            "file_name": "disclosure-public.jsonl",
            "output_format": "canonical_jsonl",
            "required": True,
            "description": "Canonical public JSONL with tombstones for restricted fields.",
        },
        {
            "file_name": "disclosure-derived.csv",
            "output_format": "derived_csv",
            "required": True,
            "description": "Derived CSV projection of public fields.",
        },
        {
            "file_name": "disclosure-tombstones.jsonl",
            "output_format": "canonical_jsonl",
            "required": True,
            "description": "Tombstone records for restricted fields with SHA-256 hashes and lengths.",
        },
        {
            "file_name": "disclosure-proof-index.json",
            "output_format": "canonical_jsonl",
            "required": True,
            "description": "Proof index mapping public records to their source evidence.",
        },
        {
            "file_name": "disclosure-output-inventory.json",
            "output_format": "canonical_jsonl",
            "required": True,
            "description": "Deterministic output inventory listing all produced files.",
        },
        {
            "file_name": "evidence.sqlite",
            "output_format": "prohibited_sqlite",
            "required": False,
            "description": "SQLite is prohibited in the first release.",
        },
    ]


def _make_authority(
    *,
    fields: list[dict] | None = None,
    outputs: list[dict] | None = None,
    content_hash: str | None = None,
    authority_id: str = "disclosure-1",
    authority_version: str = "1",
    proof_index_description: str = "Maps each public record to its source evidence hash and proof artifact path.",
) -> DisclosureAuthority:
    f = fields if fields is not None else _make_fields()
    o = outputs if outputs is not None else _make_outputs()
    if content_hash is None:
        content_hash = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id=authority_id,
            authority_version=authority_version,
            fields=f,
            outputs=o,
            proof_index_description=proof_index_description,
        )
    return DisclosureAuthority(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id=authority_id,
        authority_version=authority_version,
        fields=[DisclosureFieldEntry.model_validate(e) for e in f],
        outputs=[DisclosureOutputEntry.model_validate(e) for e in o],
        proof_index_description=proof_index_description,
        content_hash=content_hash,
    )


class TestFieldClassification:
    pytestmark = pytest.mark.unit

    def test_enum_has_three_values(self) -> None:
        assert len(list(FieldClassification)) == 3

    def test_expected_values(self) -> None:
        assert FieldClassification.PUBLIC.value == "public"
        assert FieldClassification.RESTRICTED.value == "restricted"
        assert FieldClassification.TOMBSTONE.value == "tombstone"


class TestOutputFormat:
    pytestmark = pytest.mark.unit

    def test_enum_has_three_values(self) -> None:
        assert len(list(OutputFormat)) == 3

    def test_expected_values(self) -> None:
        assert OutputFormat.CANONICAL_JSONL.value == "canonical_jsonl"
        assert OutputFormat.DERIVED_CSV.value == "derived_csv"
        assert OutputFormat.PROHIBITED_SQLITE.value == "prohibited_sqlite"


class TestDisclosureFieldEntry:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        entry = DisclosureFieldEntry(
            field_name="x",
            classification=FieldClassification.PUBLIC,
            public_column_name="x",
        )
        with pytest.raises(ValidationError):
            entry.field_name = "y"  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        with pytest.raises(ValidationError):
            DisclosureFieldEntry(
                field_name="x",
                classification=FieldClassification.PUBLIC,
                public_column_name="x",
                extra="no",  # type: ignore[call-arg]
            )

    def test_restricted_requires_tombstone_hash_field(self) -> None:
        with pytest.raises(ValidationError):
            DisclosureFieldEntry(
                field_name="x",
                classification=FieldClassification.RESTRICTED,
                public_column_name="x_tombstone",
                tombstone_hash_field="",
            )

    def test_tombstone_requires_tombstone_hash_field(self) -> None:
        with pytest.raises(ValidationError):
            DisclosureFieldEntry(
                field_name="x",
                classification=FieldClassification.TOMBSTONE,
                public_column_name="x",
                tombstone_hash_field="",
            )

    def test_public_rejects_tombstone_hash_field(self) -> None:
        with pytest.raises(ValidationError):
            DisclosureFieldEntry(
                field_name="x",
                classification=FieldClassification.PUBLIC,
                public_column_name="x",
                tombstone_hash_field="x_hash",
            )


class TestDisclosureOutputEntry:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        entry = DisclosureOutputEntry(
            file_name="out.jsonl",
            output_format=OutputFormat.CANONICAL_JSONL,
            required=True,
            description="test",
        )
        with pytest.raises(ValidationError):
            entry.file_name = "other"  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        with pytest.raises(ValidationError):
            DisclosureOutputEntry(
                file_name="out.jsonl",
                output_format=OutputFormat.CANONICAL_JSONL,
                required=True,
                description="test",
                extra="no",  # type: ignore[call-arg]
            )

    def test_prohibited_sqlite_must_not_be_required(self) -> None:
        with pytest.raises(ValidationError):
            DisclosureOutputEntry(
                file_name="evidence.sqlite",
                output_format=OutputFormat.PROHIBITED_SQLITE,
                required=True,
                description="prohibited",
            )

    def test_prohibited_sqlite_not_required_is_valid(self) -> None:
        entry = DisclosureOutputEntry(
            file_name="evidence.sqlite",
            output_format=OutputFormat.PROHIBITED_SQLITE,
            required=False,
            description="prohibited in first release",
        )
        assert entry.output_format == OutputFormat.PROHIBITED_SQLITE


class TestDisclosureAuthority:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        authority = _make_authority()
        with pytest.raises(ValidationError):
            authority.authority_id = "other"  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        authority = _make_authority()
        data = json.loads(authority.model_dump_json())
        data["secret"] = "leak"
        with pytest.raises(ValidationError):
            DisclosureAuthority.model_validate(data)

    def test_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_authority(content_hash="b" * 64)

    def test_rejects_duplicate_field_names(self) -> None:
        fields = _make_fields()
        fields.append(dict(fields[0]))
        with pytest.raises(ValidationError):
            _make_authority(fields=fields)

    def test_rejects_duplicate_public_column_names(self) -> None:
        fields = _make_fields()
        dup = dict(fields[0])
        dup["field_name"] = "different_name"
        fields.append(dup)
        with pytest.raises(ValidationError):
            _make_authority(fields=fields)

    def test_rejects_duplicate_output_file_names(self) -> None:
        outputs = _make_outputs()
        outputs.append(dict(outputs[0]))
        with pytest.raises(ValidationError):
            _make_authority(outputs=outputs)

    def test_round_trip_serialization(self) -> None:
        authority = _make_authority()
        data = json.loads(authority.model_dump_json())
        restored = DisclosureAuthority.model_validate(data)
        assert restored == authority

    def test_compute_hash_is_deterministic(self) -> None:
        f = _make_fields()
        o = _make_outputs()
        h1 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o,
            proof_index_description="test",
        )
        h2 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o,
            proof_index_description="test",
        )
        assert h1 == h2

    def test_compute_hash_changes_with_field(self) -> None:
        f1 = _make_fields()
        f2 = _make_fields()
        f2[0] = dict(f2[0])
        f2[0]["public_column_name"] = "renamed_column"
        o = _make_outputs()
        h1 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f1,
            outputs=o,
            proof_index_description="test",
        )
        h2 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f2,
            outputs=o,
            proof_index_description="test",
        )
        assert h1 != h2

    def test_compute_hash_changes_with_output(self) -> None:
        f = _make_fields()
        o1 = _make_outputs()
        o2 = _make_outputs()
        o2[0] = dict(o2[0])
        o2[0]["description"] = "changed description"
        h1 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o1,
            proof_index_description="test",
        )
        h2 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o2,
            proof_index_description="test",
        )
        assert h1 != h2

    def test_compute_hash_changes_with_authority_id(self) -> None:
        f = _make_fields()
        o = _make_outputs()
        h1 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o,
            proof_index_description="test",
        )
        h2 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-2",
            authority_version="1",
            fields=f,
            outputs=o,
            proof_index_description="test",
        )
        assert h1 != h2

    def test_compute_hash_changes_with_proof_description(self) -> None:
        f = _make_fields()
        o = _make_outputs()
        h1 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o,
            proof_index_description="description one",
        )
        h2 = compute_disclosure_authority_hash(
            schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
            authority_id="disclosure-1",
            authority_version="1",
            fields=f,
            outputs=o,
            proof_index_description="description two",
        )
        assert h1 != h2


class TestDisclosureAuthorityQueries:
    pytestmark = pytest.mark.unit

    def test_public_fields_returns_only_public(self) -> None:
        authority = _make_authority()
        public = authority.public_fields()
        assert all(e.classification == FieldClassification.PUBLIC for e in public)
        assert len(public) == 2

    def test_restricted_fields_returns_only_restricted(self) -> None:
        authority = _make_authority()
        restricted = authority.restricted_fields()
        assert all(e.classification == FieldClassification.RESTRICTED for e in restricted)
        assert len(restricted) == 2

    def test_tombstone_fields_returns_only_tombstone(self) -> None:
        authority = _make_authority()
        tombstones = authority.tombstone_fields()
        assert all(e.classification == FieldClassification.TOMBSTONE for e in tombstones)
        assert len(tombstones) == 1

    def test_required_outputs_returns_only_required(self) -> None:
        authority = _make_authority()
        required = authority.required_outputs()
        assert all(e.required for e in required)
        assert len(required) == 5

    def test_prohibited_outputs_returns_only_sqlite(self) -> None:
        authority = _make_authority()
        prohibited = authority.prohibited_outputs()
        assert len(prohibited) == 1
        assert prohibited[0].output_format == OutputFormat.PROHIBITED_SQLITE

    def test_get_field_returns_entry(self) -> None:
        authority = _make_authority()
        entry = authority.get_field("variant_id")
        assert entry is not None
        assert entry.classification == FieldClassification.PUBLIC

    def test_get_field_returns_none_for_unknown(self) -> None:
        authority = _make_authority()
        assert authority.get_field("nonexistent") is None

    def test_get_output_returns_entry(self) -> None:
        authority = _make_authority()
        entry = authority.get_output("disclosure-public.jsonl")
        assert entry is not None
        assert entry.output_format == OutputFormat.CANONICAL_JSONL

    def test_get_output_returns_none_for_unknown(self) -> None:
        authority = _make_authority()
        assert authority.get_output("nonexistent") is None


class TestDisclosureAuthorityD12Semantics:
    """Tests enforcing D12 decision semantics."""

    pytestmark = pytest.mark.unit

    def test_sqlite_is_prohibited(self) -> None:
        authority = _make_authority()
        prohibited = authority.prohibited_outputs()
        assert any(e.file_name == "evidence.sqlite" for e in prohibited)
        assert all(not e.required for e in prohibited)

    def test_restricted_fields_have_tombstone_hash_fields(self) -> None:
        authority = _make_authority()
        for entry in authority.restricted_fields():
            assert entry.tombstone_hash_field

    def test_public_fields_have_no_tombstone_hash_fields(self) -> None:
        authority = _make_authority()
        for entry in authority.public_fields():
            assert not entry.tombstone_hash_field

    def test_canonical_jsonl_output_exists(self) -> None:
        authority = _make_authority()
        jsonl_outputs = [
            e for e in authority.outputs
            if e.output_format == OutputFormat.CANONICAL_JSONL
        ]
        assert len(jsonl_outputs) >= 1

    def test_derived_csv_output_exists(self) -> None:
        authority = _make_authority()
        csv_outputs = [
            e for e in authority.outputs
            if e.output_format == OutputFormat.DERIVED_CSV
        ]
        assert len(csv_outputs) >= 1

    def test_output_inventory_is_deterministic(self) -> None:
        a1 = _make_authority()
        a2 = _make_authority()
        assert a1.content_hash == a2.content_hash
        assert [e.file_name for e in a1.outputs] == [e.file_name for e in a2.outputs]
