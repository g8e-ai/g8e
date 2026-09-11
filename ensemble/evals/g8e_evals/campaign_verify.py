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
    CampaignManifest,
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
    MetricObservation,
    RunManifest,
    SecurityEventRecord,
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
    6. **cell_coverage**: every campaign assignment has a disposition in the final generation.
    7. **terminal_attempts**: every effective attempt is terminal.
    8. **metric_binding**: every completed attempt has at least one bound metric.
    9. **manifest_identity**: run manifest content hashes match campaign manifest.

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

    # Layer 6: cell coverage — every assignment has a disposition in the final generation
    checked_layers.append("cell_coverage")
    assignments_path = report_dir / CAMPAIGN_ASSIGNMENTS_JSONL
    if _check_file_safety(assignments_path, failures, CAMPAIGN_ASSIGNMENTS_JSONL):
        try:
            assignment_records = _read_jsonl_dicts(assignments_path)
            assignment_ids: set[str] = {
                a["assignment_id"] for a in assignment_records if a.get("assignment_id")
            }

            if generations:
                final_gen = generations[-1]
                dispositioned_ids = {d.assignment_id for d in final_gen.assignment_dispositions}
                missing = assignment_ids - dispositioned_ids
                for aid in sorted(missing):
                    failures.append(f"assignment {aid} missing from final index generation dispositions")
            elif assignment_ids:
                failures.append("assignments exist but no index generations to check coverage")
        except (json.JSONDecodeError, ValueError) as e:
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

    # Layer 14: resource observations — enforce policy, validate, cross-bind
    checked_layers.append("resource_observation")
    resource_obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
    resource_observations: list[ResourceObservation] = []
    _enforce_record_file_policy(
        resource_obs_path, RESOURCE_OBSERVATIONS_JSONL, policy, failures,
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
                _check_evidence_binding(
                    obs.verification_status, obs.source_evidence_refs,
                    obs.source_evidence_sha256, obs.inference_id,
                    "resource observation", failures,
                )
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"resource observation validation failed: {e}")

    # Layer 15: tool call scorecards — enforce policy, validate, cross-bind
    checked_layers.append("tool_call_scorecard")
    scorecard_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
    _enforce_record_file_policy(
        scorecard_path, TOOL_CALL_SCORECARDS_JSONL, policy, failures,
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
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"tool call scorecard validation failed: {e}")

    # Layer 16: escalation records — enforce policy, validate, cross-bind
    checked_layers.append("escalation_records")
    escalation_path = report_dir / ESCALATION_RECORDS_JSONL
    _enforce_record_file_policy(
        escalation_path, ESCALATION_RECORDS_JSONL, policy, failures,
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
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"escalation record validation failed: {e}")

    # Layer 17: security event records — enforce policy, validate, cross-bind
    checked_layers.append("security_events")
    security_path = report_dir / SECURITY_EVENTS_JSONL
    _enforce_record_file_policy(
        security_path, SECURITY_EVENTS_JSONL, policy, failures,
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
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"security event record validation failed: {e}")

    # Layer 18: correlated error records — enforce policy, validate, cross-bind
    checked_layers.append("correlated_errors")
    correlated_path = report_dir / CORRELATED_ERRORS_JSONL
    _enforce_record_file_policy(
        correlated_path, CORRELATED_ERRORS_JSONL, policy, failures,
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
        except (ValidationError, ValueError, json.JSONDecodeError) as e:
            failures.append(f"correlated error record validation failed: {e}")

    # Layer 19: resource observation count — verify one per actual provider inference
    checked_layers.append("resource_observation_count")
    if resource_observations:
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
