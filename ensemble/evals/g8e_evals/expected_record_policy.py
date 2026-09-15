# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Frozen typed expected-record policy for campaign observation files.

The ``ExpectedRecordPolicy`` states applicability and exact cardinality
or derivation rules for each observation and event JSONL file by
suite/scenario/attempt/inference. The policy distinguishes "zero
expected" from "file omitted" so the verifier can enforce required files
and reject fabricated files for inapplicable scenarios.

The policy is frozen and content-addressed. A changed policy creates a
new identity and invalidates dependent collection. The campaign profile
binds the policy hash so any mutation is detectable.

Cardinality rules:

- ``EXACT``: The file must contain exactly ``expected_count`` records.
  ``expected_count=0`` means the file must exist and be empty.
- ``ONE_PER_INFERENCE``: The file must contain one record per actual
  provider inference. The count is derived from the inference trail.
- ``ONE_PER_ATTEMPT``: The file must contain one record per terminal
  attempt for applicable scenarios.
- ``DERIVED``: The count is derived from a frozen derivation rule
  string that the verifier evaluates against the report's typed
  records.

Applicability:

- ``REQUIRED``: The file must exist and satisfy the cardinality rule.
- ``NOT_APPLICABLE``: The file must not exist. A present file is
  rejected as fabricated.
- ``OPTIONAL``: The file may exist. When present, the cardinality rule
  applies. When absent, no failure is recorded.
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from collections.abc import Sequence
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


EXPECTED_RECORD_POLICY_SCHEMA_VERSION = "1.0.0"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class RecordApplicability(StrEnum):
    """Whether a record file is required, not applicable, or optional.

    ``REQUIRED``: The file must exist and satisfy the cardinality rule.
    ``NOT_APPLICABLE``: The file must not exist. A present file is
    rejected as fabricated for an inapplicable scenario.
    ``OPTIONAL``: The file may exist. When present, the cardinality rule
    applies. When absent, no failure is recorded.
    """

    REQUIRED = "required"
    NOT_APPLICABLE = "not_applicable"
    OPTIONAL = "optional"


class CardinalityRule(StrEnum):
    """How the expected record count is determined.

    ``EXACT``: The file must contain exactly ``expected_count`` records.
    ``ONE_PER_INFERENCE``: One record per actual provider inference.
    ``ONE_PER_ATTEMPT``: One record per terminal attempt for applicable
    scenarios.
    ``DERIVED``: The count is derived from a frozen derivation rule
    string that the verifier evaluates against typed records.
    """

    EXACT = "exact"
    ONE_PER_INFERENCE = "one_per_inference"
    ONE_PER_ATTEMPT = "one_per_attempt"
    DERIVED = "derived"


class ExpectedRecordEntry(BaseModel):
    """One expected-record file entry in the policy.

    Binds a file name to its applicability, cardinality rule, expected
    count (when exact), and derivation rule (when derived). The entry is
    frozen and forbids extra fields.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    file_name: str = Field(min_length=1, description="Artifact file name (e.g. resource-observations.jsonl).")
    applicability: RecordApplicability = Field(description="Whether the file is required, not applicable, or optional.")
    cardinality_rule: CardinalityRule = Field(description="How the expected record count is determined.")
    expected_count: int | None = Field(
        default=None, ge=0,
        description="Exact expected count when cardinality_rule is EXACT. None for other rules.",
    )
    derivation_rule: str = Field(
        default="",
        description="Frozen derivation rule string when cardinality_rule is DERIVED. Empty for other rules.",
    )

    @model_validator(mode="after")
    def _validate_cardinality_consistency(self) -> Self:
        if self.cardinality_rule == CardinalityRule.EXACT:
            if self.expected_count is None:
                raise ValueError(
                    f"expected_count must be set when cardinality_rule is EXACT for {self.file_name!r}"
                )
        elif self.expected_count is not None:
            raise ValueError(
                f"expected_count must be None when cardinality_rule is {self.cardinality_rule.value!r} "
                f"for {self.file_name!r}"
            )
        if self.cardinality_rule == CardinalityRule.DERIVED:
            if not self.derivation_rule:
                raise ValueError(
                    f"derivation_rule must be non-empty when cardinality_rule is DERIVED for {self.file_name!r}"
                )
        elif self.derivation_rule:
            raise ValueError(
                f"derivation_rule must be empty when cardinality_rule is {self.cardinality_rule.value!r} "
                f"for {self.file_name!r}"
            )
        if self.applicability == RecordApplicability.NOT_APPLICABLE:
            if self.cardinality_rule != CardinalityRule.EXACT or self.expected_count != 0:
                raise ValueError(
                    f"NOT_APPLICABLE entries must have cardinality_rule EXACT with expected_count 0 "
                    f"for {self.file_name!r}"
                )
        return self


class ExpectedRecordPolicy(BaseModel):
    """Frozen typed expected-record policy for campaign observation files.

    States applicability and exact cardinality or derivation rules for
    each observation and event JSONL file by suite/scenario/attempt/
    inference. The policy distinguishes "zero expected" from "file
    omitted" so the verifier can enforce required files and reject
    fabricated files for inapplicable scenarios.

    The policy is frozen and content-addressed. A changed policy
    creates a new identity and invalidates dependent collection. The
    campaign profile binds the policy hash so any mutation is
    detectable.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(
        default=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
        min_length=1,
        description="Schema version of the expected-record policy.",
    )
    policy_id: str = Field(min_length=1, description="Unique policy identifier.")
    policy_version: str = Field(min_length=1, description="Policy version.")
    suite_id: str = Field(min_length=1, description="Suite ID this policy applies to.")
    entries: list[ExpectedRecordEntry] = Field(
        min_length=1,
        description="Expected-record entries, one per file name.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of this policy.",
    )

    @model_validator(mode="after")
    def _validate_policy(self) -> Self:
        seen_files: set[str] = set()
        for entry in self.entries:
            if entry.file_name in seen_files:
                raise ValueError(f"duplicate file_name in policy: {entry.file_name!r}")
            seen_files.add(entry.file_name)
        expected = compute_expected_record_policy_hash(
            schema_version=self.schema_version,
            policy_id=self.policy_id,
            policy_version=self.policy_version,
            suite_id=self.suite_id,
            entries=self.entries,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"expected record policy content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self

    def get_entry(self, file_name: str) -> ExpectedRecordEntry | None:
        """Return the policy entry for a file name, or None if not in the policy."""
        for entry in self.entries:
            if entry.file_name == file_name:
                return entry
        return None

    def required_files(self) -> list[str]:
        """Return sorted list of file names with REQUIRED applicability."""
        return sorted(
            e.file_name for e in self.entries
            if e.applicability == RecordApplicability.REQUIRED
        )

    def not_applicable_files(self) -> list[str]:
        """Return sorted list of file names with NOT_APPLICABLE applicability."""
        return sorted(
            e.file_name for e in self.entries
            if e.applicability == RecordApplicability.NOT_APPLICABLE
        )

    def optional_files(self) -> list[str]:
        """Return sorted list of file names with OPTIONAL applicability."""
        return sorted(
            e.file_name for e in self.entries
            if e.applicability == RecordApplicability.OPTIONAL
        )


def compute_expected_record_policy_hash(
    *,
    schema_version: str,
    policy_id: str,
    policy_version: str,
    suite_id: str,
    entries: Sequence[ExpectedRecordEntry],
) -> str:
    """Compute the content hash for an expected-record policy without constructing the full model."""
    payload = json.dumps(
        {
            "schema_version": schema_version,
            "policy_id": policy_id,
            "policy_version": policy_version,
            "suite_id": suite_id,
            "entries": [
                entry.model_dump(mode="json")
                for entry in sorted(entries, key=lambda entry: entry.file_name)
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "EXPECTED_RECORD_POLICY_SCHEMA_VERSION",
    "CardinalityRule",
    "ExpectedRecordEntry",
    "ExpectedRecordPolicy",
    "RecordApplicability",
    "compute_expected_record_policy_hash",
]
