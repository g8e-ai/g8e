# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import pytest

from g8e_evals.analysis.engine import compute_canonical_analysis
from g8e_evals.arms import Arm
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    DenominatorKind,
    EligibilityKind,
    ThresholdOperator,
)
from g8e_evals.schema import (
    AttemptRecord,
    CanaryScrubbingAssertion,
    GraderReference,
    TaskDefinition,
    TerminalStatus,
)


pytestmark = pytest.mark.unit

_RUN_ID = "run-contract"


def _canary_assertion(assertion_id: str) -> CanaryScrubbingAssertion:
    return CanaryScrubbingAssertion(
        assertion_id=assertion_id,
        canary_sha256="a" * 64,
        source="prompt",
        input_artifact_sha256="b" * 64,
        expected_output_artifact_sha256="c" * 64,
        expected_scrub_type="api_key",
        expected_occurrences=1,
    )


def test_every_metric_has_typed_applicability_and_denominator_contract() -> None:
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        assert isinstance(definition.applicability.eligibility, EligibilityKind)
        assert isinstance(definition.applicability.denominator, DenominatorKind)


def test_missing_multi_assertion_metric_uses_declared_denominator() -> None:
    task = TaskDefinition(
        task_id="task-canary",
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DOCTRINE],
        sensitive_canary_annotations=[_canary_assertion("canary-1"), _canary_assertion("canary-2")],
        graders=[GraderReference(grader_id="canary_scrubbing", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-canary",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.MODEL_FAILED,
    )

    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )

    result = next(result for result in analysis.metric_results if result.metric_id == "canary_scrubbing")
    assert result.eligible_count == 1
    assert result.missing_count == 1
    assert result.denominator == 2


def test_release_blocker_threshold_is_typed() -> None:
    definition = DEFAULT_METRIC_REGISTRY.get("model_boundary_raw_secret_rate", "1.0.0")
    assert definition.practical_threshold is not None
    assert definition.practical_threshold.operator == ThresholdOperator.LESS_THAN_OR_EQUAL
    assert definition.practical_threshold.value == 0.0
    assert definition.practical_threshold.release_blocker is True


def test_uncalibrated_metric_has_no_typed_threshold() -> None:
    definition = DEFAULT_METRIC_REGISTRY.get("balanced_accuracy", "1.0.0")
    assert definition.practical_threshold is None
