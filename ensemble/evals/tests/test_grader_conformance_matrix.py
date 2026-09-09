# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 typed executable conformance matrix for every authoritative grader.

Replaces the former name-based conformance-coverage test with a typed
registry of executable ``ConformanceCase`` records. Each registered case
builds a ``DeterministicGradingContext``, invokes
``grade_deterministically``, and asserts the declared verification status,
failure substring, and denominator contribution. The coverage-summary
test derives required categories from ``GraderInventoryEntry.required_categories``
and fails closed on any missing pair that is not covered by a typed
exclusion. The no-extra-cases test rejects cases for excluded categories
or unknown graders.

Fixture builders are reused from ``test_authoritative_receipt_grader.py``
via rootdir-relative imports (the ``tests/`` package has no ``__init__``).
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

import pytest

from g8e_evals.grader_inventory import (
    GRADER_INVENTORY,
    ConformanceCaseCategory,
    ProducerPath,
)
from g8e_evals.graders import (
    DeterministicGradingContext,
    UnsupportedGraderError,
    VerificationStatus,
    grade_deterministically,
)
from g8e_evals.schema import StateCollectionBoundary

# Reuse the private fixture builders from the authoritative grader test.
# These builders construct valid and defective DeterministicGradingContext
# instances and are the established fixture pattern for this suite.
from test_authoritative_receipt_grader import (
    DeterministicStageKind,
    DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
    FinalStateAssertion,
    FinalStateObservation,
    PolicyOutcome,
    RejectionLayer,
    StageKind,
    StageObservation,
    StateAssertionPredicate,
    StateEvidenceKind,
    StateValue,
    TaskDefinition,
    AttemptRecord,
    Arm,
    GraderReference,
    _artifact_leakage_context,
    _canary_context,
    _canary_context_with_stage,
    _citation_backed_context,
    _context,
    _economics_context,
    _evidence_preservation_context,
    _exfiltration_attempt_context,
    _factual_qa_context,
    _identity_mismatch_context,
    _independent_state_context,
    _l3_proof_transplant_context,
    _model_boundary_context,
    _model_boundary_context_with_stage,
    _nonce_expiration_context,
    _partial_milestone_context,
    _payload_tampering_context,
    _policy_attack_context,
    _policy_context,
    _protocol_context,
    _receipt_integrity_context_with_stage,
    _rehydration_context,
    _reliability_context,
    _replay_attempt_context,
    _revoked_credential_context,
    _secret_detection_context,
    _signed_field_tampering_context,
    _signer_defect_context,
    _stale_state_root_context,
    _state_context,
    _token_persistence_failure_context,
    _token_store_persistence_context,
    _token_ttl_expiry_context,
    _tool_sequence_context,
    _unauthorized_mutation_context,
    _with_secret_detection_observation_update,
)

pytestmark = pytest.mark.unit

_VERSION = "1.0.0"
_UNSUPPORTED_VERSION = "2.0.0"
_OTHER_BOUNDARY = StateCollectionBoundary.GOVERNED_DOCUMENT_STORE


@dataclass(frozen=True)
class ConformanceCase:
    """One typed executable conformance case for a grader/category pair."""

    grader_id: str
    grader_version: str
    category: ConformanceCaseCategory
    case_id: str
    build_context: Callable[[], DeterministicGradingContext]
    expected_status: VerificationStatus | None
    expected_value: float | None
    expected_failure_contains: str | None
    expected_denominator: int | None
    raises: type[BaseException] | None


# ---------------------------------------------------------------------------
# Context mutation helpers (operate on a base passing context via model_copy)
# ---------------------------------------------------------------------------


def _obs_ctx(base: DeterministicGradingContext, obs_field: str, update: dict) -> DeterministicGradingContext:
    obs = getattr(base, obs_field)[0].model_copy(update=update)
    return base.model_copy(update={obs_field: [obs]})


def _assertions_ctx(
    base: DeterministicGradingContext,
    assertion_field: str,
    assertions: list,
) -> DeterministicGradingContext:
    task = base.task.model_copy(update={assertion_field: assertions})
    return base.model_copy(update={"task": task})


def _missing_assertions_ctx(
    base: DeterministicGradingContext,
    assertion_field: str,
) -> DeterministicGradingContext:
    return _assertions_ctx(base, assertion_field, [])


def _duplicate_ctx(base: DeterministicGradingContext, obs_field: str) -> DeterministicGradingContext:
    obs = getattr(base, obs_field)[0]
    return base.model_copy(update={obs_field: [obs, obs.model_copy()]})


def _denominator_ctx(
    base: DeterministicGradingContext,
    obs_field: str,
    assertion_field: str,
) -> DeterministicGradingContext:
    assertions = list(getattr(base.task, assertion_field))
    observations = list(getattr(base, obs_field))
    second_assertion = assertions[0].model_copy(update={"assertion_id": "denom-2"})
    second_obs = observations[0].model_copy(
        update={"assertion_id": "denom-2", "observation_id": "denom-obs-2"}
    )
    task = base.task.model_copy(update={assertion_field: assertions + [second_assertion]})
    return base.model_copy(update={obs_field: observations + [second_obs], "task": task})


def _unverified_obs_ctx(base: DeterministicGradingContext, obs_field: str) -> DeterministicGradingContext:
    return _obs_ctx(
        base,
        obs_field,
        {
            "verification_status": VerificationStatus.FAILED,
            "source_evidence_refs": [],
            "source_evidence_sha256": None,
        },
    )


# ---------------------------------------------------------------------------
# Proportion-grader spec and case generator
#
# Proportion graders return VERIFIED with value 0.0 and a "<prefix> assertion
# failed: <id>" failure for observation-level binding, source, boundary,
# duplicate, and measured-failure defects, and FAILED with "<prefix>
# assertions are missing" for missing assertions.
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class _PropSpec:
    grader_id: str
    base_context: Callable[[], DeterministicGradingContext]
    obs_field: str
    assertion_field: str
    measured_failure_context: Callable[[], DeterministicGradingContext]
    wrong_action_context: Callable[[], DeterministicGradingContext] | None = None
    not_applicable_context: Callable[[], DeterministicGradingContext] | None = None


def _prop_cases(spec: _PropSpec) -> list[ConformanceCase]:
    entry = GRADER_INVENTORY[(spec.grader_id, _VERSION)]
    required = entry.required_categories()
    cases: list[ConformanceCase] = []
    gid = spec.grader_id

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(
            ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises)
        )

    add(
        ConformanceCaseCategory.PASSING_EVIDENCE,
        "base",
        spec.base_context,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=1,
    )
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "measured",
        spec.measured_failure_context,
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-assertions",
        lambda: _missing_assertions_ctx(spec.base_context(), spec.assertion_field),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="assertions are missing",
        denom=0,
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverified-observation",
        lambda: _unverified_obs_ctx(spec.base_context(), spec.obs_field),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-observation",
        lambda: _duplicate_ctx(spec.base_context(), spec.obs_field),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.WRONG_RUN_BINDING,
        "wrong-run",
        lambda: _obs_ctx(spec.base_context(), spec.obs_field, {"run_id": "wrong-run"}),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
        "wrong-attempt",
        lambda: _obs_ctx(spec.base_context(), spec.obs_field, {"attempt_id": "wrong-attempt"}),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.WRONG_TASK_BINDING,
        "wrong-task",
        lambda: _obs_ctx(spec.base_context(), spec.obs_field, {"task_id": "wrong-task"}),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
        "wrong-boundary",
        lambda: _obs_ctx(
            spec.base_context(), spec.obs_field, {"collection_boundary": _OTHER_BOUNDARY}
        ),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _obs_ctx(
            spec.base_context(),
            spec.obs_field,
            {"source_evidence_refs": [], "source_evidence_sha256": None},
        ),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="assertion failed",
    )
    add(
        ConformanceCaseCategory.UNSUPPORTED_VERSION,
        "unsupported",
        spec.base_context,
        status=None,
        raises=UnsupportedGraderError,
    )
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        lambda: _denominator_ctx(spec.base_context(), spec.obs_field, spec.assertion_field),
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    if spec.wrong_action_context is not None:
        add(
            ConformanceCaseCategory.WRONG_ACTION_BINDING,
            "wrong-action",
            spec.wrong_action_context,
            status=VerificationStatus.VERIFIED,
            value=0.0,
            failure="assertion failed",
        )
    if spec.not_applicable_context is not None:
        add(
            ConformanceCaseCategory.NOT_APPLICABLE,
            "not-applicable",
            spec.not_applicable_context,
            status=VerificationStatus.NOT_APPLICABLE,
            value=0.0,
            denom=0,
            failure="denominator is zero",
        )
    return cases


# ---------------------------------------------------------------------------
# Proportion grader specs
# ---------------------------------------------------------------------------

_PROP_SPECS: list[_PropSpec] = [
    _PropSpec(
        grader_id="unauthorized_mutation",
        base_context=_unauthorized_mutation_context,
        obs_field="unauthorized_mutation_observations",
        assertion_field="unauthorized_mutation_assertions",
        measured_failure_context=lambda: _unauthorized_mutation_context(failed_layer=None),
        wrong_action_context=lambda: _unauthorized_mutation_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="exfiltration_attempt",
        base_context=_exfiltration_attempt_context,
        obs_field="exfiltration_attempt_observations",
        assertion_field="exfiltration_attempt_assertions",
        measured_failure_context=lambda: _exfiltration_attempt_context(failed_layer=None),
        wrong_action_context=lambda: _exfiltration_attempt_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="replay_attempt",
        base_context=_replay_attempt_context,
        obs_field="replay_attempt_observations",
        assertion_field="replay_attempt_assertions",
        measured_failure_context=lambda: _replay_attempt_context(failed_layer=None),
        wrong_action_context=lambda: _replay_attempt_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="signed_field_tampering",
        base_context=_signed_field_tampering_context,
        obs_field="signed_field_tampering_observations",
        assertion_field="signed_field_tampering_assertions",
        measured_failure_context=lambda: _signed_field_tampering_context(failed_layer=None),
        wrong_action_context=lambda: _signed_field_tampering_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="payload_tampering",
        base_context=_payload_tampering_context,
        obs_field="payload_tampering_observations",
        assertion_field="payload_tampering_assertions",
        measured_failure_context=lambda: _payload_tampering_context(failed_layer=None),
        wrong_action_context=lambda: _payload_tampering_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="stale_state_root",
        base_context=_stale_state_root_context,
        obs_field="stale_state_root_observations",
        assertion_field="stale_state_root_assertions",
        measured_failure_context=lambda: _stale_state_root_context(failed_layer=None),
        wrong_action_context=lambda: _stale_state_root_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="identity_mismatch",
        base_context=_identity_mismatch_context,
        obs_field="identity_mismatch_observations",
        assertion_field="identity_mismatch_assertions",
        measured_failure_context=lambda: _identity_mismatch_context(failed_layer=None),
        wrong_action_context=lambda: _identity_mismatch_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="nonce_expiration",
        base_context=_nonce_expiration_context,
        obs_field="nonce_expiration_observations",
        assertion_field="nonce_expiration_assertions",
        measured_failure_context=lambda: _nonce_expiration_context(failed_layer=None),
        wrong_action_context=lambda: _nonce_expiration_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="signer_defect",
        base_context=_signer_defect_context,
        obs_field="signer_defect_observations",
        assertion_field="signer_defect_assertions",
        measured_failure_context=lambda: _signer_defect_context(failed_layer=None),
        wrong_action_context=lambda: _signer_defect_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="l3_proof_transplant",
        base_context=_l3_proof_transplant_context,
        obs_field="l3_proof_transplant_observations",
        assertion_field="l3_proof_transplant_assertions",
        measured_failure_context=lambda: _l3_proof_transplant_context(failed_layer=None),
        wrong_action_context=lambda: _l3_proof_transplant_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="revoked_credential",
        base_context=_revoked_credential_context,
        obs_field="revoked_credential_observations",
        assertion_field="revoked_credential_assertions",
        measured_failure_context=lambda: _revoked_credential_context(failed_layer=None),
        wrong_action_context=lambda: _revoked_credential_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="policy_attack",
        base_context=_policy_attack_context,
        obs_field="policy_attack_observations",
        assertion_field="policy_attack_assertions",
        measured_failure_context=lambda: _policy_attack_context(failed_layer=None),
        wrong_action_context=lambda: _policy_attack_context(action_type="FILE_DELETE"),
    ),
    _PropSpec(
        grader_id="token_store_persistence",
        base_context=_token_store_persistence_context,
        obs_field="token_store_persistence_observations",
        assertion_field="token_store_persistence_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _token_store_persistence_context(),
            "token_store_persistence_observations",
            {"plaintext_in_store": True},
        ),
    ),
    _PropSpec(
        grader_id="token_ttl_expiry",
        base_context=_token_ttl_expiry_context,
        obs_field="token_ttl_expiry_observations",
        assertion_field="token_ttl_expiry_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _token_ttl_expiry_context(),
            "token_ttl_expiry_observations",
            {"token_visible_before_expiry": False},
        ),
    ),
    _PropSpec(
        grader_id="token_persistence_failure",
        base_context=_token_persistence_failure_context,
        obs_field="token_persistence_failure_observations",
        assertion_field="token_persistence_failure_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _token_persistence_failure_context(),
            "token_persistence_failure_observations",
            {"operation_refused": False},
        ),
    ),
    _PropSpec(
        grader_id="artifact_leakage",
        base_context=_artifact_leakage_context,
        obs_field="artifact_leakage_observations",
        assertion_field="artifact_leakage_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _artifact_leakage_context(),
            "artifact_leakage_observations",
            {"artifact_present": False, "artifact_sha256": None, "artifact_byte_length": 0},
        ),
    ),
    _PropSpec(
        grader_id="evidence_preservation",
        base_context=_evidence_preservation_context,
        obs_field="evidence_preservation_observations",
        assertion_field="evidence_preservation_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _evidence_preservation_context(),
            "evidence_preservation_observations",
            {"operation_refused": False},
        ),
    ),
    _PropSpec(
        grader_id="tool_sequence",
        base_context=_tool_sequence_context,
        obs_field="tool_sequence_observations",
        assertion_field="tool_sequence_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _tool_sequence_context(),
            "tool_sequence_observations",
            {"observed_sequence": ["search", "write", "summarize"]},
        ),
    ),
    _PropSpec(
        grader_id="factual_qa",
        base_context=_factual_qa_context,
        obs_field="factual_qa_observations",
        assertion_field="factual_qa_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _factual_qa_context(),
            "factual_qa_observations",
            {"observed_answer": "London"},
        ),
    ),
    _PropSpec(
        grader_id="citation_backed",
        base_context=_citation_backed_context,
        obs_field="citation_backed_observations",
        assertion_field="citation_backed_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _citation_backed_context(),
            "citation_backed_observations",
            {"observed_citation": "doi:10.1103/PhysRevLett.30.999"},
        ),
    ),
    _PropSpec(
        grader_id="partial_milestone",
        base_context=_partial_milestone_context,
        obs_field="partial_milestone_observations",
        assertion_field="partial_milestone_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _partial_milestone_context(),
            "partial_milestone_observations",
            {"milestone_reached": False, "observed_order": None},
        ),
    ),
    _PropSpec(
        grader_id="reliability",
        base_context=_reliability_context,
        obs_field="reliability_observations",
        assertion_field="reliability_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _reliability_context(),
            "reliability_observations",
            {"evidence_preserved": False},
        ),
        wrong_action_context=lambda: _obs_ctx(
            _reliability_context(),
            "reliability_observations",
            {"action_type": "WRONG_ACTION"},
        ),
    ),
    _PropSpec(
        grader_id="economics_performance",
        base_context=_economics_context,
        obs_field="economics_performance_observations",
        assertion_field="economics_performance_assertions",
        measured_failure_context=lambda: _obs_ctx(
            _economics_context(),
            "economics_performance_observations",
            {"observed_value": 0.01},
        ),
        wrong_action_context=lambda: _obs_ctx(
            _economics_context(),
            "economics_performance_observations",
            {"action_type": "WRONG_ACTION"},
        ),
    ),
]


# ---------------------------------------------------------------------------
# Strict grader cases (binding mismatches return FAILED)
# ---------------------------------------------------------------------------


def _receipt_integrity_cases() -> list[ConformanceCase]:
    gid = "receipt_integrity"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
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
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _context, status=None, raises=UnsupportedGraderError)
    return cases


def _canary_cases() -> list[ConformanceCase]:
    gid = "canary_scrubbing"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

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

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _canary_context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
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
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _canary_context, status=None, raises=UnsupportedGraderError)
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        _denominator,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


def _model_boundary_cases() -> list[ConformanceCase]:
    gid = "model_boundary_raw_secret_rate"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _model_boundary_context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "raw-secret-leak",
        lambda: _model_boundary_context(raw_secret_leaked=True),
        status=VerificationStatus.VERIFIED,
        value=0.0,
        failure="raw secret",
    )
    add(
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-attestation",
        lambda: _model_boundary_context().model_copy(update={"stages": []}),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "unverifiable-attestation",
        lambda: _model_boundary_context(attestation_verified=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary",
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
        failure="model-boundary",
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
        lambda: _model_boundary_context_with_stage(source="other-source"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="model-boundary stage source does not match",
    )
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _model_boundary_context, status=None, raises=UnsupportedGraderError)
    return cases


def _rehydration_cases() -> list[ConformanceCase]:
    gid = "exact_local_rehydration"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _rehydration_context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
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
        lambda: _rehydration_context(observation_verified=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation is not verified",
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
        lambda: _with_rehydration_observation_update(_rehydration_context(), execution_boundary="remote_provider"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration did not execute at the local runtime boundary",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _with_rehydration_observation_update(
            _rehydration_context(), source_evidence_refs=[], source_evidence_sha256=None
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="rehydration observation source evidence is missing",
    )
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _rehydration_context, status=None, raises=UnsupportedGraderError)
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        lambda: _denominator_ctx(_rehydration_context(), "rehydration_observations", "rehydration_assertions"),
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


def _secret_detection_cases(grader_id: str) -> list[ConformanceCase]:
    entry = GRADER_INVENTORY[(grader_id, _VERSION)]
    required = entry.required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(grader_id, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _secret_detection_context, status=VerificationStatus.VERIFIED, denom=5)
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
        lambda: _secret_detection_context(observation_verified=False),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation is not verified",
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
        lambda: _with_secret_detection_observation_update(_secret_detection_context(), task_id="other-task"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation context does not match",
    )
    add(
        ConformanceCaseCategory.WRONG_SOURCE_BINDING,
        "wrong-source",
        lambda: _with_secret_detection_observation_update(_secret_detection_context(), source="other-source"),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="secret-detection observation assertion binding does not match",
    )
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _secret_detection_context, status=None, raises=UnsupportedGraderError)
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
            lambda: _secret_detection_context(true_positives=0, false_positives=0, false_negatives=3, true_negatives=2),
            status=VerificationStatus.NOT_APPLICABLE,
            value=0.0,
            denom=0,
            failure="denominator is zero",
        )
    return cases


def _final_state_cases() -> list[ConformanceCase]:
    gid = "final_state_assertions"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

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

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _state_context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
    add(
        ConformanceCaseCategory.MEASURED_FAILURE,
        "observed-mismatch",
        lambda: _state_context().model_copy(
            update={
                "final_state_observations": [
                    _state_context().final_state_observations[0].model_copy(update={"state_root_after": "wrong-root"})
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
                    _state_context().final_state_observations[0].model_copy(update={"run_id": "wrong-run"})
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
                    _state_context().final_state_observations[0].model_copy(update={"attempt_id": "wrong-attempt"})
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
                    _state_context().final_state_observations[0].model_copy(update={"task_id": "wrong-task"})
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
                    _state_context().final_state_observations[0].model_copy(update={"action_type": "EXECUTE_BASH"})
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
                    _state_context().final_state_observations[0].model_copy(update={"source_receipt_id": "nonexistent"})
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="verified source receipt is missing",
    )
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _state_context, status=None, raises=UnsupportedGraderError)
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        _denominator,
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


def _independent_state_cases() -> list[ConformanceCase]:
    gid = "independent_state"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    def _with_obs(update):
        ctx = _independent_state_context()
        return ctx.model_copy(
            update={"state_observations": [ctx.state_observations[0].model_copy(update=update)]}
        )

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _independent_state_context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
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
        lambda: _with_obs({"collection_boundary": _OTHER_BOUNDARY}),
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
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _independent_state_context, status=None, raises=UnsupportedGraderError)
    add(
        ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
        "multiple-assertions",
        lambda: _denominator_ctx(_independent_state_context(), "state_observations", "state_fixture"),
        status=VerificationStatus.VERIFIED,
        value=1.0,
        denom=2,
    )
    return cases


def _policy_outcome_cases() -> list[ConformanceCase]:
    gid = "policy_outcome"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    def _with_receipt(update):
        ctx = _policy_context(expected_outcome=PolicyOutcome.ALLOW)
        return ctx.model_copy(
            update={"receipts": [ctx.receipts[0].model_copy(update=update)]}
        )

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", lambda: _policy_context(expected_outcome=PolicyOutcome.ALLOW), status=VerificationStatus.VERIFIED, value=1.0, denom=1)
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
        lambda: _policy_context(expected_outcome=PolicyOutcome.ALLOW).model_copy(update={"receipts": []}),
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
                    _policy_context(expected_outcome=PolicyOutcome.ALLOW).receipts[0].model_copy(update={"receipt_id": "receipt-2"}),
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
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", lambda: _policy_context(), status=None, raises=UnsupportedGraderError)
    return cases


def _protocol_chain_cases() -> list[ConformanceCase]:
    gid = "protocol_chain"
    required = GRADER_INVENTORY[(gid, _VERSION)].required_categories()
    cases: list[ConformanceCase] = []

    def add(category, case_id, build, *, status, value=None, failure=None, denom=None, raises=None):
        if category not in required:
            return
        cases.append(ConformanceCase(gid, _VERSION, category, case_id, build, status, value, failure, denom, raises))

    def _with_receipt(update):
        ctx = _protocol_context()
        return ctx.model_copy(
            update={"receipts": [ctx.receipts[0].model_copy(update=update)]}
        )

    add(ConformanceCaseCategory.PASSING_EVIDENCE, "base", _protocol_context, status=VerificationStatus.VERIFIED, value=1.0, denom=1)
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
        ConformanceCaseCategory.MISSING_EVIDENCE,
        "missing-stage-evidence",
        lambda: _protocol_context().model_copy(
            update={
                "receipts": [
                    _protocol_context().receipts[0].model_copy(
                        update={"action_receipt": _protocol_context().receipts[0].action_receipt}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="deterministic stage evidence is missing",
    )
    add(
        ConformanceCaseCategory.MALFORMED_EVIDENCE,
        "malformed-stage-order",
        lambda: _protocol_context().model_copy(
            update={
                "receipts": [
                    _protocol_context().receipts[0].model_copy(
                        update={"action_receipt": _protocol_context().receipts[0].action_receipt}
                    )
                ]
            }
        ),
        status=VerificationStatus.FAILED,
        value=0.0,
        failure="deterministic stage",
    )
    add(
        ConformanceCaseCategory.DUPLICATE_EVIDENCE,
        "duplicate-stage-kinds",
        lambda: _duplicate_protocol_stages(),
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
    add(ConformanceCaseCategory.UNSUPPORTED_VERSION, "unsupported", _protocol_context, status=None, raises=UnsupportedGraderError)
    return cases


def _duplicate_protocol_stages() -> DeterministicGradingContext:
    ctx = _protocol_context()
    receipt = ctx.receipts[0].action_receipt
    stages = receipt.deterministic_stage_evidence
    stages[5].kind = DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE
    return ctx


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------


def _build_all_cases() -> list[ConformanceCase]:
    cases: list[ConformanceCase] = []
    for spec in _PROP_SPECS:
        cases.extend(_prop_cases(spec))
    cases.extend(_receipt_integrity_cases())
    cases.extend(_canary_cases())
    cases.extend(_model_boundary_cases())
    cases.extend(_rehydration_cases())
    cases.extend(_secret_detection_cases("secret_detection_precision"))
    cases.extend(_secret_detection_cases("secret_detection_recall"))
    cases.extend(_final_state_cases())
    cases.extend(_independent_state_cases())
    cases.extend(_policy_outcome_cases())
    cases.extend(_protocol_chain_cases())
    return cases


_CONFORMANCE_CASES: list[ConformanceCase] = _build_all_cases()


def _case_id(case: ConformanceCase) -> str:
    return f"{case.grader_id}-{case.category.value}-{case.case_id}"


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("case", _CONFORMANCE_CASES, ids=_case_id)
def test_grader_conformance_case(case: ConformanceCase) -> None:
    if case.raises is not None:
        with pytest.raises(case.raises):
            grade_deterministically(case.grader_id, case.grader_version, case.build_context())
        return

    result = grade_deterministically(case.grader_id, case.grader_version, case.build_context())

    assert result.verification_status == case.expected_status, (
        f"{case.grader_id}/{case.category.value}: status {result.verification_status} != {case.expected_status}"
    )
    if case.expected_value is not None:
        assert result.value == case.expected_value, (
            f"{case.grader_id}/{case.category.value}: value {result.value} != {case.expected_value}"
        )
    if case.expected_failure_contains is not None:
        assert result.failure is not None
        assert case.expected_failure_contains in result.failure, (
            f"{case.grader_id}/{case.category.value}: {case.expected_failure_contains!r} not in {result.failure!r}"
        )
    elif case.expected_status != VerificationStatus.VERIFIED or case.expected_value == 0.0:
        assert result.failure is not None, (
            f"{case.grader_id}/{case.category.value}: expected a failure message"
        )
    if case.expected_denominator is not None:
        assert result.denominator_contribution == case.expected_denominator, (
            f"{case.grader_id}/{case.category.value}: denominator {result.denominator_contribution} != {case.expected_denominator}"
        )


def test_conformance_matrix_covers_every_required_category() -> None:
    covered: set[tuple[str, ConformanceCaseCategory]] = {
        (c.grader_id, c.category) for c in _CONFORMANCE_CASES
    }
    gaps: list[str] = []
    for entry in GRADER_INVENTORY.values():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        for category in entry.required_categories():
            if (entry.grader_id, category) not in covered:
                gaps.append(f"{entry.grader_id}:{category.value}")
    assert not gaps, f"missing conformance cases for required (grader, category) pairs:\n{gaps}"


def test_no_conformance_case_for_excluded_or_unknown_category() -> None:
    for case in _CONFORMANCE_CASES:
        entry = GRADER_INVENTORY.get((case.grader_id, _VERSION))
        assert entry is not None, f"case references unknown grader: {case.grader_id}"
        assert entry.producer_path != ProducerPath.PARTIAL_EXTERNAL, (
            f"case registered for partial-external grader: {case.grader_id}"
        )
        assert not entry.has_exclusion(case.category), (
            f"case registered for excluded category {case.grader_id}:{case.category.value}"
        )


def test_conformance_matrix_summary() -> None:
    """Print a coverage summary and fail if any required pair is uncovered."""
    covered: set[tuple[str, ConformanceCaseCategory]] = {
        (c.grader_id, c.category) for c in _CONFORMANCE_CASES
    }
    lines: list[str] = []
    gap_count = 0
    for entry in GRADER_INVENTORY.values():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        required = entry.required_categories()
        missing = sorted(c.value for c in required if (entry.grader_id, c) not in covered)
        if missing:
            gap_count += len(missing)
            lines.append(f"  {entry.grader_id}: missing {missing}")
    summary = (
        f"conformance matrix: {len(_CONFORMANCE_CASES)} cases, "
        f"{len(GRADER_INVENTORY)} graders, {gap_count} gaps"
    )
    assert gap_count == 0, summary + "\n" + "\n".join(lines)
