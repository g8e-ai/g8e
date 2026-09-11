# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for campaign indexing.

Verifies that the campaign runner writes append-only index generations
with parent-generation hash, creation reason, complete report
checksums, and assignment disposition. Exactly one effective valid
report exists per publishable assignment. Resume creates a new index
generation rather than mutating the existing one.
"""

from __future__ import annotations

import asyncio
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
    CAMPAIGN_INDEX_JSONL,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import (
    IndexCreationReason,
    validate_index_chain,
    validate_no_duplicate_effective_assignments,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import (
    CampaignRunner,
    CampaignSpec,
)


_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b"]
_ARM_IDS = ["direct", "ensemble_ungoverned"]
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


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_spec() -> CampaignSpec:
    cohort = ModelCohort(
        cohort_id="cohort-qwen3-8b",
        role_bindings=[RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )],
        content_hash=compute_model_cohort_hash("cohort-qwen3-8b", [RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )]),
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
        model_cohort_ids=_COHORT_IDS,
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
    import hashlib
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


class TestCampaignIndexing:
    def test_runner_writes_index_generations(self, tmp_path: Path):
        """The campaign runner writes index generations to the report directory."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        assert index_path.exists(), f"index generations file not found at {index_path}"

    def test_index_generations_form_valid_chain(self, tmp_path: Path):
        """Index generations form a valid parent-hash chain starting from zero."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        assert len(lines) >= 1

        from g8e_evals.index import IndexGeneration
        generations = [IndexGeneration.model_validate_json(line) for line in lines]
        validate_index_chain(generations)

    def test_first_generation_has_zero_parent(self, tmp_path: Path):
        """The first index generation has a zero parent hash."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        from g8e_evals.index import IndexGeneration
        first_gen = IndexGeneration.model_validate_json(lines[0])
        assert first_gen.parent_generation_hash == "0" * 64

    def test_final_generation_has_finalization_reason(self, tmp_path: Path):
        """The final index generation has a finalization creation reason."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        from g8e_evals.index import IndexGeneration
        last_gen = IndexGeneration.model_validate_json(lines[-1])
        assert last_gen.creation_reason == IndexCreationReason.FINALIZATION

    def test_exactly_one_effective_per_assignment(self, tmp_path: Path):
        """Each assignment has at most one effective disposition in the final generation."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        index_path = result.report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        from g8e_evals.index import IndexGeneration
        last_gen = IndexGeneration.model_validate_json(lines[-1])
        validate_no_duplicate_effective_assignments(last_gen)

    def test_resume_creates_new_index_generation(self, tmp_path: Path):
        """Resume creates a new index generation rather than mutating the existing one."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result1 = asyncio.run(runner.run())

        index_path = result1.report_dir / CAMPAIGN_INDEX_JSONL
        lines1 = index_path.read_text().strip().splitlines()

        # Resume
        runner2 = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        runner2._report_dir = result1.report_dir
        asyncio.run(runner2.run())

        lines2 = index_path.read_text().strip().splitlines()
        # Resume should have added at least one new generation
        assert len(lines2) > len(lines1)
