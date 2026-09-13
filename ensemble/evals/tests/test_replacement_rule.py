# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for the D24 ReplacementManifestRule, the S2-C set-child
identity contract, and S2-D fail-closed option coherence.

DEFECT S2-C (owner-dispositioned 2026-09-12): the P12 packet's child
commands passed ``--campaign-id generative-campaign-v1`` with
``--campaign-set-plan``, but ``validate_campaign_identity`` required the
parent profile ID while ``validate_campaign_set_child_preflight``
required the derived plan ``child_id`` — no single value satisfied both.
The approved contract is ``--campaign-id {child_id}``: identity
validation accepts a plan-bound ``child_id`` (or a rule-derived
replacement ID when a ``ReplacementManifestRule`` is bound) and still
checks ``plan.parent_campaign_id == profile.campaign_id``.

DEFECT S2-D (fail-closed approved): ``campaign run`` silently ignored
``--campaign-set-plan`` without ``--profile``/``--models``. Both options
are now rejected without their dependencies.

D24 (rule-bound replacement IDs): a typed ``ReplacementManifestRule``
bound to the frozen plan by ``set_id``/``set_plan_hash`` authorizes
deterministic replacement child IDs derived from
``(rule_id, original_child_id, attempt_number)``. The runner resolves
replacement lineage onto the ``CampaignBinding`` and the aggregate
verifier proves the derivation against the bound rule.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
from dataclasses import dataclass
from pathlib import Path

import pytest
from click.testing import CliRunner
from pydantic import ValidationError

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
from g8e_evals.campaign_set import (
    CAMPAIGN_SET_SCHEMA_VERSION,
    CHILD_COUNT,
    CampaignChildIndexEntry,
    CampaignChildPlan,
    CampaignSetIndex,
    CampaignSetPlan,
    compute_campaign_set_index_hash,
    compute_campaign_set_plan_hash,
    compute_child_campaign_id,
    verify_campaign_set_aggregate,
)
from g8e_evals.campaign_verify import verify_campaign
from g8e_evals.cli import (
    main,
    validate_campaign_set_child_preflight,
)
from g8e_evals.constants import CAMPAIGN_ASSIGNMENTS_JSONL, MANIFEST_JSON
from g8e_evals.harness import Response, Score, Task
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
    TrackArmAssignment,
    compute_campaign_profile_hash,
)
from g8e_evals.registry import (
    MODEL_REGISTRY_VERSION,
    ModelRegistry,
    ModelVariant,
    PublicationEligibility,
    WeightClass,
    compute_model_registry_hash,
)
from g8e_evals.replacement_rule import (
    DEAD_EVIDENCE_DISPOSITION,
    REPLACEMENT_CHILD_DERIVATION_RULE,
    REPLACEMENT_RULE_SCHEMA_VERSION,
    ReplacementManifestRule,
    compute_replacement_child_id,
    compute_replacement_rule_hash,
    resolve_replacement_child_id,
)
from g8e_evals.runner import (
    CampaignRunner,
    CampaignRunnerError,
    CampaignSpec,
)
from g8e_evals.schema import (
    CampaignBinding,
    CampaignTrack,
    ReportRole,
    RunManifest,
)

from _set_plan_fixture import make_campaign_set_plan, make_replacement_rule


_VALID_HASH = "a" * 64
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_INITIAL_STATE_ID = "no-initial-state-v1"
_COHORT_ID = "cohort-qwen3-8b"
_VARIANT_ID = "qwen3-8b-q4_0"
_PARENT_CAMPAIGN_ID = "generative-campaign-v1"


# ---------------------------------------------------------------------------
# Helpers — plan, rule, and profile-bound runner fixtures
# ---------------------------------------------------------------------------


def _preflight_kwargs(plan: CampaignSetPlan, campaign_id: str) -> dict:
    """Keyword arguments for validate_campaign_set_child_preflight bound to the plan."""
    child = next(
        (cp for cp in plan.child_plans if cp.child_id == campaign_id),
        plan.child_plans[0],
    )
    return {
        "campaign_id": campaign_id,
        "campaign_profile_hash": plan.campaign_profile_hash,
        "model_registry_hash": plan.model_registry_hash,
        "expected_record_policy_hash": plan.expected_record_policy_hash,
        "retry_policy_hash": plan.retry_policy_hash,
        "budget_authority_hash": plan.budget_authority_hash,
        "instrumentation_policy_hash": plan.instrumentation_policy_hash,
        "seed": plan.seed,
        "orchestrator_environment_scope": plan.orchestrator_environment_scope,
        "provider_environment_scope": plan.provider_environment_scope,
        "campaign_set_plan": plan,
        "task_ids": list(child.partition_task_ids),
    }


# ---------------------------------------------------------------------------
# Rule model validation
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestReplacementManifestRuleModel:
    """The frozen rule validates its content hash and closed shape."""

    def test_valid_rule_constructs(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        assert rule.set_id == plan.set_id
        assert rule.set_plan_hash == plan.content_hash
        assert rule.content_hash == compute_replacement_rule_hash(rule)

    def test_content_hash_mismatch_rejected(self):
        plan = make_campaign_set_plan()
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            ReplacementManifestRule(
                rule_id="rule-x",
                rule_version=REPLACEMENT_RULE_SCHEMA_VERSION,
                purpose="p",
                created_at="2026-09-12T00:00:00Z",
                set_id=plan.set_id,
                set_plan_hash=plan.content_hash,
                replaceable_child_ids=[plan.child_plans[0].child_id],
                max_attempts_per_child=1,
                derivation_rule=REPLACEMENT_CHILD_DERIVATION_RULE,
                fresh_report_root_required=True,
                interrupted_report_disposition=DEAD_EVIDENCE_DISPOSITION,
                approval_note="n",
                content_hash="f" * 64,
            )

    def test_material_field_mutations_change_hash(self):
        plan = make_campaign_set_plan()
        base = make_replacement_rule(plan=plan)
        mutations = [
            make_replacement_rule(plan=plan, rule_id="other-rule"),
            make_replacement_rule(plan=plan, max_attempts=4),
            make_replacement_rule(plan=plan, replaceable_child_ids=[cp.child_id for cp in plan.child_plans[:2]]),
            make_replacement_rule(plan=plan, set_plan_hash="b" * 64),
        ]
        for mutated in mutations:
            assert mutated.content_hash != base.content_hash

    def test_other_plan_binding_changes_hash(self):
        plan = make_campaign_set_plan()
        other_plan = make_campaign_set_plan(parent_campaign_id="other-parent")
        assert make_replacement_rule(plan=plan).content_hash != make_replacement_rule(plan=other_plan).content_hash

    def test_duplicate_replaceable_ids_rejected(self):
        plan = make_campaign_set_plan()
        dup = plan.child_plans[0].child_id
        fields = {
            "rule_id": "rule-x",
            "rule_version": REPLACEMENT_RULE_SCHEMA_VERSION,
            "purpose": "p",
            "created_at": "2026-09-12T00:00:00Z",
            "set_id": plan.set_id,
            "set_plan_hash": plan.content_hash,
            "replaceable_child_ids": [dup, dup],
            "max_attempts_per_child": 1,
            "derivation_rule": REPLACEMENT_CHILD_DERIVATION_RULE,
            "fresh_report_root_required": True,
            "interrupted_report_disposition": DEAD_EVIDENCE_DISPOSITION,
            "approval_note": "n",
        }
        temp = ReplacementManifestRule.model_construct(**fields, content_hash="0" * 64)
        with pytest.raises(ValidationError, match="duplicate child IDs"):
            ReplacementManifestRule(**fields, content_hash=compute_replacement_rule_hash(temp))

    def test_wrong_derivation_rule_rejected(self):
        plan = make_campaign_set_plan()
        fields = {
            "rule_id": "rule-x",
            "rule_version": REPLACEMENT_RULE_SCHEMA_VERSION,
            "purpose": "p",
            "created_at": "2026-09-12T00:00:00Z",
            "set_id": plan.set_id,
            "set_plan_hash": plan.content_hash,
            "replaceable_child_ids": [plan.child_plans[0].child_id],
            "max_attempts_per_child": 1,
            "derivation_rule": "sha256(anything-else)",
            "fresh_report_root_required": True,
            "interrupted_report_disposition": DEAD_EVIDENCE_DISPOSITION,
            "approval_note": "n",
        }
        temp = ReplacementManifestRule.model_construct(**fields, content_hash="0" * 64)
        with pytest.raises(ValidationError, match="derivation_rule"):
            ReplacementManifestRule(**fields, content_hash=compute_replacement_rule_hash(temp))

    def test_fresh_report_root_required_must_be_true(self):
        plan = make_campaign_set_plan()
        fields = {
            "rule_id": "rule-x",
            "rule_version": REPLACEMENT_RULE_SCHEMA_VERSION,
            "purpose": "p",
            "created_at": "2026-09-12T00:00:00Z",
            "set_id": plan.set_id,
            "set_plan_hash": plan.content_hash,
            "replaceable_child_ids": [plan.child_plans[0].child_id],
            "max_attempts_per_child": 1,
            "derivation_rule": REPLACEMENT_CHILD_DERIVATION_RULE,
            "fresh_report_root_required": False,
            "interrupted_report_disposition": DEAD_EVIDENCE_DISPOSITION,
            "approval_note": "n",
        }
        temp = ReplacementManifestRule.model_construct(**fields, content_hash="0" * 64)
        with pytest.raises(ValidationError, match="fresh_report_root_required"):
            ReplacementManifestRule(**fields, content_hash=compute_replacement_rule_hash(temp))

    def test_extra_field_rejected(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        with pytest.raises(ValidationError):
            ReplacementManifestRule.model_validate(
                {**rule.model_dump(), "unexpected": "x"}
            )


# ---------------------------------------------------------------------------
# Replacement child-ID derivation
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestComputeReplacementChildId:
    """Replacement IDs are deterministic and disjoint from plan child IDs."""

    def test_deterministic(self):
        a = compute_replacement_child_id("rule-1", "child-x", 1)
        b = compute_replacement_child_id("rule-1", "child-x", 1)
        assert a == b
        assert len(a) == 64

    def test_attempt_number_changes_id(self):
        a = compute_replacement_child_id("rule-1", "child-x", 1)
        b = compute_replacement_child_id("rule-1", "child-x", 2)
        assert a != b

    def test_original_child_changes_id(self):
        a = compute_replacement_child_id("rule-1", "child-x", 1)
        b = compute_replacement_child_id("rule-1", "child-y", 1)
        assert a != b

    def test_rule_id_changes_id(self):
        a = compute_replacement_child_id("rule-1", "child-x", 1)
        b = compute_replacement_child_id("rule-2", "child-x", 1)
        assert a != b

    def test_never_collides_with_plan_child_ids(self):
        """Replacement IDs derive from a different payload shape than
        compute_child_campaign_id and cannot collide with plan children."""
        plan = make_campaign_set_plan()
        child_ids = {cp.child_id for cp in plan.child_plans}
        for cp in plan.child_plans:
            for attempt in range(1, 4):
                replacement_id = compute_replacement_child_id(
                    "p12-replacement-rule-v1", cp.child_id, attempt
                )
                assert replacement_id not in child_ids

    def test_payload_shape_differs_from_child_derivation(self):
        """The replacement derivation hashes rule/child/attempt; the plan
        derivation hashes parent/partition-index/partition-tasks."""
        plan = make_campaign_set_plan()
        cp = plan.child_plans[0]
        replacement_id = compute_replacement_child_id("rule-1", cp.child_id, 1)
        plan_child_id = compute_child_campaign_id(
            parent_campaign_id=plan.parent_campaign_id,
            partition_index=0,
            partition_task_ids=list(cp.partition_task_ids),
        )
        assert replacement_id != plan_child_id


# ---------------------------------------------------------------------------
# resolve_replacement_child_id
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestResolveReplacementChildId:
    """Resolution maps a replacement ID back to its original plan child."""

    def test_resolves_valid_replacement(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        original = plan.child_plans[1]
        replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, 2)
        resolved = resolve_replacement_child_id(rule, plan, replacement_id)
        assert resolved is not None
        resolved_plan, attempt = resolved
        assert resolved_plan.child_id == original.child_id
        assert attempt == 2

    def test_attempt_at_max_resolves(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan, max_attempts=2)
        original = plan.child_plans[0]
        replacement_id = compute_replacement_child_id(
            rule.rule_id, original.child_id, 2
        )
        assert resolve_replacement_child_id(rule, plan, replacement_id) is not None

    def test_attempt_beyond_max_returns_none(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan, max_attempts=2)
        original = plan.child_plans[0]
        replacement_id = compute_replacement_child_id(
            rule.rule_id, original.child_id, 3
        )
        assert resolve_replacement_child_id(rule, plan, replacement_id) is None

    def test_plan_child_id_is_not_a_replacement(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        assert resolve_replacement_child_id(rule, plan, plan.child_plans[0].child_id) is None

    def test_unknown_id_returns_none(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        assert resolve_replacement_child_id(rule, plan, "f" * 64) is None

    def test_non_replaceable_child_returns_none(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(
            plan=plan,
            replaceable_child_ids=[plan.child_plans[0].child_id],
        )
        other = plan.child_plans[1]
        replacement_id = compute_replacement_child_id(rule.rule_id, other.child_id, 1)
        assert resolve_replacement_child_id(rule, plan, replacement_id) is None

    def test_wrong_rule_id_returns_none(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan, rule_id="rule-a")
        other_rule = make_replacement_rule(plan=plan, rule_id="rule-b")
        original = plan.child_plans[0]
        replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, 1)
        assert resolve_replacement_child_id(other_rule, plan, replacement_id) is None


# ---------------------------------------------------------------------------
# Campaign-set child preflight with a bound replacement rule
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestChildPreflightReplacementRule:
    """The child preflight resolves rule-derived replacement IDs to the
    original child for the partition and authority checks."""

    def test_plan_child_id_accepted(self):
        plan = make_campaign_set_plan()
        child = plan.child_plans[0]
        validate_campaign_set_child_preflight(**_preflight_kwargs(plan, child.child_id))

    def test_replacement_id_accepted_with_bound_rule(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        original = plan.child_plans[2]
        replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, 1)
        kwargs = _preflight_kwargs(plan, replacement_id)
        kwargs["task_ids"] = list(original.partition_task_ids)
        kwargs["replacement_rule"] = rule
        validate_campaign_set_child_preflight(**kwargs)

    def test_replacement_id_rejected_without_rule(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        original = plan.child_plans[0]
        replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, 1)
        kwargs = _preflight_kwargs(plan, replacement_id)
        kwargs["task_ids"] = list(original.partition_task_ids)
        with pytest.raises(ValueError, match="does not match any child"):
            validate_campaign_set_child_preflight(**kwargs)

    def test_replacement_id_for_non_replaceable_child_rejected(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(
            plan=plan,
            replaceable_child_ids=[plan.child_plans[0].child_id],
        )
        other = plan.child_plans[1]
        replacement_id = compute_replacement_child_id(rule.rule_id, other.child_id, 1)
        kwargs = _preflight_kwargs(plan, replacement_id)
        kwargs["task_ids"] = list(other.partition_task_ids)
        kwargs["replacement_rule"] = rule
        with pytest.raises(ValueError, match="does not match any child"):
            validate_campaign_set_child_preflight(**kwargs)

    def test_replacement_id_wrong_partition_rejected(self):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        original = plan.child_plans[0]
        replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, 1)
        kwargs = _preflight_kwargs(plan, replacement_id)
        kwargs["task_ids"] = list(plan.child_plans[1].partition_task_ids)
        kwargs["replacement_rule"] = rule
        with pytest.raises(ValueError, match="partition"):
            validate_campaign_set_child_preflight(**kwargs)


# ---------------------------------------------------------------------------
# CampaignBinding replacement lineage schema
# ---------------------------------------------------------------------------


def _binding_kwargs(**overrides) -> dict:
    fields = {
        "campaign_id": _PARENT_CAMPAIGN_ID,
        "campaign_revision": "1",
        "report_role": ReportRole.CHILD,
        "child_campaign_id": "c" * 64,
        "child_campaign_revision": "rev-1",
        "campaign_profile_hash": _VALID_HASH,
        "model_registry_hash": "b" * 64,
        "required_record_policy_hash": "d" * 64,
        "orchestrator_hardware_identity": "linux/amd64/cpu",
        "orchestrator_environment_stratum": "single-machine",
        "track_arm_assignments": [
            {"track": "direct", "arm_id": "direct"},
        ],
    }
    fields.update(overrides)
    return fields


@pytest.mark.unit
class TestCampaignBindingReplacementLineage:
    """The binding's all-or-none replacement lineage validator."""

    def test_full_lineage_validates(self):
        binding = CampaignBinding(
            **_binding_kwargs(
                supersedes_child_id="e" * 64,
                replacement_attempt=1,
                replacement_rule_hash="f" * 64,
            )
        )
        assert binding.supersedes_child_id == "e" * 64
        assert binding.replacement_attempt == 1
        assert binding.replacement_rule_hash == "f" * 64

    def test_lineage_round_trips_json(self):
        binding = CampaignBinding(
            **_binding_kwargs(
                supersedes_child_id="e" * 64,
                replacement_attempt=2,
                replacement_rule_hash="f" * 64,
            )
        )
        restored = CampaignBinding.model_validate_json(binding.model_dump_json())
        assert restored == binding

    def test_partial_lineage_rejected(self):
        with pytest.raises(ValidationError, match="set together"):
            CampaignBinding(
                **_binding_kwargs(supersedes_child_id="e" * 64)
            )
        with pytest.raises(ValidationError, match="set together"):
            CampaignBinding(
                **_binding_kwargs(
                    supersedes_child_id="e" * 64,
                    replacement_attempt=1,
                )
            )

    def test_lineage_on_single_report_rejected(self):
        with pytest.raises(ValidationError, match="report_role=child"):
            CampaignBinding(
                **_binding_kwargs(
                    report_role=ReportRole.SINGLE,
                    child_campaign_id=None,
                    child_campaign_revision=None,
                    supersedes_child_id="e" * 64,
                    replacement_attempt=1,
                    replacement_rule_hash="f" * 64,
                )
            )

    def test_supersedes_equal_to_child_id_rejected(self):
        with pytest.raises(ValidationError, match="must differ"):
            CampaignBinding(
                **_binding_kwargs(
                    child_campaign_id="e" * 64,
                    supersedes_child_id="e" * 64,
                    replacement_attempt=1,
                    replacement_rule_hash="f" * 64,
                )
            )


# ---------------------------------------------------------------------------
# S2-D fail-closed CLI option coherence
# ---------------------------------------------------------------------------


def _invoke(runner: CliRunner, args: list[str], env: dict[str, str] | None = None):
    sterile = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("G8E_OPERATOR", "OPERATOR_SESSION", "OPERATOR_ID"))
    }
    if env:
        sterile.update(env)
    return runner.invoke(main, args, env=sterile)


def _touch(path: Path) -> Path:
    path.write_text("{}")
    return path


@pytest.mark.unit
class TestFailClosedOptionCoherence:
    """S2-D: incoherent campaign run option sets fail before any loading."""

    def test_campaign_set_plan_without_profile_models_rejected(self, tmp_path: Path):
        prereg = _touch(tmp_path / "prereg.json")
        plan_file = _touch(tmp_path / "plan.json")
        result = _invoke(CliRunner(), [
            "campaign", "run",
            "--suite", "ifeval_subset",
            "--preregistration", str(prereg),
            "--campaign-id", "x",
            "--seed", "42",
            "--campaign-set-plan", str(plan_file),
        ])
        assert result.exit_code != 0
        assert "--campaign-set-plan requires --profile and --models" in result.output

    def test_campaign_set_plan_with_profile_only_rejected(self, tmp_path: Path):
        prereg = _touch(tmp_path / "prereg.json")
        plan_file = _touch(tmp_path / "plan.json")
        profile_file = _touch(tmp_path / "profile.json")
        result = _invoke(CliRunner(), [
            "campaign", "run",
            "--suite", "ifeval_subset",
            "--preregistration", str(prereg),
            "--campaign-id", "x",
            "--seed", "42",
            "--profile", str(profile_file),
            "--campaign-set-plan", str(plan_file),
        ])
        assert result.exit_code != 0
        assert "--campaign-set-plan requires --profile and --models" in result.output

    def test_replacement_rule_without_campaign_set_plan_rejected(self, tmp_path: Path):
        prereg = _touch(tmp_path / "prereg.json")
        profile_file = _touch(tmp_path / "profile.json")
        models_file = _touch(tmp_path / "models.json")
        rule_file = _touch(tmp_path / "rule.json")
        result = _invoke(CliRunner(), [
            "campaign", "run",
            "--suite", "ifeval_subset",
            "--preregistration", str(prereg),
            "--campaign-id", "x",
            "--seed", "42",
            "--profile", str(profile_file),
            "--models", str(models_file),
            "--replacement-rule", str(rule_file),
        ])
        assert result.exit_code != 0
        assert "--replacement-rule requires --campaign-set-plan" in result.output


# ---------------------------------------------------------------------------
# Runner-side rule/plan binding and binding lineage (integration)
# ---------------------------------------------------------------------------


def _make_variant() -> ModelVariant:
    return ModelVariant(
        variant_id=_VARIANT_ID,
        canonical_display_name="Qwen3 8B",
        source_list_alias="qwen3:8b",
        hf_repo="Qwen/Qwen3-8B",
        hf_sha="a" * 40,
        retrieval_date="2026-09-01",
        license_id="Apache-2.0",
        license_text_hash=_VALID_HASH,
        gated=False,
        publication_eligibility=PublicationEligibility.ELIGIBLE,
        parameter_count=8_000_000_000,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="qwen3",
        dtype="BF16",
        format="gguf",
        quantization="q4_0",
        context_length=32768,
        supported_modalities=["text"],
        reasoning_mode="non-reasoning",
        tool_call_support=True,
        chat_template_family="qwen3",
        chat_template_hash="c" * 64,
        tokenizer_digest="d" * 64,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest="b" * 64,
        artifact_bytes=8_000_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )


def _make_registry() -> ModelRegistry:
    variants = [_make_variant()]
    return ModelRegistry(
        registry_id="registry-v1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-01T00:00:00Z",
        variants=variants,
        qualification_records=[],
        content_hash=compute_model_registry_hash("registry-v1", "1", variants, []),
    )


def _make_profile(
    registry_hash: str,
    *,
    campaign_id: str = _PARENT_CAMPAIGN_ID,
    task_ids: list[str] | None = None,
) -> CampaignProfile:
    fields = {
        "campaign_id": campaign_id,
        "campaign_revision": "1",
        "schema_version": CAMPAIGN_PROFILE_VERSION,
        "purpose": "replacement-rule test campaign",
        "created_at": "2026-09-01T00:00:00Z",
        "lifecycle_status": CampaignLifecycleStatus.FROZEN,
        "generative_variant_ids": [_VARIANT_ID],
        "benchmark_ids": ["ifeval_subset"],
        "dataset_hashes": [_DATASET_HASH],
        "grader_hashes": ["g" * 64],
        "prompt_serialization_hash": _VALID_HASH,
        "task_ids": task_ids if task_ids is not None else ["task-1001"],
        "repetitions": 1,
        "track_arm_assignments": [
            TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
        ],
        "model_tier_assignments": [],
        "baseline_tier_mappings": {},
        "routing_policy": "default",
        "temperature": 0.0,
        "top_p": 1.0,
        "max_tokens": 4096,
        "seed": 42,
        "context_limit": 32768,
        "timeout_seconds": 120.0,
        "max_retries": 1,
        "warmup_excluded": True,
        "concurrency": 1,
        "hardware_identity": "linux/amd64/cpu",
        "environment_stratum": "single-machine",
        "primary_metrics": ["ifeval_subset_verifier"],
        "unit_of_analysis": "task",
        "claim_boundary": ClaimBoundary.DESCRIPTIVE_ONLY,
        "model_registry_hash": registry_hash,
    }
    temp = CampaignProfile.model_construct(**fields, content_hash="0" * 64)
    return CampaignProfile(**fields, content_hash=compute_campaign_profile_hash(temp))


def _make_spec(campaign_id: str, task_ids: list[str]) -> CampaignSpec:
    binding = RoleModelBinding(
        role="primary",
        model_id="qwen3:8b",
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )
    cohort = ModelCohort(
        cohort_id=_COHORT_ID,
        role_bindings=[binding],
        content_hash=compute_model_cohort_hash(_COHORT_ID, [binding]),
    )
    task_assignment = TaskAssignmentManifest(
        task_assignment_id=f"task-assignment-{campaign_id[:16]}",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=task_ids,
        content_hash=compute_task_assignment_hash(
            f"task-assignment-{campaign_id[:16]}", "ifeval_subset", _DATASET_HASH, task_ids,
        ),
    )
    initial_state = InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id=f"config-{campaign_id[:16]}",
        config_version="1.0.0",
        baseline_arm_id="direct",
        comparison_arm_ids=[],
        model_cohort_ids=[_COHORT_ID],
        task_assignment_id=f"task-assignment-{campaign_id[:16]}",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=["replicate-1"],
        required_replicate_count=1,
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )
    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in task_ids).encode()
    return CampaignSpec(
        campaign_id=campaign_id,
        release_version="v2.1.8",
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


def _make_tasks(task_ids: list[str]) -> list[Task]:
    return [
        Task(
            id=tid,
            prompt=f"Prompt for {tid}",
            metadata=TaskMetadata(
                benchmark="ifeval_subset",
                instruction_id_list=["punctuation:no_comma"],
                kwargs=[{"no_comma": True}],
            ),
        )
        for tid in task_ids
    ]


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


def _fake_sut_factory(cohort: ModelCohort, arm: Arm):
    return _FakeSUT(model_id=cohort.role_bindings[0].model_id)


def _make_runner(
    output_dir: Path,
    *,
    campaign_id: str,
    task_ids: list[str],
    plan: CampaignSetPlan | None = None,
    rule: ReplacementManifestRule | None = None,
    bind_profile: bool = True,
) -> CampaignRunner:
    kwargs: dict = {}
    if bind_profile:
        registry = _make_registry()
        profile = _make_profile(registry.content_hash, task_ids=task_ids)
        kwargs.update(
            campaign_profile=profile,
            model_registry=registry,
            cohort_variant_map={_COHORT_ID: _VARIANT_ID},
        )
    return CampaignRunner(
        spec=_make_spec(campaign_id, task_ids),
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(task_ids),
        grader=_FakeGrader(),
        output_dir=output_dir,
        campaign_set_plan=plan,
        replacement_rule=rule,
        **kwargs,
    )


@pytest.mark.unit
class TestBuildCampaignBindingReplacement:
    """_build_campaign_binding resolves rule-derived replacement IDs to
    the original plan child and records lineage."""

    def test_replacement_binding_carries_lineage(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        original = plan.child_plans[0]
        replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, 1)
        runner = _make_runner(
            tmp_path,
            campaign_id=replacement_id,
            task_ids=list(original.partition_task_ids),
            plan=plan,
            rule=rule,
        )
        binding = runner._build_campaign_binding()
        assert binding is not None
        assert binding.report_role == ReportRole.CHILD
        assert binding.child_campaign_id == replacement_id
        assert binding.child_campaign_revision == original.child_revision
        assert binding.supersedes_child_id == original.child_id
        assert binding.replacement_attempt == 1
        assert binding.replacement_rule_hash == rule.content_hash

    def test_plan_child_binding_has_no_lineage(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        child = plan.child_plans[0]
        runner = _make_runner(
            tmp_path,
            campaign_id=child.child_id,
            task_ids=list(child.partition_task_ids),
            plan=plan,
            rule=rule,
        )
        binding = runner._build_campaign_binding()
        assert binding is not None
        assert binding.report_role == ReportRole.CHILD
        assert binding.child_campaign_id == child.child_id
        assert binding.supersedes_child_id is None
        assert binding.replacement_attempt is None
        assert binding.replacement_rule_hash is None

    def test_unknown_id_with_rule_still_rejected(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        runner = _make_runner(
            tmp_path,
            campaign_id="not-a-child-or-replacement",
            task_ids=["task-1001"],
            plan=plan,
            rule=rule,
        )
        with pytest.raises(CampaignRunnerError, match="does not match any child"):
            runner._build_campaign_binding()


@pytest.mark.integration
class TestRunnerReplacementRuleBinding:
    """Runner-side fail-closed checks: a bound rule must bind the loaded
    plan exactly, before the report directory is created."""

    def test_rule_without_plan_rejected(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan)
        runner = _make_runner(
            tmp_path,
            campaign_id="any",
            task_ids=["task-1001"],
            plan=None,
            rule=rule,
        )
        with pytest.raises(CampaignRunnerError, match="campaign_set_plan"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []

    def test_plan_without_profile_rejected(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        runner = _make_runner(
            tmp_path,
            campaign_id=plan.child_plans[0].child_id,
            task_ids=list(plan.child_plans[0].partition_task_ids),
            plan=plan,
            bind_profile=False,
        )
        with pytest.raises(CampaignRunnerError, match="campaign_profile"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []

    def test_rule_set_id_mismatch_rejected(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        wrong_rule = make_replacement_rule(plan=plan, set_id="other-set-v1")
        runner = _make_runner(
            tmp_path,
            campaign_id=plan.child_plans[0].child_id,
            task_ids=list(plan.child_plans[0].partition_task_ids),
            plan=plan,
            rule=wrong_rule,
        )
        with pytest.raises(CampaignRunnerError, match="set_id"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []

    def test_rule_plan_hash_mismatch_rejected(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(plan=plan, set_plan_hash="e" * 64)
        runner = _make_runner(
            tmp_path,
            campaign_id=plan.child_plans[0].child_id,
            task_ids=list(plan.child_plans[0].partition_task_ids),
            plan=plan,
            rule=rule,
        )
        with pytest.raises(CampaignRunnerError, match="set_plan_hash"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []

    def test_rule_replaceable_superset_rejected(self, tmp_path: Path):
        plan = make_campaign_set_plan()
        rule = make_replacement_rule(
            plan=plan,
            replaceable_child_ids=[
                *[cp.child_id for cp in plan.child_plans],
                "not-a-plan-child",
            ],
        )
        runner = _make_runner(
            tmp_path,
            campaign_id=plan.child_plans[0].child_id,
            task_ids=list(plan.child_plans[0].partition_task_ids),
            plan=plan,
            rule=rule,
        )
        with pytest.raises(CampaignRunnerError, match="replaceable"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []


# ---------------------------------------------------------------------------
# Aggregate verifier replacement lineage (integration)
# ---------------------------------------------------------------------------


def _build_plan_small(
    partitions: list[list[str]],
    expected_child_count: int,
    expected_total_count: int,
) -> CampaignSetPlan:
    """Small-partition plan for integration tests (bypasses the 30-task
    partition constraint via model_construct, like the existing aggregate
    fixtures)."""
    kwargs = {
        "set_id": "p12-ifeval-expanded-v1",
        "parent_campaign_id": _PARENT_CAMPAIGN_ID,
        "parent_campaign_revision": "1",
        "population_task_ids": [t for p in partitions for t in p],
        "population_hash": _VALID_HASH,
        "model_registry_hash": _VALID_HASH,
        "campaign_profile_hash": _VALID_HASH,
        "repetition_ids": ["replicate-1"],
        "seed": 42,
        "retry_policy_hash": _VALID_HASH,
        "budget_authority_hash": _VALID_HASH,
        "instrumentation_policy_hash": _VALID_HASH,
        "expected_record_policy_hash": _VALID_HASH,
        "orchestrator_environment_scope": "linux/amd64/cpu",
        "provider_environment_scope": "unavailable",
        "child_revisions": [f"rev-{i + 1}" for i in range(CHILD_COUNT)],
    }
    sorted_population = sorted(kwargs["population_task_ids"])
    child_plans: list[CampaignChildPlan] = []
    for i in range(CHILD_COUNT):
        partition = sorted(partitions[i])
        child_id = compute_child_campaign_id(
            parent_campaign_id=kwargs["parent_campaign_id"],
            partition_index=i,
            partition_task_ids=partition,
        )
        child_plans.append(CampaignChildPlan.model_construct(
            child_id=child_id,
            child_revision=kwargs["child_revisions"][i],
            partition_index=i,
            partition_task_ids=partition,
            expected_assignment_count=expected_child_count,
        ))
    plan = CampaignSetPlan.model_construct(
        set_id=kwargs["set_id"],
        set_version=CAMPAIGN_SET_SCHEMA_VERSION,
        parent_campaign_id=kwargs["parent_campaign_id"],
        parent_campaign_revision=kwargs["parent_campaign_revision"],
        child_plans=child_plans,
        population_task_ids=sorted_population,
        population_hash=kwargs["population_hash"],
        model_registry_hash=kwargs["model_registry_hash"],
        campaign_profile_hash=kwargs["campaign_profile_hash"],
        repetition_ids=kwargs["repetition_ids"],
        seed=kwargs["seed"],
        retry_policy_hash=kwargs["retry_policy_hash"],
        budget_authority_hash=kwargs["budget_authority_hash"],
        instrumentation_policy_hash=kwargs["instrumentation_policy_hash"],
        expected_record_policy_hash=kwargs["expected_record_policy_hash"],
        orchestrator_environment_scope=kwargs["orchestrator_environment_scope"],
        provider_environment_scope=kwargs["provider_environment_scope"],
        expected_child_assignment_count=expected_child_count,
        expected_total_assignment_count=expected_total_count,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_set_plan_hash(plan)
    return plan.model_copy(update={"content_hash": content_hash})


def _read_assignment_ids(report_dir: Path) -> list[str]:
    assignments_path = report_dir / CAMPAIGN_ASSIGNMENTS_JSONL
    if not assignments_path.exists():
        return []
    ids: list[str] = []
    for line in assignments_path.read_text().splitlines():
        line = line.strip()
        if line:
            aid = json.loads(line).get("assignment_id")
            if aid:
                ids.append(str(aid))
    return sorted(ids)


def _build_index_from_children(
    plan: CampaignSetPlan,
    child_report_dirs: dict[str, Path],
) -> CampaignSetIndex:
    from g8e_evals.campaign_set import (
        _compute_child_verification_report_hash,
        _recompute_report_checksum,
    )

    entries: list[CampaignChildIndexEntry] = []
    total = 0
    for cp in plan.child_plans:
        report_dir = child_report_dirs[cp.child_id]
        assignment_ids = _read_assignment_ids(report_dir)
        count = len(assignment_ids)
        total += count
        verification = verify_campaign(report_dir)
        child_verification_hash = _compute_child_verification_report_hash(report_dir) or "0" * 64
        report_checksum = _recompute_report_checksum(report_dir) or "0" * 64
        entries.append(CampaignChildIndexEntry(
            child_id=cp.child_id,
            report_campaign_id=verification.campaign_id,
            finalization_generation_hash=verification.verified_index_generation_hash,
            child_verification_report_hash=child_verification_hash,
            report_checksum=report_checksum,
            assignment_count=count,
        ))
    index = CampaignSetIndex.model_construct(
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_set_index_hash(index)
    return index.model_copy(update={"content_hash": content_hash})


def _setup_set(
    tmp_path: Path,
    *,
    rule: ReplacementManifestRule | None = None,
    replacement_child_index: int | None = None,
) -> tuple[CampaignSetPlan, CampaignSetIndex, dict[str, Path]]:
    """Run four profile-bound children against a small plan. When
    ``replacement_child_index`` is set, that child's slot is filled by a
    D24 replacement run (campaign_id = derived replacement ID)."""
    partitions = [["task-001"], ["task-002"], ["task-003"], ["task-004"]]
    plan = _build_plan_small(partitions, expected_child_count=1, expected_total_count=4)

    child_report_dirs: dict[str, Path] = {}
    for i, cp in enumerate(plan.child_plans):
        out_dir = tmp_path / f"child-{i}"
        out_dir.mkdir(parents=True, exist_ok=True)
        campaign_id = cp.child_id
        if i == replacement_child_index:
            assert rule is not None
            campaign_id = compute_replacement_child_id(rule.rule_id, cp.child_id, 1)
        runner = _make_runner(
            out_dir,
            campaign_id=campaign_id,
            task_ids=list(cp.partition_task_ids),
            plan=plan,
            rule=rule,
        )
        result = asyncio.run(runner.run())
        child_report_dirs[cp.child_id] = result.report_dir

    index = _build_index_from_children(plan, child_report_dirs)
    return plan, index, child_report_dirs


def _read_binding(report_dir: Path) -> CampaignBinding:
    manifest = RunManifest.model_validate_json((report_dir / MANIFEST_JSON).read_text())
    assert manifest.campaign_binding is not None
    return manifest.campaign_binding


def _rewrite_binding(report_dir: Path, **updates) -> None:
    manifest_path = report_dir / MANIFEST_JSON
    manifest = RunManifest.model_validate_json(manifest_path.read_text())
    binding = manifest.campaign_binding
    assert binding is not None
    new_binding = binding.model_copy(update=updates)
    new_manifest = manifest.model_copy(update={"campaign_binding": new_binding})
    manifest_path.write_text(new_manifest.model_dump_json(indent=2))


@pytest.mark.integration
class TestAggregateVerifierReplacement:
    """The aggregate verifier proves replacement lineage against the
    bound rule and hardens the non-replacement child_campaign_id binding."""

    def test_plain_set_passes_without_rule(self, tmp_path: Path):
        plan, index, child_dirs = _setup_set(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.ok, result.failures

    def test_plain_set_passes_with_rule_bound(self, tmp_path: Path):
        plan, index, child_dirs = _setup_set(tmp_path)
        rule = make_replacement_rule(plan=plan)
        result = verify_campaign_set_aggregate(
            plan, index, child_dirs, replacement_rule=rule
        )
        assert result.ok, result.failures

    def test_replacement_child_passes_with_bound_rule(self, tmp_path: Path):
        plan_probe = _build_plan_small(
            [["task-001"], ["task-002"], ["task-003"], ["task-004"]],
            expected_child_count=1,
            expected_total_count=4,
        )
        rule = make_replacement_rule(plan=plan_probe)
        plan, index, child_dirs = _setup_set(
            tmp_path, rule=rule, replacement_child_index=0
        )
        binding = _read_binding(child_dirs[plan.child_plans[0].child_id])
        assert binding.supersedes_child_id == plan.child_plans[0].child_id
        assert binding.replacement_attempt == 1
        assert binding.replacement_rule_hash == rule.content_hash
        result = verify_campaign_set_aggregate(
            plan, index, child_dirs, replacement_rule=rule
        )
        assert result.ok, result.failures

    def test_replacement_lineage_without_bound_rule_fails(self, tmp_path: Path):
        plan_probe = _build_plan_small(
            [["task-001"], ["task-002"], ["task-003"], ["task-004"]],
            expected_child_count=1,
            expected_total_count=4,
        )
        rule = make_replacement_rule(plan=plan_probe)
        plan, index, child_dirs = _setup_set(
            tmp_path, rule=rule, replacement_child_index=0
        )
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("replacement lineage" in f for f in result.failures)

    def test_replacement_with_wrong_rule_fails(self, tmp_path: Path):
        plan_probe = _build_plan_small(
            [["task-001"], ["task-002"], ["task-003"], ["task-004"]],
            expected_child_count=1,
            expected_total_count=4,
        )
        rule = make_replacement_rule(plan=plan_probe)
        plan, index, child_dirs = _setup_set(
            tmp_path, rule=rule, replacement_child_index=0
        )
        wrong_rule = make_replacement_rule(plan=plan, rule_id="different-rule")
        result = verify_campaign_set_aggregate(
            plan, index, child_dirs, replacement_rule=wrong_rule
        )
        assert not result.ok
        assert any("replacement" in f for f in result.failures)

    def test_tampered_supersedes_child_id_fails(self, tmp_path: Path):
        plan_probe = _build_plan_small(
            [["task-001"], ["task-002"], ["task-003"], ["task-004"]],
            expected_child_count=1,
            expected_total_count=4,
        )
        rule = make_replacement_rule(plan=plan_probe)
        plan, index, child_dirs = _setup_set(
            tmp_path, rule=rule, replacement_child_index=0
        )
        # Point the lineage at a different plan child than the slot the
        # report directory is keyed under.
        _rewrite_binding(
            child_dirs[plan.child_plans[0].child_id],
            supersedes_child_id=plan.child_plans[1].child_id,
        )
        result = verify_campaign_set_aggregate(
            plan, index, child_dirs, replacement_rule=rule
        )
        assert not result.ok

    def test_tampered_replacement_attempt_fails(self, tmp_path: Path):
        plan_probe = _build_plan_small(
            [["task-001"], ["task-002"], ["task-003"], ["task-004"]],
            expected_child_count=1,
            expected_total_count=4,
        )
        rule = make_replacement_rule(plan=plan_probe)
        plan, index, child_dirs = _setup_set(
            tmp_path, rule=rule, replacement_child_index=0
        )
        _rewrite_binding(
            child_dirs[plan.child_plans[0].child_id],
            replacement_attempt=2,
        )
        result = verify_campaign_set_aggregate(
            plan, index, child_dirs, replacement_rule=rule
        )
        assert not result.ok

    def test_missing_binding_with_rule_bound_fails(self, tmp_path: Path):
        plan_probe = _build_plan_small(
            [["task-001"], ["task-002"], ["task-003"], ["task-004"]],
            expected_child_count=1,
            expected_total_count=4,
        )
        rule = make_replacement_rule(plan=plan_probe)
        plan, index, child_dirs = _setup_set(tmp_path, rule=rule)
        # Remove the binding entirely.
        manifest_path = child_dirs[plan.child_plans[0].child_id] / MANIFEST_JSON
        manifest = RunManifest.model_validate_json(manifest_path.read_text())
        manifest_path.write_text(
            manifest.model_copy(update={"campaign_binding": None}).model_dump_json(indent=2)
        )
        result = verify_campaign_set_aggregate(
            plan, index, child_dirs, replacement_rule=rule
        )
        assert not result.ok
        assert any("binding" in f for f in result.failures)
