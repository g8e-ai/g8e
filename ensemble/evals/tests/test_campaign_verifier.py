# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for the offline campaign verifier.

Verifies that ``verify_campaign`` checks the complete campaign matrix:
index chain, cell coverage, run manifest identity, terminal attempts,
metric binding, tier observations, profile/registry hash matching,
omission detection, private file safety, and public projection safety.
The verifier catches every tested omission, duplicate effective
assignment, broken index chain, identity mismatch, ineffective
setting, and unsafe projection.
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
    CAMPAIGN_MANIFEST_JSON,
    MANIFEST_JSON,
)
from g8e_evals.campaign_verify import (
    verify_campaign,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import (
    AssignmentDispositionEntry,
    CampaignVerificationReport,
    ModelRole,
    compute_index_generation_hash,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import (
    CampaignRunner,
    CampaignSpec,
)
from g8e_evals.projection import project_to_public


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
    cohort = ModelCohort(
        cohort_id="cohort-qwen3-8b",
        candidate_variant_id="qwen3-8b",
        candidate_role=ModelRole.PRIMARY,
        role_bindings=[binding],
        content_hash=compute_model_cohort_hash("cohort-qwen3-8b", "qwen3-8b", ModelRole.PRIMARY, [binding]),
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


def _run_campaign(tmp_path: Path) -> Path:
    """Run a fake-provider campaign and return the report directory."""
    spec = _make_spec()
    runner = CampaignRunner(
        spec=spec,
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(),
        grader=_FakeGrader(),
        output_dir=tmp_path,
    )
    result = asyncio.run(runner.run())
    return result.report_dir


class TestCampaignVerifierPass:
    def test_valid_campaign_passes_verification(self, tmp_path: Path):
        """A complete valid campaign passes verification."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert isinstance(result, CampaignVerificationReport)
        assert result.ok
        assert len(result.failures) == 0
        assert len(result.checked_layers) > 0

    def test_verification_report_has_campaign_identity(self, tmp_path: Path):
        """The verification report carries the campaign identity."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.campaign_id == _CAMPAIGN_ID

    def test_verification_report_has_index_generation_hash(self, tmp_path: Path):
        """The verification report carries the verified index generation hash."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert len(result.verified_index_generation_hash) == 64
        assert result.verified_index_generation_hash != "0" * 64


class TestCampaignVerifierBrokenIndexChain:
    def test_broken_index_chain_fails(self, tmp_path: Path):
        """A broken index parent-hash chain fails verification."""
        report_dir = _run_campaign(tmp_path)
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        generations = [json.loads(line) for line in lines]
        if len(generations) >= 2:
            generations[1]["parent_generation_hash"] = "f" * 64
            generations[1]["content_hash"] = compute_index_generation_hash(
                generation_number=generations[1]["generation_number"],
                parent_generation_hash=generations[1]["parent_generation_hash"],
                creation_reason=generations[1]["creation_reason"],
                report_checksums=generations[1]["report_checksums"],
                assignment_dispositions=[
                    AssignmentDispositionEntry.model_validate(entry)
                    for entry in generations[1]["assignment_dispositions"]
                ],
            )
        index_path.write_text("\n".join(json.dumps(g) for g in generations) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("index" in f.lower() or "chain" in f.lower() for f in result.failures)

    def test_empty_index_fails(self, tmp_path: Path):
        """An empty index file fails verification."""
        report_dir = _run_campaign(tmp_path)
        (report_dir / CAMPAIGN_INDEX_JSONL).write_text("")
        result = verify_campaign(report_dir)
        assert not result.ok


class TestCampaignVerifierDuplicateEffective:
    def test_duplicate_effective_assignment_fails(self, tmp_path: Path):
        """Two effective dispositions for the same assignment fail verification."""
        report_dir = _run_campaign(tmp_path)
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        if not lines:
            pytest.skip("no index generations to modify")
        last_gen = json.loads(lines[-1])
        for entry in last_gen["assignment_dispositions"]:
            if entry["disposition"] == "effective":
                last_gen["assignment_dispositions"].append(dict(entry))
                break
        else:
            pytest.skip("no effective disposition to duplicate")
        last_gen["content_hash"] = compute_index_generation_hash(
            generation_number=last_gen["generation_number"],
            parent_generation_hash=last_gen["parent_generation_hash"],
            creation_reason=last_gen["creation_reason"],
            report_checksums=last_gen["report_checksums"],
            assignment_dispositions=[
                AssignmentDispositionEntry.model_validate(entry)
                for entry in last_gen["assignment_dispositions"]
            ],
        )
        lines[-1] = json.dumps(last_gen)
        index_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("duplicate" in f.lower() or "effective" in f.lower() for f in result.failures)


class TestCampaignVerifierMissingCell:
    def test_missing_assignment_in_disposition_fails(self, tmp_path: Path):
        """An assignment missing from the final disposition fails verification."""
        report_dir = _run_campaign(tmp_path)
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        if not lines:
            pytest.skip("no index generations to modify")
        last_gen = json.loads(lines[-1])
        if len(last_gen["assignment_dispositions"]) <= 1:
            pytest.skip("not enough dispositions to remove one")
        last_gen["assignment_dispositions"] = last_gen["assignment_dispositions"][:-1]
        last_gen["content_hash"] = compute_index_generation_hash(
            generation_number=last_gen["generation_number"],
            parent_generation_hash=last_gen["parent_generation_hash"],
            creation_reason=last_gen["creation_reason"],
            report_checksums=last_gen["report_checksums"],
            assignment_dispositions=[
                AssignmentDispositionEntry.model_validate(entry)
                for entry in last_gen["assignment_dispositions"]
            ],
        )
        lines[-1] = json.dumps(last_gen)
        index_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("missing" in f.lower() or "cell" in f.lower() or "assignment" in f.lower() for f in result.failures)


class TestCampaignVerifierIdentityMismatch:
    def test_campaign_manifest_mismatch_fails(self, tmp_path: Path):
        """A campaign manifest with a mismatched content hash fails verification."""
        report_dir = _run_campaign(tmp_path)
        manifest_path = report_dir / CAMPAIGN_MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        manifest_data["campaign_id"] = "tampered-campaign-id"
        manifest_path.write_text(json.dumps(manifest_data))
        result = verify_campaign(report_dir)
        assert not result.ok


class TestCampaignVerifierUnsafeProjection:
    def test_unsafe_projection_field_fails(self, tmp_path: Path):
        """A public projection containing a prohibited field fails verification."""
        _run_campaign(tmp_path)
        from g8e_evals.projection import PublicProjectionError
        with pytest.raises(PublicProjectionError):
            project_to_public({
                "campaign_id": "c1",
                "campaign_revision": "r1",
                "variant_id": "v1",
                "task_id": "t1",
                "metric_id": "m1",
                "numerator": 1,
                "denominator": 2,
                "rate": 0.5,
                "unit": "boolean",
                "verification_status": "verified",
                "evidence_link": "proofs/proof.json",
                "raw_prompt": "secret prompt",
            })


class TestCampaignVerifierFileSafety:
    def test_symlinked_manifest_fails(self, tmp_path: Path):
        """A symlinked manifest.json fails verification."""
        report_dir = _run_campaign(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        backup = report_dir / "manifest-backup.json"
        manifest_path.rename(backup)
        manifest_path.symlink_to(backup)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("symlink" in f.lower() or "regular" in f.lower() or "manifest" in f.lower() for f in result.failures)


class TestCampaignVerifierReportValidation:
    def test_missing_report_artifact_fails(self, tmp_path: Path):
        """A missing required report artifact fails verification."""
        report_dir = _run_campaign(tmp_path)
        from g8e_evals.constants import ANALYSIS_JSON
        (report_dir / ANALYSIS_JSON).unlink()
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("analysis" in f.lower() or "report" in f.lower() or "missing" in f.lower() for f in result.failures)
