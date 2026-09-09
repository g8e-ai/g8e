# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Strict-grader conformance case builders for the 10 graders with unique logic.

Each builder returns a list of ``ConformanceCase`` records covering every
required category for its grader. Cases that fall under a typed exclusion in
the inventory are skipped automatically.
"""

from __future__ import annotations

from g8e_evals.grader_inventory import (
    GRADER_INVENTORY,
    ConformanceCaseCategory,
)
from g8e_evals.graders import (
    DeterministicGradingContext,
    UnsupportedGraderError,
    VerificationStatus,
)
from g8e_evals.schema import (
    FinalStateAssertion,
    FinalStateObservation,
    ModelBoundaryPrivacyAttestation,
    StateAssertionPredicate,
    StateEvidenceKind,
    StateValue,
)

from test_authoritative_receipt_grader import (
    DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
    Arm,
    PolicyOutcome,
    _canary_context,
    _canary_context_with_stage,
    _context,
    _independent_state_context,
    _model_boundary_context,
    _model_boundary_context_with_stage,
    _policy_context,
    _protocol_context,
    _receipt_integrity_context_with_stage,
    _rehydration_context,
    _secret_detection_context,
    _state_context,
    _with_rehydration_observation_update,
    _with_secret_detection_observation_update,
)

from .cases import (
    VERSION,
    ConformanceCase,
    _denominator_ctx,
)


def _make_add(gid: str, cases: list[ConformanceCase]):
    required = GRADER_INVENTORY[(gid, VERSION)].required_categories()

    def add(
        category: ConformanceCaseCategory,
        case_id: str,
        build,
        *,
        status,
        value=None,
        failure=None,
        denom=None,
        raises=None,
    ):
        if category not in required:
            return
        cases.append(
            ConformanceCase(
                grader_id=gid,
                category=category,
                case_id=case_id,
                build_context=build,
                expected_status=status,
                expected_value=value,
                expected_failure_contains=failure,
                expected_denominator=denom,
                raises=raises,
            )
        )

    return add


# ---------------------------------------------------------------------------
# receipt_integrity
# ---------------------------------------------------------------------------


def receipt_integrity_cases() -> list[ConformanceCase]:
    gid = "receipt_integrity"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "missing-persistence",
        lambda: _context(include_persistence=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="verified final-persistence evidence is missing",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-receipts",
        lambda: _context().model_copy(update={"receipts": []}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt",
        denom=0,
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-receipt",
        lambda: _context(verified=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt signature verification failed",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-persistence",
        lambda: _context().model_copy(
            update={
                "stages": [
                    _context().stages[0],
                    _context().stages[0].model_copy(update={"stage_id": "persistence-2"}),
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="verified final-persistence evidence is missing",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _receipt_integrity_context_with_stage(run_id="other-run"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="persistence stage run does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _receipt_integrity_context_with_stage(attempt_id="other-attempt"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="persistence stage attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _receipt_integrity_context_with_stage(task_id="other-task"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="persistence stage task does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ACTION_BINDING,
        "wrong-action",
        lambda: _context().model_copy(
            update={
                "receipts": [
                    _context().receipts[0].model_copy(update={"action_type": "EXECUTE_BASH"})
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt action does not match the expected action class",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _context,
        status=None,
        raises=UnsupportedGraderError,
    )
    return cases


# ---------------------------------------------------------------------------
# canary_scrubbing
# ---------------------------------------------------------------------------


def canary_scrubbing_cases() -> list[ConformanceCase]:
    gid = "canary_scrubbing"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    def _denominator():
        ctx = _canary_context()
        a1 = ctx.task.sensitive_canary_annotations[0]
        a2 = a1.model_copy(update={
            "assertion_id": "canary-2",
            "source": "conversation_history:assistant",
            "input_artifact_sha256": "d" * 64,
            "expected_output_artifact_sha256": "e" * 64,
            "expected_scrub_type": "api_key",
        })
        s1 = ctx.stages[0]
        s2 = s1.model_copy(update={
            "stage_id": "scrub-2",
            "source": "conversation_history:assistant",
            "input_artifact_hash": "d" * 64,
            "output_artifact_hash": "e" * 64,
            "scrub_types": ["api_key"],
        })
        task = ctx.task.model_copy(update={"sensitive_canary_annotations": [a1, a2]})
        return ctx.model_copy(update={"stages": [s1, s2], "task": task})

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _canary_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "hash-mismatch",
        lambda: _canary_context(output_hash="d" * 64),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="scrubbed output hash does not match",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-stage",
        lambda: _canary_context().model_copy(update={"stages": []}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one matching scrubbing stage is required",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "type-mismatch",
        lambda: _canary_context(scrub_types=["api_key"]),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="scrub types do not match",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-stage",
        lambda: _canary_context().model_copy(
            update={
                "stages": [
                    _canary_context().stages[0],
                    _canary_context().stages[0].model_copy(update={"stage_id": "scrub-2"}),
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one matching scrubbing stage is required",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _canary_context_with_stage(run_id="other-run"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="scrubbing stage run does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _canary_context_with_stage(attempt_id="other-attempt"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="scrubbing stage attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _canary_context_with_stage(task_id="other-task"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="scrubbing stage task does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _canary_context_with_stage(source="other-source"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one matching scrubbing stage is required",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _canary_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        _denominator,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


# ---------------------------------------------------------------------------
# model_boundary_raw_secret_rate
# ---------------------------------------------------------------------------


def _model_boundary_malformed() -> DeterministicGradingContext:
    """Construct an attestation where occurrences and types are inconsistent."""
    ctx = _model_boundary_context()
    stage = ctx.stages[0]
    attestation = stage.model_boundary_privacy
    assert attestation is not None
    malformed = attestation.model_copy(update={"raw_sensitive_types": []})
    new_stage = stage.model_copy(update={"model_boundary_privacy": malformed})
    return ctx.model_copy(update={"stages": [new_stage]})


def model_boundary_cases() -> list[ConformanceCase]:
    gid = "model_boundary_raw_secret_rate"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _model_boundary_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "raw-secret-leak",
        lambda: _model_boundary_context(raw_sensitive_occurrences=1),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="raw secret",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-attestation",
        lambda: _model_boundary_context(include_attestation=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary privacy attestation is missing",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "inconsistent-attestation",
        _model_boundary_malformed,
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary privacy attestation is inconsistent",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-stage",
        lambda: _model_boundary_context().model_copy(
            update={
                "stages": [
                    _model_boundary_context().stages[0],
                    _model_boundary_context().stages[0].model_copy(update={"stage_id": "model-call-2"}),
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary stages contain duplicate kinds",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _model_boundary_context_with_stage(run_id="other-run"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary stage run does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _model_boundary_context_with_stage(attempt_id="other-attempt"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary stage attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _model_boundary_context_with_stage(task_id="other-task"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary stage task does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _model_boundary_context(input_hash="b" * 64),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary privacy attestation payload hash does not match",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _model_boundary_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    return cases


# ---------------------------------------------------------------------------
# exact_local_rehydration
# ---------------------------------------------------------------------------


def rehydration_cases() -> list[ConformanceCase]:
    gid = "exact_local_rehydration"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _rehydration_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "verified-mismatch",
        lambda: _rehydration_context(restored_tokens=1),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="rehydration",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-observation",
        lambda: _rehydration_context(include_observation=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation is missing",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-observation",
        lambda: _with_rehydration_observation_update(
            _rehydration_context(), verification_status=VerificationStatus.FAILED
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation is unverified",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-observation",
        lambda: _rehydration_context(duplicate_observation=True),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one rehydration observation is required",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _rehydration_context(observation_run_id="other-run"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _rehydration_context(observation_attempt_id="other-attempt"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _with_rehydration_observation_update(_rehydration_context(), task_id="other-task"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
        "wrong-boundary",
        lambda: _with_rehydration_observation_update(
            _rehydration_context(), execution_boundary="remote_provider"
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration did not execute at the local runtime boundary",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _with_rehydration_observation_update(
            _rehydration_context(), source="other-source"
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation assertion binding does not match",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _rehydration_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        lambda: _denominator_ctx(
            _rehydration_context(), "rehydration_observations", "rehydration_assertions"
        ),
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


# ---------------------------------------------------------------------------
# secret_detection_precision / secret_detection_recall
# ---------------------------------------------------------------------------


def secret_detection_cases(grader_id: str) -> list[ConformanceCase]:
    cases: list[ConformanceCase] = []
    add = _make_add(grader_id, cases)

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _secret_detection_context,
        status=VerificationStatus.VERIFIED,
        denom=5,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "below-one",
        _secret_detection_context,
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="is below one",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-observation",
        lambda: _secret_detection_context(include_observation=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation is missing",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-observation",
        lambda: _with_secret_detection_observation_update(
            _secret_detection_context(), verification_status=VerificationStatus.FAILED
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation is unverified",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-observation",
        lambda: _secret_detection_context(duplicate_observation=True),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one secret-detection observation is required",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _secret_detection_context(observation_run_id="other-run"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _secret_detection_context(observation_attempt_id="other-attempt"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _with_secret_detection_observation_update(
            _secret_detection_context(), task_id="other-task"
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _with_secret_detection_observation_update(
            _secret_detection_context(), source="other-source"
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation assertion binding does not match",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _secret_detection_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        lambda: _secret_detection_context(),
        status=VerificationStatus.VERIFIED,
        denom=5,
    )
    if grader_id == "secret_detection_precision":
        add(
            ConformanceCaseCategory.NOT_APPLICABLE,
            "zero-denominator",
            lambda: _secret_detection_context(
                true_positives=0, false_positives=0, false_negatives=3, true_negatives=2
            ),
            status=VerificationStatus.NOT_APPLICABLE,
            value=0.0,
            denom=0,
            failure="denominator is zero",
        )
    return cases


# ---------------------------------------------------------------------------
# final_state_assertions
# ---------------------------------------------------------------------------


def final_state_cases() -> list[ConformanceCase]:
    gid = "final_state_assertions"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    def _denominator():
        ctx = _state_context()
        a1 = ctx.task.expected_final_state_assertions[0]
        a2 = FinalStateAssertion(
            assertion_id="secondary-state-root",
            predicate=StateAssertionPredicate.STATE_ROOT_CHANGED,
            action_type="FILE_EDIT",
        )
        o1 = ctx.final_state_observations[0]
        o2 = FinalStateObservation(
            observation_id="final-state-2",
            attempt_id=ctx.attempt.attempt_id,
            run_id=ctx.attempt.run_id,
            task_id=ctx.task.task_id,
            assertion_id=a2.assertion_id,
            action_type=a2.action_type,
            state_root_before="root-before",
            state_root_after="root-after",
            source_receipt_id="receipt-1",
            verification_status=VerificationStatus.VERIFIED,
        )
        task = ctx.task.model_copy(update={"expected_final_state_assertions": [a1, a2]})
        return ctx.model_copy(update={"final_state_observations": [o1, o2], "task": task})

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _state_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "observed-mismatch",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"state_root_after": "wrong-root"}
                    )
                ]
            }
        ),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-observation",
        lambda: _state_context().model_copy(update={"final_state_observations": []}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="final-state observation is missing",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-observation",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"verification_status": VerificationStatus.FAILED}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="final-state observation is not verified",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-observation",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0],
                    _state_context().final_state_observations[0].model_copy(),
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one final-state observation is required",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"run_id": "wrong-run"}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="final-state observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"attempt_id": "wrong-attempt"}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="final-state observation context does not match attempt",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"task_id": "wrong-task"}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="final-state observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ACTION_BINDING,
        "wrong-action",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"action_type": "EXECUTE_BASH"}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="final-state observation action does not match assertion",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(
                        update={"source_receipt_id": "nonexistent"}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="verified source receipt is missing",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _state_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        _denominator,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


# ---------------------------------------------------------------------------
# independent_state
# ---------------------------------------------------------------------------


def independent_state_cases() -> list[ConformanceCase]:
    gid = "independent_state"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    def _with_obs(update):
        ctx = _independent_state_context()
        return ctx.model_copy(
            update={"state_observations": [ctx.state_observations[0].model_copy(update=update)]}
        )

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _independent_state_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "mismatch",
        lambda: _with_obs({"observed": StateValue(kind=StateEvidenceKind.FILE, exists=False)}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation does not match",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-observation",
        lambda: _independent_state_context().model_copy(update={"state_observations": []}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one state observation is required",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-observation",
        lambda: _with_obs({"verification_status": VerificationStatus.FAILED}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation is not verified",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-observation",
        lambda: _independent_state_context().model_copy(
            update={
                "state_observations": [
                    _independent_state_context().state_observations[0],
                    _independent_state_context().state_observations[0].model_copy(),
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one state observation is required",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _with_obs({"run_id": "wrong-run"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _independent_state_context(attempt_id="wrong-attempt"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _with_obs({"task_id": "wrong-task"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
        "wrong-boundary",
        lambda: _with_obs(
            {"collection_boundary": StateCollectionBoundary.GOVERNED_DOCUMENT_STORE}
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation assertion binding does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _with_obs({"source_evidence_refs": []}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="state observation source evidence is missing",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _independent_state_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        lambda: _denominator_ctx(
            _independent_state_context(), "state_observations", "state_fixture"
        ),
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


# ---------------------------------------------------------------------------
# policy_outcome
# ---------------------------------------------------------------------------


def policy_outcome_cases() -> list[ConformanceCase]:
    gid = "policy_outcome"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    def _with_receipt(update):
        ctx = _policy_context(expected_outcome=PolicyOutcome.ALLOW)
        return ctx.model_copy(
            update={"receipts": [ctx.receipts[0].model_copy(update=update)]}
        )

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        lambda: _policy_context(expected_outcome=PolicyOutcome.ALLOW),
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "policy-mismatch",
        lambda: _policy_context(expected_outcome=PolicyOutcome.BLOCK),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="policy outcome does not match",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-receipt",
        lambda: _policy_context(expected_outcome=PolicyOutcome.ALLOW).model_copy(
            update={"receipts": []}
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-receipt",
        lambda: _policy_context(expected_outcome=PolicyOutcome.ALLOW, verified=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt signature verification failed",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-receipt",
        lambda: _policy_context(expected_outcome=PolicyOutcome.ALLOW).model_copy(
            update={
                "receipts": [
                    _policy_context(expected_outcome=PolicyOutcome.ALLOW).receipts[0],
                    _policy_context(expected_outcome=PolicyOutcome.ALLOW).receipts[0].model_copy(
                        update={"receipt_id": "receipt-2"}
                    ),
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="exactly one primary receipt is required",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _with_receipt({"run_id": "wrong-run"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt run does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _with_receipt({"attempt_id": "wrong-attempt"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ACTION_BINDING,
        "wrong-action",
        lambda: _with_receipt({"action_type": "EXECUTE_BASH"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt action does not match the expected action class",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        lambda: _policy_context(),
        status=None,
        raises=UnsupportedGraderError,
    )
    return cases


# ---------------------------------------------------------------------------
# protocol_chain
# ---------------------------------------------------------------------------


def _duplicate_protocol_stages() -> DeterministicGradingContext:
    ctx = _protocol_context()
    receipt = ctx.receipts[0].action_receipt
    stages = receipt.deterministic_stage_evidence
    stages[5].kind = DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE
    return ctx


def protocol_chain_cases() -> list[ConformanceCase]:
    gid = "protocol_chain"
    cases: list[ConformanceCase] = []
    add = _make_add(gid, cases)

    def _with_receipt(update):
        ctx = _protocol_context()
        return ctx.model_copy(
            update={"receipts": [ctx.receipts[0].model_copy(update=update)]}
        )

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        _protocol_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "posture-mismatch",
        lambda: _protocol_context().model_copy(
            update={"attempt": _protocol_context().attempt.model_copy(update={"arm_id": Arm.DOCTRINE})}
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="observed posture does not match",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-stage-kinds",
        _duplicate_protocol_stages,
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="deterministic stage kinds are invalid or duplicated",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _with_receipt({"run_id": "wrong-run"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt run does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _with_receipt({"attempt_id": "wrong-attempt"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt attempt does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_ACTION_BINDING,
        "wrong-action",
        lambda: _with_receipt({"action_type": "EXECUTE_BASH"}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="primary receipt action does not match the expected action class",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        _protocol_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    return cases
