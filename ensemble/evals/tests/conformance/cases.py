# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Typed executable conformance case registry and shared helpers.

Each ``ConformanceCase`` builds a ``DeterministicGradingContext``, invokes
``grade_deterministically``, and declares the expected verification status,
failure substring, and denominator contribution. The per-suite conformance
test files parametrize over the subset of cases that belong to their graders.

The proportion-grader spec table (``_PROP_SPECS``) generates cases for the
23 graders that share the receipt-proportion grading pattern. The
strict-grader case builders (``_receipt_integrity_cases``, ``_canary_cases``,
etc.) generate cases for the 10 graders with unique grading logic.

Fixture builders are reused from ``test_authoritative_receipt_grader.py``
via rootdir-relative imports.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

from g8e_evals.grader_inventory import (
    GRADER_INVENTORY,
    ConformanceCaseCategory,
)
from g8e_evals.graders import (
    DeterministicGradingContext,
    UnsupportedGraderError,
    VerificationStatus,
)
from g8e_evals.schema import StateCollectionBoundary

from test_authoritative_receipt_grader import (
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
    _reliability_context,
    _replay_attempt_context,
    _revoked_credential_context,
    _secret_detection_context,
    _signed_field_tampering_context,
    _signer_defect_context,
    _stale_state_root_context,
    _token_persistence_failure_context,
    _token_store_persistence_context,
    _token_ttl_expiry_context,
    _tool_sequence_context,
    _unauthorized_mutation_context,
    _with_secret_detection_observation_update,
)

VERSION = "1.0.0"
UNSUPPORTED_VERSION = "2.0.0"
_OTHER_BOUNDARY = StateCollectionBoundary.GOVERNED_DOCUMENT_STORE


@dataclass(frozen=True)
class ConformanceCase:
    """One typed executable conformance case for a grader/category pair."""

    grader_id: str
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


def _obs_ctx(
    base: DeterministicGradingContext,
    obs_field: str,
    update: dict,
) -> DeterministicGradingContext:
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


def _duplicate_ctx(
    base: DeterministicGradingContext,
    obs_field: str,
) -> DeterministicGradingContext:
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


def _unverified_obs_ctx(
    base: DeterministicGradingContext,
    obs_field: str,
) -> DeterministicGradingContext:
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
    entry = GRADER_INVENTORY[(spec.grader_id, VERSION)]
    required = entry.required_categories()
    cases: list[ConformanceCase] = []
    gid = spec.grader_id

    def add(
        category: ConformanceCaseCategory,
        case_id: str,
        build: Callable[[], DeterministicGradingContext],
        *,
        status: VerificationStatus | None,
        value: float | None = None,
        failure: str | None = None,
        denom: int | None = None,
        raises: type[BaseException] | None = None,
    ) -> None:
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
        lambda: _obs_ctx(
            spec.base_context(), spec.obs_field, {"attempt_id": "wrong-attempt"}
        ),
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
            {"action_class": "WRONG_ACTION"},
        ),
    ),
]


def proportion_cases() -> list[ConformanceCase]:
    """Build conformance cases for every proportion grader spec."""
    cases: list[ConformanceCase] = []
    for spec in _PROP_SPECS:
        cases.extend(_prop_cases(spec))
    return cases


def case_id(case: ConformanceCase) -> str:
    """Generate a pytest parametrize ID for a conformance case."""
    return f"{case.grader_id}-{case.category.value}-{case.case_id}"
