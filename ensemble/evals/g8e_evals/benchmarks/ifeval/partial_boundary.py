# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed declaration of the ifeval_subset evaluation boundary.

The ``ifeval_subset`` suite is a curated import of the upstream
google-research IFEval dataset. The verifier (``IFEvalVerifier``)
implements all 25 upstream instruction types using regex-based
utilities without external dependencies (nltk, langdetect). The dataset
is a subset of the full 541-task upstream dataset, selected to maximize
instruction-type coverage with at least 3 tasks per type where
available. The complete upstream dataset import (all 541 tasks) is a
separately scoped follow-on task that requires pinned revision,
license, transformation hash, output hash, partitions, and domain
strata.

This module is the single source of truth for the evaluation boundary.
It is consumed by tests that verify the boundary is explicit, the
subset size is declared, all instruction types are enumerated as
supported, and the complete-dataset follow-on is documented.
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
    """Typed declaration of the ifeval_subset evaluation boundary.

    Records the subset size, the pinned upstream revision, all
    supported instruction types, and the explicit statement that the
    complete upstream dataset import is a separately scoped follow-on
    task. The verifier implements all 25 upstream instruction types;
    there are no unsupported types.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    suite_id: str = Field(default="ifeval_subset", min_length=1)
    subset_size: int = Field(ge=1)
    upstream_revision: str = Field(min_length=1)
    selected_keys: list[int] = Field(min_length=1)
    instruction_types: list[InstructionTypeBoundary] = Field(min_length=1)
    complete_import_is_separately_scoped: bool = True
    complete_import_follow_on: str = Field(
        default="Follow-on milestone A, item 5: expand ifeval_subset from the curated subset to the complete 541-task upstream IFEval dataset under pinned provenance and licensing; the verifier already implements all 25 instruction types",
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
            reason="Checks that forbidden words do not appear in the response using word-boundary matching",
        ),
        InstructionTypeBoundary(
            instruction_type="keywords:existence",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that required keywords appear in the response",
        ),
        InstructionTypeBoundary(
            instruction_type="keywords:frequency",
            support=InstructionSupport.SUPPORTED,
            reason="Checks keyword occurrence count against a relation and threshold",
        ),
        InstructionTypeBoundary(
            instruction_type="keywords:letter_frequency",
            support=InstructionSupport.SUPPORTED,
            reason="Checks letter occurrence count against a relation and threshold",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:json_format",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that the response is valid JSON or a JSON code block",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:title",
            support=InstructionSupport.SUPPORTED,
            reason="Checks for a title wrapped in double angle brackets",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:number_bullet_lists",
            support=InstructionSupport.SUPPORTED,
            reason="Checks the exact count of markdown bullet points",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:number_highlighted_sections",
            support=InstructionSupport.SUPPORTED,
            reason="Checks the count of highlighted sections against a minimum",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:multiple_sections",
            support=InstructionSupport.SUPPORTED,
            reason="Checks the count of sections split by a section delimiter against a minimum",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_format:constrained_response",
            support=InstructionSupport.SUPPORTED,
            reason="Checks for one of the three constrained response options",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_content:number_placeholders",
            support=InstructionSupport.SUPPORTED,
            reason="Checks the count of placeholder brackets against a minimum",
        ),
        InstructionTypeBoundary(
            instruction_type="detectable_content:postscript",
            support=InstructionSupport.SUPPORTED,
            reason="Checks for a postscript marker at the end of the response",
        ),
        InstructionTypeBoundary(
            instruction_type="length_constraints:number_words",
            support=InstructionSupport.SUPPORTED,
            reason="Checks word count against a relation and threshold",
        ),
        InstructionTypeBoundary(
            instruction_type="length_constraints:number_sentences",
            support=InstructionSupport.SUPPORTED,
            reason="Checks sentence count against a relation and threshold using regex-based splitting",
        ),
        InstructionTypeBoundary(
            instruction_type="length_constraints:number_paragraphs",
            support=InstructionSupport.SUPPORTED,
            reason="Checks paragraph count split by markdown dividers against an exact count",
        ),
        InstructionTypeBoundary(
            instruction_type="length_constraints:nth_paragraph_first_word",
            support=InstructionSupport.SUPPORTED,
            reason="Checks the first word of the nth paragraph matches the expected word",
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
            instruction_type="change_case:capital_word_frequency",
            support=InstructionSupport.SUPPORTED,
            reason="Checks all-caps word count against a relation and threshold",
        ),
        InstructionTypeBoundary(
            instruction_type="language:response_language",
            support=InstructionSupport.SUPPORTED,
            reason="Checks response language using script-based detection; uncertain detection counts as followed matching upstream behavior",
        ),
        InstructionTypeBoundary(
            instruction_type="combination:two_responses",
            support=InstructionSupport.SUPPORTED,
            reason="Checks for exactly two different non-empty responses separated by a delimiter",
        ),
        InstructionTypeBoundary(
            instruction_type="combination:repeat_prompt",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that the response starts with the prompt to repeat",
        ),
        InstructionTypeBoundary(
            instruction_type="startend:end_checker",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that the response ends with the expected phrase",
        ),
        InstructionTypeBoundary(
            instruction_type="startend:quotation",
            support=InstructionSupport.SUPPORTED,
            reason="Checks that the response is wrapped in double quotation marks",
        ),
    ],
)


__all__ = [
    "IFEVAL_SUBSET_BOUNDARY",
    "IFEvalSubsetBoundary",
    "InstructionSupport",
    "InstructionTypeBoundary",
]
