# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed inventory of every registered deterministic grader.

The inventory is the single source of truth for grader identity, eligible
task shapes, required evidence, denominator behavior, metric linkage,
CLI wiring, producer paths, and conformance-case coverage status. It
is consumed by conformance tests (Phase 0.2) and release-gate checks to
verify that every registered grader is either backed by a complete
conformance matrix or represented by an explicit typed exclusion.

The inventory is validated against the live ``_GRADERS`` registry and
the ``DEFAULT_METRIC_REGISTRY`` at import time so that drift between the
inventory, the grader registry, and the metric registry is detected
immediately rather than at release time.
"""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.graders import _GRADERS
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.schema import GraderClass


class ProducerPath(StrEnum):
    """How evidence for a grader is produced in the evaluation pipeline."""

    SYNTHETIC = "synthetic"
    REAL_PROVIDER = "real_provider"
    CROSS_CUTTING = "cross_cutting"
    PARTIAL_EXTERNAL = "partial_external"


class ConformanceCaseCategory(StrEnum):
    """The required conformance-case categories for every authoritative grader.

    Every grader must have at least one Tier 1 conformance case for each
    category, or an explicit typed exclusion explaining why the category
    does not apply. Absence is never treated as success.
    """

    PASSING_EVIDENCE = "passing_evidence"
    MEASURED_FAILURE = "measured_failure"
    MISSING_EVIDENCE = "missing_evidence"
    MALFORMED_EVIDENCE = "malformed_evidence"
    DUPLICATE_EVIDENCE = "duplicate_evidence"
    WRONG_RUN_BINDING = "wrong_run_binding"
    WRONG_ATTEMPT_BINDING = "wrong_attempt_binding"
    WRONG_TASK_BINDING = "wrong_task_binding"
    WRONG_ACTION_BINDING = "wrong_action_binding"
    WRONG_BOUNDARY_BINDING = "wrong_boundary_binding"
    WRONG_SOURCE_BINDING = "wrong_source_binding"
    UNSUPPORTED_VERSION = "unsupported_version"
    NOT_APPLICABLE = "not_applicable"
    DENOMINATOR_BEHAVIOR = "denominator_behavior"


class ConformanceExclusion(BaseModel):
    """An explicit typed exclusion for a conformance-case category.

    Records why a category does not apply to a grader rather than
    treating absence as success. The ``reason`` must be non-empty and
    specific.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    category: ConformanceCaseCategory
    reason: str = Field(min_length=1)


class GraderInventoryEntry(BaseModel):
    """Typed inventory entry for one registered deterministic grader.

    Binds grader identity (ID, version, class) to its eligible task
    shape (assertion field on ``TaskDefinition`` and observation field
    on ``DeterministicGradingContext``), required evidence types,
    denominator behavior, metric IDs produced, CLI wiring constant,
    producer path, and conformance-case coverage status.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    grader_id: str = Field(min_length=1)
    grader_version: str = Field(min_length=1)
    grader_class: GraderClass

    assertion_field: str | None = Field(
        default=None,
        description=(
            "Typed assertion-list field name on TaskDefinition that "
            "declares the expected evidence for this grader, or None "
            "if the grader consumes cross-cutting fields (receipts, "
            "stages, posture, expected_action_class, etc.)."
        ),
    )
    observation_field: str | None = Field(
        default=None,
        description=(
            "Typed observation-list field name on "
            "DeterministicGradingContext that carries the observed "
            "evidence for this grader, or None if the grader consumes "
            "receipts or stages directly."
        ),
    )

    metric_ids: list[str] = Field(
        min_length=1,
        description="Metric IDs produced by this grader, linked to the metric registry.",
    )
    evidence_requirements: list[str] = Field(
        min_length=1,
        description="Evidence artifact types required to support this grader's metrics.",
    )
    denominator: str = Field(
        min_length=1,
        description="Description of denominator semantics for aggregation.",
    )

    cli_constant: str = Field(
        min_length=1,
        description="CLI module constant name that wires this grader ID.",
    )

    producer_path: ProducerPath
    producer_suite_ids: list[str] = Field(
        default_factory=list,
        description="Benchmark suite IDs that produce evidence for this grader.",
    )

    conformance_exclusions: list[ConformanceExclusion] = Field(default_factory=list)

    @property
    def key(self) -> tuple[str, str]:
        return (self.grader_id, self.grader_version)

    def has_exclusion(self, category: ConformanceCaseCategory) -> bool:
        return any(excl.category == category for excl in self.conformance_exclusions)

    def excluded_categories(self) -> set[ConformanceCaseCategory]:
        return {excl.category for excl in self.conformance_exclusions}

    def required_categories(self) -> set[ConformanceCaseCategory]:
        """Categories that must have conformance cases (not excluded)."""
        return set(ConformanceCaseCategory) - self.excluded_categories()


_GRADER_VERSION = "1.0.0"
_DET = GraderClass.DETERMINISTIC


_INVENTORY: list[GraderInventoryEntry] = [
    GraderInventoryEntry(
        grader_id="ifeval_subset_verifier",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field=None,
        observation_field=None,
        metric_ids=["ifeval_subset_verifier"],
        evidence_requirements=["normalized_attempt_evidence", "ifeval_verifier_score"],
        denominator="Total number of attempted IFEval subset tasks.",
        cli_constant="_IFEVAL_GRADER_ID",
        producer_path=ProducerPath.PARTIAL_EXTERNAL,
        producer_suite_ids=["ifeval_subset"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="ifeval_subset_verifier consumes instruction-following verification, not action-class receipts",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="ifeval_subset_verifier consumes instruction-following verification, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_SOURCE_BINDING,
                reason="ifeval_subset_verifier consumes instruction-following verification, not source-bound observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="ifeval_subset_verifier is binary pass/fail; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
                reason="ifeval_subset_verifier is binary pass/fail with denominator_contribution=1; no variable denominator",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.DUPLICATE_EVIDENCE,
                reason="ifeval_subset_verifier is a partial external grader with separate dispatch via IFEvalVerifier; duplicate-evidence handling is covered by the ifeval_subset suite's own conformance tests",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.MALFORMED_EVIDENCE,
                reason="ifeval_subset_verifier is a partial external grader with separate dispatch via IFEvalVerifier; malformed-evidence handling is covered by the ifeval_subset suite's own conformance tests",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_RUN_BINDING,
                reason="ifeval_subset_verifier is a partial external grader with separate dispatch via IFEvalVerifier; run-binding handling is covered by the ifeval_subset suite's own conformance tests",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ATTEMPT_BINDING,
                reason="ifeval_subset_verifier is a partial external grader with separate dispatch via IFEvalVerifier; attempt-binding handling is covered by the ifeval_subset suite's own conformance tests",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_TASK_BINDING,
                reason="ifeval_subset_verifier is a partial external grader with separate dispatch via IFEvalVerifier; task-binding handling is covered by the ifeval_subset suite's own conformance tests",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="receipt_integrity",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field=None,
        observation_field=None,
        metric_ids=["receipt_integrity"],
        evidence_requirements=["primary_receipt", "receipt_persistence_stage"],
        denominator="Total number of attempts with a declared expected action class.",
        cli_constant="_RECEIPT_INTEGRITY_GRADER_ID",
        producer_path=ProducerPath.CROSS_CUTTING,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="receipt_integrity consumes receipts and persistence stages, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_SOURCE_BINDING,
                reason="receipt_integrity consumes receipts and persistence stages, not source-bound observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="receipt_integrity is binary pass/fail; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
                reason="receipt_integrity is binary pass/fail with denominator_contribution=1; no variable denominator",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="canary_scrubbing",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="sensitive_canary_annotations",
        observation_field=None,
        metric_ids=["canary_scrubbing"],
        evidence_requirements=["scrubbing_stage", "canary_scrubbing_assertion"],
        denominator="Total number of declared canary scrubbing assertions.",
        cli_constant="_CANARY_SCRUBBING_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_boundary_leakage"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="canary_scrubbing consumes scrubbing stages, not action-class receipts",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="canary_scrubbing consumes scrubbing stages, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="canary_scrubbing is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="model_boundary_raw_secret_rate",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="sensitive_canary_annotations",
        observation_field=None,
        metric_ids=["model_boundary_raw_secret_rate"],
        evidence_requirements=["model_boundary_privacy_attestation", "canary_scrubbing_assertion"],
        denominator="Total number of injected canary occurrences across all assertions.",
        cli_constant="_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID",
        producer_path=ProducerPath.REAL_PROVIDER,
        producer_suite_ids=["privacy_boundary_leakage"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="model_boundary_raw_secret_rate consumes model-boundary attestations, not action-class receipts",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="model_boundary_raw_secret_rate consumes model-boundary attestations, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="model_boundary_raw_secret_rate is a rate; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
                reason="model_boundary_raw_secret_rate denominator is the injected canary count, not a per-assertion variable denominator",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="exact_local_rehydration",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="rehydration_assertions",
        observation_field="rehydration_observations",
        metric_ids=["exact_local_rehydration"],
        evidence_requirements=["rehydration_observation", "rehydration_assertion"],
        denominator="Total number of declared rehydration assertions.",
        cli_constant="_EXACT_LOCAL_REHYDRATION_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_boundary_leakage"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="exact_local_rehydration consumes rehydration observations, not action-class receipts",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="exact_local_rehydration is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="secret_detection_precision",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="secret_detection_assertions",
        observation_field="secret_detection_observations",
        metric_ids=["secret_detection_precision"],
        evidence_requirements=["secret_detection_observation", "secret_detection_assertion"],
        denominator="True positives plus false positives across all assertions.",
        cli_constant="_SECRET_DETECTION_PRECISION_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_boundary_leakage"],
    ),
    GraderInventoryEntry(
        grader_id="secret_detection_recall",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="secret_detection_assertions",
        observation_field="secret_detection_observations",
        metric_ids=["secret_detection_recall"],
        evidence_requirements=["secret_detection_observation", "secret_detection_assertion"],
        denominator="True positives plus false negatives across all assertions.",
        cli_constant="_SECRET_DETECTION_RECALL_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_boundary_leakage"],
    ),
    GraderInventoryEntry(
        grader_id="final_state_assertions",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="expected_final_state_assertions",
        observation_field="final_state_observations",
        metric_ids=["final_state_accuracy"],
        evidence_requirements=["final_state_observation", "source_receipt"],
        denominator="Total number of declared final-state assertions.",
        cli_constant="_FINAL_STATE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["final_state"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="final_state_assertions consumes final-state observations bound to receipts, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="final_state_assertions is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="independent_state",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="state_fixture",
        observation_field="state_observations",
        metric_ids=["independent_state_accuracy"],
        evidence_requirements=["state_observation", "state_fixture_definition"],
        denominator="Total number of declared state fixture assertions.",
        cli_constant="_INDEPENDENT_STATE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["final_state", "ledger_consistency"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="independent_state consumes state observations, not action-class receipts",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="policy_outcome",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field=None,
        observation_field=None,
        metric_ids=["policy_outcome"],
        evidence_requirements=["primary_receipt", "l4_verification_stage"],
        denominator="Total number of attempts with a declared expected allow/block outcome.",
        cli_constant="_POLICY_OUTCOME_GRADER_ID",
        producer_path=ProducerPath.CROSS_CUTTING,
        producer_suite_ids=["benign_overblock", "governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="policy_outcome consumes receipts and L4 stages, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_SOURCE_BINDING,
                reason="policy_outcome consumes receipts and L4 stages, not source-bound observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="policy_outcome is binary pass/fail; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
                reason="policy_outcome is binary pass/fail with denominator_contribution=1; no variable denominator",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="protocol_chain",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field=None,
        observation_field=None,
        metric_ids=["protocol_chain"],
        evidence_requirements=["primary_receipt", "deterministic_stage_evidence"],
        denominator="Total number of attempts with a declared expected action class and a governed posture.",
        cli_constant="_PROTOCOL_CHAIN_GRADER_ID",
        producer_path=ProducerPath.CROSS_CUTTING,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_BOUNDARY_BINDING,
                reason="protocol_chain consumes receipts and deterministic stages, not collection-boundary observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_SOURCE_BINDING,
                reason="protocol_chain consumes receipts and deterministic stages, not source-bound observations",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="protocol_chain is binary pass/fail; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.DENOMINATOR_BEHAVIOR,
                reason="protocol_chain is binary pass/fail with denominator_contribution=1; no variable denominator",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="unauthorized_mutation",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="unauthorized_mutation_assertions",
        observation_field="unauthorized_mutation_observations",
        metric_ids=["unauthorized_mutation"],
        evidence_requirements=["primary_receipt", "unauthorized_mutation_observation"],
        denominator="Total number of declared unauthorized-mutation assertions.",
        cli_constant="_UNAUTHORIZED_MUTATION_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="unauthorized_mutation is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="token_store_persistence",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="token_store_persistence_assertions",
        observation_field="token_store_persistence_observations",
        metric_ids=["token_store_persistence"],
        evidence_requirements=["token_store_persistence_observation", "token_store_persistence_assertion"],
        denominator="Total number of declared token-store persistence assertions.",
        cli_constant="_TOKEN_STORE_PERSISTENCE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_token_lifecycle"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="token_store_persistence is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="token_store_persistence consumes token-store persistence observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="token_ttl_expiry",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="token_ttl_expiry_assertions",
        observation_field="token_ttl_expiry_observations",
        metric_ids=["token_ttl_expiry"],
        evidence_requirements=["token_ttl_expiry_observation", "token_ttl_expiry_assertion"],
        denominator="Total number of declared token TTL expiry assertions.",
        cli_constant="_TOKEN_TTL_EXPIRY_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_token_lifecycle"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="token_ttl_expiry is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="token_ttl_expiry consumes token TTL expiry observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="token_persistence_failure",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="token_persistence_failure_assertions",
        observation_field="token_persistence_failure_observations",
        metric_ids=["token_persistence_failure"],
        evidence_requirements=["token_persistence_failure_observation", "token_persistence_failure_assertion"],
        denominator="Total number of declared token persistence failure assertions.",
        cli_constant="_TOKEN_PERSISTENCE_FAILURE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_token_lifecycle"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="token_persistence_failure is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="token_persistence_failure consumes token persistence failure observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="exfiltration_attempt",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="exfiltration_attempt_assertions",
        observation_field="exfiltration_attempt_observations",
        metric_ids=["exfiltration_attempt"],
        evidence_requirements=["primary_receipt", "exfiltration_attempt_observation"],
        denominator="Total number of declared exfiltration attempt assertions.",
        cli_constant="_EXFILTRATION_ATTEMPT_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_token_lifecycle"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="exfiltration_attempt is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="artifact_leakage",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="artifact_leakage_assertions",
        observation_field="artifact_leakage_observations",
        metric_ids=["artifact_leakage"],
        evidence_requirements=["artifact_leakage_observation", "artifact_leakage_assertion"],
        denominator="Total number of declared artifact-leakage assertions.",
        cli_constant="_ARTIFACT_LEAKAGE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["privacy_boundary_leakage"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="artifact_leakage is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="artifact_leakage consumes artifact leakage observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="replay_attempt",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="replay_attempt_assertions",
        observation_field="replay_attempt_observations",
        metric_ids=["replay_attempt"],
        evidence_requirements=["primary_receipt", "replay_attempt_observation"],
        denominator="Total number of declared replay-attempt assertions.",
        cli_constant="_REPLAY_ATTEMPT_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="replay_attempt is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="signed_field_tampering",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="signed_field_tampering_assertions",
        observation_field="signed_field_tampering_observations",
        metric_ids=["signed_field_tampering"],
        evidence_requirements=["primary_receipt", "signed_field_tampering_observation"],
        denominator="Total number of declared signed-field tampering assertions.",
        cli_constant="_SIGNED_FIELD_TAMPERING_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="signed_field_tampering is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="payload_tampering",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="payload_tampering_assertions",
        observation_field="payload_tampering_observations",
        metric_ids=["payload_tampering"],
        evidence_requirements=["primary_receipt", "payload_tampering_observation"],
        denominator="Total number of declared payload-tampering assertions.",
        cli_constant="_PAYLOAD_TAMPERING_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="payload_tampering is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="stale_state_root",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="stale_state_root_assertions",
        observation_field="stale_state_root_observations",
        metric_ids=["stale_state_root"],
        evidence_requirements=["primary_receipt", "stale_state_root_observation"],
        denominator="Total number of declared stale-state-root assertions.",
        cli_constant="_STALE_STATE_ROOT_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="stale_state_root is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="identity_mismatch",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="identity_mismatch_assertions",
        observation_field="identity_mismatch_observations",
        metric_ids=["identity_mismatch"],
        evidence_requirements=["primary_receipt", "identity_mismatch_observation"],
        denominator="Total number of declared identity-mismatch assertions.",
        cli_constant="_IDENTITY_MISMATCH_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="identity_mismatch is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="nonce_expiration",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="nonce_expiration_assertions",
        observation_field="nonce_expiration_observations",
        metric_ids=["nonce_expiration"],
        evidence_requirements=["primary_receipt", "nonce_expiration_observation"],
        denominator="Total number of declared nonce-expiration assertions.",
        cli_constant="_NONCE_EXPIRATION_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="nonce_expiration is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="signer_defect",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="signer_defect_assertions",
        observation_field="signer_defect_observations",
        metric_ids=["signer_defect"],
        evidence_requirements=["primary_receipt", "signer_defect_observation"],
        denominator="Total number of declared signer-defect assertions.",
        cli_constant="_SIGNER_DEFECT_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="signer_defect is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="l3_proof_transplant",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="l3_proof_transplant_assertions",
        observation_field="l3_proof_transplant_observations",
        metric_ids=["l3_proof_transplant"],
        evidence_requirements=["primary_receipt", "l3_proof_transplant_observation"],
        denominator="Total number of declared L3-proof-transplant assertions.",
        cli_constant="_L3_PROOF_TRANSPLANT_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="l3_proof_transplant is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="revoked_credential",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="revoked_credential_assertions",
        observation_field="revoked_credential_observations",
        metric_ids=["revoked_credential"],
        evidence_requirements=["primary_receipt", "revoked_credential_observation"],
        denominator="Total number of declared revoked-credential assertions.",
        cli_constant="_REVOKED_CREDENTIAL_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="revoked_credential is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="evidence_preservation",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="evidence_preservation_assertions",
        observation_field="evidence_preservation_observations",
        metric_ids=["evidence_preservation"],
        evidence_requirements=["evidence_preservation_observation"],
        denominator="Total number of declared evidence-preservation assertions.",
        cli_constant="_EVIDENCE_PRESERVATION_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["governance_adversarial"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="evidence_preservation is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="evidence_preservation consumes evidence-preservation observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="policy_attack",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="policy_attack_assertions",
        observation_field="policy_attack_observations",
        metric_ids=["policy_attack"],
        evidence_requirements=["primary_receipt", "policy_attack_observation"],
        denominator="Total number of declared policy-violating attack assertions.",
        cli_constant="_POLICY_ATTACK_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["policy_attack"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="policy_attack is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="tool_sequence",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="tool_sequence_assertions",
        observation_field="tool_sequence_observations",
        metric_ids=["tool_sequence"],
        evidence_requirements=["tool_sequence_observation"],
        denominator="Total number of declared tool-sequence assertions.",
        cli_constant="_TOOL_SEQUENCE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["tool_sequence"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="tool_sequence is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="tool_sequence consumes tool-sequence observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="factual_qa",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="factual_qa_assertions",
        observation_field="factual_qa_observations",
        metric_ids=["factual_qa"],
        evidence_requirements=["factual_qa_observation"],
        denominator="Total number of declared factual-QA assertions.",
        cli_constant="_FACTUAL_QA_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["factual_qa"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="factual_qa is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="factual_qa consumes factual-QA observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="citation_backed",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="citation_backed_assertions",
        observation_field="citation_backed_observations",
        metric_ids=["citation_backed"],
        evidence_requirements=["citation_backed_observation"],
        denominator="Total number of declared citation-backed assertions.",
        cli_constant="_CITATION_BACKED_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["citation_backed"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="citation_backed is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="citation_backed consumes citation-backed observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="partial_milestone",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="partial_milestone_assertions",
        observation_field="partial_milestone_observations",
        metric_ids=["partial_milestone"],
        evidence_requirements=["partial_milestone_observation"],
        denominator="Total number of declared partial-milestone assertions.",
        cli_constant="_PARTIAL_MILESTONE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["partial_milestone"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="partial_milestone is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
            ConformanceExclusion(
                category=ConformanceCaseCategory.WRONG_ACTION_BINDING,
                reason="partial_milestone consumes partial-milestone observations, not action-class receipts; it does not check expected_action_class or action_type",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="reliability",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="reliability_assertions",
        observation_field="reliability_observations",
        metric_ids=["reliability"],
        evidence_requirements=["reliability_observation"],
        denominator="Total number of declared reliability assertions.",
        cli_constant="_RELIABILITY_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["reliability"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="reliability is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
    GraderInventoryEntry(
        grader_id="economics_performance",
        grader_version=_GRADER_VERSION,
        grader_class=_DET,
        assertion_field="economics_performance_assertions",
        observation_field="economics_performance_observations",
        metric_ids=["economics_performance"],
        evidence_requirements=["economics_performance_observation"],
        denominator="Total number of declared economics-performance assertions.",
        cli_constant="_ECONOMICS_PERFORMANCE_GRADER_ID",
        producer_path=ProducerPath.SYNTHETIC,
        producer_suite_ids=["economics_performance"],
        conformance_exclusions=[
            ConformanceExclusion(
                category=ConformanceCaseCategory.NOT_APPLICABLE,
                reason="economics_performance is a proportion over declared assertions; not-applicable is not a valid outcome",
            ),
        ],
    ),
]


GRADER_INVENTORY: dict[tuple[str, str], GraderInventoryEntry] = {
    entry.key: entry for entry in _INVENTORY
}


class InventoryDriftError(RuntimeError):
    """Raised when the inventory, grader registry, or metric registry drift apart."""


def validate_inventory_against_registries() -> None:
    """Verify the inventory matches the live grader and metric registries.

    Checks that every grader in ``_GRADERS`` has an inventory entry,
    that every inventory entry has a registered grader, that every
    inventory metric ID is registered in ``DEFAULT_METRIC_REGISTRY``,
    and that every metric with a ``grader_ref`` has a matching inventory
    entry. Raises ``InventoryDriftError`` on any mismatch.
    """
    registry_keys = set(_GRADERS.keys())
    inventory_keys = set(GRADER_INVENTORY.keys())

    missing_from_inventory = registry_keys - inventory_keys
    if missing_from_inventory:
        raise InventoryDriftError(
            f"graders registered but missing from inventory: {sorted(missing_from_inventory)}"
        )

    partial_external_keys = {
        key for key, entry in GRADER_INVENTORY.items()
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL
    }
    missing_from_registry = inventory_keys - registry_keys - partial_external_keys
    if missing_from_registry:
        raise InventoryDriftError(
            f"graders in inventory but not registered (and not partial_external): "
            f"{sorted(missing_from_registry)}"
        )

    for entry in GRADER_INVENTORY.values():
        for metric_id in entry.metric_ids:
            if not DEFAULT_METRIC_REGISTRY.is_registered(metric_id, entry.grader_version):
                raise InventoryDriftError(
                    f"grader {entry.grader_id}: metric {metric_id}@{entry.grader_version} "
                    f"is not registered in DEFAULT_METRIC_REGISTRY"
                )

    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        ref = definition.grader_ref
        if ref is None:
            continue
        if ref.grader_class != GraderClass.DETERMINISTIC:
            continue
        if (ref.grader_id, ref.grader_version) not in GRADER_INVENTORY:
            raise InventoryDriftError(
                f"metric {definition.metric_id}@{definition.metric_version} references "
                f"grader {ref.grader_id}@{ref.grader_version} but no inventory entry exists"
            )


validate_inventory_against_registries()


__all__ = [
    "ConformanceCaseCategory",
    "ConformanceExclusion",
    "GRADER_INVENTORY",
    "GraderInventoryEntry",
    "InventoryDriftError",
    "ProducerPath",
    "validate_inventory_against_registries",
]
