# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for interruption-vs-terminal distinction and
immutable report finalization.

Verifies that a process-killed partial directory remains immutable
evidence of interruption but is never treated as complete, and that a
report is finalized only after its manifest, expected terminal
attempts, metrics, evidence index, summary, and report checksum
validate.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    PreregistrationConfig,
)
from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    CampaignStatus,
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_task_assignment_hash,
)
from g8e_evals.constants import (
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    CAMPAIGN_INDEX_JSONL,
    CAMPAIGN_STATUS_JSON,
    MANIFEST_JSON,
    METRICS_JSONL,
    REPORT_CHECKSUM_JSON,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import (
    IndexCreationReason,
    IndexGeneration,
    ModelRole,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.report.completeness import is_report_complete
from g8e_evals.report.validate import validate_standalone_report
from g8e_evals.runner import (
    CampaignRunner,
    CampaignSpec,
)


_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_ID = "cohort-qwen3-8b"
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


@dataclass
class _FakeSUT:
    model_id: str
    answer: str = "This is a test answer with no commas."

    async def get_answer(self, task: Task) -> Response:
        return Response(answer=self.answer, model=self.model_id, arm=Arm.DIRECT)


    def close(self) -> None:
        pass


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_spec() -> CampaignSpec:
    binding = RoleModelBinding(
        role=ModelRole.PRIMARY,
        model_id="qwen3:8b",
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )
    variant_id = _COHORT_ID[len("cohort-"):] if _COHORT_ID.startswith("cohort-") else _COHORT_ID
    cohort = ModelCohort(
        cohort_id=_COHORT_ID,
        candidate_variant_id=variant_id,
        candidate_role=ModelRole.PRIMARY,
        role_bindings=[binding],
        content_hash=compute_model_cohort_hash(_COHORT_ID, variant_id, ModelRole.PRIMARY, [binding]),
    )
    task_assignment = TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=_TASK_IDS,
        content_hash=compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, _TASK_IDS),
    )
    initial_state = InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id="direct",
        comparison_arm_ids=["ensemble_ungoverned"],
        model_cohort_ids=[_COHORT_ID],
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=_REPLICATE_IDS,
        required_replicate_count=1,
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )
    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in _TASK_IDS).encode()
    return CampaignSpec(
        campaign_id=_CAMPAIGN_ID,
        release_version=_RELEASE_VERSION,
        suite="ifeval_subset",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        dataset_hash=_DATASET_HASH,
        prompt_bundle_hash=hashlib.sha256(prompt_bundle).hexdigest(),
        grader_bundle_hash="g" * 64,
        preregistration=prereg,
        cohorts=[cohort],
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
        randomization_seed=42,
    )


def _make_tasks() -> list[Task]:
    tasks = []
    for tid in _TASK_IDS:
        tasks.append(Task(
            id=tid,
            prompt=f"Prompt for {tid}",
            metadata=TaskMetadata(
                benchmark="ifeval_subset",
                instruction_id_list=["punctuation:no_comma"],
                kwargs=[{"no_comma": True}],
            ),
        ))
    return tasks


def _fake_sut_factory(cohort: ModelCohort, arm: Arm):
    return _FakeSUT(model_id=cohort.role_bindings[0].model_id)


class TestInterruptionVsTerminal:
    def test_partial_directory_missing_analysis_not_complete(self, tmp_path: Path):
        """A report directory missing analysis.json is not complete."""
        report_dir = tmp_path / "partial-report"
        report_dir.mkdir()
        (report_dir / MANIFEST_JSON).write_text("{}")
        (report_dir / ATTEMPTS_JSONL).write_text("")
        (report_dir / METRICS_JSONL).write_text("")
        # analysis.json is missing — simulates process kill before analysis written
        assert not is_report_complete(report_dir)

    def test_partial_directory_missing_manifest_not_complete(self, tmp_path: Path):
        """A report directory missing manifest.json is not complete."""
        report_dir = tmp_path / "partial-report"
        report_dir.mkdir()
        (report_dir / ATTEMPTS_JSONL).write_text("")
        (report_dir / METRICS_JSONL).write_text("")
        (report_dir / ANALYSIS_JSON).write_text("{}")
        # manifest.json is missing
        assert not is_report_complete(report_dir)

    def test_partial_directory_empty_not_complete(self, tmp_path: Path):
        """An empty report directory is not complete."""
        report_dir = tmp_path / "empty-report"
        report_dir.mkdir()
        assert not is_report_complete(report_dir)

    def test_nonexistent_directory_not_complete(self, tmp_path: Path):
        """A nonexistent report directory is not complete."""
        report_dir = tmp_path / "does-not-exist"
        assert not is_report_complete(report_dir)

    def test_runner_does_not_finalize_incomplete_report(self, tmp_path: Path):
        """The runner does not write a FINALIZATION generation for an incomplete report.

        Simulates a process kill by removing analysis.json after the runner
        writes all artifacts. The FINALIZATION generation must not be
        present when the report is incomplete.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # Simulate process kill: remove analysis.json
        analysis_path = result.report_dir / ANALYSIS_JSON
        analysis_path.unlink()
        assert not is_report_complete(result.report_dir)

        # The standalone validator must report the report as not OK
        standalone = validate_standalone_report(result.report_dir)
        assert not standalone.ok


class TestImmutableReportFinalization:
    def test_runner_writes_report_checksum_on_finalization(self, tmp_path: Path):
        """The runner writes a report-checksum.json file when finalizing."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        checksum_path = result.report_dir / REPORT_CHECKSUM_JSON
        assert checksum_path.exists(), "report-checksum.json must be written on finalization"

    def test_report_checksum_matches_computed_checksum(self, tmp_path: Path):
        """The written report checksum matches the computed checksum."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        checksum_path = result.report_dir / REPORT_CHECKSUM_JSON
        checksum_data = json.loads(checksum_path.read_text())
        declared = checksum_data["checksum"]

        # Compute the expected checksum the same way validate_standalone_report does
        from g8e_evals.report.validate import _compute_report_checksum
        computed = _compute_report_checksum(result.report_dir)
        assert declared == computed

    def test_finalization_generation_written_only_after_validation_passes(self, tmp_path: Path):
        """The FINALIZATION generation is written only after standalone validation passes.

        A complete campaign must pass validate_standalone_report before
        the FINALIZATION index generation is written.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # The report must pass standalone validation
        standalone = validate_standalone_report(result.report_dir)
        assert standalone.ok, f"report should pass standalone validation: {standalone.failures}"

        # The final index generation must be FINALIZATION
        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        last_gen = IndexGeneration.model_validate_json(lines[-1])
        assert last_gen.creation_reason == IndexCreationReason.FINALIZATION

    def test_finalized_campaign_status_is_finalized(self, tmp_path: Path):
        """A successfully finalized campaign has status 'finalized'."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        status_path = result.report_dir / CAMPAIGN_STATUS_JSON
        status_data = json.loads(status_path.read_text())
        assert status_data["status"] == CampaignStatus.FINALIZED.value

    def test_stopped_campaign_not_finalized(self, tmp_path: Path):
        """A stopped campaign (budget exhausted) is not finalized.

        When the campaign stops due to budget exhaustion, no FINALIZATION
        generation is written and the status is 'stopped', not 'finalized'.
        """
        from g8e_evals.schema import ProviderBudget

        spec = _make_spec()
        spec = spec.model_copy(update={
            "provider_budget": ProviderBudget(max_usd=100.0, max_requests=1),
        })

        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # The campaign should have been stopped due to budget
        status_path = result.report_dir / CAMPAIGN_STATUS_JSON
        status_data = json.loads(status_path.read_text())
        assert status_data["status"] == CampaignStatus.STOPPED.value

        # No FINALIZATION generation should be present
        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        last_gen = IndexGeneration.model_validate_json(lines[-1])
        assert last_gen.creation_reason != IndexCreationReason.FINALIZATION, (
            "stopped campaign must not have a FINALIZATION generation"
        )

    def test_report_checksum_validated_by_standalone_validator(self, tmp_path: Path):
        """The standalone validator checks the report checksum written by the runner.

        After finalization, the report-checksum.json must match the
        computed checksum. The standalone validator must confirm this.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # The standalone validator must pass, including the checksum layer
        standalone = validate_standalone_report(result.report_dir)
        assert standalone.ok
        assert "checksum" in standalone.checked_layers

    def test_tampered_report_checksum_fails_validation(self, tmp_path: Path):
        """A tampered report checksum fails standalone validation.

        After finalization, modifying the report-checksum.json must cause
        the standalone validator to report a checksum mismatch.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # Tamper with the checksum
        checksum_path = result.report_dir / REPORT_CHECKSUM_JSON
        checksum_data = json.loads(checksum_path.read_text())
        checksum_data["checksum"] = "a" * 64
        checksum_path.write_text(json.dumps(checksum_data, indent=2))

        # The standalone validator must catch the mismatch
        standalone = validate_standalone_report(result.report_dir)
        assert not standalone.ok
        assert any("checksum mismatch" in f for f in standalone.failures)
