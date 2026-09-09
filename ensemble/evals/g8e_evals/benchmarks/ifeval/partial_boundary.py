# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed declaration of the ifeval_subset partial-evaluation boundary.

The ``ifeval_subset`` suite is an explicitly partial import of the
upstream google-research IFEval dataset. The partial verifier
(``IFEvalVerifier``) implements a subset of the upstream instruction
checks; unsupported instruction types fail closed rather than becoming
implicit passes. The complete upstream evaluator and dataset import is
a separately scoped follow-on task (Follow-on milestone A, item 5) that
requires pinned revision, license, transformation hash, output hash,
partitions, and domain strata.

This module is the single source of truth for the partial boundary. It
is consumed by tests that verify the boundary is explicit, the subset
size is declared, the supported and unsupported instruction types are
enumerated, and the complete-import follow-on is documented.
"""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.benchmarks.ifeval.import_subset import SELECTED_KEYS, UPSTREAM_REVISION


class InstructionSupport(StrEnum):
    """Support level for an instruction type in the partial verifier."""

    SUPPORTED = "supported"
    UNSUPPORTED_FAILS_CLOSED = "unsupported_fails_closed"


class InstructionTypeBoundary(BaseModel):
    """One instruction type and its support level in the partial verifier."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    instruction_type: str = Field(min_length=1)
    support: InstructionSupport
    reason: str = Field(min_length=1)


class IFEvalSubsetBoundary(BaseModel):
    """Typed declaration of the ifeval_subset partial-evaluation boundary.

    Records the subset size, the pinned upstream revision, the supported
    and unsupported instruction types, and the explicit statement that
    the complete upstream evaluator and dataset import is a separately
    scoped follow-on task.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    suite_id: str = Field(default="ifeval_subset", min_length=1)
    subset_size: int = Field(ge=1)
    upstream_revision: str = Field(min_length=1)
    selected_keys: list[int] = Field(min_length=1)
    instruction_types: list[InstructionTypeBoundary] = Field(min_length=1)
    complete_import_is_separately_scoped: bool = True
    complete_import_follow_on: str = Field(
        default="Follow-on milestone A, item 5: replace ifeval_subset with the complete canonical upstream IFEval evaluator and dataset under pinned provenance and licensing",
        min_length=1,
    )

    @property
    def supported_types(self) -> set[str]:
        return {
            it.instruction_type
            for it in self.instruction_types
            if it.support == InstructionSupport.SUPPORTED
        }

    @property
    def unsupported_types(self) -> set[str]:
        return {
            it.instruction_type
            for it in self.instruction_types
            if it.support == InstructionSupport.UNSUPPORTED_FAILS_CLOSED
        }


IFEVAL_SUBSET_BOUNDARY = IFEvalSubsetBoundary(
    suite_id="ifeval_subset",
    subset_size=len(SELECTED_KEYS),
    upstream_revision=UPSTREAM_REVISION,
    selected_keys=list(SELECTED_KEYS),
    instruction_types=[
        InstructionTypeBoundary(
            instruction_type="punctuation:no_comma",
            support=InstructionSupport.SUPPORTED,
            reason="Checks for comma absence in the response",
        ),
        InstructionTypeBoundary(
            instruction_type="keywords:forbidden_words",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that forbidden words do not appear in the response",
        ),
        InstructionTypeBoundary(
            instruction_type="keywords:existence",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that required keywords appear in the response",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:json_format",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that the response is valid JSON or a JSON code block",
        ),
        InstructionTypeBoundary(
            instruction_type="length_constraints:number_words",
            support=InstructionSupport.SUPPORTED,
            reason="Checks word count against a relation and threshold",
        ),
        InstructionTypeBoundary(
            instruction_type="change_case:english_capital",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that all alpha characters are uppercase",
        ),
        InstructionTypeBoundary(
            instruction_type="change_case:english_lowercase",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that all alpha characters are lowercase",
        ),
        InstructionTypeBoundary(
            instruction_type="language:response_language",
            support=InstructionSupport.UNSUPPORTED_FAILS_CLOSED,
            reason="Language verification is not implemented in this partial evaluator; the canonical upstream evaluator replaces this partial implementation",
        ),
    ],
)


__all__ = [
    "IFEVAL_SUBSET_BOUNDARY",
    "IFEvalSubsetBoundary",
    "InstructionSupport",
    "InstructionTypeBoundary",
]
