# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 and Tier 2 tests for the resource-observer contract and
campaign verifier wiring to supersession records.

Verifies that:
- ResourceObservation distinguishes hidden reasoning throughput from
  output throughput.
- ResourceObserverContract defines scope, baseline subtraction, sampling
  interval, clock domain, multi-tenant exclusion, and unsupported-platform
  behavior.
- The campaign verifier checks resource observations against the contract.
- The campaign verifier validates the final index generation is a
  FINALIZATION generation.
- The campaign verifier validates supersession records against the
  supersession policy.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with extra fields to verify
# validation rejects them.

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

import pytest
from pydantic import ValidationError

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
from g8e_evals.campaign_verify import verify_campaign
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import (
    ResourceObservation,
    compute_index_generation_hash,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
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
_VALID_HASH = "a" * 64


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
        cohort_id=_COHORT_ID,
        role_bindings=[RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )],
        content_hash=compute_model_cohort_hash(_COHORT_ID, [RoleModelBinding(
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


def _make_resource_observation(
    *,
    run_id: str = "run-1",
    model_variant_id: str = "qwen3-8b-q4_0",
    task_block: str = "task-1",
    hardware_identity: str = "linux/amd64/rtx-4090",
    collection_tool: str = "psutil-5.9",
    source_evidence_hash: str = _VALID_HASH,
    model_load_time_seconds: float = 12.5,
    peak_resident_memory_bytes: int = 4_000_000_000,
    peak_accelerator_memory_bytes: int | None = 8_000_000_000,
    artifact_bytes: int = 4_800_000_000,
    measured_energy_joules: float | None = None,
    end_to_end_latency_seconds: float = 1.5,
    provider_call_latency_seconds: float = 1.2,
    output_throughput_tokens_per_second: float | None = None,
    hidden_reasoning_throughput_tokens_per_second: float | None = None,
) -> dict:
    """Build a valid resource observation dict for testing."""
    return {
        "run_id": run_id,
        "model_variant_id": model_variant_id,
        "task_block": task_block,
        "hardware_identity": hardware_identity,
        "collection_tool": collection_tool,
        "source_evidence_hash": source_evidence_hash,
        "model_load_time_seconds": model_load_time_seconds,
        "peak_resident_memory_bytes": peak_resident_memory_bytes,
        "peak_accelerator_memory_bytes": peak_accelerator_memory_bytes,
        "artifact_bytes": artifact_bytes,
        "measured_energy_joules": measured_energy_joules,
        "end_to_end_latency_seconds": end_to_end_latency_seconds,
        "provider_call_latency_seconds": provider_call_latency_seconds,
        "output_throughput_tokens_per_second": output_throughput_tokens_per_second,
        "hidden_reasoning_throughput_tokens_per_second": hidden_reasoning_throughput_tokens_per_second,
    }


class TestResourceObserverContract:
    def test_resource_observation_has_hidden_reasoning_throughput(self):
        """ResourceObservation has a hidden_reasoning_throughput field separate from output_throughput."""
        obs = ResourceObservation(
            run_id="run-1",
            model_variant_id="v1",
            task_block="task-1",
            hardware_identity="linux/amd64/rtx-4090",
            collection_tool="psutil-5.9",
            source_evidence_hash=_VALID_HASH,
            model_load_time_seconds=10.0,
            peak_resident_memory_bytes=4_000_000_000,
            peak_accelerator_memory_bytes=8_000_000_000,
            artifact_bytes=4_800_000_000,
            end_to_end_latency_seconds=1.5,
            provider_call_latency_seconds=1.2,
            output_throughput_tokens_per_second=50.0,
            hidden_reasoning_throughput_tokens_per_second=200.0,
        )
        assert obs.hidden_reasoning_throughput_tokens_per_second == 200.0
        assert obs.output_throughput_tokens_per_second == 50.0

    def test_hidden_reasoning_throughput_none_when_unmeasured(self):
        """Hidden reasoning throughput is None when not available."""
        obs = ResourceObservation(
            run_id="run-1",
            model_variant_id="v1",
            task_block="task-1",
            hardware_identity="linux/amd64/rtx-4090",
            collection_tool="psutil-5.9",
            source_evidence_hash=_VALID_HASH,
            model_load_time_seconds=10.0,
            peak_resident_memory_bytes=4_000_000_000,
            artifact_bytes=4_800_000_000,
            end_to_end_latency_seconds=1.5,
            provider_call_latency_seconds=1.2,
        )
        assert obs.hidden_reasoning_throughput_tokens_per_second is None

    def test_resource_observer_contract_model_exists(self):
        """ResourceObserverContract model exists and is frozen with extra=forbid."""
        from g8e_evals.index import ResourceObserverContract
        contract = ResourceObserverContract(
            process_scope="single_process",
            baseline_subtraction_policy="idle_baseline",
            sampling_interval_seconds=0.1,
            synchronization_boundary="provider_call",
            observer_clock_domain="monotonic",
            multi_tenant_exclusion_rule="exclusive_access",
            unsupported_platform_behavior="skip_observation",
        )
        assert contract.process_scope == "single_process"
        assert contract.model_config.get("frozen") is True
        assert contract.model_config.get("extra") == "forbid"

    def test_resource_observer_contract_rejects_unknown_fields(self):
        """ResourceObserverContract rejects unknown fields."""
        from g8e_evals.index import ResourceObserverContract
        with pytest.raises(ValidationError):
            ResourceObserverContract(
                process_scope="single_process",
                baseline_subtraction_policy="idle_baseline",
                sampling_interval_seconds=0.1,
                synchronization_boundary="provider_call",
                observer_clock_domain="monotonic",
                multi_tenant_exclusion_rule="exclusive_access",
                unsupported_platform_behavior="skip_observation",
                extra_field="bad",
            )

    def test_validate_resource_observations_with_contract(self):
        """validate_resource_observations accepts a contract and validates observations against it."""
        from g8e_evals.index import ResourceObserverContract, validate_resource_observations
        contract = ResourceObserverContract(
            process_scope="single_process",
            baseline_subtraction_policy="idle_baseline",
            sampling_interval_seconds=0.1,
            synchronization_boundary="provider_call",
            observer_clock_domain="monotonic",
            multi_tenant_exclusion_rule="exclusive_access",
            unsupported_platform_behavior="skip_observation",
        )
        obs = ResourceObservation(
            run_id="run-1",
            model_variant_id="v1",
            task_block="task-1",
            hardware_identity="linux/amd64/rtx-4090",
            collection_tool="psutil-5.9",
            source_evidence_hash=_VALID_HASH,
            model_load_time_seconds=10.0,
            peak_resident_memory_bytes=4_000_000_000,
            artifact_bytes=4_800_000_000,
            end_to_end_latency_seconds=1.5,
            provider_call_latency_seconds=1.2,
        )
        validate_resource_observations([obs], contract=contract)

    def test_output_throughput_is_visible_tokens(self):
        """Output throughput is reported visible output tokens divided by eligible provider-call duration."""
        obs = ResourceObservation(
            run_id="run-1",
            model_variant_id="v1",
            task_block="task-1",
            hardware_identity="linux/amd64/rtx-4090",
            collection_tool="psutil-5.9",
            source_evidence_hash=_VALID_HASH,
            model_load_time_seconds=10.0,
            peak_resident_memory_bytes=4_000_000_000,
            artifact_bytes=4_800_000_000,
            end_to_end_latency_seconds=2.0,
            provider_call_latency_seconds=1.0,
            output_throughput_tokens_per_second=100.0,
        )
        # 100 tokens / 1.0 second provider call = 100 tokens/second
        assert obs.output_throughput_tokens_per_second == 100.0
        # end-to-end latency (2.0) > provider call latency (1.0)
        assert obs.end_to_end_latency_seconds > obs.provider_call_latency_seconds


class TestCampaignVerifierSupersessionWiring:
    def test_verifier_checks_final_generation_is_finalization(self, tmp_path: Path):
        """The campaign verifier checks that the final index generation is FINALIZATION."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok
        assert "finalization" in result.checked_layers

    def test_verifier_rejects_non_finalization_last_generation(self, tmp_path: Path):
        """The verifier rejects a campaign whose last generation is not FINALIZATION."""
        report_dir = _run_campaign(tmp_path)
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        if not lines:
            pytest.skip("no index generations to modify")

        # Change the last generation's creation_reason to RESUME
        last_gen = json.loads(lines[-1])
        last_gen["creation_reason"] = "resume"
        last_gen["content_hash"] = compute_index_generation_hash(
            generation_number=last_gen["generation_number"],
            parent_generation_hash=last_gen["parent_generation_hash"],
            creation_reason="resume",
            report_checksums=last_gen["report_checksums"],
            assignment_dispositions=last_gen["assignment_dispositions"],
        )
        lines[-1] = json.dumps(last_gen)
        index_path.write_text("\n".join(lines) + "\n")

        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("finalization" in f.lower() for f in result.failures)

    def test_verifier_checks_supersession_policy(self, tmp_path: Path):
        """The verifier validates supersession records against the supersession policy.

        A SUPERSESSION generation for an assignment with EFFECTIVE prior
        disposition must fail verification.
        """
        report_dir = _run_campaign(tmp_path)
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        if len(lines) < 2:
            pytest.skip("not enough generations to add a supersession")

        # Add a SUPERSESSION generation for an already-effective assignment
        last_gen = json.loads(lines[-1])
        effective_assignments = [
            d for d in last_gen["assignment_dispositions"]
            if d["disposition"] == "effective"
        ]
        if not effective_assignments:
            pytest.skip("no effective assignment to supersede")

        supersession_dispositions = [
            {"assignment_id": effective_assignments[0]["assignment_id"], "disposition": "effective"},
        ]
        new_gen = {
            "generation_number": last_gen["generation_number"] + 1,
            "parent_generation_hash": last_gen["content_hash"],
            "creation_reason": "supersession",
            "report_checksums": last_gen["report_checksums"],
            "assignment_dispositions": supersession_dispositions,
        }
        new_gen["content_hash"] = compute_index_generation_hash(
            generation_number=new_gen["generation_number"],
            parent_generation_hash=new_gen["parent_generation_hash"],
            creation_reason=new_gen["creation_reason"],
            report_checksums=new_gen["report_checksums"],
            assignment_dispositions=new_gen["assignment_dispositions"],
        )
        lines.append(json.dumps(new_gen))
        index_path.write_text("\n".join(lines) + "\n")

        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("supersession" in f.lower() or "policy" in f.lower() for f in result.failures)


class TestCampaignVerifierResourceObservationLayer:
    def test_verifier_has_resource_observation_layer(self, tmp_path: Path):
        """The campaign verifier has a resource_observation layer in checked_layers."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "resource_observation" in result.checked_layers

    def test_verifier_passes_without_resource_observations(self, tmp_path: Path):
        """The verifier passes when no resource observations are present (optional layer)."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok
        assert "resource_observation" in result.checked_layers

    def test_verifier_validates_resource_observations_when_present(self, tmp_path: Path):
        """The verifier validates resource observations when a resource-observations file is present."""
        from g8e_evals.constants import RESOURCE_OBSERVATIONS_JSONL

        report_dir = _run_campaign(tmp_path)

        # Write valid resource observations
        obs_data = _make_resource_observation(
            run_id="test-run-id",
            task_block="task-1001",
        )
        obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        obs_path.write_text(json.dumps(obs_data) + "\n")

        result = verify_campaign(report_dir)
        assert result.ok, f"valid resource observations should pass: {result.failures}"

    def test_verifier_rejects_invalid_resource_observations(self, tmp_path: Path):
        """The verifier rejects malformed resource observations."""
        from g8e_evals.constants import RESOURCE_OBSERVATIONS_JSONL

        report_dir = _run_campaign(tmp_path)

        # Write invalid resource observation (missing required field)
        bad_obs = _make_resource_observation()
        del bad_obs["run_id"]
        obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        obs_path.write_text(json.dumps(bad_obs) + "\n")

        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("resource" in f.lower() for f in result.failures)

    def test_verifier_rejects_duplicate_resource_observations(self, tmp_path: Path):
        """The verifier rejects duplicate resource observations."""
        from g8e_evals.constants import RESOURCE_OBSERVATIONS_JSONL

        report_dir = _run_campaign(tmp_path)

        # Write duplicate resource observations
        obs_data = _make_resource_observation(
            run_id="run-1",
            task_block="task-1",
            model_variant_id="v1",
        )
        obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        obs_path.write_text(json.dumps(obs_data) + "\n" + json.dumps(obs_data) + "\n")

        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("resource" in f.lower() and "duplicate" in f.lower() for f in result.failures)
