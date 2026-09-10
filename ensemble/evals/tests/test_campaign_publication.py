# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for campaign-aware eval publication (R6).

Verifies that ``build_publication_request`` on a multi-arm, multi-cohort
campaign analysis produces a publication request with ``arm_ids`` (list),
``campaign_id``, ``model_cohort_ids``, ``assignment_count``, and
cohort-stratified metric summaries (each carrying ``model_cohort_id`` and
``arm_id``). The current single-arm contract rejects multi-arm analyses at
``_resolve_arm_id`` and the wire model has no ``arm_ids`` field; these tests
demonstrate that defect and drive the campaign-aware protocol evolution.

Also verifies cross-language contract parity: the Python request field names
match the Go struct JSON tags byte-for-byte, including the new campaign-aware
fields.
"""

from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.arms import Arm
from g8e_evals.bundle import build_publication_request
from g8e_evals.bundle.manifest import ArtifactType, BundleArtifactEntry, BundleManifest, PrivacyClass
from g8e_evals.bundle.verify import VerificationReport
from g8e_evals.campaign import (
    CampaignAssignment,
    CampaignManifest,
    ExecutionSchedule,
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_assignment_id,
    compute_campaign_manifest_hash,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_retry_policy_hash,
    compute_schedule_hash,
    compute_task_assignment_hash,
)
from g8e_evals.constants import (
    ANALYSIS_INPUT_JSON,
    ANALYSIS_JSON,
)
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.schema import (
    AttemptRecord,
    GraderReference,
    MetricObservation,
    RunManifest,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_RUN_ID = "run-campaign-pub-1"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b", "cohort-granite-3.3-8b"]
_ARM_IDS = ["direct", "ensemble_ungoverned"]
_REPLICATE_IDS = ["replicate-1", "replicate-2"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_METRIC_ID = "ifeval_subset_verifier"
_METRIC_VERSION = "1.0.0"
_TS = datetime(2026, 1, 1, tzinfo=UTC)


# ---------------------------------------------------------------------------
# Helpers (mirrors test_cohort_stratified_analysis.py fixtures)
# ---------------------------------------------------------------------------


def _make_sampling() -> SamplingSettings:
    return SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42)


def _make_role_binding(role: str = "primary", model_id: str = "qwen3:8b") -> RoleModelBinding:
    return RoleModelBinding(
        role=role,
        model_id=model_id,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=_make_sampling(),
        timeout_seconds=120.0,
        seed_capable=True,
    )


def _make_cohort(cohort_id: str = "cohort-qwen3-8b", model_id: str = "qwen3:8b") -> ModelCohort:
    bindings = [_make_role_binding("primary", model_id)]
    ch = compute_model_cohort_hash(cohort_id, bindings)
    return ModelCohort(cohort_id=cohort_id, role_bindings=bindings, content_hash=ch)


def _make_task_assignment(task_ids: list[str] | None = None) -> TaskAssignmentManifest:
    if task_ids is None:
        task_ids = _TASK_IDS
    ch = compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, task_ids)
    return TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=task_ids,
        content_hash=ch,
    )


def _make_initial_state() -> InitialStateAssignmentManifest:
    ch = compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH)
    return InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=ch,
    )


def _make_all_assignments() -> list[CampaignAssignment]:
    assignments: list[CampaignAssignment] = []
    pos = 0
    for task_id in _TASK_IDS:
        for cohort_id in _COHORT_IDS:
            for arm_id in _ARM_IDS:
                for replicate_id in _REPLICATE_IDS:
                    aid = compute_assignment_id(
                        _CAMPAIGN_ID, task_id, cohort_id, arm_id, _INITIAL_STATE_ID, replicate_id,
                    )
                    assignments.append(CampaignAssignment(
                        campaign_id=_CAMPAIGN_ID,
                        assignment_id=aid,
                        task_id=task_id,
                        model_cohort_id=cohort_id,
                        arm_id=arm_id,
                        initial_state_assignment_id=_INITIAL_STATE_ID,
                        replicate_id=replicate_id,
                        schedule_position=pos,
                    ))
                    pos += 1
    return assignments


def _make_schedule(assignments: list[CampaignAssignment]) -> ExecutionSchedule:
    ordered_ids: list[str] = [""] * len(assignments)
    for a in assignments:
        ordered_ids[a.schedule_position] = a.assignment_id
    ch = compute_schedule_hash("schedule-v1", _CAMPAIGN_ID, 42, "shuffle", "1.0.0", ordered_ids)
    return ExecutionSchedule(
        schedule_id="schedule-v1",
        campaign_id=_CAMPAIGN_ID,
        randomization_seed=42,
        algorithm_id="shuffle",
        algorithm_version="1.0.0",
        ordered_assignment_ids=ordered_ids,
        content_hash=ch,
    )


def _make_retry_policy() -> RetryPolicy:
    return RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"])


def _make_campaign_manifest() -> CampaignManifest:
    cohort_hashes = [_make_cohort(c).content_hash for c in _COHORT_IDS]
    ch = compute_campaign_manifest_hash(
        campaign_id=_CAMPAIGN_ID,
        campaign_version="1.0.0",
        release_version=_RELEASE_VERSION,
        preregistration_hash="a" * 64,
        cohort_hashes=cohort_hashes,
        task_assignment_hash=_make_task_assignment().content_hash,
        initial_state_assignment_hash=_make_initial_state().content_hash,
        schedule_hash=_make_schedule(_make_all_assignments()).content_hash,
        retry_policy_hash=compute_retry_policy_hash(1, ["infrastructure_failed"]),
        metric_registry_hash="m" * 64,
        release_metric_set_hash="r" * 64,
        threshold_authority_hash="t" * 64,
        missingness_authority_hash="n" * 64,
        provider_budget_hash="p" * 64,
        source_build_provenance_hash="s" * 64,
        claim_exclusion_hash="c" * 64,
    )
    return CampaignManifest(
        campaign_id=_CAMPAIGN_ID,
        campaign_version="1.0.0",
        release_version=_RELEASE_VERSION,
        preregistration_hash="a" * 64,
        cohort_hashes=cohort_hashes,
        task_assignment_hash=_make_task_assignment().content_hash,
        initial_state_assignment_hash=_make_initial_state().content_hash,
        schedule_hash=_make_schedule(_make_all_assignments()).content_hash,
        retry_policy_hash=compute_retry_policy_hash(1, ["infrastructure_failed"]),
        metric_registry_hash="m" * 64,
        release_metric_set_hash="r" * 64,
        threshold_authority_hash="t" * 64,
        missingness_authority_hash="n" * 64,
        provider_budget_hash="p" * 64,
        source_build_provenance_hash="s" * 64,
        claim_exclusion_hash="c" * 64,
        content_hash=ch,
    )


def _make_task(task_id: str = "task-1001") -> TaskDefinition:
    return TaskDefinition(
        task_id=task_id,
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="IFEVAL",
        compatible_arms=[Arm.DIRECT, Arm.ENSEMBLE_UNGOVERNED],
        graders=[GraderReference(grader_id=_METRIC_ID, grader_version=_METRIC_VERSION)],
    )


def _make_attempt(
    attempt_id: str,
    assignment_id: str,
    task_id: str,
    arm_id: Arm,
    cohort_id: str,
    replicate_id: str,
    terminal_status: TerminalStatus = TerminalStatus.COMPLETED,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=task_id,
        arm_id=arm_id,
        model_cohort_id=cohort_id,
        state_snapshot_hash=_NO_STATE_HASH,
        replicate_id=replicate_id,
        assignment_id=assignment_id,
        terminal_status=terminal_status,
    )


def _make_metric_obs(
    attempt_id: str,
    task_id: str,
    arm_id: Arm,
    value: float | None = 1.0,
) -> MetricObservation:
    definition = DEFAULT_METRIC_REGISTRY.get(_METRIC_ID, _METRIC_VERSION)
    return MetricObservation(
        metric_id=_METRIC_ID,
        metric_version=_METRIC_VERSION,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        arm_id=arm_id,
        task_id=task_id,
        value=value,
        unit=definition.unit,
        eligible=True,
        verification_status=VerificationStatus.VERIFIED,
        grader_class=definition.grader_class,
    )


def _make_campaign_analysis_input() -> AnalysisInputRecord:
    """Build a campaign-shaped analysis input with 2 cohorts, 2 arms, 2 tasks, 2 replicates."""
    assignments = _make_all_assignments()
    attempts: list[AttemptRecord] = []
    observations: list[MetricObservation] = []
    for i, a in enumerate(assignments):
        arm = Arm(a.arm_id)
        att_id = f"att-{i}"
        attempts.append(_make_attempt(
            attempt_id=att_id,
            assignment_id=a.assignment_id,
            task_id=a.task_id,
            arm_id=arm,
            cohort_id=a.model_cohort_id,
            replicate_id=a.replicate_id,
        ))
        observations.append(_make_metric_obs(
            attempt_id=att_id,
            task_id=a.task_id,
            arm_id=arm,
            value=1.0,
        ))
    return AnalysisInputRecord(
        run_id=_RUN_ID,
        release_version=_RELEASE_VERSION,
        tasks=[_make_task(tid) for tid in _TASK_IDS],
        attempts=attempts,
        metric_observations=observations,
        campaign_manifest=_make_campaign_manifest(),
        model_cohorts=[_make_cohort(c) for c in _COHORT_IDS],
        campaign_assignments=assignments,
        execution_schedule=_make_schedule(assignments),
        retry_policy=_make_retry_policy(),
    )


def _write_campaign_bundle_dir(tmp_path: Path) -> tuple[Path, AnalysisInputRecord]:
    """Write a campaign-shaped analysis-input.json and analysis.json to a bundle dir.

    Returns (bundle_dir, analysis_input). The bundle dir also contains a
    minimal public artifact so the download catalog is non-empty.
    """
    bundle_dir = tmp_path / "bundle"
    bundle_dir.mkdir(parents=True, exist_ok=True)

    analysis_input = _make_campaign_analysis_input()
    analysis = compute_canonical_analysis_from_record(analysis_input)

    (bundle_dir / ANALYSIS_INPUT_JSON).write_text(canonical_model_json(analysis_input))
    (bundle_dir / ANALYSIS_JSON).write_text(canonical_model_json(analysis))

    # Write a minimal public artifact so the download catalog is non-empty.
    public_artifact_path = bundle_dir / "analysis.json"
    public_artifact_path.write_text(canonical_model_json(analysis))

    return bundle_dir, analysis_input


def _make_run_manifest() -> RunManifest:
    return RunManifest(
        run_id=_RUN_ID,
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        created_at=_TS,
    )


def _make_bundle_manifest(bundle_dir: Path) -> BundleManifest:
    """Build a minimal bundle manifest with one public artifact."""
    public_content = (bundle_dir / "analysis.json").read_bytes()
    import hashlib
    sha = hashlib.sha256(public_content).hexdigest()
    entry = BundleArtifactEntry(
        path="analysis.json",
        media_type="application/json",
        privacy_class=PrivacyClass.PUBLIC,
        sha256=sha,
        byte_length=len(public_content),
        artifact_type=ArtifactType.ANALYSIS_JSON,
        record_count=1,
    )
    return BundleManifest(
        bundle_id="bundle-campaign-1",
        run_id=_RUN_ID,
        release_version=_RELEASE_VERSION,
        created_at=_TS,
        artifacts=[entry],
    )


def _make_verification_report() -> VerificationReport:
    """Build a minimal passing verification report for the campaign bundle."""
    from g8e_evals.bundle.verify import LayerResult, VerificationLayer
    layers = [
        LayerResult(layer=vl, passed=True, failure_count=0)
        for vl in VerificationLayer
    ]
    return VerificationReport(
        bundle_id="bundle-campaign-1",
        run_id=_RUN_ID,
        release_version=_RELEASE_VERSION,
        verified_at=_TS,
        ok=True,
        layers=layers,
        failures=[],
    )


# ---------------------------------------------------------------------------
# Campaign-aware publication request tests
# ---------------------------------------------------------------------------


class TestCampaignAwarePublicationRequest:
    """build_publication_request on a multi-arm campaign produces campaign-aware fields."""

    def test_multi_arm_campaign_produces_arm_ids_list(self, tmp_path: Path) -> None:
        """The request carries arm_ids (list) for a multi-arm campaign, not a single arm_id."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        # The request must carry arm_ids as a list with both campaign arms.
        assert hasattr(request, "arm_ids")
        assert set(request.arm_ids) == set(_ARM_IDS)

    def test_multi_arm_campaign_produces_campaign_id(self, tmp_path: Path) -> None:
        """The request carries the campaign_id from the campaign manifest."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        assert hasattr(request, "campaign_id")
        assert request.campaign_id == _CAMPAIGN_ID

    def test_multi_arm_campaign_produces_model_cohort_ids(self, tmp_path: Path) -> None:
        """The request carries model_cohort_ids from the campaign cohorts."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        assert hasattr(request, "model_cohort_ids")
        assert set(request.model_cohort_ids) == set(_COHORT_IDS)

    def test_multi_arm_campaign_produces_assignment_count(self, tmp_path: Path) -> None:
        """The request carries assignment_count from the analysis input summary."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        assert hasattr(request, "assignment_count")
        # 2 tasks x 2 cohorts x 2 arms x 2 replicates = 16 assignments.
        assert request.assignment_count == 16

    def test_metric_summaries_carry_model_cohort_id_and_arm_id(self, tmp_path: Path) -> None:
        """Each metric summary carries model_cohort_id and arm_id for cohort stratification."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        assert len(request.metrics) > 0
        for metric in request.metrics:
            assert hasattr(metric, "model_cohort_id")
            assert hasattr(metric, "arm_id")
            assert metric.model_cohort_id in _COHORT_IDS
            assert metric.arm_id in _ARM_IDS

    def test_metric_summaries_cover_all_cohort_arm_strata(self, tmp_path: Path) -> None:
        """Metric summaries cover every (cohort, arm) stratum in the campaign."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        # Each (cohort, arm) stratum produces at least one metric summary.
        strata = {(m.model_cohort_id, m.arm_id) for m in request.metrics}
        expected = {(c, a) for c in _COHORT_IDS for a in _ARM_IDS}
        assert strata == expected

    def test_request_has_no_single_arm_id_field(self, tmp_path: Path) -> None:
        """The campaign-aware request must not carry a single arm_id scalar."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        dumped = request.model_dump(mode="json", by_alias=True, exclude_none=True)
        assert "arm_id" not in dumped, "campaign-aware request must not carry single arm_id"
        assert "arm_ids" in dumped

    def test_request_has_no_model_id_or_model_provider(self, tmp_path: Path) -> None:
        """The campaign-aware request must not carry single model_id or model_provider scalars."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        dumped = request.model_dump(mode="json", by_alias=True, exclude_none=True)
        assert "model_id" not in dumped, "campaign-aware request must not carry single model_id"
        assert "model_provider" not in dumped, "campaign-aware request must not carry single model_provider"


class TestCampaignAwareCrossLanguageContract:
    """Python and Go campaign-aware request field names match byte-for-byte."""

    def test_python_request_field_names_match_go_json_tags(self, tmp_path: Path) -> None:
        """The Python model field names match the Go struct JSON tags including campaign fields."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )
        dumped = request.model_dump(mode="json", by_alias=True, exclude_none=False)

        # These are the exact JSON field names from the Go struct tags in
        # internal/models/eval_bundle.go ObserveProducerEvalPublicationRequest
        # after the campaign-aware evolution: arm_id -> arm_ids, model_id and
        # model_provider removed, campaign_id/model_cohort_ids/assignment_count
        # added.
        expected_fields = {
            "schema_version", "bundle_id", "run_id", "release_version",
            "suite_id", "suite_version",
            "campaign_id", "arm_ids", "model_cohort_ids", "assignment_count",
            "receipt_count", "assigned_tasks", "terminal_attempts",
            "metrics", "verification_report", "bundle_manifest",
            "downloads", "web_session_id", "cli_session_id",
        }
        assert set(dumped.keys()) == expected_fields

    def test_python_metric_summary_field_names_match_go_json_tags(self, tmp_path: Path) -> None:
        """EvalMetricSummary field names match Go struct tags including cohort and arm fields."""
        bundle_dir, _ = _write_campaign_bundle_dir(tmp_path)
        analysis = compute_canonical_analysis_from_record(_make_campaign_analysis_input())
        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        assert len(request.metrics) > 0
        dumped = request.metrics[0].model_dump(mode="json", by_alias=True, exclude_none=False)
        # Go struct tags for EvalMetricSummary after campaign-aware evolution:
        # model_cohort_id and arm_id added.
        expected_fields = {
            "schema_version", "metric_id", "metric_version",
            "value", "unit", "eligible", "denominator",
            "verification_status", "recorded_at",
            "model_cohort_id", "arm_id",
        }
        assert set(dumped.keys()) == expected_fields


class TestSingleArmDiagnosticBackwardCompatibility:
    """A single-arm diagnostic analysis without campaign records still publishes."""

    def test_single_arm_diagnostic_produces_arm_ids_with_one_entry(self, tmp_path: Path) -> None:
        """A non-campaign single-arm analysis produces arm_ids with one entry and empty campaign_id."""
        from g8e_evals.analysis.input import AnalysisInputRecord
        from g8e_evals.schema import (
            AttemptRecord,
            MetricObservation,
            TaskDefinition,
            TerminalStatus,
            VerificationStatus,
        )

        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir(parents=True, exist_ok=True)

        task = TaskDefinition(
            task_id="task-1",
            suite_id="ifeval_subset",
            suite_version="1.0.0",
            prompt_hash="0" * 64,
        )
        attempt = AttemptRecord(
            attempt_id="attempt-1",
            run_id=_RUN_ID,
            task_id="task-1",
            arm_id=Arm.DOCTRINE,
            terminal_status=TerminalStatus.COMPLETED,
            started_at=_TS,
            ended_at=_TS,
        )
        definition = DEFAULT_METRIC_REGISTRY.get("receipt_integrity", "1.0.0")
        metric = MetricObservation(
            metric_id="receipt_integrity",
            metric_version="1.0.0",
            attempt_id="attempt-1",
            run_id=_RUN_ID,
            task_id="task-1",
            arm_id=Arm.DOCTRINE,
            value=1.0,
            unit=definition.unit,
            eligible=True,
            verification_status=VerificationStatus.VERIFIED,
            grader_class=definition.grader_class,
        )
        analysis_input = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE_VERSION,
            tasks=[task],
            attempts=[attempt],
            metric_observations=[metric],
        )
        analysis = compute_canonical_analysis_from_record(analysis_input)
        (bundle_dir / ANALYSIS_INPUT_JSON).write_text(canonical_model_json(analysis_input))
        (bundle_dir / ANALYSIS_JSON).write_text(canonical_model_json(analysis))

        run_manifest = _make_run_manifest()
        manifest = _make_bundle_manifest(bundle_dir)
        report = _make_verification_report()

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        # Single-arm diagnostic: arm_ids has one entry, campaign_id is empty.
        assert request.arm_ids == [Arm.DOCTRINE.value]
        assert request.campaign_id == ""
        assert request.model_cohort_ids == []
        assert request.assignment_count == 0
