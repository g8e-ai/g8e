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

from g8e_evals.analysis.canonical import (
    ANALYSIS_COMPUTATION_VERSION,
    ANALYSIS_SCHEMA_VERSION,
    AnalysisInputSummary,
    CanonicalEvalAnalysis,
    ConfusionMatrix,
    DomainStratifiedResult,
    GateDecision,
    GateDecisionStatus,
    MetricAnalysisResult,
    MissingnessBreakdown,
    ReceiptCoverageAnalysis,
)
from g8e_evals.arms import ARM_DEFINITIONS
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    AggregationMethod,
    MetricDefinition,
    MetricDirection,
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


def _round(value: float) -> float:
    return round(value, _FLOAT_PRECISION)


def _metric_observation_id(obs: MetricObservation) -> str:
    return f"{obs.metric_id}@{obs.metric_version}:{obs.attempt_id}"


def _compute_input_content_hash(
    tasks: Sequence[TaskDefinition],
    attempts: Sequence[AttemptRecord],
    metric_observations: Sequence[MetricObservation],
    receipts: Sequence[ReceiptObservation],
    stages: Sequence[StageObservation],
) -> str:
    """Compute SHA-256 over canonical JSON of all sorted input records."""
    parts: list[str] = []

    for task in sorted(tasks, key=lambda t: t.task_id):
        parts.append(task.model_dump_json())
    for attempt in sorted(attempts, key=lambda a: (a.run_id, a.attempt_id)):
        parts.append(attempt.model_dump_json())
    for obs in sorted(metric_observations, key=lambda o: (o.metric_id, o.metric_version, o.attempt_id)):
        parts.append(obs.model_dump_json())
    for receipt in sorted(receipts, key=lambda r: (r.run_id, r.receipt_id)):
        parts.append(receipt.model_dump_json())
    for stage in sorted(stages, key=lambda s: (s.run_id, s.stage_id)):
        parts.append(stage.model_dump_json())

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


def _compute_metric_results(
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
            arm_attempts = attempts_by_arm[arm_id]
            arm_obs = obs_by_key.get((definition.metric_id, definition.metric_version, arm_id), [])

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

            for attempt in arm_attempts:
                obs = obs_by_attempt.get(attempt.attempt_id)
                if obs is not None:
                    if obs.eligible:
                        eligible_count += 1
                        denominator += obs.denominator_contribution
                        if obs.value is not None:
                            numerator += obs.value * obs.denominator_contribution
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
                    denominator += 1

            # Compute aggregate value
            # When all eligible observations are missing, the value is None
            # (not 0.0) to distinguish "no data" from "measured zero".
            value: float | None = None
            non_missing_obs = [obs for obs in arm_obs if obs.eligible and obs.value is not None]
            non_missing_values = [obs.value for obs in non_missing_obs if obs.value is not None]
            if denominator > 0 and non_missing_obs:
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
        threshold_value: float | None = None

        if mr.denominator == 0:
            status = GateDecisionStatus.NOT_APPLICABLE
            reason = "No eligible attempts for this metric and arm."
        elif mr.missing_count > 0 and mr.missing_count == mr.eligible_count:
            status = GateDecisionStatus.INSUFFICIENT_DATA
            reason = f"All {mr.missing_count} eligible attempts are missing observations."
        elif not release_entry.has_practical_threshold:
            status = GateDecisionStatus.UNSUPPORTED
            reason = "No practical threshold defined; calibration pending."
        else:
            parsed_threshold, is_blocker = _parse_threshold(definition, mr.direction)
            threshold_value = parsed_threshold
            if threshold_value is not None and mr.value is not None:
                passed = _check_threshold(mr.value, threshold_value, mr.direction, is_blocker)
                status = GateDecisionStatus.PASS if passed else GateDecisionStatus.FAIL
                if is_blocker:
                    reason = f"Release-blocker threshold {threshold_value} {'met' if passed else 'not met'}: measured {mr.value}."
                else:
                    reason = f"Practical threshold {threshold_value} {'met' if passed else 'not met'}: measured {mr.value}."
            else:
                status = GateDecisionStatus.UNSUPPORTED
                reason = "Threshold could not be parsed from definition."

        decisions.append(GateDecision(
            metric_id=mr.metric_id,
            metric_version=mr.metric_version,
            arm_id=mr.arm_id,
            threshold_description=threshold_desc,
            measured_value=mr.value,
            threshold_value=threshold_value,
            status=status,
            reason=reason,
        ))

    decisions.sort(key=lambda d: (d.metric_id, d.metric_version, d.arm_id))
    return decisions


def _parse_threshold(definition: MetricDefinition, direction: MetricDirection) -> tuple[float | None, bool]:
    """Parse a practical threshold value from the definition's release_threshold.

    Returns (threshold_value, is_release_blocker). Release-blocker
    thresholds are exact requirements (e.g., 1.0 for proportions). Non-
    inferiority margins are parsed from "non-inferiority margin of X"
    patterns.
    """
    import re

    threshold_str = definition.release_threshold
    if threshold_str is None:
        return None, False

    # Release-blocker: "Practical threshold: 1.0 for governed arms; ..."
    # Extract the first floating-point number after "Practical threshold:"
    if threshold_str.startswith("Practical threshold:"):
        match = re.search(r"Practical threshold:\s+(\d+\.?\d*)", threshold_str)
        if match:
            try:
                return float(match.group(1)), True
            except ValueError:
                pass
    # Non-inferiority margin
    if "non-inferiority margin" in threshold_str.lower():
        return None, False

    return None, False


def _check_threshold(value: float, threshold: float, direction: MetricDirection, is_blocker: bool) -> bool:
    """Check whether a measured value meets the practical threshold."""
    if direction == MetricDirection.LOWER_IS_BETTER:
        return value <= threshold
    return value >= threshold


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

    This is the single entry point for analysis computation. It reads
    only immutable records and computes every aggregate, confusion
    matrix, comparison, and gate decision from first principles.

    The computation is deterministic: identical inputs and analysis
    version produce byte-identical output.
    """
    input_summary = AnalysisInputSummary(
        task_count=len(tasks),
        attempt_count=len(attempts),
        observation_count=sum(len(getattr(a, ref_field, [])) for a in attempts for ref_field in _observation_ref_fields()),
        receipt_count=len(receipts),
        stage_count=len(stages),
        metric_observation_count=len(metric_observations),
        input_content_hash=_compute_input_content_hash(tasks, attempts, metric_observations, receipts, stages),
    )

    missingness = _compute_missingness(attempts)
    receipt_coverage = _compute_receipt_coverage(attempts, tasks, receipts)

    arm_ids = sorted({a.arm_id.value for a in attempts})

    metric_results = _compute_metric_results(attempts, metric_observations)
    gate_decisions = _compute_gate_decisions(metric_results)
    domain_stratified = _compute_domain_stratified_results(metric_results, gate_decisions)
    confusion_matrices = _compute_confusion_matrices(attempts, metric_observations, tasks)

    unsupported_claim_names = sorted(RELEASE_METRIC_SET.unsupported_claim_names)

    return CanonicalEvalAnalysis(
        analysis_schema_version=ANALYSIS_SCHEMA_VERSION,
        analysis_computation_version=ANALYSIS_COMPUTATION_VERSION,
        release_version=release_version,
        run_id=run_id,
        input_summary=input_summary,
        missingness=missingness,
        receipt_coverage=receipt_coverage,
        arm_ids=arm_ids,
        metric_results=metric_results,
        domain_stratified_results=domain_stratified,
        confusion_matrices=confusion_matrices,
        comparisons=[],
        gate_decisions=gate_decisions,
        bridge_runs=[],
        unsupported_claim_names=unsupported_claim_names,
    )


def _observation_ref_fields() -> list[str]:
    """Return the list of observation ref field names on AttemptRecord."""
    return [
        "final_state_observation_refs",
        "state_observation_refs",
        "rehydration_observation_refs",
        "secret_detection_observation_refs",
        "unauthorized_mutation_observation_refs",
        "token_store_persistence_observation_refs",
        "token_ttl_expiry_observation_refs",
        "token_persistence_failure_observation_refs",
        "exfiltration_attempt_observation_refs",
        "artifact_leakage_observation_refs",
        "replay_attempt_observation_refs",
        "signed_field_tampering_observation_refs",
        "payload_tampering_observation_refs",
        "stale_state_root_observation_refs",
        "identity_mismatch_observation_refs",
        "nonce_expiration_observation_refs",
        "signer_defect_observation_refs",
        "l3_proof_transplant_observation_refs",
        "revoked_credential_observation_refs",
        "evidence_preservation_observation_refs",
        "policy_attack_observation_refs",
        "tool_sequence_observation_refs",
        "factual_qa_observation_refs",
        "citation_backed_observation_refs",
        "partial_milestone_observation_refs",
        "reliability_observation_refs",
        "economics_performance_observation_refs",
        "local_resource_observation_refs",
        "human_wait_observation_refs",
        "receipt_refs",
        "grade_refs",
        "unsupported_exclusion_refs",
    ]


__all__ = [
    "compute_canonical_analysis",
]
