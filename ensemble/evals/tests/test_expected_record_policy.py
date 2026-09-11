# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the frozen expected-record policy.

Verifies that the ``ExpectedRecordPolicy`` typed model enforces
applicability, cardinality, and content-addressed identity for each
observation and event JSONL file. The policy distinguishes "zero
expected" from "file omitted" so the verifier can enforce required
files and reject fabricated files for inapplicable scenarios.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with invalid combinations to
# verify pydantic validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.expected_record_policy import (
    EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
    CardinalityRule,
    ExpectedRecordEntry,
    ExpectedRecordPolicy,
    RecordApplicability,
    compute_expected_record_policy_hash,
)


pytestmark = pytest.mark.unit


def _entry_dict(
    *,
    file_name: str = "resource-observations.jsonl",
    applicability: RecordApplicability = RecordApplicability.REQUIRED,
    cardinality_rule: CardinalityRule = CardinalityRule.ONE_PER_INFERENCE,
    expected_count: int | None = None,
    derivation_rule: str = "",
) -> dict:
    return {
        "file_name": file_name,
        "applicability": applicability.value,
        "cardinality_rule": cardinality_rule.value,
        "expected_count": expected_count,
        "derivation_rule": derivation_rule,
    }


def _policy_dict(
    *,
    policy_id: str = "policy-1",
    policy_version: str = "1.0.0",
    suite_id: str = "ifeval_subset",
    entries: list[dict] | None = None,
) -> dict:
    if entries is None:
        entries = [_entry_dict()]
    return {
        "schema_version": EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
        "policy_id": policy_id,
        "policy_version": policy_version,
        "suite_id": suite_id,
        "entries": entries,
        "content_hash": compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id=policy_id,
            policy_version=policy_version,
            suite_id=suite_id,
            entries=entries,
        ),
    }


class TestRecordApplicability:
    def test_has_three_values(self):
        assert len(list(RecordApplicability)) == 3
        values = {a.value for a in RecordApplicability}
        assert values == {"required", "not_applicable", "optional"}

    def test_required_value(self):
        assert RecordApplicability.REQUIRED.value == "required"

    def test_not_applicable_value(self):
        assert RecordApplicability.NOT_APPLICABLE.value == "not_applicable"

    def test_optional_value(self):
        assert RecordApplicability.OPTIONAL.value == "optional"


class TestCardinalityRule:
    def test_has_four_values(self):
        assert len(list(CardinalityRule)) == 4
        values = {c.value for c in CardinalityRule}
        assert values == {"exact", "one_per_inference", "one_per_attempt", "derived"}

    def test_exact_value(self):
        assert CardinalityRule.EXACT.value == "exact"

    def test_one_per_inference_value(self):
        assert CardinalityRule.ONE_PER_INFERENCE.value == "one_per_inference"

    def test_one_per_attempt_value(self):
        assert CardinalityRule.ONE_PER_ATTEMPT.value == "one_per_attempt"

    def test_derived_value(self):
        assert CardinalityRule.DERIVED.value == "derived"


class TestExpectedRecordEntry:
    def test_exact_requires_expected_count(self):
        with pytest.raises(ValidationError, match="expected_count must be set"):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.EXACT,
                expected_count=None,
            )

    def test_exact_with_expected_count_passes(self):
        entry = ExpectedRecordEntry(
            file_name="resource-observations.jsonl",
            applicability=RecordApplicability.REQUIRED,
            cardinality_rule=CardinalityRule.EXACT,
            expected_count=5,
        )
        assert entry.expected_count == 5

    def test_non_exact_rejects_expected_count(self):
        with pytest.raises(ValidationError, match="expected_count must be None"):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
                expected_count=5,
            )

    def test_derived_requires_derivation_rule(self):
        with pytest.raises(ValidationError, match="derivation_rule must be non-empty"):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.DERIVED,
                derivation_rule="",
            )

    def test_derived_with_rule_passes(self):
        entry = ExpectedRecordEntry(
            file_name="resource-observations.jsonl",
            applicability=RecordApplicability.REQUIRED,
            cardinality_rule=CardinalityRule.DERIVED,
            derivation_rule="count_inferences_from_event_trail",
        )
        assert entry.derivation_rule == "count_inferences_from_event_trail"

    def test_non_derived_rejects_derivation_rule(self):
        with pytest.raises(ValidationError, match="derivation_rule must be empty"):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
                derivation_rule="some_rule",
            )

    def test_not_applicable_requires_exact_zero(self):
        with pytest.raises(ValidationError, match="NOT_APPLICABLE"):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.NOT_APPLICABLE,
                cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
            )

    def test_not_applicable_with_exact_zero_passes(self):
        entry = ExpectedRecordEntry(
            file_name="resource-observations.jsonl",
            applicability=RecordApplicability.NOT_APPLICABLE,
            cardinality_rule=CardinalityRule.EXACT,
            expected_count=0,
        )
        assert entry.applicability == RecordApplicability.NOT_APPLICABLE
        assert entry.expected_count == 0

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.EXACT,
                expected_count=1,
                extra_field="bad",
            )

    def test_frozen_model(self):
        entry = ExpectedRecordEntry(
            file_name="resource-observations.jsonl",
            applicability=RecordApplicability.REQUIRED,
            cardinality_rule=CardinalityRule.EXACT,
            expected_count=1,
        )
        with pytest.raises((TypeError, ValueError)):
            entry.file_name = "changed"  # type: ignore[misc]

    def test_empty_file_name_rejected(self):
        with pytest.raises(ValidationError):
            ExpectedRecordEntry(
                file_name="",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.EXACT,
                expected_count=1,
            )

    def test_negative_expected_count_rejected(self):
        with pytest.raises(ValidationError):
            ExpectedRecordEntry(
                file_name="resource-observations.jsonl",
                applicability=RecordApplicability.REQUIRED,
                cardinality_rule=CardinalityRule.EXACT,
                expected_count=-1,
            )


class TestExpectedRecordPolicy:
    def test_valid_policy_passes(self):
        policy = ExpectedRecordPolicy(
            **_policy_dict(
                entries=[
                    _entry_dict(
                        file_name="resource-observations.jsonl",
                        applicability=RecordApplicability.REQUIRED,
                        cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
                    ),
                    _entry_dict(
                        file_name="tool-call-scorecards.jsonl",
                        applicability=RecordApplicability.NOT_APPLICABLE,
                        cardinality_rule=CardinalityRule.EXACT,
                        expected_count=0,
                    ),
                    _entry_dict(
                        file_name="escalation-records.jsonl",
                        applicability=RecordApplicability.OPTIONAL,
                        cardinality_rule=CardinalityRule.ONE_PER_ATTEMPT,
                    ),
                ],
            )
        )
        assert len(policy.entries) == 3

    def test_frozen_model(self):
        policy = ExpectedRecordPolicy(**_policy_dict())
        with pytest.raises((TypeError, ValueError)):
            policy.policy_id = "changed"  # type: ignore[misc]

    def test_rejects_unknown_fields(self):
        data = _policy_dict()
        data["extra_field"] = "bad"
        with pytest.raises(ValidationError):
            ExpectedRecordPolicy(**data)

    def test_duplicate_file_names_rejected(self):
        entries = [
            _entry_dict(file_name="resource-observations.jsonl"),
            _entry_dict(file_name="resource-observations.jsonl"),
        ]
        data = _policy_dict(entries=entries)
        with pytest.raises(ValidationError, match="duplicate file_name"):
            ExpectedRecordPolicy(**data)

    def test_empty_entries_rejected(self):
        data = _policy_dict(entries=[])
        with pytest.raises(ValidationError):
            ExpectedRecordPolicy(**data)

    def test_content_hash_mismatch_rejected(self):
        data = _policy_dict()
        data["content_hash"] = "b" * 64
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            ExpectedRecordPolicy(**data)

    def test_content_hash_is_deterministic(self):
        """The same policy inputs produce the same content hash."""
        entries = [_entry_dict()]
        hash1 = compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id="policy-1",
            policy_version="1.0.0",
            suite_id="ifeval_subset",
            entries=entries,
        )
        hash2 = compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id="policy-1",
            policy_version="1.0.0",
            suite_id="ifeval_subset",
            entries=entries,
        )
        assert hash1 == hash2

    def test_content_hash_changes_with_different_entries(self):
        """Different entries produce different content hashes."""
        entries1 = [_entry_dict(file_name="resource-observations.jsonl")]
        entries2 = [_entry_dict(file_name="tool-call-scorecards.jsonl")]
        hash1 = compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id="policy-1",
            policy_version="1.0.0",
            suite_id="ifeval_subset",
            entries=entries1,
        )
        hash2 = compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id="policy-1",
            policy_version="1.0.0",
            suite_id="ifeval_subset",
            entries=entries2,
        )
        assert hash1 != hash2

    def test_content_hash_independent_of_entry_order(self):
        """Entry order does not affect the content hash (entries are sorted by file_name)."""
        entries_a = [
            _entry_dict(file_name="resource-observations.jsonl"),
            _entry_dict(file_name="tool-call-scorecards.jsonl"),
        ]
        entries_b = [
            _entry_dict(file_name="tool-call-scorecards.jsonl"),
            _entry_dict(file_name="resource-observations.jsonl"),
        ]
        hash_a = compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id="policy-1",
            policy_version="1.0.0",
            suite_id="ifeval_subset",
            entries=entries_a,
        )
        hash_b = compute_expected_record_policy_hash(
            schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
            policy_id="policy-1",
            policy_version="1.0.0",
            suite_id="ifeval_subset",
            entries=entries_b,
        )
        assert hash_a == hash_b


class TestExpectedRecordPolicyQueries:
    def test_required_files(self):
        entries = [
            _entry_dict(file_name="resource-observations.jsonl", applicability=RecordApplicability.REQUIRED),
            _entry_dict(file_name="tool-call-scorecards.jsonl", applicability=RecordApplicability.NOT_APPLICABLE, cardinality_rule=CardinalityRule.EXACT, expected_count=0),
            _entry_dict(file_name="escalation-records.jsonl", applicability=RecordApplicability.OPTIONAL),
        ]
        policy = ExpectedRecordPolicy(**_policy_dict(entries=entries))
        assert policy.required_files() == ["resource-observations.jsonl"]

    def test_not_applicable_files(self):
        entries = [
            _entry_dict(file_name="resource-observations.jsonl", applicability=RecordApplicability.REQUIRED),
            _entry_dict(file_name="tool-call-scorecards.jsonl", applicability=RecordApplicability.NOT_APPLICABLE, cardinality_rule=CardinalityRule.EXACT, expected_count=0),
            _entry_dict(file_name="escalation-records.jsonl", applicability=RecordApplicability.OPTIONAL),
        ]
        policy = ExpectedRecordPolicy(**_policy_dict(entries=entries))
        assert policy.not_applicable_files() == ["tool-call-scorecards.jsonl"]

    def test_optional_files(self):
        entries = [
            _entry_dict(file_name="resource-observations.jsonl", applicability=RecordApplicability.REQUIRED),
            _entry_dict(file_name="tool-call-scorecards.jsonl", applicability=RecordApplicability.NOT_APPLICABLE, cardinality_rule=CardinalityRule.EXACT, expected_count=0),
            _entry_dict(file_name="escalation-records.jsonl", applicability=RecordApplicability.OPTIONAL),
        ]
        policy = ExpectedRecordPolicy(**_policy_dict(entries=entries))
        assert policy.optional_files() == ["escalation-records.jsonl"]

    def test_get_entry_returns_entry(self):
        entries = [
            _entry_dict(file_name="resource-observations.jsonl"),
        ]
        policy = ExpectedRecordPolicy(**_policy_dict(entries=entries))
        entry = policy.get_entry("resource-observations.jsonl")
        assert entry is not None
        assert entry.file_name == "resource-observations.jsonl"

    def test_get_entry_returns_none_for_unknown(self):
        policy = ExpectedRecordPolicy(**_policy_dict())
        assert policy.get_entry("nonexistent.jsonl") is None
