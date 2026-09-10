# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for append-only index generation parent chains.

Verifies that IndexGeneration models enforce parent-generation hash
chaining, creation reasons, complete report checksums, and assignment
dispositions. The first generation has a zero parent hash; each
subsequent generation's parent hash must match the previous
generation's content hash. Broken chains, unknown fields, and
non-frozen models are rejected. No external dependencies.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields
# and unknown extra fields to verify pydantic validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.index import (
    AssignmentDisposition,
    AssignmentDispositionEntry,
    IndexCreationReason,
    IndexGeneration,
    compute_index_generation_hash,
    validate_index_chain,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_ZERO_HASH = "0" * 64


def _make_generation(
    *,
    generation_number: int = 0,
    parent_generation_hash: str = _ZERO_HASH,
    creation_reason: IndexCreationReason = IndexCreationReason.INITIAL,
    report_checksums: list[str] | None = None,
    assignment_dispositions: list[dict] | None = None,
    content_hash: str | None = None,
) -> IndexGeneration:
    if report_checksums is None:
        report_checksums = [_VALID_HASH]
    if assignment_dispositions is None:
        assignment_dispositions = [
            {"assignment_id": "assign-1", "disposition": AssignmentDisposition.EFFECTIVE},
        ]
    if content_hash is None:
        content_hash = compute_index_generation_hash(
            generation_number=generation_number,
            parent_generation_hash=parent_generation_hash,
            creation_reason=creation_reason,
            report_checksums=report_checksums,
            assignment_dispositions=assignment_dispositions,
        )
    disposition_entries = [
        AssignmentDispositionEntry(
            assignment_id=d["assignment_id"],
            disposition=AssignmentDisposition(d["disposition"])
            if isinstance(d["disposition"], str)
            else d["disposition"],
        )
        for d in assignment_dispositions
    ]
    return IndexGeneration(
        generation_number=generation_number,
        parent_generation_hash=parent_generation_hash,
        creation_reason=creation_reason,
        report_checksums=report_checksums,
        assignment_dispositions=disposition_entries,
        content_hash=content_hash,
    )


class TestIndexGenerationModel:
    def test_round_trip_preserves_all_fields(self):
        gen = _make_generation()
        restored = IndexGeneration.model_validate_json(gen.model_dump_json())
        assert restored == gen

    def test_rejects_unknown_fields(self):
        data = _make_generation().model_dump()
        data["extra_field"] = "bad"
        with pytest.raises(ValidationError):
            IndexGeneration(**data)

    def test_frozen_model(self):
        gen = _make_generation()
        with pytest.raises(ValidationError):
            gen.generation_number = 99  # type: ignore[misc]

    def test_requires_generation_number(self):
        data = _make_generation().model_dump()
        del data["generation_number"]
        with pytest.raises(ValidationError):
            IndexGeneration(**data)

    def test_requires_parent_generation_hash(self):
        data = _make_generation().model_dump()
        del data["parent_generation_hash"]
        with pytest.raises(ValidationError):
            IndexGeneration(**data)

    def test_content_hash_mismatch_raises(self):
        data = _make_generation().model_dump()
        data["content_hash"] = _ZERO_HASH
        with pytest.raises(ValueError, match="content_hash mismatch"):
            IndexGeneration(**data)


class TestIndexCreationReason:
    def test_initial_value(self):
        assert IndexCreationReason.INITIAL.value == "initial"

    def test_resume_value(self):
        assert IndexCreationReason.RESUME.value == "resume"

    def test_supersession_value(self):
        assert IndexCreationReason.SUPERSESSION.value == "supersession"

    def test_finalization_value(self):
        assert IndexCreationReason.FINALIZATION.value == "finalization"


class TestAssignmentDisposition:
    def test_effective_value(self):
        assert AssignmentDisposition.EFFECTIVE.value == "effective"

    def test_superseded_value(self):
        assert AssignmentDisposition.SUPERSEDED.value == "superseded"

    def test_qualification_value(self):
        assert AssignmentDisposition.QUALIFICATION.value == "qualification"

    def test_unavailable_value(self):
        assert AssignmentDisposition.UNAVAILABLE.value == "unavailable"


class TestIndexChainValidation:
    def test_single_generation_with_zero_parent_passes(self):
        """The first generation has a zero parent hash and passes validation."""
        gen0 = _make_generation(generation_number=0, parent_generation_hash=_ZERO_HASH)
        validate_index_chain([gen0])

    def test_valid_two_generation_chain_passes(self):
        """A chain where gen1's parent hash matches gen0's content hash passes."""
        gen0 = _make_generation(generation_number=0, parent_generation_hash=_ZERO_HASH)
        gen1 = _make_generation(
            generation_number=1,
            parent_generation_hash=gen0.content_hash,
            creation_reason=IndexCreationReason.SUPERSESSION,
        )
        validate_index_chain([gen0, gen1])

    def test_broken_parent_chain_rejected(self):
        """A chain where gen1's parent hash does not match gen0's content hash is rejected."""
        gen0 = _make_generation(generation_number=0, parent_generation_hash=_ZERO_HASH)
        gen1 = _make_generation(
            generation_number=1,
            parent_generation_hash=_VALID_HASH,  # Wrong: should be gen0.content_hash
            creation_reason=IndexCreationReason.SUPERSESSION,
        )
        with pytest.raises(ValueError, match="parent_generation_hash mismatch"):
            validate_index_chain([gen0, gen1])

    def test_non_zero_parent_on_first_generation_rejected(self):
        """The first generation must have a zero parent hash."""
        gen0 = _make_generation(generation_number=0, parent_generation_hash=_VALID_HASH)
        with pytest.raises(ValueError, match=r"first generation.*zero"):
            validate_index_chain([gen0])

    def test_non_contiguous_generation_numbers_rejected(self):
        """Generation numbers must be contiguous starting from 0."""
        gen0 = _make_generation(generation_number=0, parent_generation_hash=_ZERO_HASH)
        gen2 = _make_generation(
            generation_number=2,
            parent_generation_hash=gen0.content_hash,
            creation_reason=IndexCreationReason.SUPERSESSION,
        )
        with pytest.raises(ValueError, match="non-contiguous"):
            validate_index_chain([gen0, gen2])

    def test_empty_chain_rejected(self):
        """An empty index chain is rejected."""
        with pytest.raises(ValueError, match="empty"):
            validate_index_chain([])

    def test_append_only_new_generation_must_reference_previous(self):
        """A new generation must reference the previous generation's content hash."""
        gen0 = _make_generation(generation_number=0, parent_generation_hash=_ZERO_HASH)
        gen1 = _make_generation(
            generation_number=1,
            parent_generation_hash=gen0.content_hash,
            creation_reason=IndexCreationReason.RESUME,
        )
        gen2_bad = _make_generation(
            generation_number=2,
            parent_generation_hash=gen0.content_hash,  # Should be gen1.content_hash
            creation_reason=IndexCreationReason.SUPERSESSION,
        )
        with pytest.raises(ValueError, match="parent_generation_hash mismatch"):
            validate_index_chain([gen0, gen1, gen2_bad])
