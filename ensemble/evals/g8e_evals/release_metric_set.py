# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed v2.1.8 release metric set and claim boundary.

Defines the exact set of metrics that feed the v2.1.8 release analysis
and the claims that can and cannot be made from them. The release metric
set is the subset of the full metric registry that has an authoritative
evidence producer (a registered deterministic grader or the partial
IFEval verifier). Metrics without an authoritative producer are not in
the release set.

The claim boundary explicitly marks unsupported claims as typed
exclusions. Unsupported claims remain unsupported unless separately
implemented and measured in a future milestone. No unsupported claim
becomes an implied pass.

This module is the single source of truth for the v2.1.8 release metric
set and claim boundary. It is consumed by release-gate checks and
canonical analysis to verify that every published metric is in the
release set and every published claim is within the claim boundary.
"""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY


class MetricDomain(StrEnum):
    """Domain classification for release metrics."""

    UTILITY = "utility"
    PRIVACY = "privacy"
    TOKEN_LIFECYCLE = "token_lifecycle"
    GOVERNANCE = "governance"
    GOVERNANCE_ADVERSARIAL = "governance_adversarial"
    STATE = "state"
    RELIABILITY = "reliability"
    ECONOMICS = "economics"
    TELEMETRY = "telemetry"


class ReleaseMetricEntry(BaseModel):
    """One metric in the v2.1.8 release set.

    Binds a registered metric definition to its release domain and
    whether it has a practical threshold (non-inferiority margin or
    release-blocker threshold) defined.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)
    domain: MetricDomain
    has_practical_threshold: bool
    threshold_description: str = Field(min_length=1)


class UnsupportedClaim(BaseModel):
    """An explicit typed exclusion for a claim that is not supported in v2.1.8.

    Records the unsupported claim and the reason it remains unsupported.
    Unsupported claims never become implied passes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    claim: str = Field(min_length=1)
    reason: str = Field(min_length=1)
    follow_on_milestone: str = Field(min_length=1)


class ReleaseMetricSet(BaseModel):
    """The complete v2.1.8 release metric set and claim boundary.

    The release metric set is the exact set of metrics that feed the
    v2.1.8 release analysis. The claim boundary defines what claims can
    and cannot be made from these metrics. Unsupported claims are
    explicit typed exclusions.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    release_version: str = Field(min_length=1)
    metrics: list[ReleaseMetricEntry] = Field(min_length=1)
    unsupported_claims: list[UnsupportedClaim] = Field(min_length=1)

    @property
    def metric_ids(self) -> set[tuple[str, str]]:
        return {(m.metric_id, m.metric_version) for m in self.metrics}

    @property
    def unsupported_claim_names(self) -> set[str]:
        return {uc.claim for uc in self.unsupported_claims}


def _build_release_metrics() -> list[ReleaseMetricEntry]:
    """Build the release metric entries from the live metric registry.

    Every metric in the release set must be registered in
    ``DEFAULT_METRIC_REGISTRY`` with an authoritative evidence producer
    (a grader reference or the IFEval verifier). The domain mapping is
    declared here and verified against the registry at construction time.
    """
    domain_map: dict[str, MetricDomain] = {
        "ifeval_subset_verifier": MetricDomain.UTILITY,
        "eval_judge": MetricDomain.UTILITY,
        "factual_qa": MetricDomain.UTILITY,
        "citation_backed": MetricDomain.UTILITY,
        "partial_milestone": MetricDomain.UTILITY,
        "tool_sequence": MetricDomain.UTILITY,
        "canary_scrubbing": MetricDomain.PRIVACY,
        "model_boundary_raw_secret_rate": MetricDomain.PRIVACY,
        "exact_local_rehydration": MetricDomain.PRIVACY,
        "secret_detection_precision": MetricDomain.PRIVACY,
        "secret_detection_recall": MetricDomain.PRIVACY,
        "artifact_leakage": MetricDomain.PRIVACY,
        "token_store_persistence": MetricDomain.TOKEN_LIFECYCLE,
        "token_ttl_expiry": MetricDomain.TOKEN_LIFECYCLE,
        "token_persistence_failure": MetricDomain.TOKEN_LIFECYCLE,
        "exfiltration_attempt": MetricDomain.TOKEN_LIFECYCLE,
        "receipt_integrity": MetricDomain.GOVERNANCE,
        "protocol_chain": MetricDomain.GOVERNANCE,
        "policy_outcome": MetricDomain.GOVERNANCE,
        "unauthorized_mutation": MetricDomain.GOVERNANCE,
        "replay_attempt": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "signed_field_tampering": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "payload_tampering": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "stale_state_root": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "identity_mismatch": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "nonce_expiration": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "signer_defect": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "l3_proof_transplant": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "revoked_credential": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "evidence_preservation": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "policy_attack": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "final_state_accuracy": MetricDomain.STATE,
        "independent_state_accuracy": MetricDomain.STATE,
        "reliability": MetricDomain.RELIABILITY,
        "economics_performance": MetricDomain.ECONOMICS,
        "stage_usage_reconciled": MetricDomain.TELEMETRY,
        # Derived analysis metrics
        "allow_block_confusion_matrix": MetricDomain.GOVERNANCE,
        "attack_success_rate": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "expected_layer_detection": MetricDomain.GOVERNANCE,
        "balanced_accuracy": MetricDomain.GOVERNANCE,
        "matthews_correlation_coefficient": MetricDomain.GOVERNANCE,
        "harm_weighted_loss": MetricDomain.GOVERNANCE_ADVERSARIAL,
        "l2_proof_property": MetricDomain.GOVERNANCE,
        "l3_proof_property": MetricDomain.GOVERNANCE,
        "l4_proof_property": MetricDomain.GOVERNANCE,
        "l5_proof_property": MetricDomain.GOVERNANCE,
        "receipt_linkage": MetricDomain.GOVERNANCE,
        "envelope_linkage": MetricDomain.GOVERNANCE,
        "state_linkage": MetricDomain.STATE,
        "persistence_linkage": MetricDomain.GOVERNANCE,
        "commitment_linkage": MetricDomain.GOVERNANCE,
        "audit_linkage": MetricDomain.GOVERNANCE,
        "evidence_validity": MetricDomain.RELIABILITY,
        # New primary telemetry metrics
        "stage_latency_seconds": MetricDomain.TELEMETRY,
        "provider_usage_tokens": MetricDomain.TELEMETRY,
        "provider_cost_usd": MetricDomain.ECONOMICS,
        "local_resource_peak_memory_bytes": MetricDomain.TELEMETRY,
        "local_resource_cpu_seconds": MetricDomain.TELEMETRY,
        "human_wait_seconds": MetricDomain.TELEMETRY,
    }

    entries: list[ReleaseMetricEntry] = []
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        domain = domain_map.get(definition.metric_id)
        if domain is None:
            raise ValueError(
                f"metric {definition.metric_id}@{definition.metric_version} "
                f"is registered but has no release domain mapping"
            )
        entries.append(ReleaseMetricEntry(
            metric_id=definition.metric_id,
            metric_version=definition.metric_version,
            domain=domain,
            has_practical_threshold=definition.practical_threshold is not None,
            threshold_description=definition.release_threshold or "No practical threshold defined; calibration pending.",
        ))
    return entries


_UNSUPPORTED_CLAIMS: list[UnsupportedClaim] = [
    UnsupportedClaim(
        claim="reasoner_independence",
        reason="Reasoner-independence requires distinct reasoners and independent evidence; no distinct reasoners are measured in v2.1.8",
        follow_on_milestone="Requires distinct reasoner implementations and independent evidence production",
    ),
    UnsupportedClaim(
        claim="heterogeneous_l2_reasoning",
        reason="Heterogeneous L2 reasoning requires multiple independent L2 consensus implementations; only one L2 implementation is measured",
        follow_on_milestone="Requires multiple independent L2 consensus implementations",
    ),
    UnsupportedClaim(
        claim="independent_quorum_error_reduction",
        reason="Independent quorum-error reduction requires multiple independent quorum members; only one quorum configuration is measured",
        follow_on_milestone="Requires multiple independent quorum member implementations",
    ),
    UnsupportedClaim(
        claim="certification",
        reason="Certification requires external assessor attestation; no assessor attestation is produced in v2.1.8",
        follow_on_milestone="Follow-on milestone C: signed, scoped, time-bounded, revocable assessor-attestation import",
    ),
    UnsupportedClaim(
        claim="legal_compliance",
        reason="Legal compliance requires legal review; no legal review is conducted or represented in v2.1.8",
        follow_on_milestone="Requires external legal review and is never an engineering-only claim",
    ),
    UnsupportedClaim(
        claim="recurring_operating_effectiveness",
        reason="Recurring operating effectiveness requires scheduled CI workflows and historical evidence windows; only point-in-time evidence is produced in v2.1.8",
        follow_on_milestone="Follow-on milestone B: recurring compliance operating effectiveness",
    ),
    UnsupportedClaim(
        claim="human_semantic_grading",
        reason="Human semantic grading requires blinded human raters and inter-rater agreement; no human grading is conducted in v2.1.8",
        follow_on_milestone="Follow-on milestone A: semantic evaluation and frozen holdouts",
    ),
    UnsupportedClaim(
        claim="complete_ifeval_import",
        reason="The ifeval_subset suite is an explicitly partial import of 5 tasks; the complete upstream IFEval evaluator and dataset is a separately scoped follow-on",
        follow_on_milestone="Follow-on milestone A, item 5: complete canonical upstream IFEval evaluator and dataset",
    ),
]


RELEASE_METRIC_SET = ReleaseMetricSet(
    release_version="v2.1.8",
    metrics=_build_release_metrics(),
    unsupported_claims=_UNSUPPORTED_CLAIMS,
)


__all__ = [
    "RELEASE_METRIC_SET",
    "MetricDomain",
    "ReleaseMetricEntry",
    "ReleaseMetricSet",
    "UnsupportedClaim",
]
