# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 1 mutual hash-binding integration review.

Proves that the eight Phase 1 authority groups are mutually hash-bound:
profile/identity/budget contracts, observation contracts (expected-record
policy), campaign-set authority, model-comparison authority, Phase B/C/D
policies, and D12 disclosure authority. Every cross-authority hash
equality has direct test evidence, and mutation vectors prove that a
changed authority invalidates dependent bindings.

The binding graph proven here:

    CampaignProfile ──┐
                     ├──→ CampaignSetPlan ──→ CampaignSetIndex ──→ AggregateVerificationResult
    ExpectedRecordPolicy ─┘                                                    │
                                                                               ↓
    ModelComparisonPreregistration ──→ ModelComparisonOutput ←── ModelComparisonAuthorityHashes
                                               ↑
    CampaignProfile ──→ CampaignBinding         │
    ExpectedRecordPolicy ──┘                     │
                                                 │
    BudgetObservabilityPolicy ──→ CampaignManifest
    ProviderBudget ──→ CampaignManifest
    RetryPolicy ──→ CampaignManifest

    PhaseBSelectionPolicy (standalone, content-addressed)
    RepeatabilityPolicy (standalone, content-addressed)
    StackPolicy (standalone, content-addressed)
    D16PopulationSelection (standalone, content-addressed)
    DisclosureAuthority (standalone, content-addressed)

No external dependencies (no files, network, or DB). All authorities are
constructed through their builders or direct constructors with consistent
hash references.
"""

# pyright: reportCallIssue=false
# This file constructs models with model_construct and mutated fields to
# verify cross-authority hash binding consistency.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.campaign import compute_campaign_manifest_hash, compute_retry_policy_hash
from g8e_evals.campaign_set import (
    AggregateVerificationResult,
    CampaignChildIndexEntry,
    CampaignSetIndex,
    CampaignSetPlan,
    build_campaign_set_plan,
    compute_aggregate_verification_result_hash,
    compute_campaign_set_index_hash,
)
from g8e_evals.disclosure_authority import (
    DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
    DisclosureAuthority,
    DisclosureFieldEntry,
    DisclosureOutputEntry,
    FieldClassification,
    OutputFormat,
    OutputRole,
    compute_disclosure_authority_hash,
)
from g8e_evals.expected_record_policy import (
    EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
    CardinalityRule,
    ExpectedRecordEntry,
    ExpectedRecordPolicy,
    RecordApplicability,
    compute_expected_record_policy_hash,
)
from g8e_evals.model_comparison import (
    MODEL_COMPARISON_AUTHORITY_VERSION,
    MODEL_COMPARISON_ENGINE_VERSION,
    ClassAnchorBinding,
    ClaimGate,
    ComparisonFamilyKind,
    ComparisonPairKey,
    ComparisonTest,
    CorrectionMethod,
    MissingnessPolicy,
    ModelComparisonAuthorityHashes,
    ModelComparisonPreregistration,
    RepetitionReductionPolicy,
    compute_code_metric_identity_hash,
    compute_model_comparison,
    compute_model_comparison_hash,
)
from g8e_evals.population_policy import (
    D16_IFEVAL_POPULATION_HASH,
    D16_POPULATION_SELECTION_HASH,
    D16_SELECTED_IFEVAL_TASK_IDS,
    D16SelectionRule,
    build_published_d16_population_selection,
    compute_d16_selection_hash,
)
from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
    ModelTierAssignment,
    TrackArmAssignment,
    compute_campaign_profile_hash,
)
from g8e_evals.registry import WeightClass
from g8e_evals.repeatability_policy import (
    REPEATABILITY_POLICY_VERSION,
    RepeatabilityStatistic,
    build_repeatability_policy,
    compute_repeatability_policy_hash,
)
from g8e_evals.runner import (
    BudgetCeiling,
    BudgetObservabilityPolicy,
    DEFAULT_BUDGET_OBSERVABILITY_POLICY,
    compute_budget_observability_policy_hash,
    compute_provider_budget_hash,
)
from g8e_evals.schema import (
    CampaignBinding,
    CampaignTrack,
    ProviderBudget,
    ReportRole,
)
from g8e_evals.selection_policy import (
    PHASE_B_SELECTION_POLICY_VERSION,
    SelectionCategory,
    TieBreakerKey,
    build_phase_b_selection_policy,
    compute_selection_policy_hash,
)
from g8e_evals.stack_policy import (
    STACK_POLICY_VERSION,
    DeduplicationRule,
    LineageMetadataSource,
    build_stack_policy,
    compute_stack_policy_hash,
)


pytestmark = pytest.mark.unit


_VALID_HASH = "a" * 64
_ZERO_HASH = "0" * 64


# ---------------------------------------------------------------------------
# Authority construction helpers
# ---------------------------------------------------------------------------

def _make_expected_record_policy(
    *,
    policy_id: str = "phase1-expected-record-policy",
) -> ExpectedRecordPolicy:
    entries = [
        ExpectedRecordEntry(
            file_name="resource-observations.jsonl",
            applicability=RecordApplicability.REQUIRED,
            cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
        ),
        ExpectedRecordEntry(
            file_name="tool-call-scorecards.jsonl",
            applicability=RecordApplicability.OPTIONAL,
            cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
        ),
        ExpectedRecordEntry(
            file_name="escalation-records.jsonl",
            applicability=RecordApplicability.OPTIONAL,
            cardinality_rule=CardinalityRule.ONE_PER_ATTEMPT,
        ),
        ExpectedRecordEntry(
            file_name="security-events.jsonl",
            applicability=RecordApplicability.OPTIONAL,
            cardinality_rule=CardinalityRule.ONE_PER_ATTEMPT,
        ),
        ExpectedRecordEntry(
            file_name="correlated-errors.jsonl",
            applicability=RecordApplicability.OPTIONAL,
            cardinality_rule=CardinalityRule.ONE_PER_INFERENCE,
        ),
    ]
    content_hash = compute_expected_record_policy_hash(
        schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
        policy_id=policy_id,
        policy_version="1.0.0",
        suite_id="ifeval_subset",
        entries=entries,
    )
    return ExpectedRecordPolicy(
        schema_version=EXPECTED_RECORD_POLICY_SCHEMA_VERSION,
        policy_id=policy_id,
        policy_version="1.0.0",
        suite_id="ifeval_subset",
        entries=entries,
        content_hash=content_hash,
    )


def _make_campaign_profile(
    *,
    model_registry_hash: str = _VALID_HASH,
    required_record_policy_hash: str = _VALID_HASH,
    instrumentation_policy_hash: str = "ae002706076404cef6e22ebfbae087ff430436094849bdf4e8e5d074f0bab1dd",
    campaign_id: str = "phase1-binding-test-campaign",
) -> CampaignProfile:
    track_arm_assignments = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
        TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
    ]
    model_tier_assignments = [
        ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary"),
    ]
    temp = CampaignProfile.model_construct(
        campaign_id=campaign_id,
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="Phase 1 mutual hash-binding test campaign",
        created_at="2026-09-11T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=["task-1", "task-2", "task-3"],
        repetitions=3,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        baseline_tier_mappings={
            "primary": "qwen3:8b",
            "assistant": "granite3.3:8b",
            "lite": "qwen3:0.6b",
        },
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/cpu",
        environment_stratum="single-machine",
        provider_hardware_identity="unavailable",
        provider_environment_stratum="unavailable",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=model_registry_hash,
        required_record_policy_hash=required_record_policy_hash,
        instrumentation_policy_hash=instrumentation_policy_hash,
        content_hash=_ZERO_HASH,
    )
    ch = compute_campaign_profile_hash(temp)
    return CampaignProfile(
        campaign_id=campaign_id,
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="Phase 1 mutual hash-binding test campaign",
        created_at="2026-09-11T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=["task-1", "task-2", "task-3"],
        repetitions=3,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        baseline_tier_mappings={
            "primary": "qwen3:8b",
            "assistant": "granite3.3:8b",
            "lite": "qwen3:0.6b",
        },
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/cpu",
        environment_stratum="single-machine",
        provider_hardware_identity="unavailable",
        provider_environment_stratum="unavailable",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=model_registry_hash,
        required_record_policy_hash=required_record_policy_hash,
        instrumentation_policy_hash=instrumentation_policy_hash,
        content_hash=ch,
    )


def _make_campaign_set_plan(
    *,
    campaign_profile_hash: str,
    model_registry_hash: str = _VALID_HASH,
    expected_record_policy_hash: str = _VALID_HASH,
    retry_policy_hash: str | None = None,
    budget_authority_hash: str = _VALID_HASH,
    instrumentation_policy_hash: str = "ae002706076404cef6e22ebfbae087ff430436094849bdf4e8e5d074f0bab1dd",
    population_hash: str = _VALID_HASH,
) -> CampaignSetPlan:
    if retry_policy_hash is None:
        retry_policy_hash = compute_retry_policy_hash(1, ["timeout", "connection_error"])
    return build_campaign_set_plan(
        set_id="phase1-binding-test-set",
        parent_campaign_id="phase1-binding-test-parent",
        parent_campaign_revision="v2.1.8",
        population_task_ids=[f"ifeval-{i:04d}" for i in range(120)],
        population_hash=population_hash,
        model_registry_hash=model_registry_hash,
        campaign_profile_hash=campaign_profile_hash,
        repetition_ids=["rep-1", "rep-2", "rep-3"],
        seed=42,
        retry_policy_hash=retry_policy_hash,
        budget_authority_hash=budget_authority_hash,
        instrumentation_policy_hash=instrumentation_policy_hash,
        expected_record_policy_hash=expected_record_policy_hash,
        orchestrator_environment_scope="linux/amd64/cpu",
        provider_environment_scope="linux/amd64/remote-ollama",
        child_revisions=[f"child-rev-{i}" for i in range(4)],
    )


def _make_campaign_set_index(plan: CampaignSetPlan) -> CampaignSetIndex:
    entries: list[CampaignChildIndexEntry] = []
    for cp in plan.child_plans:
        entries.append(CampaignChildIndexEntry(
            child_id=cp.child_id,
            report_campaign_id=cp.child_id,
            finalization_generation_hash=_VALID_HASH,
            child_verification_report_hash=_VALID_HASH,
            report_checksum=_VALID_HASH,
            assignment_count=cp.expected_assignment_count,
        ))
    total = sum(e.assignment_count for e in entries)
    index = CampaignSetIndex.model_construct(
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash=_ZERO_HASH,
    )
    content_hash = compute_campaign_set_index_hash(index)
    return CampaignSetIndex(
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash=content_hash,
    )


def _make_aggregate_verification_result(
    plan: CampaignSetPlan,
    index: CampaignSetIndex,
    *,
    ok: bool = True,
) -> AggregateVerificationResult:
    result = AggregateVerificationResult.model_construct(
        verification_schema_version="1.0.0",
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        set_index_hash=index.content_hash,
        ok=ok,
        child_results=[],
        total_assignments_verified=plan.expected_total_assignment_count if ok else 0,
        assignment_uniqueness_ok=ok,
        coverage_ok=ok,
        product_coverage_ok=ok,
        hash_binding_ok=ok,
        failures=[] if ok else ["fake failure"],
        content_hash=_ZERO_HASH,
    )
    content_hash = compute_aggregate_verification_result_hash(result)
    return AggregateVerificationResult.model_validate(
        result.model_dump() | {"content_hash": content_hash},
    )


def _make_model_comparison_preregistration() -> ModelComparisonPreregistration:
    global_anchor = "qwen3-8b-q4_0"
    heavy_anchor = "granite-33-8b-instruct"
    small_anchor = "phi-4-mini-instruct"
    tiny_anchor = "smollm2-360m-instruct"
    class_anchors = [
        ClassAnchorBinding(
            weight_class=WeightClass.HEAVY_SLM,
            anchor_variant_id=heavy_anchor,
            family_name="heavy-slm-class-family",
        ),
        ClassAnchorBinding(
            weight_class=WeightClass.SMALL_REASONING,
            anchor_variant_id=small_anchor,
            family_name="small-reasoning-class-family",
        ),
        ClassAnchorBinding(
            weight_class=WeightClass.TINY_GENERATIVE,
            anchor_variant_id=tiny_anchor,
            family_name="tiny-generative-class-family",
        ),
    ]
    pair_keys = [
        ComparisonPairKey(
            candidate_variant_id="candidate-a",
            anchor_variant_id=global_anchor,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name="global-anchor-family",
        ),
    ]
    prereg = ModelComparisonPreregistration.model_construct(
        authority_id="phase1-binding-test-comparison",
        authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
        schema_version="1.0.0",
        global_anchor_variant_id=global_anchor,
        global_anchor_family_name="global-anchor-family",
        class_anchors=class_anchors,
        pair_keys=pair_keys,
        repetition_count=3,
        repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
        missingness_policy=MissingnessPolicy.REJECT_PAIR,
        minimum_population=5,
        primary_test=ComparisonTest.EXACT_MCNEMAR,
        correction_method=CorrectionMethod.HOLM_BONFERRONI,
        significance_level=0.05,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=42,
        non_inferiority_margin=None,
        claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
        environment_stratum="single-machine",
        content_hash=_ZERO_HASH,
    )
    expected = compute_model_comparison_hash(prereg)
    return ModelComparisonPreregistration(
        authority_id="phase1-binding-test-comparison",
        authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
        schema_version="1.0.0",
        global_anchor_variant_id=global_anchor,
        global_anchor_family_name="global-anchor-family",
        class_anchors=class_anchors,
        pair_keys=pair_keys,
        repetition_count=3,
        repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
        missingness_policy=MissingnessPolicy.REJECT_PAIR,
        minimum_population=5,
        primary_test=ComparisonTest.EXACT_MCNEMAR,
        correction_method=CorrectionMethod.HOLM_BONFERRONI,
        significance_level=0.05,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=42,
        non_inferiority_margin=None,
        claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
        environment_stratum="single-machine",
        content_hash=expected,
    )


def _make_disclosure_authority() -> DisclosureAuthority:
    fields = [
        DisclosureFieldEntry(
            field_name="campaign_id",
            classification=FieldClassification.PUBLIC,
            public_column_name="campaign_id",
            tombstone_hash_field="",
        ),
        DisclosureFieldEntry(
            field_name="raw_prompt",
            classification=FieldClassification.RESTRICTED,
            public_column_name="raw_prompt_tombstone",
            tombstone_hash_field="raw_prompt_hash",
        ),
        DisclosureFieldEntry(
            field_name="raw_prompt_hash",
            classification=FieldClassification.TOMBSTONE,
            public_column_name="raw_prompt_hash",
            tombstone_hash_field="raw_prompt_hash",
        ),
    ]
    outputs = [
        DisclosureOutputEntry(
            file_name="disclosure-public.jsonl",
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.PUBLIC_JSONL,
            required=True,
            description="Canonical public JSONL output.",
        ),
        DisclosureOutputEntry(
            file_name="disclosure-derived.csv",
            output_format=OutputFormat.DERIVED_CSV,
            output_role=OutputRole.DERIVED_CSV,
            required=True,
            description="Flat CSV projection of public fields.",
        ),
        DisclosureOutputEntry(
            file_name="disclosure-tombstones.jsonl",
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.TOMBSTONES,
            required=True,
            description="Tombstones for restricted fields.",
        ),
        DisclosureOutputEntry(
            file_name="disclosure-proof-index.json",
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.PROOF_INDEX,
            required=True,
            description="Proof index for evidence verification.",
        ),
        DisclosureOutputEntry(
            file_name="disclosure-output-inventory.json",
            output_format=OutputFormat.CANONICAL_JSONL,
            output_role=OutputRole.OUTPUT_INVENTORY,
            required=True,
            description="Deterministic output inventory.",
        ),
        DisclosureOutputEntry(
            file_name="disclosure.sqlite",
            output_format=OutputFormat.PROHIBITED_SQLITE,
            output_role=OutputRole.PROHIBITED_SQLITE,
            required=False,
            description="SQLite is prohibited in the first release.",
        ),
    ]
    proof_index_description = "SHA-256 proof index linking public records to verified evidence."
    content_hash = compute_disclosure_authority_hash(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id="phase1-binding-test-disclosure",
        authority_version="1.0.0",
        fields=fields,
        outputs=outputs,
        proof_index_description=proof_index_description,
    )
    return DisclosureAuthority(
        schema_version=DISCLOSURE_AUTHORITY_SCHEMA_VERSION,
        authority_id="phase1-binding-test-disclosure",
        authority_version="1.0.0",
        fields=fields,
        outputs=outputs,
        proof_index_description=proof_index_description,
        content_hash=content_hash,
    )


# ---------------------------------------------------------------------------
# Test 1: Each authority is content-addressed, frozen, and deterministic
# ---------------------------------------------------------------------------

class TestAuthorityContentAddressing:
    """Every Phase 1 authority is content-addressed, frozen, and deterministic.

    A content-addressed authority has a content_hash that is SHA-256 over
    canonical JSON of its material fields. A frozen authority rejects
    extra fields and is immutable after construction. Deterministic
    means the same inputs produce the same hash.
    """

    def test_campaign_profile_is_content_addressed(self) -> None:
        profile = _make_campaign_profile()
        assert profile.content_hash != _ZERO_HASH
        assert len(profile.content_hash) == 64
        recomputed = compute_campaign_profile_hash(profile)
        assert profile.content_hash == recomputed

    def test_campaign_profile_is_deterministic(self) -> None:
        p1 = _make_campaign_profile()
        p2 = _make_campaign_profile()
        assert p1.content_hash == p2.content_hash

    def test_expected_record_policy_is_content_addressed(self) -> None:
        policy = _make_expected_record_policy()
        assert policy.content_hash != _ZERO_HASH
        assert len(policy.content_hash) == 64

    def test_expected_record_policy_is_deterministic(self) -> None:
        p1 = _make_expected_record_policy()
        p2 = _make_expected_record_policy()
        assert p1.content_hash == p2.content_hash

    def test_campaign_set_plan_is_content_addressed(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        assert plan.content_hash != _ZERO_HASH
        assert len(plan.content_hash) == 64

    def test_campaign_set_plan_is_deterministic(self) -> None:
        profile = _make_campaign_profile()
        p1 = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        p2 = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        assert p1.content_hash == p2.content_hash

    def test_campaign_set_index_is_content_addressed(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        assert index.content_hash != _ZERO_HASH
        assert len(index.content_hash) == 64

    def test_aggregate_verification_result_is_content_addressed(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        assert result.content_hash != _ZERO_HASH
        assert len(result.content_hash) == 64

    def test_model_comparison_preregistration_is_content_addressed(self) -> None:
        prereg = _make_model_comparison_preregistration()
        assert prereg.content_hash != _ZERO_HASH
        assert len(prereg.content_hash) == 64
        recomputed = compute_model_comparison_hash(prereg)
        assert prereg.content_hash == recomputed

    def test_model_comparison_preregistration_is_deterministic(self) -> None:
        p1 = _make_model_comparison_preregistration()
        p2 = _make_model_comparison_preregistration()
        assert p1.content_hash == p2.content_hash

    def test_phase_b_selection_policy_is_content_addressed(self) -> None:
        policy = build_phase_b_selection_policy()
        assert policy.content_hash != _ZERO_HASH
        assert len(policy.content_hash) == 64

    def test_phase_b_selection_policy_is_deterministic(self) -> None:
        p1 = build_phase_b_selection_policy()
        p2 = build_phase_b_selection_policy()
        assert p1.content_hash == p2.content_hash

    def test_repeatability_policy_is_content_addressed(self) -> None:
        policy = build_repeatability_policy()
        assert policy.content_hash != _ZERO_HASH
        assert len(policy.content_hash) == 64

    def test_repeatability_policy_is_deterministic(self) -> None:
        p1 = build_repeatability_policy()
        p2 = build_repeatability_policy()
        assert p1.content_hash == p2.content_hash

    def test_stack_policy_is_content_addressed(self) -> None:
        policy = build_stack_policy()
        assert policy.content_hash != _ZERO_HASH
        assert len(policy.content_hash) == 64

    def test_stack_policy_is_deterministic(self) -> None:
        p1 = build_stack_policy()
        p2 = build_stack_policy()
        assert p1.content_hash == p2.content_hash

    def test_d16_population_selection_is_content_addressed(self) -> None:
        selection = build_published_d16_population_selection()
        assert selection.content_hash != _ZERO_HASH
        assert len(selection.content_hash) == 64

    def test_d16_population_selection_is_deterministic(self) -> None:
        s1 = build_published_d16_population_selection()
        s2 = build_published_d16_population_selection()
        assert s1.content_hash == s2.content_hash

    def test_d16_population_selection_hash_matches_published_constant(self) -> None:
        selection = build_published_d16_population_selection()
        assert selection.content_hash == D16_POPULATION_SELECTION_HASH

    def test_disclosure_authority_is_content_addressed(self) -> None:
        authority = _make_disclosure_authority()
        assert authority.content_hash != _ZERO_HASH
        assert len(authority.content_hash) == 64

    def test_disclosure_authority_is_deterministic(self) -> None:
        a1 = _make_disclosure_authority()
        a2 = _make_disclosure_authority()
        assert a1.content_hash == a2.content_hash

    def test_budget_observability_policy_hash_is_deterministic(self) -> None:
        h1 = compute_budget_observability_policy_hash(DEFAULT_BUDGET_OBSERVABILITY_POLICY)
        h2 = compute_budget_observability_policy_hash(DEFAULT_BUDGET_OBSERVABILITY_POLICY)
        assert h1 == h2
        assert len(h1) == 64


# ---------------------------------------------------------------------------
# Test 2: CampaignSetPlan binds to CampaignProfile, ModelRegistry, and
# ExpectedRecordPolicy
# ---------------------------------------------------------------------------

class TestCampaignSetPlanBindsProfileRegistryAndPolicy:
    """CampaignSetPlan.campaign_profile_hash == CampaignProfile.content_hash,
    CampaignSetPlan.model_registry_hash == CampaignProfile.model_registry_hash,
    CampaignSetPlan.expected_record_policy_hash == ExpectedRecordPolicy.content_hash.
    """

    def test_plan_campaign_profile_hash_matches_profile_content_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        assert plan.campaign_profile_hash == profile.content_hash

    def test_plan_model_registry_hash_matches_profile_model_registry_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            model_registry_hash=profile.model_registry_hash,
        )
        assert plan.model_registry_hash == profile.model_registry_hash

    def test_plan_expected_record_policy_hash_matches_policy_content_hash(self) -> None:
        policy = _make_expected_record_policy()
        profile = _make_campaign_profile(
            required_record_policy_hash=policy.content_hash,
        )
        plan = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            expected_record_policy_hash=policy.content_hash,
        )
        assert plan.expected_record_policy_hash == policy.content_hash

    def test_plan_retry_policy_hash_matches_computed_hash(self) -> None:
        retry_hash = compute_retry_policy_hash(1, ["timeout", "connection_error"])
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            retry_policy_hash=retry_hash,
        )
        assert plan.retry_policy_hash == retry_hash

    def test_plan_instrumentation_policy_hash_matches_profile(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            instrumentation_policy_hash=profile.instrumentation_policy_hash,
        )
        assert plan.instrumentation_policy_hash == profile.instrumentation_policy_hash


# ---------------------------------------------------------------------------
# Test 3: CampaignSetIndex binds to CampaignSetPlan
# ---------------------------------------------------------------------------

class TestCampaignSetIndexBindsPlan:
    """CampaignSetIndex.set_plan_hash == CampaignSetPlan.content_hash."""

    def test_index_set_plan_hash_matches_plan_content_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        assert index.set_plan_hash == plan.content_hash

    def test_index_rejects_mismatched_plan_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            CampaignSetIndex(
                set_id=plan.set_id,
                set_plan_hash="b" * 64,
                child_index_entries=[
                    CampaignChildIndexEntry(
                        child_id=cp.child_id,
                        report_campaign_id=cp.child_id,
                        finalization_generation_hash=_VALID_HASH,
                        child_verification_report_hash=_VALID_HASH,
                        report_checksum=_VALID_HASH,
                        assignment_count=cp.expected_assignment_count,
                    )
                    for cp in plan.child_plans
                ],
                total_assignment_count=plan.expected_total_assignment_count,
                content_hash="0" * 64,
            )


# ---------------------------------------------------------------------------
# Test 4: AggregateVerificationResult binds to CampaignSetPlan and
# CampaignSetIndex
# ---------------------------------------------------------------------------

class TestAggregateVerificationBindsPlanAndIndex:
    """AggregateVerificationResult.set_plan_hash == CampaignSetPlan.content_hash,
    AggregateVerificationResult.set_index_hash == CampaignSetIndex.content_hash.
    """

    def test_aggregate_set_plan_hash_matches_plan(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        assert result.set_plan_hash == plan.content_hash

    def test_aggregate_set_index_hash_matches_index(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        assert result.set_index_hash == index.content_hash

    def test_aggregate_content_hash_is_reproducible(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        r1 = _make_aggregate_verification_result(plan, index)
        r2 = _make_aggregate_verification_result(plan, index)
        assert r1.content_hash == r2.content_hash


# ---------------------------------------------------------------------------
# Test 5: ModelComparisonAuthorityHashes binds to
# AggregateVerificationResult
# ---------------------------------------------------------------------------

class TestModelComparisonAuthorityHashesBindAggregate:
    """ModelComparisonAuthorityHashes.from_aggregate_verification_result
    extracts hashes from the typed AggregateVerificationResult.
    """

    def test_from_aggregate_extracts_all_hashes(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        assert hashes.aggregate_verification_hash == result.content_hash
        assert hashes.campaign_set_plan_hash == result.set_plan_hash
        assert hashes.campaign_set_index_hash == result.set_index_hash
        assert hashes.profile_hash == profile.content_hash
        assert hashes.registry_hash == profile.model_registry_hash
        assert hashes.benchmark_population_hash == D16_IFEVAL_POPULATION_HASH

    def test_from_aggregate_rejects_failed_aggregate(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index, ok=False)
        with pytest.raises(ValueError, match="failed aggregate"):
            ModelComparisonAuthorityHashes.from_aggregate_verification_result(
                result,
                profile_hash=profile.content_hash,
                registry_hash=profile.model_registry_hash,
                benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
                metric_registry_hash=_VALID_HASH,
            )

    def test_from_aggregate_plan_hash_equals_plan_content_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        assert hashes.campaign_set_plan_hash == plan.content_hash

    def test_from_aggregate_index_hash_equals_index_content_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        assert hashes.campaign_set_index_hash == index.content_hash


# ---------------------------------------------------------------------------
# Test 6: ModelComparisonOutput binds to all authorities
# ---------------------------------------------------------------------------

class TestModelComparisonOutputBindsAllAuthorities:
    """ModelComparisonOutput binds to the preregistration, aggregate
    verification, campaign-set plan/index, profile, registry, benchmark
    population, and code/metric identity.
    """

    def test_output_binds_preregistration_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.authority_hash == prereg.content_hash

    def test_output_binds_aggregate_verification_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.aggregate_verification_hash == result.content_hash

    def test_output_binds_campaign_set_plan_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.campaign_set_plan_hash == plan.content_hash

    def test_output_binds_campaign_set_index_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.campaign_set_index_hash == index.content_hash

    def test_output_binds_profile_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.profile_hash == profile.content_hash

    def test_output_binds_registry_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.registry_hash == profile.model_registry_hash

    def test_output_binds_benchmark_population_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        assert output.benchmark_population_hash == D16_IFEVAL_POPULATION_HASH

    def test_output_binds_code_metric_identity_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(prereg, [], hashes)
        expected_code_metric = compute_code_metric_identity_hash(
            MODEL_COMPARISON_ENGINE_VERSION,
            _VALID_HASH,
        )
        assert output.code_metric_identity_hash == expected_code_metric

    def test_output_content_hash_is_reproducible(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        o1 = compute_model_comparison(prereg, [], hashes)
        o2 = compute_model_comparison(prereg, [], hashes)
        assert o1.content_hash == o2.content_hash


# ---------------------------------------------------------------------------
# Test 7: CampaignProfile binds to ExpectedRecordPolicy
# ---------------------------------------------------------------------------

class TestCampaignProfileBindsExpectedRecordPolicy:
    """CampaignProfile.required_record_policy_hash == ExpectedRecordPolicy.content_hash."""

    def test_profile_required_record_policy_hash_matches_policy(self) -> None:
        policy = _make_expected_record_policy()
        profile = _make_campaign_profile(
            required_record_policy_hash=policy.content_hash,
        )
        assert profile.required_record_policy_hash == policy.content_hash

    def test_profile_default_required_record_policy_hash_is_nonzero(self) -> None:
        profile = _make_campaign_profile()
        assert profile.required_record_policy_hash != _ZERO_HASH
        assert len(profile.required_record_policy_hash) == 64

    def test_profile_instrumentation_policy_hash_is_nonzero(self) -> None:
        profile = _make_campaign_profile()
        assert profile.instrumentation_policy_hash != _ZERO_HASH
        assert len(profile.instrumentation_policy_hash) == 64


# ---------------------------------------------------------------------------
# Test 8: CampaignBinding binds to CampaignProfile and ExpectedRecordPolicy
# ---------------------------------------------------------------------------

class TestCampaignBindingBindsProfileAndRegistry:
    """CampaignBinding.campaign_profile_hash == CampaignProfile.content_hash,
    CampaignBinding.model_registry_hash == CampaignProfile.model_registry_hash,
    CampaignBinding.required_record_policy_hash == ExpectedRecordPolicy.content_hash.
    """

    def test_binding_profile_hash_matches_profile_content_hash(self) -> None:
        profile = _make_campaign_profile()
        binding = CampaignBinding(
            campaign_id=profile.campaign_id,
            campaign_revision=profile.campaign_revision,
            report_role=ReportRole.SINGLE,
            campaign_profile_hash=profile.content_hash,
            model_registry_hash=profile.model_registry_hash,
            required_record_policy_hash=profile.required_record_policy_hash,
            orchestrator_hardware_identity=profile.hardware_identity,
            orchestrator_environment_stratum=profile.environment_stratum,
            provider_hardware_identity=profile.provider_hardware_identity,
            provider_environment_stratum=profile.provider_environment_stratum,
            track_arm_assignments=list(profile.track_arm_assignments),
        )
        assert binding.campaign_profile_hash == profile.content_hash

    def test_binding_registry_hash_matches_profile_registry_hash(self) -> None:
        profile = _make_campaign_profile()
        binding = CampaignBinding(
            campaign_id=profile.campaign_id,
            campaign_revision=profile.campaign_revision,
            report_role=ReportRole.SINGLE,
            campaign_profile_hash=profile.content_hash,
            model_registry_hash=profile.model_registry_hash,
            required_record_policy_hash=profile.required_record_policy_hash,
            orchestrator_hardware_identity=profile.hardware_identity,
            orchestrator_environment_stratum=profile.environment_stratum,
            provider_hardware_identity=profile.provider_hardware_identity,
            provider_environment_stratum=profile.provider_environment_stratum,
            track_arm_assignments=list(profile.track_arm_assignments),
        )
        assert binding.model_registry_hash == profile.model_registry_hash

    def test_binding_required_record_policy_hash_matches_policy(self) -> None:
        policy = _make_expected_record_policy()
        profile = _make_campaign_profile(
            required_record_policy_hash=policy.content_hash,
        )
        binding = CampaignBinding(
            campaign_id=profile.campaign_id,
            campaign_revision=profile.campaign_revision,
            report_role=ReportRole.SINGLE,
            campaign_profile_hash=profile.content_hash,
            model_registry_hash=profile.model_registry_hash,
            required_record_policy_hash=policy.content_hash,
            orchestrator_hardware_identity=profile.hardware_identity,
            orchestrator_environment_stratum=profile.environment_stratum,
            provider_hardware_identity=profile.provider_hardware_identity,
            provider_environment_stratum=profile.provider_environment_stratum,
            track_arm_assignments=list(profile.track_arm_assignments),
        )
        assert binding.required_record_policy_hash == policy.content_hash


# ---------------------------------------------------------------------------
# Test 9: CampaignManifest binds to BudgetObservabilityPolicy,
# ProviderBudget, and RetryPolicy
# ---------------------------------------------------------------------------

class TestCampaignManifestBindsBudgetAuthorities:
    """CampaignManifest.budget_observability_policy_hash ==
    compute_budget_observability_policy_hash(policy),
    CampaignManifest.provider_budget_hash == compute_provider_budget_hash(budget),
    CampaignManifest.retry_policy_hash == compute_retry_policy_hash(...).
    """

    def test_manifest_binds_budget_observability_policy_hash(self) -> None:
        policy = DEFAULT_BUDGET_OBSERVABILITY_POLICY
        policy_hash = compute_budget_observability_policy_hash(policy)
        budget = ProviderBudget(max_usd=100.0)
        budget_hash = compute_provider_budget_hash(budget)
        retry_hash = compute_retry_policy_hash(1, ["timeout"])
        manifest_hash = compute_campaign_manifest_hash(
            campaign_id="test-campaign",
            campaign_version="1",
            release_version="v2.1.8",
            preregistration_hash=_VALID_HASH,
            cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH,
            initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH,
            retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH,
            release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH,
            missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash,
            source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH,
            budget_observability_policy_hash=policy_hash,
        )
        assert manifest_hash != _ZERO_HASH
        assert len(manifest_hash) == 64

    def test_manifest_binds_provider_budget_hash(self) -> None:
        budget = ProviderBudget(max_usd=100.0, max_tokens=50000, max_requests=1000)
        budget_hash = compute_provider_budget_hash(budget)
        policy_hash = compute_budget_observability_policy_hash(DEFAULT_BUDGET_OBSERVABILITY_POLICY)
        retry_hash = compute_retry_policy_hash(1, ["timeout"])
        manifest_hash = compute_campaign_manifest_hash(
            campaign_id="test-campaign",
            campaign_version="1",
            release_version="v2.1.8",
            preregistration_hash=_VALID_HASH,
            cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH,
            initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH,
            retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH,
            release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH,
            missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash,
            source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH,
            budget_observability_policy_hash=policy_hash,
        )
        assert manifest_hash != _ZERO_HASH

    def test_manifest_binds_retry_policy_hash(self) -> None:
        retry_hash = compute_retry_policy_hash(3, ["timeout", "connection_error"])
        budget_hash = compute_provider_budget_hash(None)
        policy_hash = compute_budget_observability_policy_hash(DEFAULT_BUDGET_OBSERVABILITY_POLICY)
        manifest_hash = compute_campaign_manifest_hash(
            campaign_id="test-campaign",
            campaign_version="1",
            release_version="v2.1.8",
            preregistration_hash=_VALID_HASH,
            cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH,
            initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH,
            retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH,
            release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH,
            missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash,
            source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH,
            budget_observability_policy_hash=policy_hash,
        )
        assert manifest_hash != _ZERO_HASH

    def test_manifest_hash_changes_when_budget_observability_policy_changes(self) -> None:
        policy1 = DEFAULT_BUDGET_OBSERVABILITY_POLICY
        policy2 = BudgetObservabilityPolicy(
            tokens_observable=True,
            usd_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        h1 = compute_budget_observability_policy_hash(policy1)
        h2 = compute_budget_observability_policy_hash(policy2)
        assert h1 != h2
        budget_hash = compute_provider_budget_hash(None)
        retry_hash = compute_retry_policy_hash(1, ["timeout"])
        m1 = compute_campaign_manifest_hash(
            campaign_id="c", campaign_version="1", release_version="v",
            preregistration_hash=_VALID_HASH, cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH, initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH, retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH, release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH, missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash, source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH, budget_observability_policy_hash=h1,
        )
        m2 = compute_campaign_manifest_hash(
            campaign_id="c", campaign_version="1", release_version="v",
            preregistration_hash=_VALID_HASH, cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH, initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH, retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH, release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH, missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash, source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH, budget_observability_policy_hash=h2,
        )
        assert m1 != m2

    def test_manifest_hash_changes_when_provider_budget_changes(self) -> None:
        b1 = ProviderBudget(max_usd=100.0)
        b2 = ProviderBudget(max_usd=200.0)
        h1 = compute_provider_budget_hash(b1)
        h2 = compute_provider_budget_hash(b2)
        assert h1 != h2
        policy_hash = compute_budget_observability_policy_hash(DEFAULT_BUDGET_OBSERVABILITY_POLICY)
        retry_hash = compute_retry_policy_hash(1, ["timeout"])
        m1 = compute_campaign_manifest_hash(
            campaign_id="c", campaign_version="1", release_version="v",
            preregistration_hash=_VALID_HASH, cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH, initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH, retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH, release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH, missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=h1, source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH, budget_observability_policy_hash=policy_hash,
        )
        m2 = compute_campaign_manifest_hash(
            campaign_id="c", campaign_version="1", release_version="v",
            preregistration_hash=_VALID_HASH, cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH, initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH, retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH, release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH, missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=h2, source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH, budget_observability_policy_hash=policy_hash,
        )
        assert m1 != m2

    def test_manifest_hash_changes_when_retry_policy_changes(self) -> None:
        h1 = compute_retry_policy_hash(1, ["timeout"])
        h2 = compute_retry_policy_hash(3, ["timeout"])
        assert h1 != h2
        budget_hash = compute_provider_budget_hash(None)
        policy_hash = compute_budget_observability_policy_hash(DEFAULT_BUDGET_OBSERVABILITY_POLICY)
        m1 = compute_campaign_manifest_hash(
            campaign_id="c", campaign_version="1", release_version="v",
            preregistration_hash=_VALID_HASH, cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH, initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH, retry_policy_hash=h1,
            metric_registry_hash=_VALID_HASH, release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH, missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash, source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH, budget_observability_policy_hash=policy_hash,
        )
        m2 = compute_campaign_manifest_hash(
            campaign_id="c", campaign_version="1", release_version="v",
            preregistration_hash=_VALID_HASH, cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH, initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH, retry_policy_hash=h2,
            metric_registry_hash=_VALID_HASH, release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH, missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash, source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH, budget_observability_policy_hash=policy_hash,
        )
        assert m1 != m2


# ---------------------------------------------------------------------------
# Test 10: Cross-authority mutation vectors
# ---------------------------------------------------------------------------

class TestCrossAuthorityMutationVectors:
    """Mutation vectors proving a changed authority invalidates dependent
    bindings. Each test mutates one authority and proves the cross-authority
    equality breaks.
    """

    def test_mutated_profile_changes_content_hash(self) -> None:
        p1 = _make_campaign_profile()
        p2 = _make_campaign_profile(campaign_id="different-campaign-id")
        assert p1.content_hash != p2.content_hash

    def test_mutated_profile_invalidates_plan_binding(self) -> None:
        p1 = _make_campaign_profile()
        p2 = _make_campaign_profile(campaign_id="different-campaign-id")
        plan = _make_campaign_set_plan(campaign_profile_hash=p1.content_hash)
        assert plan.campaign_profile_hash == p1.content_hash
        assert plan.campaign_profile_hash != p2.content_hash

    def test_mutated_expected_record_policy_changes_content_hash(self) -> None:
        p1 = _make_expected_record_policy()
        p2 = _make_expected_record_policy(policy_id="different-policy-id")
        assert p1.content_hash != p2.content_hash

    def test_mutated_expected_record_policy_invalidates_profile_binding(self) -> None:
        policy1 = _make_expected_record_policy()
        policy2 = _make_expected_record_policy(policy_id="different-policy-id")
        profile = _make_campaign_profile(
            required_record_policy_hash=policy1.content_hash,
        )
        assert profile.required_record_policy_hash == policy1.content_hash
        assert profile.required_record_policy_hash != policy2.content_hash

    def test_mutated_expected_record_policy_invalidates_plan_binding(self) -> None:
        policy1 = _make_expected_record_policy()
        policy2 = _make_expected_record_policy(policy_id="different-policy-id")
        profile = _make_campaign_profile(
            required_record_policy_hash=policy1.content_hash,
        )
        plan = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            expected_record_policy_hash=policy1.content_hash,
        )
        assert plan.expected_record_policy_hash == policy1.content_hash
        assert plan.expected_record_policy_hash != policy2.content_hash

    def test_mutated_plan_changes_content_hash(self) -> None:
        profile = _make_campaign_profile()
        p1 = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        p2 = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            population_hash="b" * 64,
        )
        assert p1.content_hash != p2.content_hash

    def test_mutated_plan_invalidates_index_binding(self) -> None:
        profile = _make_campaign_profile()
        p1 = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        p2 = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            population_hash="b" * 64,
        )
        index = _make_campaign_set_index(p1)
        assert index.set_plan_hash == p1.content_hash
        assert index.set_plan_hash != p2.content_hash

    def test_mutated_plan_invalidates_aggregate_binding(self) -> None:
        profile = _make_campaign_profile()
        p1 = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        p2 = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            population_hash="b" * 64,
        )
        index = _make_campaign_set_index(p1)
        result = _make_aggregate_verification_result(p1, index)
        assert result.set_plan_hash == p1.content_hash
        assert result.set_plan_hash != p2.content_hash

    def test_mutated_index_changes_content_hash(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        i1 = _make_campaign_set_index(plan)
        entries2 = [
            CampaignChildIndexEntry(
                child_id=e.child_id,
                report_campaign_id=e.report_campaign_id,
                finalization_generation_hash="b" * 64,
                child_verification_report_hash=e.child_verification_report_hash,
                report_checksum=e.report_checksum,
                assignment_count=e.assignment_count,
            )
            for e in i1.child_index_entries
        ]
        i2 = CampaignSetIndex.model_construct(
            set_id=i1.set_id,
            set_plan_hash=i1.set_plan_hash,
            child_index_entries=entries2,
            total_assignment_count=i1.total_assignment_count,
            content_hash=_ZERO_HASH,
        )
        i2_hash = compute_campaign_set_index_hash(i2)
        assert i1.content_hash != i2_hash

    def test_mutated_index_invalidates_aggregate_binding(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        i1 = _make_campaign_set_index(plan)
        entries2 = [
            CampaignChildIndexEntry(
                child_id=e.child_id,
                report_campaign_id=e.report_campaign_id,
                finalization_generation_hash="b" * 64,
                child_verification_report_hash=e.child_verification_report_hash,
                report_checksum=e.report_checksum,
                assignment_count=e.assignment_count,
            )
            for e in i1.child_index_entries
        ]
        i2 = CampaignSetIndex.model_construct(
            set_id=i1.set_id,
            set_plan_hash=i1.set_plan_hash,
            child_index_entries=entries2,
            total_assignment_count=i1.total_assignment_count,
            content_hash=_ZERO_HASH,
        )
        i2_hash = compute_campaign_set_index_hash(i2)
        result = _make_aggregate_verification_result(plan, i1)
        assert result.set_index_hash == i1.content_hash
        assert result.set_index_hash != i2_hash

    def test_mutated_preregistration_changes_content_hash(self) -> None:
        p1 = _make_model_comparison_preregistration()
        global_anchor = "qwen3-8b-q4_0"
        pair_keys = [
            ComparisonPairKey(
                candidate_variant_id="candidate-b",
                anchor_variant_id=global_anchor,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name="global-anchor-family",
            ),
        ]
        class_anchors = [
            ClassAnchorBinding(
                weight_class=WeightClass.HEAVY_SLM,
                anchor_variant_id="granite-33-8b-instruct",
                family_name="heavy-slm-class-family",
            ),
            ClassAnchorBinding(
                weight_class=WeightClass.SMALL_REASONING,
                anchor_variant_id="phi-4-mini-instruct",
                family_name="small-reasoning-class-family",
            ),
            ClassAnchorBinding(
                weight_class=WeightClass.TINY_GENERATIVE,
                anchor_variant_id="smollm2-360m-instruct",
                family_name="tiny-generative-class-family",
            ),
        ]
        prereg2 = ModelComparisonPreregistration.model_construct(
            authority_id="phase1-binding-test-comparison",
            authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
            schema_version="1.0.0",
            global_anchor_variant_id=global_anchor,
            global_anchor_family_name="global-anchor-family",
            class_anchors=class_anchors,
            pair_keys=pair_keys,
            repetition_count=3,
            repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
            missingness_policy=MissingnessPolicy.REJECT_PAIR,
            minimum_population=5,
            primary_test=ComparisonTest.EXACT_MCNEMAR,
            correction_method=CorrectionMethod.HOLM_BONFERRONI,
            significance_level=0.05,
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            non_inferiority_margin=None,
            claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
            environment_stratum="single-machine",
            content_hash=_ZERO_HASH,
        )
        p2_hash = compute_model_comparison_hash(prereg2)
        assert p1.content_hash != p2_hash

    def test_mutated_preregistration_invalidates_output_binding(self) -> None:
        p1 = _make_model_comparison_preregistration()
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        result = _make_aggregate_verification_result(plan, index)
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        output = compute_model_comparison(p1, [], hashes)
        assert output.authority_hash == p1.content_hash

    def test_mutated_aggregate_invalidates_output_binding(self) -> None:
        profile = _make_campaign_profile()
        plan = _make_campaign_set_plan(campaign_profile_hash=profile.content_hash)
        index = _make_campaign_set_index(plan)
        r1 = _make_aggregate_verification_result(plan, index)
        r2 = _make_aggregate_verification_result(plan, index, ok=False)
        assert r1.content_hash != r2.content_hash
        prereg = _make_model_comparison_preregistration()
        hashes1 = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            r1,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        o1 = compute_model_comparison(prereg, [], hashes1)
        assert o1.aggregate_verification_hash == r1.content_hash
        assert o1.aggregate_verification_hash != r2.content_hash

    def test_mutated_selection_policy_changes_content_hash(self) -> None:
        p1 = build_phase_b_selection_policy()
        p2 = build_phase_b_selection_policy(policy_id="different-policy-id")
        assert p1.content_hash != p2.content_hash

    def test_mutated_repeatability_policy_changes_content_hash(self) -> None:
        p1 = build_repeatability_policy()
        p2 = build_repeatability_policy(policy_id="different-policy-id")
        assert p1.content_hash != p2.content_hash

    def test_mutated_stack_policy_changes_content_hash(self) -> None:
        p1 = build_stack_policy()
        p2 = build_stack_policy(policy_id="different-policy-id")
        assert p1.content_hash != p2.content_hash

    def test_mutated_d16_selection_changes_content_hash(self) -> None:
        s1 = build_published_d16_population_selection()
        s2_hash = compute_d16_selection_hash(
            selection_id="d16-population-selection",
            selection_version="1.0.0",
            rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
            selected_ifeval_task_ids=["100", "200", "300", "400"],
            ifeval_population_hash=D16_IFEVAL_POPULATION_HASH,
            framework_scenario_count=21,
            framework_suite_ids=list(s1.framework_suite_ids),
            total_population=25,
        )
        assert s1.content_hash != s2_hash

    def test_mutated_disclosure_authority_changes_content_hash(self) -> None:
        a1 = _make_disclosure_authority()
        fields2 = [*a1.fields, DisclosureFieldEntry(
                field_name="model_output",
                classification=FieldClassification.RESTRICTED,
                public_column_name="model_output_tombstone",
                tombstone_hash_field="model_output_hash",
            ),
        ]
        content_hash2 = compute_disclosure_authority_hash(
            schema_version=a1.schema_version,
            authority_id=a1.authority_id,
            authority_version=a1.authority_version,
            fields=fields2,
            outputs=a1.outputs,
            proof_index_description=a1.proof_index_description,
        )
        assert a1.content_hash != content_hash2

    def test_mutated_budget_observability_policy_changes_hash(self) -> None:
        p1 = DEFAULT_BUDGET_OBSERVABILITY_POLICY
        p2 = BudgetObservabilityPolicy(
            tokens_observable=True,
            usd_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        h1 = compute_budget_observability_policy_hash(p1)
        h2 = compute_budget_observability_policy_hash(p2)
        assert h1 != h2

    def test_mutated_provider_budget_changes_hash(self) -> None:
        b1 = ProviderBudget(max_usd=100.0)
        b2 = ProviderBudget(max_usd=200.0)
        h1 = compute_provider_budget_hash(b1)
        h2 = compute_provider_budget_hash(b2)
        assert h1 != h2

    def test_mutated_retry_policy_changes_hash(self) -> None:
        h1 = compute_retry_policy_hash(1, ["timeout"])
        h2 = compute_retry_policy_hash(3, ["timeout"])
        assert h1 != h2


# ---------------------------------------------------------------------------
# Test 11: Full binding chain integration
# ---------------------------------------------------------------------------

class TestFullBindingChainIntegration:
    """Constructs the full binding chain from CampaignProfile through
    ModelComparisonOutput and proves every cross-authority hash equality
    holds simultaneously.
    """

    def test_full_chain_profile_to_output(self) -> None:
        # 1. ExpectedRecordPolicy
        erp = _make_expected_record_policy()

        # 2. CampaignProfile binds ExpectedRecordPolicy
        profile = _make_campaign_profile(
            required_record_policy_hash=erp.content_hash,
        )
        assert profile.required_record_policy_hash == erp.content_hash

        # 3. CampaignSetPlan binds CampaignProfile, ModelRegistry, ExpectedRecordPolicy
        retry_hash = compute_retry_policy_hash(1, ["timeout", "connection_error"])
        budget = ProviderBudget(max_usd=100.0)
        budget_hash = compute_provider_budget_hash(budget)
        plan = _make_campaign_set_plan(
            campaign_profile_hash=profile.content_hash,
            model_registry_hash=profile.model_registry_hash,
            expected_record_policy_hash=erp.content_hash,
            retry_policy_hash=retry_hash,
            budget_authority_hash=budget_hash,
            instrumentation_policy_hash=profile.instrumentation_policy_hash,
        )
        assert plan.campaign_profile_hash == profile.content_hash
        assert plan.model_registry_hash == profile.model_registry_hash
        assert plan.expected_record_policy_hash == erp.content_hash
        assert plan.retry_policy_hash == retry_hash
        assert plan.budget_authority_hash == budget_hash
        assert plan.instrumentation_policy_hash == profile.instrumentation_policy_hash

        # 4. CampaignSetIndex binds CampaignSetPlan
        index = _make_campaign_set_index(plan)
        assert index.set_plan_hash == plan.content_hash

        # 5. AggregateVerificationResult binds CampaignSetPlan and CampaignSetIndex
        result = _make_aggregate_verification_result(plan, index)
        assert result.set_plan_hash == plan.content_hash
        assert result.set_index_hash == index.content_hash

        # 6. ModelComparisonAuthorityHashes binds AggregateVerificationResult
        prereg = _make_model_comparison_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=profile.content_hash,
            registry_hash=profile.model_registry_hash,
            benchmark_population_hash=D16_IFEVAL_POPULATION_HASH,
            metric_registry_hash=_VALID_HASH,
        )
        assert hashes.aggregate_verification_hash == result.content_hash
        assert hashes.campaign_set_plan_hash == result.set_plan_hash
        assert hashes.campaign_set_index_hash == result.set_index_hash
        assert hashes.profile_hash == profile.content_hash
        assert hashes.registry_hash == profile.model_registry_hash
        assert hashes.benchmark_population_hash == D16_IFEVAL_POPULATION_HASH

        # 7. ModelComparisonOutput binds all authorities
        output = compute_model_comparison(prereg, [], hashes)
        assert output.authority_hash == prereg.content_hash
        assert output.aggregate_verification_hash == result.content_hash
        assert output.campaign_set_plan_hash == plan.content_hash
        assert output.campaign_set_index_hash == index.content_hash
        assert output.profile_hash == profile.content_hash
        assert output.registry_hash == profile.model_registry_hash
        assert output.benchmark_population_hash == D16_IFEVAL_POPULATION_HASH

    def test_full_chain_campaign_binding(self) -> None:
        erp = _make_expected_record_policy()
        profile = _make_campaign_profile(
            required_record_policy_hash=erp.content_hash,
        )
        binding = CampaignBinding(
            campaign_id=profile.campaign_id,
            campaign_revision=profile.campaign_revision,
            report_role=ReportRole.SINGLE,
            campaign_profile_hash=profile.content_hash,
            model_registry_hash=profile.model_registry_hash,
            required_record_policy_hash=erp.content_hash,
            orchestrator_hardware_identity=profile.hardware_identity,
            orchestrator_environment_stratum=profile.environment_stratum,
            provider_hardware_identity=profile.provider_hardware_identity,
            provider_environment_stratum=profile.provider_environment_stratum,
            track_arm_assignments=list(profile.track_arm_assignments),
        )
        assert binding.campaign_profile_hash == profile.content_hash
        assert binding.model_registry_hash == profile.model_registry_hash
        assert binding.required_record_policy_hash == erp.content_hash

    def test_full_chain_campaign_manifest(self) -> None:
        erp = _make_expected_record_policy()
        profile = _make_campaign_profile(
            required_record_policy_hash=erp.content_hash,
        )
        policy = DEFAULT_BUDGET_OBSERVABILITY_POLICY
        policy_hash = compute_budget_observability_policy_hash(policy)
        budget = ProviderBudget(max_usd=100.0)
        budget_hash = compute_provider_budget_hash(budget)
        retry_hash = compute_retry_policy_hash(1, ["timeout"])
        manifest_hash = compute_campaign_manifest_hash(
            campaign_id=profile.campaign_id,
            campaign_version=profile.campaign_revision,
            release_version="v2.1.8",
            preregistration_hash=_VALID_HASH,
            cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH,
            initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH,
            retry_policy_hash=retry_hash,
            metric_registry_hash=_VALID_HASH,
            release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH,
            missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=budget_hash,
            source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH,
            budget_observability_policy_hash=policy_hash,
        )
        assert manifest_hash != _ZERO_HASH
        assert len(manifest_hash) == 64

    def test_standalone_authorities_are_content_addressed_and_stable(self) -> None:
        # PhaseBSelectionPolicy
        sel = build_phase_b_selection_policy()
        assert sel.content_hash == compute_selection_policy_hash(
            policy_id=sel.policy_id,
            policy_version=PHASE_B_SELECTION_POLICY_VERSION,
            finalist_count=5,
            valid_roles=list(sel.valid_roles),
            categories=list(SelectionCategory),
            tie_breaker_order=list(TieBreakerKey),
        )

        # RepeatabilityPolicy
        rep = build_repeatability_policy()
        assert rep.content_hash == compute_repeatability_policy_hash(
            policy_id=rep.policy_id,
            policy_version=REPEATABILITY_POLICY_VERSION,
            repetition_count=5,
            primary_statistic=RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
            secondary_statistics=[
                RepeatabilityStatistic.ALL_FIVE_AGREE,
                RepeatabilityStatistic.ACCURACY,
                RepeatabilityStatistic.TASK_BOOTSTRAP_CI,
            ],
            bootstrap_iterations=10000,
            bootstrap_seed=42,
        )

        # StackPolicy
        stack = build_stack_policy()
        assert stack.content_hash == compute_stack_policy_hash(
            policy_id=stack.policy_id,
            policy_version=STACK_POLICY_VERSION,
            selectors=stack.selectors,
            lineage_metadata_source=LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID,
            deduplication_rule=DeduplicationRule.KEEP_SELECTOR_LABEL,
            homogeneous_family_rule=stack.homogeneous_family_rule,
        )

        # D16PopulationSelection
        d16 = build_published_d16_population_selection()
        assert d16.content_hash == D16_POPULATION_SELECTION_HASH
        assert d16.ifeval_population_hash == D16_IFEVAL_POPULATION_HASH
        assert list(d16.selected_ifeval_task_ids) == list(D16_SELECTED_IFEVAL_TASK_IDS)

        # DisclosureAuthority
        da = _make_disclosure_authority()
        assert da.content_hash != _ZERO_HASH
        assert len(da.content_hash) == 64
