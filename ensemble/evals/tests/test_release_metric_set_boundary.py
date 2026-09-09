# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 0.5 tests for the v2.1.8 release metric set and claim boundary.

Verifies that the release metric set is complete (every registered
metric with an authoritative producer is in the release set), the claim
boundary is explicit (unsupported claims are typed exclusions), and no
unsupported claim becomes an implied pass.
"""

from __future__ import annotations

import pytest

from g8e_evals.grader_inventory import GRADER_INVENTORY, ProducerPath
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.release_metric_set import (
    MetricDomain,
    RELEASE_METRIC_SET,
    ReleaseMetricSet,
)

_TELEMETRY_METRIC_IDS = frozenset({"stage_usage_reconciled"})


@pytest.mark.unit
def test_release_metric_set_covers_every_registered_metric_with_grader_ref():
    """Every metric in the registry that has a grader_ref (authoritative
    producer) must be in the release set."""
    release_ids = RELEASE_METRIC_SET.metric_ids
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        if definition.grader_ref is not None:
            key = (definition.metric_id, definition.metric_version)
            assert key in release_ids, (
                f"metric {definition.metric_id}@{definition.metric_version} "
                f"has a grader_ref but is not in the release set"
            )


@pytest.mark.unit
def test_release_metric_set_does_not_include_metrics_without_grader_ref():
    """Metrics without a grader_ref (no authoritative producer) must not
    be in the release set unless they have a partial-external producer or
    are telemetry metrics produced by the stage normalization pipeline."""
    release_ids = RELEASE_METRIC_SET.metric_ids
    partial_external_grader_ids = {
        entry.grader_id for entry in GRADER_INVENTORY.values()
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL
    }
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        if definition.grader_ref is None:
            key = (definition.metric_id, definition.metric_version)
            if key in release_ids:
                assert definition.metric_id in partial_external_grader_ids or definition.metric_id in _TELEMETRY_METRIC_IDS, (
                    f"metric {definition.metric_id}@{definition.metric_version} "
                    f"has no grader_ref, is not partial-external, and is not telemetry but is in the release set"
                )


@pytest.mark.unit
def test_every_release_metric_is_registered_in_the_registry():
    for entry in RELEASE_METRIC_SET.metrics:
        assert DEFAULT_METRIC_REGISTRY.is_registered(entry.metric_id, entry.metric_version), (
            f"release metric {entry.metric_id}@{entry.metric_version} is not registered"
        )


@pytest.mark.unit
def test_release_metric_set_has_at_least_one_metric_per_domain():
    domains_present = {m.domain for m in RELEASE_METRIC_SET.metrics}
    for domain in MetricDomain:
        assert domain in domains_present, (
            f"release metric set has no metrics in domain {domain}"
        )


@pytest.mark.unit
def test_release_metric_set_has_at_least_one_unsupported_claim():
    assert len(RELEASE_METRIC_SET.unsupported_claims) >= 1


@pytest.mark.unit
def test_unsupported_claims_are_unique():
    claims = [uc.claim for uc in RELEASE_METRIC_SET.unsupported_claims]
    assert len(claims) == len(set(claims)), "duplicate unsupported claims"


@pytest.mark.unit
def test_unsupported_claims_have_non_empty_reasons_and_milestones():
    for uc in RELEASE_METRIC_SET.unsupported_claims:
        assert uc.reason.strip(), (
            f"unsupported claim '{uc.claim}' has an empty reason"
        )
        assert uc.follow_on_milestone.strip(), (
            f"unsupported claim '{uc.claim}' has an empty follow-on milestone"
        )


@pytest.mark.unit
def test_reasoner_independence_is_unsupported():
    assert "reasoner_independence" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_heterogeneous_l2_reasoning_is_unsupported():
    assert "heterogeneous_l2_reasoning" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_independent_quorum_error_reduction_is_unsupported():
    assert "independent_quorum_error_reduction" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_certification_is_unsupported():
    assert "certification" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_legal_compliance_is_unsupported():
    assert "legal_compliance" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_recurring_operating_effectiveness_is_unsupported():
    assert "recurring_operating_effectiveness" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_human_semantic_grading_is_unsupported():
    assert "human_semantic_grading" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_complete_ifeval_import_is_unsupported():
    assert "complete_ifeval_import" in RELEASE_METRIC_SET.unsupported_claim_names


@pytest.mark.unit
def test_release_metric_set_is_frozen():
    with pytest.raises((TypeError, ValueError)):
        RELEASE_METRIC_SET.release_version = "v9.9.9"  # type: ignore[misc]


@pytest.mark.unit
def test_release_metric_set_rejects_extra_fields():
    with pytest.raises((TypeError, ValueError)):
        ReleaseMetricSet(
            release_version="v2.1.8",
            metrics=[],
            unsupported_claims=[],
            extra_field="not allowed",  # type: ignore[call-arg]
        )


@pytest.mark.unit
def test_every_release_metric_entry_has_a_domain():
    for entry in RELEASE_METRIC_SET.metrics:
        assert isinstance(entry.domain, MetricDomain)


@pytest.mark.unit
def test_every_release_metric_entry_has_a_threshold_description():
    for entry in RELEASE_METRIC_SET.metrics:
        assert entry.threshold_description.strip(), (
            f"metric {entry.metric_id} has an empty threshold description"
        )


@pytest.mark.unit
def test_release_metric_count_matches_registry():
    """The release set size must equal the number of registered metrics
    that have an authoritative producer (grader_ref or partial-external)."""
    partial_external_grader_ids = {
        entry.grader_id for entry in GRADER_INVENTORY.values()
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL
    }
    expected_count = sum(
        1 for d in DEFAULT_METRIC_REGISTRY.all_definitions()
        if d.grader_ref is not None or d.metric_id in partial_external_grader_ids or d.metric_id in _TELEMETRY_METRIC_IDS
    )
    assert len(RELEASE_METRIC_SET.metrics) == expected_count


@pytest.mark.unit
def test_release_metric_set_version_is_v218():
    assert RELEASE_METRIC_SET.release_version == "v2.1.8"
