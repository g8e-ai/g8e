# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Computation engine for the canonical eval analysis.

Builds a ``CanonicalEvalAnalysis`` from immutable task, attempt,
observation, receipt, stage, and ``MetricObservation`` records. The
computation is deterministic: identical inputs and analysis version
produce byte-identical output.

The engine does not trust any descriptive aggregate or legacy summary.
It reads only immutable records and computes every aggregate, confusion
matrix, comparison, and gate decision from first principles.
"""

from __future__ import annotations

import hashlib
from collections import defaultdict
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Protocol

from pydantic import BaseModel

from g8e_evals.analysis import statistics
from g8e_evals.analysis.canonical import (
    ANALYSIS_COMPUTATION_VERSION,
    ANALYSIS_SCHEMA_VERSION,
    AnalysisInputSummary,
    BridgeRunComparison,
    CanonicalEvalAnalysis,
    ComparisonDirection,
    ConfusionMatrix,
    DomainStratifiedResult,
    GateDecision,
    GateDecisionStatus,
    MetricAnalysisResult,
    MissingnessBreakdown,
    PairedComparison,
    PooledConfusionMatrix,
    ReceiptCoverageAnalysis,
    canonical_model_json,
)
from g8e_evals.analysis.derived import run_all_derived_producers
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.analysis.telemetry import run_all_telemetry_producers
from g8e_evals.arms import ARM_DEFINITIONS
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    AggregationMethod,
    ArmRequirement,
    DenominatorKind,
    EligibilityKind,
    MetricDefinition,
    MetricDirection,
    ThresholdOperator,
)
from g8e_evals.release_metric_set import (
    RELEASE_METRIC_SET,
    MetricDomain,
    ReleaseMetricEntry,
)
from g8e_evals.schema import (
    AttemptRecord,
    MetricObservation,
    ReceiptObservation,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
)


_FLOAT_PRECISION = 10


class _BoundRecord(Protocol):
    """Structural type for records bound to an attempt by identity fields."""

    attempt_id: str
    run_id: str
    task_id: str


@dataclass(frozen=True)
class _BoundRecordSpec:
    """Specification for a bound record sequence on ``AnalysisInputRecord``.

    Each bound record sequence contains records that carry ``attempt_id``,
    ``run_id``, and ``task_id`` fields binding them to an attempt. The
    identity field name varies by record class (``observation_id``,
    ``envelope_id``, ``attestation_id``, ``audit_link_id``).
    """

    field_name: str
    id_field: str
    type_name: str
    hash_prefix: str


_BOUND_RECORD_SPECS: list[_BoundRecordSpec] = [
    _BoundRecordSpec("final_state_observations", "observation_id", "final_state_observation", "final_state_observation"),
    _BoundRecordSpec("state_observations", "observation_id", "state_observation", "state_observation"),
    _BoundRecordSpec("rehydration_observations", "observation_id", "rehydration_observation", "rehydration_observation"),
    _BoundRecordSpec("secret_detection_observations", "observation_id", "secret_detection_observation", "secret_detection_observation"),
    _BoundRecordSpec("unauthorized_mutation_observations", "observation_id", "unauthorized_mutation_observation", "unauthorized_mutation_observation"),
    _BoundRecordSpec("token_store_persistence_observations", "observation_id", "token_store_persistence_observation", "token_store_persistence_observation"),
    _BoundRecordSpec("token_ttl_expiry_observations", "observation_id", "token_ttl_expiry_observation", "token_ttl_expiry_observation"),
    _BoundRecordSpec("token_persistence_failure_observations", "observation_id", "token_persistence_failure_observation", "token_persistence_failure_observation"),
    _BoundRecordSpec("exfiltration_attempt_observations", "observation_id", "exfiltration_attempt_observation", "exfiltration_attempt_observation"),
    _BoundRecordSpec("artifact_leakage_observations", "observation_id", "artifact_leakage_observation", "artifact_leakage_observation"),
    _BoundRecordSpec("replay_attempt_observations", "observation_id", "replay_attempt_observation", "replay_attempt_observation"),
    _BoundRecordSpec("signed_field_tampering_observations", "observation_id", "signed_field_tampering_observation", "signed_field_tampering_observation"),
    _BoundRecordSpec("payload_tampering_observations", "observation_id", "payload_tampering_observation", "payload_tampering_observation"),
    _BoundRecordSpec("stale_state_root_observations", "observation_id", "stale_state_root_observation", "stale_state_root_observation"),
    _BoundRecordSpec("identity_mismatch_observations", "observation_id", "identity_mismatch_observation", "identity_mismatch_observation"),
    _BoundRecordSpec("nonce_expiration_observations", "observation_id", "nonce_expiration_observation", "nonce_expiration_observation"),
    _BoundRecordSpec("signer_defect_observations", "observation_id", "signer_defect_observation", "signer_defect_observation"),
    _BoundRecordSpec("l3_proof_transplant_observations", "observation_id", "l3_proof_transplant_observation", "l3_proof_transplant_observation"),
    _BoundRecordSpec("revoked_credential_observations", "observation_id", "revoked_credential_observation", "revoked_credential_observation"),
    _BoundRecordSpec("evidence_preservation_observations", "observation_id", "evidence_preservation_observation", "evidence_preservation_observation"),
    _BoundRecordSpec("policy_attack_observations", "observation_id", "policy_attack_observation", "policy_attack_observation"),
    _BoundRecordSpec("tool_sequence_observations", "observation_id", "tool_sequence_observation", "tool_sequence_observation"),
    _BoundRecordSpec("factual_qa_observations", "observation_id", "factual_qa_observation", "factual_qa_observation"),
    _BoundRecordSpec("citation_backed_observations", "observation_id", "citation_backed_observation", "citation_backed_observation"),
    _BoundRecordSpec("partial_milestone_observations", "observation_id", "partial_milestone_observation", "partial_milestone_observation"),
    _BoundRecordSpec("reliability_observations", "observation_id", "reliability_observation", "reliability_observation"),
    _BoundRecordSpec("economics_performance_observations", "observation_id", "economics_performance_observation", "economics_performance_observation"),
    _BoundRecordSpec("local_resource_observations", "observation_id", "local_resource_observation", "local_resource_observation"),
    _BoundRecordSpec("human_wait_observations", "observation_id", "human_wait_observation", "human_wait_observation"),
    _BoundRecordSpec("governance_envelopes", "envelope_id", "governance_envelope", "governance_envelope"),
    _BoundRecordSpec("persistence_attestations", "attestation_id", "persistence_attestation", "persistence_attestation"),
    _BoundRecordSpec("commitment_attestations", "attestation_id", "commitment_attestation", "commitment_attestation"),
    _BoundRecordSpec("audit_links", "audit_link_id", "audit_link", "audit_link"),
]


@dataclass(frozen=True)
class _RawComparison:
    """Intermediate comparison record before Holm correction."""

    metric_id: str
    metric_version: str
    baseline_arm_id: str
    comparison_arm_id: str
    paired_count: int
    baseline_value: float
    comparison_value: float
    absolute_delta: float
    relative_delta: float | None
    standardized_effect_size: float | None
    direction: ComparisonDirection
    mcnemar_statistic: float | None
    mcnemar_p_value: float | None
    paired_t_statistic: float | None
    paired_t_p_value: float | None
    wilcoxon_statistic: float | None
    wilcoxon_p_value: float | None
    bootstrap_ci_lower: float | None
    bootstrap_ci_upper: float | None
    p_value: float | None


def _round(value: float) -> float:
    return round(value, _FLOAT_PRECISION)


def _metric_observation_id(obs: MetricObservation) -> str:
    return f"{obs.metric_id}@{obs.metric_version}:{obs.attempt_id}"


def _validate_bound_sequence(
    records: Sequence[_BoundRecord],
    id_field: str,
    type_name: str,
    attempt_by_id: dict[str, AttemptRecord],
    run_id: str,
) -> None:
    """Validate duplicate identity, unknown attempt, run match, and task match for a bound record sequence."""
    seen_ids: set[str] = set()
    for record in records:
        record_id: str = getattr(record, id_field)
        if record_id in seen_ids:
            raise ValueError(f"duplicate {type_name} ID: {record_id}")
        seen_ids.add(record_id)
        attempt_id: str = record.attempt_id
        attempt = attempt_by_id.get(attempt_id)
        if attempt is None:
            raise ValueError(f"{type_name} references unknown attempt: {attempt_id}")
        record_run_id: str = record.run_id
        if record_run_id != run_id:
            raise ValueError(f"{type_name} run does not match analysis run")
        record_task_id: str = record.task_id
        if record_task_id != attempt.task_id:
            raise ValueError(f"{type_name} task does not match attempt task")


def _validate_analysis_input_record(
    record: AnalysisInputRecord,
) -> dict[str, AttemptRecord]:
    """Validate all cross-record bindings on an ``AnalysisInputRecord``.

    Builds typed identity indexes and checks every cross-record binding
    before any producer or aggregate runs. Returns the attempt index for
    downstream use. Failures are raised as ``ValueError`` with stable
    messages; they do not silently disappear from denominators.
    """
    run_id = record.run_id

    task_ids = [task.task_id for task in record.tasks]
    if len(task_ids) != len(set(task_ids)):
        raise ValueError("duplicate task ID")
    task_by_id = {task.task_id: task for task in record.tasks}

    attempt_ids = [attempt.attempt_id for attempt in record.attempts]
    if len(attempt_ids) != len(set(attempt_ids)):
        raise ValueError("duplicate attempt ID")
    attempt_by_id = {attempt.attempt_id: attempt for attempt in record.attempts}
    for attempt in record.attempts:
        if attempt.run_id != run_id:
            raise ValueError("attempt run does not match analysis run")
        if attempt.task_id not in task_by_id:
            raise ValueError("attempt references unknown task")

    metric_keys: set[tuple[str, str, str]] = set()
    for observation in record.metric_observations:
        key = (observation.metric_id, observation.metric_version, observation.attempt_id)
        if key in metric_keys:
            raise ValueError("duplicate metric observation")
        metric_keys.add(key)
        attempt = attempt_by_id.get(observation.attempt_id)
        if attempt is None:
            raise ValueError("metric observation references unknown attempt")
        if observation.run_id != run_id or observation.run_id != attempt.run_id:
            raise ValueError("metric observation run does not match")
        if observation.task_id != attempt.task_id:
            raise ValueError("metric observation task does not match")
        if observation.arm_id != attempt.arm_id:
            raise ValueError("metric observation arm does not match")
        DEFAULT_METRIC_REGISTRY.validate(observation)

    receipt_ids: set[str] = set()
    for receipt in record.receipts:
        if receipt.receipt_id in receipt_ids:
            raise ValueError("duplicate receipt ID")
        receipt_ids.add(receipt.receipt_id)
        attempt = attempt_by_id.get(receipt.attempt_id)
        if attempt is None:
            raise ValueError("receipt references unknown attempt")
        if receipt.run_id != run_id or receipt.run_id != attempt.run_id:
            raise ValueError("receipt run does not match")

    stage_ids: set[str] = set()
    for stage in record.stages:
        if stage.stage_id in stage_ids:
            raise ValueError("duplicate stage ID")
        stage_ids.add(stage.stage_id)
        attempt = attempt_by_id.get(stage.attempt_id)
        if attempt is None:
            raise ValueError("stage references unknown attempt")
        if stage.run_id != run_id or stage.run_id != attempt.run_id:
            raise ValueError("stage run does not match")
        if stage.task_id != attempt.task_id:
            raise ValueError("stage task does not match")

    for spec in _BOUND_RECORD_SPECS:
        records: Sequence[_BoundRecord] = getattr(record, spec.field_name)
        _validate_bound_sequence(records, spec.id_field, spec.type_name, attempt_by_id, run_id)

    return attempt_by_id


def _compute_observation_count_from_record(record: AnalysisInputRecord) -> int:
    """Count all supplied observation, attestation, envelope, and audit-link records."""
    count = 0
    for spec in _BOUND_RECORD_SPECS:
        records: Sequence[_BoundRecord] = getattr(record, spec.field_name)
        count += len(records)
    return count


def _compute_input_content_hash_from_record(record: AnalysisInputRecord) -> str:
    """Compute SHA-256 over canonical JSON of all sorted input records.

    Covers run and release identity, tasks, attempts, every observation
    class, receipts, stages, envelopes, persistence and commitment
    attestations, audit links, and the price table. Semantically unordered
    collections are sorted by stable typed identity before hashing.
    """
    parts: list[str] = []
    parts.append(f"run_id:{record.run_id}")
    parts.append(f"release_version:{record.release_version}")

    for task in sorted(record.tasks, key=lambda t: t.task_id):
        parts.append(f"task:{canonical_model_json(task)}")
    for attempt in sorted(record.attempts, key=lambda a: (a.run_id, a.attempt_id)):
        parts.append(f"attempt:{canonical_model_json(attempt)}")
    for obs in sorted(record.metric_observations, key=lambda o: (o.metric_id, o.metric_version, o.attempt_id)):
        parts.append(f"metric_observation:{canonical_model_json(obs)}")
    for receipt in sorted(record.receipts, key=lambda r: (r.run_id, r.receipt_id)):
        parts.append(f"receipt:{canonical_model_json(receipt)}")
    for stage in sorted(record.stages, key=lambda s: (s.run_id, s.stage_id)):
        parts.append(f"stage:{canonical_model_json(stage)}")

    for spec in _BOUND_RECORD_SPECS:
        records: Sequence[BaseModel] = getattr(record, spec.field_name)
        for rec in sorted(records, key=lambda r: str(getattr(r, spec.id_field))):
            parts.append(f"{spec.hash_prefix}:{canonical_model_json(rec)}")

    if record.price_table is not None:
        parts.append(f"price_table:{canonical_model_json(record.price_table)}")

    joined = "\n".join(parts)
    return hashlib.sha256(joined.encode()).hexdigest()


def _compute_missingness(attempts: Sequence[AttemptRecord]) -> MissingnessBreakdown:
    counts: dict[str, int] = defaultdict(int)
    for attempt in attempts:
        counts[attempt.terminal_status.value] += 1
    return MissingnessBreakdown(
        completed=counts.get(TerminalStatus.COMPLETED.value, 0),
        model_failed=counts.get(TerminalStatus.MODEL_FAILED.value, 0),
        governance_rejected=counts.get(TerminalStatus.GOVERNANCE_REJECTED.value, 0),
        human_denied=counts.get(TerminalStatus.HUMAN_DENIED.value, 0),
        timed_out=counts.get(TerminalStatus.TIMED_OUT.value, 0),
        infrastructure_failed=counts.get(TerminalStatus.INFRASTRUCTURE_FAILED.value, 0),
        invalid_evidence=counts.get(TerminalStatus.INVALID_EVIDENCE.value, 0),
    )


def _compute_receipt_coverage(
    attempts: Sequence[AttemptRecord],
    tasks: Sequence[TaskDefinition],
    receipts: Sequence[ReceiptObservation],
) -> ReceiptCoverageAnalysis:
    """Compute receipt coverage only over receipt-eligible mutation attempts.

    An attempt is receipt-eligible when the arm supports receipt binding
    (governed arms: doctrine, consensus, notary) and the task declares a
    non-empty expected action class (mutation action). Ungoverned arms and
    answer-only turns are excluded from the denominator by design.
    """
    task_by_id: dict[str, TaskDefinition] = {t.task_id: t for t in tasks}

    receipts_by_attempt: dict[str, list[ReceiptObservation]] = defaultdict(list)
    for receipt in receipts:
        receipts_by_attempt[receipt.attempt_id].append(receipt)

    eligible_count = 0
    receipt_bound_count = 0
    receipt_verified_count = 0

    for attempt in attempts:
        arm_def = ARM_DEFINITIONS.get(attempt.arm_id)
        if arm_def is None or not arm_def.receipt_binding:
            continue
        task = task_by_id.get(attempt.task_id)
        if task is None or not task.expected_action_class:
            continue
        eligible_count += 1
        arm_receipts = receipts_by_attempt.get(attempt.attempt_id, [])
        primary_receipts = [r for r in arm_receipts if r.primary]
        if primary_receipts:
            receipt_bound_count += 1
            if all(r.verified for r in primary_receipts):
                receipt_verified_count += 1

    return ReceiptCoverageAnalysis(
        eligible_attempt_count=eligible_count,
        receipt_bound_count=receipt_bound_count,
        receipt_verified_count=receipt_verified_count,
    )


def _build_release_metric_lookup() -> dict[tuple[str, str], ReleaseMetricEntry]:
    return {(m.metric_id, m.metric_version): m for m in RELEASE_METRIC_SET.metrics}


def _task_declares_grader(task: TaskDefinition, definition: MetricDefinition) -> bool:
    grader_ref = definition.grader_ref
    if grader_ref is None:
        return False
    return any(
        grader.grader_id == grader_ref.grader_id
        and grader.grader_version == grader_ref.grader_version
        and grader.grader_class == grader_ref.grader_class
        for grader in task.graders
    )


def _task_assertion_count(task: TaskDefinition, task_field: str | None) -> int:
    if task_field is None:
        return 0
    assertions = getattr(task, task_field)
    if isinstance(assertions, list):
        return len(assertions)
    if task_field == "state_fixture" and task.state_fixture is not None:
        return len(task.state_fixture.assertions)
    return 0


def _arm_satisfies_requirement(attempt: AttemptRecord, requirement: ArmRequirement) -> bool:
    """Check whether an attempt's arm satisfies the metric's arm requirement."""
    if requirement == ArmRequirement.ANY:
        return True
    arm_def = ARM_DEFINITIONS.get(attempt.arm_id)
    if arm_def is None:
        return False
    if requirement == ArmRequirement.GOVERNED:
        return arm_def.uses_gateway
    if requirement == ArmRequirement.NOTARY:
        return attempt.arm_id.value == "notary"
    return False


def _metric_is_eligible(
    task: TaskDefinition,
    attempt: AttemptRecord,
    definition: MetricDefinition,
) -> bool:
    contract = definition.applicability
    if not _arm_satisfies_requirement(attempt, contract.arm_requirement):
        return False
    if contract.eligibility == EligibilityKind.TASK_SUITE:
        return task.suite_id == contract.suite_id
    if contract.eligibility == EligibilityKind.COMPLETED_ANSWER:
        return attempt.terminal_status == TerminalStatus.COMPLETED and attempt.answer_ref is not None
    if contract.eligibility == EligibilityKind.EXPECTED_ACTION_CLASS:
        return bool(task.expected_action_class) and _task_declares_grader(task, definition)
    if contract.eligibility == EligibilityKind.EXPECTED_POLICY_OUTCOME:
        return task.expected_allow_block_outcome is not None and _task_declares_grader(task, definition)
    if contract.eligibility in (EligibilityKind.TASK_ASSERTIONS, EligibilityKind.TASK_STATE_ASSERTIONS):
        return _task_assertion_count(task, contract.task_field) > 0 and _task_declares_grader(task, definition)
    if contract.eligibility == EligibilityKind.USAGE_RECONCILIATION:
        return attempt.usage_reconciliation is not None
    if contract.eligibility in (
        EligibilityKind.STAGE_TIMING,
        EligibilityKind.STAGE_PROVIDER_USAGE,
        EligibilityKind.LOCAL_RESOURCE_OBSERVATION,
        EligibilityKind.HUMAN_WAIT_OBSERVATION,
    ):
        return True
    if contract.eligibility == EligibilityKind.DERIVED:
        return False
    return False


def _missing_denominator_contribution(task: TaskDefinition, definition: MetricDefinition) -> int:
    contract = definition.applicability
    if contract.denominator == DenominatorKind.ATTEMPT:
        return 1
    if contract.denominator in (DenominatorKind.TASK_ASSERTION_COUNT, DenominatorKind.TASK_STATE_ASSERTION_COUNT):
        return _task_assertion_count(task, contract.task_field)
    if contract.denominator == DenominatorKind.EXPECTED_CANARY_OCCURRENCES:
        return sum(assertion.expected_occurrences for assertion in task.sensitive_canary_annotations)
    if contract.denominator == DenominatorKind.EXPECTED_SENSITIVE_OCCURRENCES:
        return sum(assertion.expected_sensitive_occurrences for assertion in task.secret_detection_assertions)
    return 0


def _compute_metric_results(
    tasks: Sequence[TaskDefinition],
    attempts: Sequence[AttemptRecord],
    metric_observations: Sequence[MetricObservation],
) -> list[MetricAnalysisResult]:
    """Compute per-metric per-arm aggregated results.

    Every eligible attempt is retained in the denominator regardless of
    outcome. Missing observations (eligible attempt with no
    MetricObservation or a None value) are counted but do not reduce the
    denominator.
    """
    release_lookup = _build_release_metric_lookup()
    task_by_id = {task.task_id: task for task in tasks}

    # Group attempts by (arm_id)
    attempts_by_arm: dict[str, list[AttemptRecord]] = defaultdict(list)
    for attempt in attempts:
        attempts_by_arm[attempt.arm_id.value].append(attempt)

    # Group metric observations by (metric_id, metric_version, arm_id)
    obs_by_key: dict[tuple[str, str, str], list[MetricObservation]] = defaultdict(list)
    for obs in metric_observations:
        obs_by_key[(obs.metric_id, obs.metric_version, obs.arm_id.value)].append(obs)

    results: list[MetricAnalysisResult] = []

    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        release_entry = release_lookup.get((definition.metric_id, definition.metric_version))
        if release_entry is None:
            continue

        for arm_id in sorted(attempts_by_arm.keys()):
            arm_obs = obs_by_key.get((definition.metric_id, definition.metric_version, arm_id), [])
            observed_attempt_ids = {observation.attempt_id for observation in arm_obs}
            arm_attempts = [
                attempt
                for attempt in attempts_by_arm[arm_id]
                if attempt.attempt_id in observed_attempt_ids
                or _metric_is_eligible(task_by_id[attempt.task_id], attempt, definition)
            ]
            if not arm_attempts:
                continue

            # Determine eligible attempts for this metric
            # An attempt is eligible if it has a MetricObservation with eligible=True
            # or if it's in the attempt set for this arm (denominator preservation)
            obs_by_attempt: dict[str, MetricObservation] = {}
            for obs in arm_obs:
                obs_by_attempt[obs.attempt_id] = obs

            eligible_count = 0
            not_eligible_count = 0
            missing_count = 0
            numerator = 0.0
            denominator = 0
            verification_counts: dict[str, int] = defaultdict(int)
            evidence_ref_count = 0
            obs_ids: list[str] = []
            accepted_non_missing_obs: list[MetricObservation] = []

            for attempt in arm_attempts:
                obs = obs_by_attempt.get(attempt.attempt_id)
                task = task_by_id[attempt.task_id]
                contract_eligible = _metric_is_eligible(task, attempt, definition)
                is_derived = definition.applicability.eligibility == EligibilityKind.DERIVED
                if obs is not None:
                    if is_derived:
                        if obs.eligible:
                            eligible_count += 1
                            denominator += obs.denominator_contribution
                            if obs.value is not None:
                                numerator += obs.value * obs.denominator_contribution
                                accepted_non_missing_obs.append(obs)
                            else:
                                missing_count += 1
                            verification_counts[obs.verification_status.value] += 1
                            evidence_ref_count += len(obs.evidence_refs)
                            obs_ids.append(_metric_observation_id(obs))
                        else:
                            not_eligible_count += 1
                    elif obs.eligible and not contract_eligible:
                        # Producer marked eligible=True but the typed contract
                        # says the attempt is ineligible (wrong arm, wrong suite,
                        # missing assertions, etc.). Reject the producer's flag;
                        # the contract is the authority.
                        not_eligible_count += 1
                    elif not obs.eligible and contract_eligible:
                        # Producer marked eligible=False but the typed contract
                        # says the attempt is eligible. The producer cannot
                        # remove an assigned attempt from the denominator by
                        # setting a flag. Override to eligible+missing.
                        eligible_count += 1
                        missing_count += 1
                        denominator += _missing_denominator_contribution(task, definition)
                    elif obs.eligible:
                        eligible_count += 1
                        denominator += obs.denominator_contribution
                        if obs.value is not None:
                            numerator += obs.value * obs.denominator_contribution
                            accepted_non_missing_obs.append(obs)
                        else:
                            missing_count += 1
                        verification_counts[obs.verification_status.value] += 1
                        evidence_ref_count += len(obs.evidence_refs)
                        obs_ids.append(_metric_observation_id(obs))
                    else:
                        not_eligible_count += 1
                else:
                    # Attempt has no observation for this metric
                    # It's in the arm's attempt set but has no metric observation
                    # Count as eligible but missing (denominator preservation)
                    eligible_count += 1
                    missing_count += 1
                    denominator += _missing_denominator_contribution(
                        task, definition,
                    )

            # Compute aggregate value
            # When all eligible observations are missing, the value is None
            # (not 0.0) to distinguish "no data" from "measured zero".
            value: float | None = None
            non_missing_values = [obs.value for obs in accepted_non_missing_obs if obs.value is not None]
            if denominator > 0 and accepted_non_missing_obs:
                if definition.aggregation == AggregationMethod.MEAN:
                    value = _round(sum(non_missing_values) / len(non_missing_values))
                elif definition.aggregation in (AggregationMethod.PROPORTION, AggregationMethod.RATE, AggregationMethod.BOOLEAN_FRACTION):
                    value = _round(numerator / denominator)
                elif definition.aggregation == AggregationMethod.SUM:
                    value = _round(numerator)

            results.append(MetricAnalysisResult(
                metric_id=definition.metric_id,
                metric_version=definition.metric_version,
                arm_id=arm_id,
                domain=release_entry.domain,
                direction=definition.direction,
                unit=definition.unit,
                numerator=_round(numerator),
                denominator=denominator,
                value=value,
                eligible_count=eligible_count,
                not_eligible_count=not_eligible_count,
                missing_count=missing_count,
                verification_status_counts=dict(sorted(verification_counts.items())),
                evidence_ref_count=evidence_ref_count,
                metric_observation_ids=sorted(obs_ids),
            ))

    results.sort(key=lambda r: (r.metric_id, r.metric_version, r.arm_id))
    return results


def _compute_domain_stratified_results(
    metric_results: list[MetricAnalysisResult],
    gate_decisions: list[GateDecision],
) -> list[DomainStratifiedResult]:
    """Compute per-domain per-arm stratified results."""
    gate_by_metric_arm: dict[tuple[str, str, str], GateDecisionStatus] = {}
    for gd in gate_decisions:
        gate_by_metric_arm[(gd.metric_id, gd.metric_version, gd.arm_id)] = gd.status

    results_by_arm_domain: dict[tuple[str, MetricDomain], list[MetricAnalysisResult]] = defaultdict(list)
    for mr in metric_results:
        results_by_arm_domain[(mr.arm_id, mr.domain)].append(mr)

    results: list[DomainStratifiedResult] = []
    for (arm_id, domain), mrs in sorted(results_by_arm_domain.items(), key=lambda kv: (kv[0][0], kv[0][1].value)):
        passing = 0
        failing = 0
        not_applicable = 0
        for mr in mrs:
            status = gate_by_metric_arm.get((mr.metric_id, mr.metric_version, mr.arm_id), GateDecisionStatus.INSUFFICIENT_DATA)
            if status == GateDecisionStatus.PASS:
                passing += 1
            elif status == GateDecisionStatus.FAIL:
                failing += 1
            elif status == GateDecisionStatus.NOT_APPLICABLE:
                not_applicable += 1
        results.append(DomainStratifiedResult(
            arm_id=arm_id,
            domain=domain,
            metric_count=len(mrs),
            passing_metric_count=passing,
            failing_metric_count=failing,
            not_applicable_metric_count=not_applicable,
        ))

    results.sort(key=lambda r: (r.arm_id, r.domain.value))
    return results


def _compute_confusion_matrices(
    attempts: Sequence[AttemptRecord],
    metric_observations: Sequence[MetricObservation],
    tasks: Sequence[TaskDefinition],
) -> list[ConfusionMatrix]:
    """Compute allow/block confusion matrices for binary allow/block metrics.

    For policy_outcome and policy_attack metrics, the MetricObservation
    value is 1.0 (correct outcome) or 0.0 (incorrect outcome). The
    confusion matrix maps these to TP/FP/TN/FN based on the expected
    allow/block outcome from the task definition.
    """
    task_by_id: dict[str, TaskDefinition] = {t.task_id: t for t in tasks}
    release_lookup = _build_release_metric_lookup()

    # Only compute confusion matrices for binary allow/block metrics
    confusion_metric_ids = {"policy_outcome", "policy_attack"}

    obs_by_key: dict[tuple[str, str, str], list[MetricObservation]] = defaultdict(list)
    for obs in metric_observations:
        if obs.metric_id in confusion_metric_ids:
            obs_by_key[(obs.metric_id, obs.metric_version, obs.arm_id.value)].append(obs)

    matrices: list[ConfusionMatrix] = []
    for key in sorted(obs_by_key.keys()):
        metric_id, metric_version, arm_id = key
        obs_list = obs_by_key[key]
        release_entry = release_lookup.get((metric_id, metric_version))
        if release_entry is None:
            continue

        tp = fp = tn = fn = 0
        for obs in obs_list:
            if obs.value is None:
                continue
            task = task_by_id.get(obs.task_id)
            if task is None or task.expected_allow_block_outcome is None:
                continue
            expected_allow = task.expected_allow_block_outcome.value == "allow"
            observed_correct = obs.value >= 1.0

            # For allow/block metrics, value=1.0 means the outcome matched
            # We need to infer the observed outcome from the value and expected outcome
            if expected_allow:
                if observed_correct:
                    tp += 1  # Expected allow, observed allow
                else:
                    fn += 1  # Expected allow, observed block
            elif observed_correct:
                tn += 1  # Expected block, observed block
            else:
                fp += 1  # Expected block, observed allow

        matrices.append(ConfusionMatrix(
            metric_id=metric_id,
            metric_version=metric_version,
            arm_id=arm_id,
            domain=release_entry.domain,
            true_positive=tp,
            false_positive=fp,
            true_negative=tn,
            false_negative=fn,
        ))

    matrices.sort(key=lambda m: (m.metric_id, m.metric_version, m.arm_id))
    return matrices


_POOLING_METHOD = "preregistered_simple_summation_across_arms"


def _compute_pooled_confusion_matrices(
    confusion_matrices: list[ConfusionMatrix],
) -> list[PooledConfusionMatrix]:
    """Compute preregistered pooled confusion matrices across arms.

    Sums the arm-level ``ConfusionMatrix`` TP/FP/TN/FN counts into a
    single pooled estimate per metric. The pooling rule (simple
    summation across arms) is declared before analysis and applied
    uniformly. A hierarchical model that accounts for arm-level
    variance is not implemented in v2.1.8.

    Returns pooled matrices sorted by ``(metric_id, metric_version)``.
    """
    by_metric: dict[tuple[str, str], list[ConfusionMatrix]] = defaultdict(list)
    for cm in confusion_matrices:
        by_metric[(cm.metric_id, cm.metric_version)].append(cm)

    pooled: list[PooledConfusionMatrix] = []
    for (metric_id, metric_version), arm_matrices in sorted(by_metric.items()):
        arm_ids = sorted(cm.arm_id for cm in arm_matrices)
        tp = sum(cm.true_positive for cm in arm_matrices)
        fp = sum(cm.false_positive for cm in arm_matrices)
        tn = sum(cm.true_negative for cm in arm_matrices)
        fn = sum(cm.false_negative for cm in arm_matrices)
        domain = arm_matrices[0].domain
        pooled.append(PooledConfusionMatrix(
            metric_id=metric_id,
            metric_version=metric_version,
            domain=domain,
            arm_count=len(arm_matrices),
            arm_ids=arm_ids,
            true_positive=tp,
            false_positive=fp,
            true_negative=tn,
            false_negative=fn,
            pooling_method=_POOLING_METHOD,
        ))

    pooled.sort(key=lambda p: (p.metric_id, p.metric_version))
    return pooled


def _compute_gate_decisions(
    metric_results: list[MetricAnalysisResult],
) -> list[GateDecision]:
    """Compute practical-threshold gate decisions for each metric and arm.

    Statistical significance alone cannot pass a release. Every headline
    metric has a gate decision with a practical threshold or
    non-inferiority margin.
    """
    release_lookup = _build_release_metric_lookup()
    decisions: list[GateDecision] = []

    for mr in metric_results:
        release_entry = release_lookup.get((mr.metric_id, mr.metric_version))
        if release_entry is None:
            continue

        threshold_desc = release_entry.threshold_description
        definition = DEFAULT_METRIC_REGISTRY.get(mr.metric_id, mr.metric_version)
        threshold = definition.practical_threshold
        threshold_value = threshold.value if threshold is not None else None
        ni_margin = definition.non_inferiority_margin

        if mr.eligible_count == 0 and mr.missing_count == 0:
            status = GateDecisionStatus.NOT_APPLICABLE
            reason = "No eligible attempts for this metric and arm."
        elif mr.missing_count > 0:
            status = GateDecisionStatus.INSUFFICIENT_DATA
            reason = f"{mr.missing_count} of {mr.eligible_count} eligible attempts are missing observations."
        elif mr.denominator == 0:
            status = GateDecisionStatus.NOT_APPLICABLE
            reason = "Denominator is zero for this metric and arm."
        elif threshold is None:
            status = GateDecisionStatus.UNSUPPORTED
            reason = "No practical threshold defined; calibration pending."
        elif mr.value is None:
            status = GateDecisionStatus.INSUFFICIENT_DATA
            reason = "No measured value is available for the practical threshold."
        else:
            passed = _check_threshold(mr.value, threshold.value, threshold.operator)
            status = GateDecisionStatus.PASS if passed else GateDecisionStatus.FAIL
            threshold_kind = "Release-blocker" if threshold.release_blocker else "Practical"
            reason = f"{threshold_kind} threshold {threshold.value} {'met' if passed else 'not met'}: measured {mr.value}."

        decisions.append(GateDecision(
            metric_id=mr.metric_id,
            metric_version=mr.metric_version,
            arm_id=mr.arm_id,
            threshold_description=threshold_desc,
            measured_value=mr.value,
            threshold_value=threshold_value,
            non_inferiority_margin=ni_margin,
            status=status,
            reason=reason,
        ))

    decisions.sort(key=lambda d: (d.metric_id, d.metric_version, d.arm_id))
    return decisions


def _check_threshold(value: float, threshold: float, operator: ThresholdOperator) -> bool:
    if operator == ThresholdOperator.LESS_THAN_OR_EQUAL:
        return value <= threshold
    return value >= threshold


def _compute_paired_comparisons(
    tasks: Sequence[TaskDefinition],
    attempts: Sequence[AttemptRecord],
    metric_observations: Sequence[MetricObservation],
) -> list[PairedComparison]:
    """Compute paired comparisons between arms over identical task instances.

    Pairs observations by ``(task_id, state_snapshot_hash)`` so that only
    attempts on the same task instance and initial-state snapshot are
    compared. For each metric and each ordered pair of arms (baseline,
    comparison) where both arms have observations on the same task
    instances, computes:

    - Absolute and relative deltas.
    - Cohen's d standardized effect size.
    - McNemar test for binary outcomes (value >= 0.5 treated as pass).
    - Paired t-test for continuous outcomes.
    - Task-cluster bootstrap 95% confidence interval (seeded).
    - Holm-Bonferroni correction across the family of comparisons for
      each metric.

    Returns comparisons sorted by ``(metric_id, metric_version,
    baseline_arm_id, comparison_arm_id)``.
    """
    release_lookup = _build_release_metric_lookup()

    # Build attempt lookup by attempt_id
    attempt_by_id: dict[str, AttemptRecord] = {a.attempt_id: a for a in attempts}

    # Group metric observations by (metric_id, metric_version, arm_id, task_id, state_snapshot_hash)
    # to find paired task instances across arms
    obs_by_metric: dict[tuple[str, str], list[MetricObservation]] = defaultdict(list)
    for obs in metric_observations:
        release_entry = release_lookup.get((obs.metric_id, obs.metric_version))
        if release_entry is None:
            continue
        obs_by_metric[(obs.metric_id, obs.metric_version)].append(obs)

    comparisons: list[PairedComparison] = []

    for (metric_id, metric_version), metric_obs_list in sorted(obs_by_metric.items()):
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, metric_version)
        release_entry = release_lookup[(metric_id, metric_version)]

        # Group observations by (arm_id, pairing_key) where pairing_key = (task_id, state_snapshot_hash)
        obs_by_arm_pair: dict[tuple[str, tuple[str, str]], list[MetricObservation]] = defaultdict(list)
        for obs in metric_obs_list:
            attempt = attempt_by_id.get(obs.attempt_id)
            if attempt is None:
                continue
            if obs.value is None:
                continue
            pairing_key = (obs.task_id, attempt.state_snapshot_hash)
            arm_str = obs.arm_id.value
            obs_by_arm_pair[(arm_str, pairing_key)].append(obs)

        # Find all arm IDs that have observations for this metric
        arm_ids_with_obs = sorted({arm for (arm, _) in obs_by_arm_pair.keys()})
        if len(arm_ids_with_obs) < 2:
            continue

        # For each ordered pair of arms, find common pairing keys
        raw_comparisons: list[_RawComparison] = []
        for baseline_arm in arm_ids_with_obs:
            for comparison_arm in arm_ids_with_obs:
                if baseline_arm >= comparison_arm:
                    continue

                baseline_keys = {pk for (arm, pk) in obs_by_arm_pair if arm == baseline_arm}
                comparison_keys = {pk for (arm, pk) in obs_by_arm_pair if arm == comparison_arm}
                common_keys = sorted(baseline_keys & comparison_keys)
                if len(common_keys) < 2:
                    continue

                baseline_vals: list[float] = []
                comparison_vals: list[float] = []
                for pk in common_keys:
                    b_obs = obs_by_arm_pair[(baseline_arm, pk)]
                    c_obs = obs_by_arm_pair[(comparison_arm, pk)]
                    # Average multiple observations per pairing key (replicates)
                    b_vals = [o.value for o in b_obs if o.value is not None]
                    c_vals = [o.value for o in c_obs if o.value is not None]
                    if not b_vals or not c_vals:
                        continue
                    baseline_vals.append(sum(b_vals) / len(b_vals))
                    comparison_vals.append(sum(c_vals) / len(c_vals))

                if len(baseline_vals) < 2:
                    continue

                baseline_value = sum(baseline_vals) / len(baseline_vals)
                comparison_value = sum(comparison_vals) / len(comparison_vals)
                abs_d = statistics.absolute_delta(baseline_value, comparison_value)
                rel_d = statistics.relative_delta(baseline_value, comparison_value)
                effect_size = statistics.cohens_d_paired(baseline_vals, comparison_vals)

                # McNemar for binary outcomes
                baseline_binary = [v >= 0.5 for v in baseline_vals]
                comparison_binary = [v >= 0.5 for v in comparison_vals]
                mcnemar_stat, mcnemar_p = statistics.mcnemar_test(baseline_binary, comparison_binary)

                # Paired t-test for continuous outcomes
                t_stat, t_p = statistics.paired_t_test(baseline_vals, comparison_vals)
                wilcoxon_stat, wilcoxon_p = statistics.wilcoxon_signed_rank_test(
                    baseline_vals, comparison_vals,
                )

                # Bootstrap CI
                ci_lower, ci_upper = statistics.bootstrap_ci(
                    baseline_vals, comparison_vals, seed=0,
                )

                # Direction
                if definition.direction == MetricDirection.LOWER_IS_BETTER:
                    if abs_d < 0:
                        direction = ComparisonDirection.IMPROVEMENT
                    elif abs_d > 0:
                        direction = ComparisonDirection.REGRESSION
                    else:
                        direction = ComparisonDirection.NEUTRAL
                elif definition.direction in (MetricDirection.HIGHER_IS_BETTER, MetricDirection.BINARY_PASS_FAIL):
                    if abs_d > 0:
                        direction = ComparisonDirection.IMPROVEMENT
                    elif abs_d < 0:
                        direction = ComparisonDirection.REGRESSION
                    else:
                        direction = ComparisonDirection.NEUTRAL
                else:
                    direction = ComparisonDirection.NEUTRAL

                # Use the p-value from the more appropriate test
                # For binary pass/fail metrics, use McNemar; for continuous, use t-test
                if definition.direction == MetricDirection.BINARY_PASS_FAIL:
                    p_value = mcnemar_p
                else:
                    p_value = t_p

                raw_comparisons.append(_RawComparison(
                    metric_id=metric_id,
                    metric_version=metric_version,
                    baseline_arm_id=baseline_arm,
                    comparison_arm_id=comparison_arm,
                    paired_count=len(baseline_vals),
                    baseline_value=_round(baseline_value),
                    comparison_value=_round(comparison_value),
                    absolute_delta=abs_d,
                    relative_delta=rel_d,
                    standardized_effect_size=effect_size,
                    direction=direction,
                    mcnemar_statistic=mcnemar_stat,
                    mcnemar_p_value=mcnemar_p,
                    paired_t_statistic=t_stat,
                    paired_t_p_value=t_p,
                    wilcoxon_statistic=wilcoxon_stat,
                    wilcoxon_p_value=wilcoxon_p,
                    bootstrap_ci_lower=ci_lower,
                    bootstrap_ci_upper=ci_upper,
                    p_value=p_value,
                ))

        if not raw_comparisons:
            continue

        # Apply Holm correction across the family of comparisons for this metric
        p_values = [rc.p_value for rc in raw_comparisons]
        # Replace None p-values with 1.0 for correction (no evidence of difference)
        p_values_for_correction: list[float] = [p if p is not None else 1.0 for p in p_values]
        holm_results = statistics.holm_correction(p_values_for_correction)

        # Look up non-inferiority margin from the metric definition
        ni_definition = DEFAULT_METRIC_REGISTRY.get(metric_id, metric_version)
        ni_margin = ni_definition.non_inferiority_margin

        for rc, (rank, corrected_p) in zip(raw_comparisons, holm_results, strict=True):
            comparisons.append(PairedComparison(
                metric_id=rc.metric_id,
                metric_version=rc.metric_version,
                baseline_arm_id=rc.baseline_arm_id,
                comparison_arm_id=rc.comparison_arm_id,
                paired_count=rc.paired_count,
                baseline_value=rc.baseline_value,
                comparison_value=rc.comparison_value,
                absolute_delta=rc.absolute_delta,
                relative_delta=rc.relative_delta,
                standardized_effect_size=rc.standardized_effect_size,
                direction=rc.direction,
                mcnemar_statistic=rc.mcnemar_statistic,
                mcnemar_p_value=rc.mcnemar_p_value,
                paired_t_statistic=rc.paired_t_statistic,
                paired_t_p_value=rc.paired_t_p_value,
                wilcoxon_statistic=rc.wilcoxon_statistic,
                wilcoxon_p_value=rc.wilcoxon_p_value,
                bootstrap_ci_lower=rc.bootstrap_ci_lower,
                bootstrap_ci_upper=rc.bootstrap_ci_upper,
                holm_corrected_p_value=corrected_p,
                holm_rank=rank,
                non_inferiority_margin=ni_margin,
                gate_decision=_paired_gate_decision(
                    rc.p_value, corrected_p, rc.absolute_delta, definition.direction,
                    ni_margin, rc.bootstrap_ci_lower, rc.bootstrap_ci_upper,
                ),
            ))

    comparisons.sort(key=lambda c: (c.metric_id, c.metric_version, c.baseline_arm_id, c.comparison_arm_id))
    return comparisons


def _paired_gate_decision(
    raw_p_value: float | None,
    corrected_p_value: float,
    abs_delta: float,
    direction: MetricDirection,
    non_inferiority_margin: float | None,
    bootstrap_ci_lower: float | None,
    bootstrap_ci_upper: float | None,
) -> GateDecisionStatus:
    """Determine the gate decision for a paired comparison.

    When a non-inferiority margin is defined, the gate uses the
    bootstrap confidence interval of the paired delta: the comparison
    is non-inferior when the relevant CI bound does not cross the
    margin. For HIGHER_IS_BETTER and BINARY_PASS_FAIL metrics, the
    lower CI bound must be >= -margin. For LOWER_IS_BETTER metrics,
    the upper CI bound must be <= margin.

    When no non-inferiority margin is defined, the gate uses the
    default superiority test: Holm-corrected p-value < 0.05 AND a
    practically meaningful (non-zero) absolute delta. Statistical
    significance alone cannot pass a release.
    """
    if non_inferiority_margin is not None:
        # Non-inferiority testing via bootstrap CI
        if bootstrap_ci_lower is None or bootstrap_ci_upper is None:
            return GateDecisionStatus.INSUFFICIENT_DATA
        if direction in (MetricDirection.HIGHER_IS_BETTER, MetricDirection.BINARY_PASS_FAIL):
            # Delta = comparison - baseline. Non-inferior if lower bound >= -margin.
            if bootstrap_ci_lower >= -non_inferiority_margin:
                return GateDecisionStatus.PASS
            return GateDecisionStatus.FAIL
        if direction == MetricDirection.LOWER_IS_BETTER:
            # Delta = comparison - baseline. Non-inferior if upper bound <= margin.
            if bootstrap_ci_upper <= non_inferiority_margin:
                return GateDecisionStatus.PASS
            return GateDecisionStatus.FAIL
        # NEUTRAL direction: no non-inferiority test
        return GateDecisionStatus.UNSUPPORTED

    # Default superiority gate: significance + practical significance
    if raw_p_value is None:
        return GateDecisionStatus.INSUFFICIENT_DATA
    if corrected_p_value > 0.05:
        return GateDecisionStatus.INSUFFICIENT_DATA
    if abs_delta == 0.0:
        return GateDecisionStatus.INSUFFICIENT_DATA
    return GateDecisionStatus.PASS


def compute_bridge_run_comparison(
    bridge_id: str,
    old_analysis: CanonicalEvalAnalysis,
    new_analysis: CanonicalEvalAnalysis,
) -> list[BridgeRunComparison]:
    """Compute per-metric bridge-run comparisons between two analysis versions.

    Compares old and new ``CanonicalEvalAnalysis`` instances produced by
    executing old and new suite, grader, metric, doctrine, or analysis
    versions over the same model cohort. For each metric present in both
    analyses, records the old and new values and a gate decision that
    fails when any release-blocker metric regresses beyond its
    non-inferiority margin.

    Returns comparisons sorted by ``(metric_id, metric_version)``.
    """
    # Build lookup of metric results by (metric_id, metric_version) -> value
    # Pool across arms using the metric's registered aggregation semantics.
    def _pool_metric_values(analysis: CanonicalEvalAnalysis) -> dict[tuple[str, str], float | None]:
        by_metric: dict[tuple[str, str], list[MetricAnalysisResult]] = defaultdict(list)
        for metric_result in analysis.metric_results:
            by_metric[(metric_result.metric_id, metric_result.metric_version)].append(metric_result)
        result: dict[tuple[str, str], float | None] = {}
        for key, metric_results in by_metric.items():
            definition = DEFAULT_METRIC_REGISTRY.get(*key)
            if any(metric_result.missing_count > 0 for metric_result in metric_results):
                result[key] = None
                continue
            numerator = sum(metric_result.numerator for metric_result in metric_results)
            denominator = sum(metric_result.denominator for metric_result in metric_results)
            if definition.aggregation == AggregationMethod.SUM:
                result[key] = _round(numerator)
            else:
                result[key] = _round(numerator / denominator) if denominator > 0 else None
        return result

    old_values = _pool_metric_values(old_analysis)
    new_values = _pool_metric_values(new_analysis)

    all_keys = sorted(set(old_values.keys()) | set(new_values.keys()))

    comparisons: list[BridgeRunComparison] = []
    for (metric_id, metric_version) in all_keys:
        old_val = old_values.get((metric_id, metric_version))
        new_val = new_values.get((metric_id, metric_version))

        if old_val is not None and new_val is not None:
            abs_delta = _round(new_val - old_val)
        else:
            abs_delta = None

        # Gate decision: fail if a release-blocker metric regresses
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, metric_version)
        ni_margin = definition.non_inferiority_margin

        if old_val is None or new_val is None:
            status = GateDecisionStatus.INSUFFICIENT_DATA
            reason = "Metric present in only one analysis version."
        elif ni_margin is not None and abs_delta is not None:
            margin = ni_margin
            direction = definition.direction
            if direction in (MetricDirection.HIGHER_IS_BETTER, MetricDirection.BINARY_PASS_FAIL):
                # Regression = new < old by more than margin
                if abs_delta < -margin:
                    status = GateDecisionStatus.FAIL
                    reason = f"Regression exceeds non-inferiority margin {margin}: delta {abs_delta}."
                else:
                    status = GateDecisionStatus.PASS
                    reason = f"Non-inferior within margin {margin}: delta {abs_delta}."
            elif direction == MetricDirection.LOWER_IS_BETTER:
                # Regression = new > old by more than margin
                if abs_delta > margin:
                    status = GateDecisionStatus.FAIL
                    reason = f"Regression exceeds non-inferiority margin {margin}: delta {abs_delta}."
                else:
                    status = GateDecisionStatus.PASS
                    reason = f"Non-inferior within margin {margin}: delta {abs_delta}."
            else:
                status = GateDecisionStatus.UNSUPPORTED
                reason = "Neutral direction; no non-inferiority test."
        elif abs_delta is not None and abs_delta == 0.0:
            status = GateDecisionStatus.PASS
            reason = "No change between versions."
        elif abs_delta is not None:
            status = GateDecisionStatus.INSUFFICIENT_DATA
            reason = "No non-inferiority margin; delta requires manual review."
        else:
            status = GateDecisionStatus.INSUFFICIENT_DATA
            reason = "Insufficient data for comparison."

        comparisons.append(BridgeRunComparison(
            bridge_id=bridge_id,
            metric_id=metric_id,
            metric_version=metric_version,
            old_value=old_val,
            new_value=new_val,
            absolute_delta=abs_delta,
            gate_decision=status,
            reason=reason,
        ))

    comparisons.sort(key=lambda c: (c.bridge_id, c.metric_id, c.metric_version))
    return comparisons


def compute_canonical_analysis_from_record(
    record: AnalysisInputRecord,
) -> CanonicalEvalAnalysis:
    """Compute the canonical eval analysis from a complete immutable input record.

    This is the authoritative complete-input entry point. It validates
    all cross-record bindings, computes the observation count from
    supplied records, and hashes the complete record before producing
    any analysis output. The computation is deterministic: identical
    inputs and analysis version produce byte-identical output.
    """
    _validate_analysis_input_record(record)

    input_summary = AnalysisInputSummary(
        task_count=len(record.tasks),
        attempt_count=len(record.attempts),
        observation_count=_compute_observation_count_from_record(record),
        receipt_count=len(record.receipts),
        stage_count=len(record.stages),
        metric_observation_count=len(record.metric_observations),
        input_content_hash=_compute_input_content_hash_from_record(record),
    )

    missingness = _compute_missingness(record.attempts)
    receipt_coverage = _compute_receipt_coverage(record.attempts, record.tasks, record.receipts)

    arm_ids = sorted({a.arm_id.value for a in record.attempts})

    all_metric_observations = run_all_telemetry_producers(record, record.metric_observations)
    all_metric_observations = run_all_derived_producers(record, all_metric_observations)

    metric_results = _compute_metric_results(record.tasks, record.attempts, all_metric_observations)
    gate_decisions = _compute_gate_decisions(metric_results)
    domain_stratified = _compute_domain_stratified_results(metric_results, gate_decisions)
    confusion_matrices = _compute_confusion_matrices(record.attempts, all_metric_observations, record.tasks)
    pooled_confusion_matrices = _compute_pooled_confusion_matrices(confusion_matrices)
    comparisons = _compute_paired_comparisons(record.tasks, record.attempts, all_metric_observations)

    unsupported_claim_names = sorted(RELEASE_METRIC_SET.unsupported_claim_names)

    return CanonicalEvalAnalysis(
        analysis_schema_version=ANALYSIS_SCHEMA_VERSION,
        analysis_computation_version=ANALYSIS_COMPUTATION_VERSION,
        release_version=record.release_version,
        run_id=record.run_id,
        input_summary=input_summary,
        missingness=missingness,
        receipt_coverage=receipt_coverage,
        arm_ids=arm_ids,
        metric_results=metric_results,
        domain_stratified_results=domain_stratified,
        confusion_matrices=confusion_matrices,
        pooled_confusion_matrices=pooled_confusion_matrices,
        comparisons=comparisons,
        gate_decisions=gate_decisions,
        bridge_runs=[],
        bridge_run_comparisons=[],
        unsupported_claim_names=unsupported_claim_names,
    )


def compute_canonical_analysis(
    tasks: Sequence[TaskDefinition],
    attempts: Sequence[AttemptRecord],
    metric_observations: Sequence[MetricObservation],
    receipts: Sequence[ReceiptObservation],
    stages: Sequence[StageObservation],
    run_id: str,
    release_version: str = "v2.1.8",
) -> CanonicalEvalAnalysis:
    """Compute the canonical eval analysis from immutable records.

    Adapter that constructs a complete ``AnalysisInputRecord`` from the
    positional arguments and delegates to
    ``compute_canonical_analysis_from_record``. The record-based entry
    point is the authoritative implementation; this helper exists only
    for tests and callers that supply the legacy five-field subset.
    Every observation class not supplied here defaults to an empty list
    on the constructed record.
    """
    record = AnalysisInputRecord(
        run_id=run_id,
        release_version=release_version,
        tasks=list(tasks),
        attempts=list(attempts),
        metric_observations=list(metric_observations),
        receipts=list(receipts),
        stages=list(stages),
    )
    return compute_canonical_analysis_from_record(record)


__all__ = [
    "compute_bridge_run_comparison",
    "compute_canonical_analysis",
    "compute_canonical_analysis_from_record",
]
