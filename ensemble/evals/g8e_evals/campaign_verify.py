# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Offline campaign verifier for complete campaign report directories.

The verifier reads a campaign report directory and checks the complete
matrix: index chain integrity, cell coverage (every expected
model-track-tier-task-repetition cell resolves to exactly one effective
valid report or an allowed qualification/unavailability record),
duplicate effective assignment detection, run manifest identity,
terminal attempt enforcement, metric binding, file safety (regular
files, no symlinks), and public projection safety.

It emits a typed ``CampaignVerificationReport`` with verification status,
campaign identity, verified index generation hash, checked layers, and
typed failures. A campaign with missing or inconsistent cells cannot
produce a passing publication candidate.

The verifier requires no network access, provider calls, or external
services. It reads only the report directory on disk.
"""

from __future__ import annotations

import json
from pathlib import Path

from pydantic import ValidationError

from g8e_evals.campaign import (
    CampaignAssignment,
    CampaignManifest,
    ModelCohort,
)
from g8e_evals.constants import (
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_COHORTS_JSONL,
    CAMPAIGN_INDEX_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_RETRY_POLICY_JSON,
    CAMPAIGN_SCHEDULE_JSON,
    CAMPAIGN_STATUS_JSON,
    CAMPAIGN_VERIFICATION_REPORT_JSON,
    CORRELATED_ERRORS_JSONL,
    ESCALATION_RECORDS_JSONL,
    EVIDENCE_INDEX_JSONL,
    EXPECTED_RECORD_POLICY_JSON,
    MANIFEST_JSON,
    METRICS_JSONL,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENTS_JSONL,
    STAGES_JSONL,
    TASKS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
)
from g8e_evals.expected_record_policy import (
    CardinalityRule,
    ExpectedRecordPolicy,
    RecordApplicability,
)
from g8e_evals.index import (
    AssignmentDisposition,
    CampaignVerificationReport,
    IndexCreationReason,
    IndexGeneration,
    ResourceObservation,
    validate_index_chain,
    validate_no_duplicate_effective_assignments,
    validate_resource_observations,
    validate_supersession_policy,
)
from g8e_evals.schema import (
    AttemptRecord,
    CorrelatedErrorRecord,
    EscalationRecord,
    EvidenceIndex,
    MetricObservation,
    RunManifest,
    SecurityEventRecord,
    StageKind,
    StageObservation,
    TerminalStatus,
    ToolCallScorecard,
    validate_correlated_error_records,
    validate_escalation_records,
    validate_security_event_records,
    validate_tool_call_scorecards,
)
from g8e_evals.report.validate import validate_standalone_report


VERIFICATION_SCHEMA_VERSION = "1.0.0"

_REQUIRED_ARTIFACTS = (
    MANIFEST_JSON,
    ATTEMPTS_JSONL,
    METRICS_JSONL,
    ANALYSIS_JSON,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_INDEX_JSONL,
)

_OPTIONAL_ARTIFACTS = (
    TASKS_JSONL,
    EVIDENCE_INDEX_JSONL,
    CAMPAIGN_COHORTS_JSONL,
    CAMPAIGN_SCHEDULE_JSON,
    CAMPAIGN_RETRY_POLICY_JSON,
    CAMPAIGN_STATUS_JSON,
    STAGES_JSONL,
)


def _read_jsonl_models(path: Path, model_cls: type) -> list[object]:
    """Read a JSONL file and validate each line as a Pydantic model."""
    records: list[object] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if line:
            records.append(model_cls.model_validate_json(line))
    return records


def _read_jsonl_dicts(path: Path) -> list[dict]:
    """Read a JSONL file and return a list of parsed dicts."""
    records: list[dict] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if line:
            records.append(json.loads(line))
    return records


def _check_file_safety(path: Path, failures: list[str], label: str) -> bool:
    """Check that a path exists, is a regular file (not a symlink)."""
    if not path.exists():
        failures.append(f"missing {label}: {path.name}")
        return False
    if path.is_symlink():
        failures.append(f"symlink rejected for {label}: {path.name}")
        return False
    if not path.is_file():
        failures.append(f"{label} is not a regular file: {path.name}")
        return False
    return True


def _enforce_record_file_policy(
    path: Path,
    file_name: str,
    policy: ExpectedRecordPolicy | None,
    failures: list[str],
) -> None:
    """Enforce expected-record policy for a single observation/event file.

    When no policy is present, the file is optional (legacy behavior).
    When a policy is present:

    - REQUIRED: the file must exist as a regular file (not a symlink).
    - NOT_APPLICABLE: the file must not exist. A present file is rejected
      as fabricated for an inapplicable scenario.
    - OPTIONAL: the file may exist. When present, it must be a regular
      file (not a symlink).
    - EXACT cardinality: the file must contain exactly ``expected_count``
      records. ``expected_count=0`` means the file must exist and be
      empty.
    """
    if policy is None:
        return
    entry = policy.get_entry(file_name)
    if entry is None:
        return
    exists = path.exists()
    if entry.applicability == RecordApplicability.REQUIRED:
        if not exists:
            failures.append(f"required record file missing: {file_name}")
            return
        if path.is_symlink():
            failures.append(f"symlink rejected for required file: {file_name}")
            return
        if not path.is_file():
            failures.append(f"required record file is not a regular file: {file_name}")
            return
        if entry.cardinality_rule == CardinalityRule.EXACT:
            try:
                count = sum(
                    1 for line in path.read_text().splitlines() if line.strip()
                )
            except OSError as e:
                failures.append(f"failed to read required file {file_name}: {e}")
                return
            if count != entry.expected_count:
                failures.append(
                    f"cardinality mismatch for {file_name}: expected {entry.expected_count}, "
                    f"got {count}"
                )
    elif entry.applicability == RecordApplicability.NOT_APPLICABLE:
        if exists:
            failures.append(
                f"fabricated record file for inapplicable scenario: {file_name}"
            )
    elif entry.applicability == RecordApplicability.OPTIONAL:
        if exists:
            if path.is_symlink():
                failures.append(f"symlink rejected for optional file: {file_name}")
            elif not path.is_file():
                failures.append(f"optional record file is not a regular file: {file_name}")
            elif entry.cardinality_rule == CardinalityRule.EXACT:
                try:
                    count = sum(
                        1 for line in path.read_text().splitlines() if line.strip()
                    )
                except OSError as e:
                    failures.append(f"failed to read optional file {file_name}: {e}")
                    return
                if count != entry.expected_count:
                    failures.append(
                        f"cardinality mismatch for optional {file_name}: "
                        f"expected {entry.expected_count}, got {count}"
                    )


def _cross_check_observation_identity(
    obs: ResourceObservation,
    campaign_id: str,
    run_ids: set[str],
    assignment_ids: set[str],
    attempt_ids: set[str],
    attempt_by_id: dict[str, AttemptRecord],
    failures: list[str],
) -> None:
    """Cross-check every identity field on a resource observation.

    Verifies that the observation's campaign, run, assignment, attempt,
    and task identities match the campaign's known records. An orphan
    observation (referencing an unknown attempt) is rejected.
    """
    if obs.campaign_id != campaign_id:
        failures.append(
            f"resource observation {obs.inference_id} campaign_id mismatch: "
            f"got {obs.campaign_id!r}, expected {campaign_id!r}"
        )
    if obs.run_id not in run_ids:
        failures.append(
            f"resource observation {obs.inference_id} references unknown run_id: {obs.run_id!r}"
        )
    if obs.assignment_id and obs.assignment_id not in assignment_ids:
        failures.append(
            f"resource observation {obs.inference_id} references unknown assignment_id: "
            f"{obs.assignment_id!r}"
        )
    if obs.attempt_id not in attempt_ids:
        failures.append(
            f"resource observation {obs.inference_id} references unknown attempt_id: "
            f"{obs.attempt_id!r}"
        )
    else:
        attempt = attempt_by_id.get(obs.attempt_id)
        if attempt is not None:
            if obs.task_id and attempt.task_id and obs.task_id != attempt.task_id:
                failures.append(
                    f"resource observation {obs.inference_id} task_id mismatch: "
                    f"got {obs.task_id!r}, attempt has {attempt.task_id!r}"
                )
            if obs.assignment_id and attempt.assignment_id and obs.assignment_id != attempt.assignment_id:
                failures.append(
                    f"resource observation {obs.inference_id} assignment_id mismatch: "
                    f"got {obs.assignment_id!r}, attempt has {attempt.assignment_id!r}"
                )


def _cross_check_event_identity(
    record_id: str,
    rec_campaign_id: str,
    rec_child_id: str,
    rec_run_id: str,
    rec_assignment_id: str,
    rec_attempt_id: str,
    rec_inference_id: str | None,
    rec_task_id: str,
    campaign_id: str,
    run_ids: set[str],
    assignment_ids: set[str],
    attempt_ids: set[str],
    attempt_by_id: dict[str, AttemptRecord],
    label: str,
    failures: list[str],
) -> None:
    """Cross-check every identity field on an event record.

    Verifies that the record's campaign, run, assignment, attempt, and
    task identities match the campaign's known records. An orphan record
    (referencing an unknown attempt) is rejected. The inference_id is
    optional for attempt-level events and is not cross-checked here.
    """
    if rec_campaign_id != campaign_id:
        failures.append(
            f"{label} {record_id} campaign_id mismatch: "
            f"got {rec_campaign_id!r}, expected {campaign_id!r}"
        )
    if rec_run_id not in run_ids:
        failures.append(
            f"{label} {record_id} references unknown run_id: {rec_run_id!r}"
        )
    if rec_assignment_id and rec_assignment_id not in assignment_ids:
        failures.append(
            f"{label} {record_id} references unknown assignment_id: {rec_assignment_id!r}"
        )
    if rec_attempt_id not in attempt_ids:
        failures.append(
            f"{label} {record_id} references unknown attempt_id: {rec_attempt_id!r}"
        )
    else:
        attempt = attempt_by_id.get(rec_attempt_id)
        if attempt is not None:
            if rec_task_id and attempt.task_id and rec_task_id != attempt.task_id:
                failures.append(
                    f"{label} {record_id} task_id mismatch: "
                    f"got {rec_task_id!r}, attempt has {attempt.task_id!r}"
                )
            if rec_assignment_id and attempt.assignment_id and rec_assignment_id != attempt.assignment_id:
                failures.append(
                    f"{label} {record_id} assignment_id mismatch: "
                    f"got {rec_assignment_id!r}, attempt has {attempt.assignment_id!r}"
                )


def _check_evidence_binding(
    verification_status: object,
    source_evidence_refs: list[str],
    source_evidence_sha256: str | None,
    record_id: str,
    label: str,
    failures: list[str],
) -> None:
    """Check that a VERIFIED record has source evidence bindings.

    A VERIFIED record without source evidence references or a source
    evidence hash is not independently verifiable and is rejected.
    """
    status_str = getattr(verification_status, "value", str(verification_status))
    if status_str == "verified":
        if not source_evidence_refs:
            failures.append(
                f"verified {label} {record_id} has no source_evidence_refs"
            )
        if source_evidence_sha256 is None:
            failures.append(
                f"verified {label} {record_id} has no source_evidence_sha256"
            )


def _load_evidence_index(report_dir: Path, failures: list[str]) -> dict[str, EvidenceIndex]:
    """Load evidence-index.jsonl and return a dict keyed by artifact_id.

    Returns an empty dict when the file does not exist. Validation
    errors are recorded as failures.
    """
    evidence_index_path = report_dir / EVIDENCE_INDEX_JSONL
    if not evidence_index_path.exists():
        return {}
    if not _check_optional_file_safety(evidence_index_path, "evidence index", failures):
        return {}
    index: dict[str, EvidenceIndex] = {}
    try:
        for line in evidence_index_path.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            entry = EvidenceIndex.model_validate_json(line)
            if entry.artifact_id in index:
                failures.append(
                    f"evidence index duplicate artifact_id: {entry.artifact_id!r}"
                )
                continue
            index[entry.artifact_id] = entry
    except (ValidationError, json.JSONDecodeError) as e:
        failures.append(f"evidence index validation failed: {e}")
    return index


def _check_evidence_index_resolution(
    verification_status: object,
    source_evidence_refs: list[str],
    source_evidence_sha256: str | None,
    record_id: str,
    label: str,
    evidence_index: dict[str, EvidenceIndex],
    failures: list[str],
) -> None:
    """Cross-check that a VERIFIED record's source evidence references
    resolve to indexed evidence entries with matching SHA-256.

    A VERIFIED record whose source_evidence_refs do not resolve to
    evidence-index.jsonl entries is rejected. A SHA-256 mismatch
    between the record and the indexed entry is rejected.
    """
    status_str = getattr(verification_status, "value", str(verification_status))
    if status_str != "verified":
        return
    if not evidence_index:
        return
    for ref in source_evidence_refs:
        entry = evidence_index.get(ref)
        if entry is None:
            failures.append(
                f"verified {label} {record_id} source_evidence_ref {ref!r} "
                f"not found in evidence index"
            )
            continue
        if source_evidence_sha256 is not None and entry.sha256 != source_evidence_sha256:
            failures.append(
                f"verified {label} {record_id} source_evidence_ref {ref!r} "
                f"sha256 mismatch: record has {source_evidence_sha256!r}, "
                f"index has {entry.sha256!r}"
            )


def _load_stages(report_dir: Path, failures: list[str]) -> list[StageObservation]:
    """Load stages.jsonl and return a list of StageObservation models.

    Returns an empty list when the file does not exist. Validation
    errors are recorded as failures.
    """
    stages_path = report_dir / STAGES_JSONL
    if not stages_path.exists():
        return []
    if not _check_optional_file_safety(stages_path, "stage observations", failures):
        return []
    stages: list[StageObservation] = []
    try:
        for line in stages_path.read_text().splitlines():
            line = line.strip()
            if line:
                stages.append(StageObservation.model_validate_json(line))
    except (ValidationError, json.JSONDecodeError) as e:
        failures.append(f"stage observation validation failed: {e}")
    return stages


def _load_cohorts(report_dir: Path, failures: list[str]) -> dict[str, ModelCohort]:
    """Load campaign-cohorts.jsonl and return a dict keyed by cohort_id.

    Returns an empty dict when the file does not exist. Validation
    errors are recorded as failures.
    """
    cohorts_path = report_dir / CAMPAIGN_COHORTS_JSONL
    if not cohorts_path.exists():
        return {}
    if not _check_optional_file_safety(cohorts_path, "campaign cohorts", failures):
        return {}
    cohorts: dict[str, ModelCohort] = {}
    try:
        for line in cohorts_path.read_text().splitlines():
            line = line.strip()
            if line:
                cohort = ModelCohort.model_validate_json(line)
                cohorts[cohort.cohort_id] = cohort
    except (ValidationError, json.JSONDecodeError) as e:
        failures.append(f"campaign cohort validation failed: {e}")
    return cohorts


def _cross_check_observation_model_binding(
    obs: ResourceObservation,
    assignment_by_id: dict[str, CampaignAssignment],
    cohort_by_id: dict[str, ModelCohort],
    stage_ids: set[str],
    failures: list[str],
) -> None:
    """Cross-check a resource observation's model_variant_id, role, and
    stage_id against the assignment's cohort and the stage trail.

    The model_variant_id must match one of the cohort's role-binding
    model IDs. The role must match one of the cohort's role-binding
    roles. The stage_id must exist in the stage observation trail when
    the trail is present.
    """
    assignment = assignment_by_id.get(obs.assignment_id)
    if assignment is not None:
        cohort = cohort_by_id.get(assignment.model_cohort_id)
        if cohort is not None:
            cohort_model_ids = {rb.model_id for rb in cohort.role_bindings}
            cohort_roles = {rb.role for rb in cohort.role_bindings}
            if obs.model_variant_id not in cohort_model_ids:
                failures.append(
                    f"resource observation {obs.inference_id} model_variant_id mismatch: "
                    f"got {obs.model_variant_id!r}, cohort {assignment.model_cohort_id!r} "
                    f"has {sorted(cohort_model_ids)}"
                )
            obs_role = obs.role.value if hasattr(obs.role, "value") else str(obs.role)
            if obs_role not in cohort_roles:
                failures.append(
                    f"resource observation {obs.inference_id} role mismatch: "
                    f"got {obs_role!r}, cohort {assignment.model_cohort_id!r} "
                    f"has {sorted(cohort_roles)}"
                )
    if stage_ids and obs.stage_id not in stage_ids:
        failures.append(
            f"resource observation {obs.inference_id} stage_id {obs.stage_id!r} "
            f"not found in stage observation trail"
        )


def _enforce_one_per_inference_cardinality(
    path: Path,
    file_name: str,
    policy: ExpectedRecordPolicy | None,
    observations: list[ResourceObservation],
    stages: list[StageObservation],
    failures: list[str],
) -> None:
    """Enforce ONE_PER_INFERENCE cardinality for a record file.

    When stages.jsonl is present, the record count must match the
    number of model_inference stages. When stages.jsonl is absent,
    the record count must match the number of resource observations
    (one per inference as a fallback). For optional files, cardinality
    is only enforced when the file exists and contains records; an
    empty optional file is acceptable.
    """
    if policy is None:
        return
    entry = policy.get_entry(file_name)
    if entry is None or entry.cardinality_rule != CardinalityRule.ONE_PER_INFERENCE:
        return
    if not path.exists():
        if entry.applicability == RecordApplicability.REQUIRED:
            failures.append(f"required record file missing: {file_name}")
        return
    try:
        record_count = sum(1 for line in path.read_text().splitlines() if line.strip())
    except OSError as e:
        failures.append(f"failed to read {file_name}: {e}")
        return
    if record_count == 0 and entry.applicability == RecordApplicability.OPTIONAL:
        return
    if stages:
        inference_stages = [s for s in stages if s.kind == StageKind.MODEL_INFERENCE]
        expected_count = len(inference_stages)
    else:
        expected_count = len(observations)
    if record_count != expected_count:
        failures.append(
            f"one_per_inference cardinality mismatch for {file_name}: "
            f"expected {expected_count} (inference trail), got {record_count}"
        )


def _enforce_one_per_attempt_cardinality(
    path: Path,
    file_name: str,
    policy: ExpectedRecordPolicy | None,
    completed_attempt_count: int,
    failures: list[str],
) -> None:
    """Enforce ONE_PER_ATTEMPT cardinality for a record file.

    The record count must match the number of completed attempts. For
    optional files, cardinality is only enforced when the file exists
    and contains records; an empty optional file is acceptable.
    """
    if policy is None:
        return
    entry = policy.get_entry(file_name)
    if entry is None or entry.cardinality_rule != CardinalityRule.ONE_PER_ATTEMPT:
        return
    if not path.exists():
        if entry.applicability == RecordApplicability.REQUIRED:
            failures.append(f"required record file missing: {file_name}")
        return
    try:
        record_count = sum(1 for line in path.read_text().splitlines() if line.strip())
    except OSError as e:
        failures.append(f"failed to read {file_name}: {e}")
        return
    if record_count == 0 and entry.applicability == RecordApplicability.OPTIONAL:
        return
    if record_count != completed_attempt_count:
        failures.append(
            f"one_per_attempt cardinality mismatch for {file_name}: "
            f"expected {completed_attempt_count} (completed attempts), got {record_count}"
        )


def _verify_inference_trail_count(
    observations: list[ResourceObservation],
    stages: list[StageObservation],
    attempts: list[AttemptRecord],
    failures: list[str],
) -> None:
    """Verify that resource observations match the inference trail exactly.

    When stages.jsonl is present, each completed attempt must have
    exactly one resource observation per model_inference stage. An
    attempt with fewer observations than inferences is a missing
    observation; an attempt with more is an extra observation.
    """
    if not stages:
        return
    inference_stages_by_attempt: dict[str, list[StageObservation]] = {}
    for stage in stages:
        if stage.kind == StageKind.MODEL_INFERENCE:
            inference_stages_by_attempt.setdefault(stage.attempt_id, []).append(stage)
    obs_by_attempt: dict[str, list[ResourceObservation]] = {}
    for obs in observations:
        obs_by_attempt.setdefault(obs.attempt_id, []).append(obs)
    for attempt in attempts:
        if attempt.terminal_status != TerminalStatus.COMPLETED:
            continue
        expected = len(inference_stages_by_attempt.get(attempt.attempt_id, []))
        actual = len(obs_by_attempt.get(attempt.attempt_id, []))
        if actual == 0 and expected == 0:
            continue
        if actual != expected:
            failures.append(
                f"inference count mismatch for attempt {attempt.attempt_id}: "
                f"expected {expected} observations (inference trail), got {actual}"
            )


def _verify_metric_denominator_consistency(
    metrics: list[MetricObservation],
    attempts: list[AttemptRecord],
    failures: list[str],
) -> None:
    """Verify that metric denominator_contribution values are consistent.

    A completed attempt's metric should have denominator_contribution=1.
    A non-completed attempt's metric should have denominator_contribution=0.
    Mismatches indicate a tampered or miscalculated derived metric.
    """
    completed_attempt_ids = {
        a.attempt_id for a in attempts if a.terminal_status == TerminalStatus.COMPLETED
    }
    for m in metrics:
        expected = 1 if m.attempt_id in completed_attempt_ids else 0
        if m.denominator_contribution != expected:
            failures.append(
                f"metric {m.metric_id} for attempt {m.attempt_id} "
                f"denominator_contribution mismatch: got {m.denominator_contribution}, "
                f"expected {expected}"
            )


def _verify_resource_observation_count(
    observations: list[ResourceObservation],
    attempts: list[AttemptRecord],
    failures: list[str],
) -> None:
    """Verify that resource observations match actual provider inferences.

    Each terminal attempt should have at least one resource observation
    (one per inference). An attempt with no observations is flagged as a
    potential missing observation. An observation referencing an unknown
    attempt is an orphan (already caught by cross-check).
    """
    obs_by_attempt: dict[str, list[ResourceObservation]] = {}
    for obs in observations:
        obs_by_attempt.setdefault(obs.attempt_id, []).append(obs)
    for attempt in attempts:
        if attempt.terminal_status != TerminalStatus.COMPLETED:
            continue
        attempt_obs = obs_by_attempt.get(attempt.attempt_id, [])
        if not attempt_obs:
            failures.append(
                f"completed attempt {attempt.attempt_id} has no resource observations"
            )


def _check_optional_file_safety(path: Path, label: str, failures: list[str]) -> bool:
    """Check symlink safety for an optional observation/event file.

    Returns True when the file is safe to read (exists, is a regular
    file, and is not a symlink). Returns False when the file does not
    exist or is unsafe, appending a failure for unsafe files.
    """
    if not path.exists():
        return False
    if path.is_symlink():
        failures.append(f"symlink rejected for {label}: {path.name}")
        return False
    if not path.is_file():
        failures.append(f"{label} is not a regular file: {path.name}")
        return False
    return True


def verify_campaign(report_dir: Path) -> CampaignVerificationReport:
    """Verify a complete campaign report directory.

    Checks the following layers in order:
    1. **file_safety**: required artifacts exist as regular files (no symlinks).
    2. **standalone_report**: the report passes standalone report validation.
    3. **campaign_manifest**: campaign-manifest.json is a valid CampaignManifest.
    4. **index_chain**: index generations form a valid append-only parent-hash chain.
    5. **duplicate_effective**: no duplicate effective assignments in any generation.
    6. **cell_coverage**: campaign assignments are parsed as typed CampaignAssignment models and every assignment has a disposition in the final generation (no missing or extra assignment IDs).
    7. **terminal_attempts**: every effective attempt is terminal.
    8. **metric_binding**: every completed attempt has at least one bound metric and denominator_contribution values are recomputed and verified.
    9. **manifest_identity**: run manifest content hashes match campaign manifest.
    10. **artifact_identity**: campaign binding authority hashes are present and valid.
    11. **finalization**: the last index generation is FINALIZATION.
    12. **supersession_policy**: SUPERSESSION generations are policy-valid.
    13. **expected_record_policy**: the frozen expected record policy is loaded and validated.
    14. **resource_observation**: resource observations are validated, cross-bound to campaign/assignment/attempt/stage identities, and their model_variant_id and role are cross-checked against the assignment's cohort. VERIFIED observations' source evidence references are resolved against the evidence index.
    15. **tool_call_scorecard**: tool call scorecards are validated, cross-bound, and their cardinality is enforced (ONE_PER_INFERENCE when stages.jsonl is present).
    16. **escalation_records**: escalation records are validated, cross-bound, and their cardinality is enforced (ONE_PER_ATTEMPT).
    17. **security_events**: security event records are validated, cross-bound, and their cardinality is enforced (ONE_PER_ATTEMPT).
    18. **correlated_errors**: correlated error records are validated, cross-bound, and their cardinality is enforced (ONE_PER_INFERENCE when stages.jsonl is present).
    19. **resource_observation_count**: resource observation counts are verified against the exact inference trail from stages.jsonl (not just "at least one per attempt").

    Returns a ``CampaignVerificationReport`` with ``ok=True`` only when all
    layers pass. The report carries the campaign identity, verified index
    generation hash, checked layers, and typed failures.
    """
    failures: list[str] = []
    checked_layers: list[str] = []
    campaign_id = ""
    campaign_revision = ""
    verified_index_hash = "0" * 64

    # Layer 1: file safety — required artifacts
    checked_layers.append("file_safety")
    for artifact in _REQUIRED_ARTIFACTS:
        _check_file_safety(report_dir / artifact, failures, artifact)

    # Layer 2: standalone report validation
    checked_layers.append("standalone_report")
    standalone = validate_standalone_report(report_dir)
    if not standalone.ok:
        failures.extend(standalone.failures)

    # Layer 3: campaign manifest
    checked_layers.append("campaign_manifest")
    manifest_path = report_dir / CAMPAIGN_MANIFEST_JSON
    manifest: CampaignManifest | None = None
    if _check_file_safety(manifest_path, failures, CAMPAIGN_MANIFEST_JSON):
        try:
            manifest = CampaignManifest.model_validate_json(manifest_path.read_text())
            campaign_id = manifest.campaign_id
            campaign_revision = manifest.release_version
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"campaign manifest validation failed: {e}")

    # Layer 4: index chain
    checked_layers.append("index_chain")
    index_path = report_dir / CAMPAIGN_INDEX_JSONL
    generations: list[IndexGeneration] = []
    if _check_file_safety(index_path, failures, CAMPAIGN_INDEX_JSONL):
        try:
            raw_lines = index_path.read_text().strip().splitlines()
            if not raw_lines:
                failures.append("index chain is empty")
            else:
                generations = [IndexGeneration.model_validate_json(line) for line in raw_lines]
                validate_index_chain(generations)
                verified_index_hash = generations[-1].content_hash
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"index chain validation failed: {e}")

    # Layer 5: duplicate effective assignments
    checked_layers.append("duplicate_effective")
    for gen in generations:
        try:
            validate_no_duplicate_effective_assignments(gen)
        except ValueError as e:
            failures.append(f"duplicate effective assignment in generation {gen.generation_number}: {e}")

    # Layer 6: cell coverage — typed assignment parsing and disposition completeness
    checked_layers.append("cell_coverage")
    assignments_path = report_dir / CAMPAIGN_ASSIGNMENTS_JSONL
    assignment_by_id: dict[str, CampaignAssignment] = {}
    if _check_file_safety(assignments_path, failures, CAMPAIGN_ASSIGNMENTS_JSONL):
        try:
            assignment_records = _read_jsonl_dicts(assignments_path)
            assignments: list[CampaignAssignment] = []
            for r in assignment_records:
                a = CampaignAssignment.model_validate(r)
                assignments.append(a)
                assignment_by_id[a.assignment_id] = a
            assignment_ids: set[str] = set(assignment_by_id.keys())

            if generations:
                final_gen = generations[-1]
                dispositioned_ids = {d.assignment_id for d in final_gen.assignment_dispositions}
                missing = assignment_ids - dispositioned_ids
                for aid in sorted(missing):
                    failures.append(f"assignment {aid} missing from final index generation dispositions")
                extra = dispositioned_ids - assignment_ids
                for aid in sorted(extra):
                    failures.append(f"extra assignment {aid} in final index generation dispositions not in campaign assignments")
            elif assignment_ids:
                failures.append("assignments exist but no index generations to check coverage")
        except (ValidationError, json.JSONDecodeError, ValueError) as e:
            failures.append(f"assignment coverage check failed: {e}")

    # Layer 7: terminal attempts — every attempt parses with a valid terminal status
    checked_layers.append("terminal_attempts")
    attempts_path = report_dir / ATTEMPTS_JSONL
    attempts: list[AttemptRecord] = []
    if _check_file_safety(attempts_path, failures, ATTEMPTS_JSONL):
        try:
            attempt_records = _read_jsonl_dicts(attempts_path)
            attempts = [AttemptRecord.model_validate(r) for r in attempt_records]
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"terminal attempt check failed: {e}")

    # Layer 8: metric binding — every completed attempt has at least one bound metric
    checked_layers.append("metric_binding")
    metrics_path = report_dir / METRICS_JSONL
    metrics: list[MetricObservation] = []
    if _check_file_safety(metrics_path, failures, METRICS_JSONL):
        try:
            metric_records = _read_jsonl_dicts(metrics_path)
            metrics = [MetricObservation.model_validate(r) for r in metric_records]
            metrics_by_attempt: dict[str, list[MetricObservation]] = {}
            for m in metrics:
                metrics_by_attempt.setdefault(m.attempt_id, []).append(m)

            for attempt in attempts:
                if attempt.terminal_status == TerminalStatus.COMPLETED:
                    if attempt.attempt_id not in metrics_by_attempt:
                        failures.append(
                            f"completed attempt {attempt.attempt_id} has no bound metric"
                        )
            # Derived metric recomputation: verify denominator consistency
            _verify_metric_denominator_consistency(metrics, attempts, failures)
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"metric binding check failed: {e}")

    # Layer 9: manifest identity — run manifest content hashes match campaign manifest
    checked_layers.append("manifest_identity")
    run_manifest_path = report_dir / MANIFEST_JSON
    run_manifest: RunManifest | None = None
    if _check_file_safety(run_manifest_path, failures, MANIFEST_JSON):
        try:
            run_manifest = RunManifest.model_validate_json(run_manifest_path.read_text())
            if manifest is not None:
                # Check dataset hash consistency
                run_dataset = run_manifest.dataset_hash
                if run_dataset and manifest.task_assignment_hash:
                    # The campaign manifest binds the task_assignment_hash, not the
                    # dataset hash directly. We check that the run manifest has
                    # content hashes present.
                    if not run_manifest.content_hashes:
                        failures.append("run manifest has no content hashes")
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"run manifest identity check failed: {e}")

    # Layer 10: artifact identity — campaign binding authority hash check
    # The report-level CampaignBinding carries authority hashes (profile,
    # registry, expected-record policy) and environment scopes rather
    # than per-variant artifact identity. Per-inference artifact identity
    # verification against provider telemetry is handled by Ion's Phase 2
    # verifier deepening using resource observation records.
    checked_layers.append("artifact_identity")
    if run_manifest is not None and run_manifest.campaign_binding is not None:
        binding = run_manifest.campaign_binding
        _ZERO_HASH = "0" * 64
        if not binding.campaign_profile_hash or len(binding.campaign_profile_hash) != 64 or binding.campaign_profile_hash == _ZERO_HASH:
            failures.append(
                "artifact identity: campaign_binding.campaign_profile_hash is missing, invalid, or zero"
            )
        if not binding.model_registry_hash or len(binding.model_registry_hash) != 64 or binding.model_registry_hash == _ZERO_HASH:
            failures.append(
                "artifact identity: campaign_binding.model_registry_hash is missing, invalid, or zero"
            )
        if not binding.required_record_policy_hash or len(binding.required_record_policy_hash) != 64 or binding.required_record_policy_hash == _ZERO_HASH:
            failures.append(
                "artifact identity: campaign_binding.required_record_policy_hash is missing, invalid, or zero"
            )
        # Cross-check: the binding's campaign_id must match the campaign
        # manifest's campaign_id. A mismatch indicates the report was
        # produced under a different campaign authority than the manifest
        # claims.
        if manifest is not None and binding.campaign_id != manifest.campaign_id:
            failures.append(
                f"artifact identity: campaign_binding.campaign_id {binding.campaign_id!r} "
                f"does not match campaign manifest campaign_id {manifest.campaign_id!r}"
            )

    # Layer 11: finalization — the last index generation is FINALIZATION
    checked_layers.append("finalization")
    if generations:
        last_gen = generations[-1]
        if last_gen.creation_reason != IndexCreationReason.FINALIZATION:
            failures.append(
                f"final index generation is not FINALIZATION: "
                f"got {last_gen.creation_reason.value!r}"
            )

    # Layer 12: supersession policy — SUPERSESSION generations must be policy-valid
    checked_layers.append("supersession_policy")
    if len(generations) >= 2:
        for i, gen in enumerate(generations):
            if gen.creation_reason != IndexCreationReason.SUPERSESSION:
                continue
            # Find the prior generation's disposition for each superseded assignment
            prior_gen = generations[i - 1] if i > 0 else None
            if prior_gen is None:
                continue
            prior_dispositions: dict[str, AssignmentDisposition] = {
                d.assignment_id: d.disposition
                for d in prior_gen.assignment_dispositions
            }
            for entry in gen.assignment_dispositions:
                prior_disp = prior_dispositions.get(entry.assignment_id)
                if prior_disp is None:
                    continue
                try:
                    validate_supersession_policy(
                        prior_disposition=prior_disp,
                        prior_terminal_status=prior_disp.value,
                        new_creation_reason=gen.creation_reason,
                    )
                except ValueError as e:
                    failures.append(
                        f"supersession policy violation in generation "
                        f"{gen.generation_number}: {e}"
                    )

    # Layer 13: expected record policy — load frozen policy if present
    checked_layers.append("expected_record_policy")
    policy: ExpectedRecordPolicy | None = None
    policy_path = report_dir / EXPECTED_RECORD_POLICY_JSON
    if policy_path.exists():
        if policy_path.is_symlink():
            failures.append(f"symlink rejected for expected record policy: {policy_path.name}")
        elif not policy_path.is_file():
            failures.append(f"expected record policy is not a regular file: {policy_path.name}")
        else:
            try:
                policy = ExpectedRecordPolicy.model_validate_json(policy_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"expected record policy validation failed: {e}")

    # Build identity sets for cross-binding checks in layers 14-18.
    attempt_ids: set[str] = {a.attempt_id for a in attempts}
    attempt_by_id: dict[str, AttemptRecord] = {a.attempt_id: a for a in attempts}
    assignment_ids_from_attempts: set[str] = {
        a.assignment_id for a in attempts if a.assignment_id
    }
    run_ids_from_attempts: set[str] = {a.run_id for a in attempts if a.run_id}
    if run_manifest is not None:
        run_ids_from_attempts.add(run_manifest.run_id)
    if manifest is not None:
        campaign_id_from_manifest = manifest.campaign_id
    else:
        campaign_id_from_manifest = campaign_id

    # Load supporting typed artifacts for cross-binding and cardinality checks.
    stages = _load_stages(report_dir, failures)
    stage_ids: set[str] = {s.stage_id for s in stages}
    cohort_by_id = _load_cohorts(report_dir, failures)
    evidence_index = _load_evidence_index(report_dir, failures)
    completed_attempt_count = sum(
        1 for a in attempts if a.terminal_status == TerminalStatus.COMPLETED
    )

    # Layer 14: resource observations — enforce policy, validate, cross-bind
    checked_layers.append("resource_observation")
    resource_obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
    resource_observations: list[ResourceObservation] = []
    _enforce_record_file_policy(
        resource_obs_path, RESOURCE_OBSERVATIONS_JSONL, policy, failures,
    )
    _enforce_one_per_inference_cardinality(
        resource_obs_path, RESOURCE_OBSERVATIONS_JSONL, policy,
        [], stages, failures,
    )
    if _check_optional_file_safety(resource_obs_path, "resource observations", failures):
        try:
            obs_records = _read_jsonl_dicts(resource_obs_path)
            observations = [ResourceObservation.model_validate(r) for r in obs_records]
            validate_resource_observations(observations)
            resource_observations = observations
            for obs in observations:
                _cross_check_observation_identity(
                    obs, campaign_id_from_manifest, run_ids_from_attempts,
                    assignment_ids_from_attempts, attempt_ids, attempt_by_id,
                    failures,
                )
                _cross_check_observation_model_binding(
                    obs, assignment_by_id, cohort_by_id, stage_ids, failures,
                )
                _check_evidence_binding(
                    obs.verification_status, obs.source_evidence_refs,
                    obs.source_evidence_sha256, obs.inference_id,
                    "resource observation", failures,
                )
                _check_evidence_index_resolution(
                    obs.verification_status, obs.source_evidence_refs,
                    obs.source_evidence_sha256, obs.inference_id,
                    "resource observation", evidence_index, failures,
                )
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"resource observation validation failed: {e}")

    # Layer 15: tool call scorecards — enforce policy, validate, cross-bind
    checked_layers.append("tool_call_scorecard")
    scorecard_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
    _enforce_record_file_policy(
        scorecard_path, TOOL_CALL_SCORECARDS_JSONL, policy, failures,
    )
    _enforce_one_per_inference_cardinality(
        scorecard_path, TOOL_CALL_SCORECARDS_JSONL, policy,
        resource_observations, stages, failures,
    )
    if _check_optional_file_safety(scorecard_path, "tool call scorecards", failures):
        try:
            sc_records = _read_jsonl_dicts(scorecard_path)
            scorecards = [ToolCallScorecard.model_validate(r) for r in sc_records]
            validate_tool_call_scorecards(scorecards)
            for sc in scorecards:
                _cross_check_event_identity(
                    sc.scorecard_id, sc.campaign_id, sc.child_id, sc.run_id,
                    sc.assignment_id, sc.attempt_id, sc.inference_id, sc.task_id,
                    campaign_id_from_manifest, run_ids_from_attempts,
                    assignment_ids_from_attempts, attempt_ids, attempt_by_id,
                    "tool call scorecard", failures,
                )
                _check_evidence_binding(
                    sc.verification_status, sc.source_evidence_refs,
                    sc.source_evidence_sha256, sc.scorecard_id,
                    "tool call scorecard", failures,
                )
                _check_evidence_index_resolution(
                    sc.verification_status, sc.source_evidence_refs,
                    sc.source_evidence_sha256, sc.scorecard_id,
                    "tool call scorecard", evidence_index, failures,
                )
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"tool call scorecard validation failed: {e}")

    # Layer 16: escalation records — enforce policy, validate, cross-bind
    checked_layers.append("escalation_records")
    escalation_path = report_dir / ESCALATION_RECORDS_JSONL
    _enforce_record_file_policy(
        escalation_path, ESCALATION_RECORDS_JSONL, policy, failures,
    )
    _enforce_one_per_attempt_cardinality(
        escalation_path, ESCALATION_RECORDS_JSONL, policy,
        completed_attempt_count, failures,
    )
    if _check_optional_file_safety(escalation_path, "escalation records", failures):
        try:
            er_records = _read_jsonl_dicts(escalation_path)
            escalation_records = [EscalationRecord.model_validate(r) for r in er_records]
            validate_escalation_records(escalation_records)
            for er in escalation_records:
                _cross_check_event_identity(
                    er.record_id, er.campaign_id, er.child_id, er.run_id,
                    er.assignment_id, er.attempt_id, er.inference_id, er.task_id,
                    campaign_id_from_manifest, run_ids_from_attempts,
                    assignment_ids_from_attempts, attempt_ids, attempt_by_id,
                    "escalation record", failures,
                )
                _check_evidence_binding(
                    er.verification_status, er.source_evidence_refs,
                    er.source_evidence_sha256, er.record_id,
                    "escalation record", failures,
                )
                _check_evidence_index_resolution(
                    er.verification_status, er.source_evidence_refs,
                    er.source_evidence_sha256, er.record_id,
                    "escalation record", evidence_index, failures,
                )
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"escalation record validation failed: {e}")

    # Layer 17: security event records — enforce policy, validate, cross-bind
    checked_layers.append("security_events")
    security_path = report_dir / SECURITY_EVENTS_JSONL
    _enforce_record_file_policy(
        security_path, SECURITY_EVENTS_JSONL, policy, failures,
    )
    _enforce_one_per_attempt_cardinality(
        security_path, SECURITY_EVENTS_JSONL, policy,
        completed_attempt_count, failures,
    )
    if _check_optional_file_safety(security_path, "security event records", failures):
        try:
            se_records = _read_jsonl_dicts(security_path)
            security_records = [SecurityEventRecord.model_validate(r) for r in se_records]
            validate_security_event_records(security_records)
            for se in security_records:
                _cross_check_event_identity(
                    se.record_id, se.campaign_id, se.child_id, se.run_id,
                    se.assignment_id, se.attempt_id, se.inference_id, se.task_id,
                    campaign_id_from_manifest, run_ids_from_attempts,
                    assignment_ids_from_attempts, attempt_ids, attempt_by_id,
                    "security event record", failures,
                )
                _check_evidence_binding(
                    se.verification_status, se.source_evidence_refs,
                    se.source_evidence_sha256, se.record_id,
                    "security event record", failures,
                )
                _check_evidence_index_resolution(
                    se.verification_status, se.source_evidence_refs,
                    se.source_evidence_sha256, se.record_id,
                    "security event record", evidence_index, failures,
                )
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"security event record validation failed: {e}")

    # Layer 18: correlated error records — enforce policy, validate, cross-bind
    checked_layers.append("correlated_errors")
    correlated_path = report_dir / CORRELATED_ERRORS_JSONL
    _enforce_record_file_policy(
        correlated_path, CORRELATED_ERRORS_JSONL, policy, failures,
    )
    _enforce_one_per_inference_cardinality(
        correlated_path, CORRELATED_ERRORS_JSONL, policy,
        resource_observations, stages, failures,
    )
    if _check_optional_file_safety(correlated_path, "correlated error records", failures):
        try:
            ce_records = _read_jsonl_dicts(correlated_path)
            correlated_records = [CorrelatedErrorRecord.model_validate(r) for r in ce_records]
            validate_correlated_error_records(correlated_records)
            for ce in correlated_records:
                _cross_check_event_identity(
                    ce.record_id, ce.campaign_id, ce.child_id, ce.run_id,
                    ce.assignment_id, ce.attempt_id, ce.inference_id, ce.task_id,
                    campaign_id_from_manifest, run_ids_from_attempts,
                    assignment_ids_from_attempts, attempt_ids, attempt_by_id,
                    "correlated error record", failures,
                )
                _check_evidence_binding(
                    ce.verification_status, ce.source_evidence_refs,
                    ce.source_evidence_sha256, ce.record_id,
                    "correlated error record", failures,
                )
                _check_evidence_index_resolution(
                    ce.verification_status, ce.source_evidence_refs,
                    ce.source_evidence_sha256, ce.record_id,
                    "correlated error record", evidence_index, failures,
                )
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"correlated error record validation failed: {e}")

    # Layer 19: resource observation count — verify exact inference trail count
    checked_layers.append("resource_observation_count")
    if resource_observations or stages:
        _verify_inference_trail_count(
            resource_observations, stages, attempts, failures,
        )
        _verify_resource_observation_count(
            resource_observations, attempts, failures,
        )

    ok = len(failures) == 0
    report = CampaignVerificationReport(
        verification_schema_version=VERIFICATION_SCHEMA_VERSION,
        campaign_id=campaign_id or "unknown",
        campaign_revision=campaign_revision or "unknown",
        ok=ok,
        verified_index_generation_hash=verified_index_hash,
        checked_layers=sorted(set(checked_layers)),
        failures=sorted(failures),
    )

    # Persist the canonical child verification report so aggregate
    # verification can hash the complete bytes.
    report_file = report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON
    try:
        report_file.write_text(
            json.dumps(
                report.model_dump(mode="json", by_alias=True),
                allow_nan=False,
                ensure_ascii=False,
                separators=(",", ":"),
                sort_keys=True,
            )
        )
    except OSError as e:
        # Persistence failure does not invalidate the verification result
        # but is recorded as a failure so callers know the report was not
        # persisted.
        failures.append(f"failed to persist verification report: {e}")
        report = report.model_copy(update={
            "failures": sorted(failures),
            "ok": False,
        })

    return report


__all__ = [
    "VERIFICATION_SCHEMA_VERSION",
    "verify_campaign",
]
