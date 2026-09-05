# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed models for evaluation harness components.

These Pydantic models replace Dict[str, Any] usage throughout the evals codebase,
providing schema validation and making the harness robust against schema changes
in the Engine and protocol definitions.
"""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from g8e_evals.schema import (
    CanaryScrubbingAssertion,
    ArtifactLeakageAssertion,
    EvidencePreservationAssertion,
    FORBIDDEN_METADATA_KEYS,
    IdentityMismatchAssertion,
    ExfiltrationAttemptAssertion,
    FinalStateAssertion,
    NonceExpirationAssertion,
    PayloadTamperingAssertion,
    PolicyAttackAssertion,
    SignerDefectAssertion,
    L3ProofTransplantAssertion,
    RevokedCredentialAssertion,
    PolicyOutcome,
    RehydrationAssertion,
    RejectionLayer,
    ReplayAttemptAssertion,
    SecretDetectionAssertion,
    SignedFieldTamperingAssertion,
    StaleStateRootAssertion,
    StateFixtureDefinition,
    TokenStorePersistenceAssertion,
    TokenTTLExpiryAssertion,
    TokenPersistenceFailureAssertion,
    UnauthorizedMutationAssertion,
    UnsupportedExclusion,
)


class InstructionResult(BaseModel):
    instruction: str
    passed: bool
    kwargs: dict[str, Any] = Field(default_factory=dict)


class ScoreDetails(BaseModel):
    """Typed details for Score evaluation results."""
    model_config = ConfigDict(extra="ignore")

    # Common evaluation metrics
    error: str = ""
    error_message: str = ""
    error_type: str = ""
    validation_errors: list[str] = Field(default_factory=list)
    instructions: list[InstructionResult] = Field(default_factory=list)

    # Benchmark-specific details can be added as extra fields
    benchmark_specific: dict[str, Any] = Field(default_factory=dict)

    @model_validator(mode="after")
    def _validate_benchmark_specific_keys(self) -> ScoreDetails:
        forbidden = self.benchmark_specific.keys() & FORBIDDEN_METADATA_KEYS
        if forbidden:
            raise ValueError(
                "benchmark_specific must not carry security- or privacy-critical known shapes: "
                f"{sorted(forbidden)[0]}"
            )
        return self


class TaskMetadata(BaseModel):
    """Typed metadata for Task objects."""
    model_config = ConfigDict(extra="ignore")

    benchmark: str = ""
    category: str = ""
    difficulty: str = ""
    tags: list[str] = Field(default_factory=list)
    expected_action_class: str = ""
    state_fixture: StateFixtureDefinition | None = None
    expected_final_state_assertions: list[FinalStateAssertion] = Field(default_factory=list)
    expected_allow_block_outcome: PolicyOutcome | None = None
    expected_rejection_layer: RejectionLayer | None = None
    sensitive_canary_annotations: list[CanaryScrubbingAssertion] = Field(default_factory=list)
    rehydration_assertions: list[RehydrationAssertion] = Field(default_factory=list)
    secret_detection_assertions: list[SecretDetectionAssertion] = Field(default_factory=list)
    unauthorized_mutation_assertions: list[UnauthorizedMutationAssertion] = Field(default_factory=list)
    token_store_persistence_assertions: list[TokenStorePersistenceAssertion] = Field(default_factory=list)
    token_ttl_expiry_assertions: list[TokenTTLExpiryAssertion] = Field(default_factory=list)
    token_persistence_failure_assertions: list[TokenPersistenceFailureAssertion] = Field(default_factory=list)
    exfiltration_attempt_assertions: list[ExfiltrationAttemptAssertion] = Field(default_factory=list)
    artifact_leakage_assertions: list[ArtifactLeakageAssertion] = Field(default_factory=list)
    replay_attempt_assertions: list[ReplayAttemptAssertion] = Field(default_factory=list)
    signed_field_tampering_assertions: list[SignedFieldTamperingAssertion] = Field(default_factory=list)
    payload_tampering_assertions: list[PayloadTamperingAssertion] = Field(default_factory=list)
    stale_state_root_assertions: list[StaleStateRootAssertion] = Field(default_factory=list)
    identity_mismatch_assertions: list[IdentityMismatchAssertion] = Field(default_factory=list)
    nonce_expiration_assertions: list[NonceExpirationAssertion] = Field(default_factory=list)
    signer_defect_assertions: list[SignerDefectAssertion] = Field(default_factory=list)
    l3_proof_transplant_assertions: list[L3ProofTransplantAssertion] = Field(default_factory=list)
    revoked_credential_assertions: list[RevokedCredentialAssertion] = Field(default_factory=list)
    evidence_preservation_assertions: list[EvidencePreservationAssertion] = Field(default_factory=list)
    policy_attack_assertions: list[PolicyAttackAssertion] = Field(default_factory=list)
    unsupported_exclusions: list[UnsupportedExclusion] = Field(default_factory=list)
    # IFEval-specific fields
    instruction_id_list: list[str] = Field(default_factory=list)
    kwargs: list[dict[str, Any]] = Field(default_factory=list)
    # Other benchmark-specific data
    benchmark_specific: dict[str, Any] = Field(default_factory=dict)

    @field_validator("kwargs", mode="after")
    @classmethod
    def _validate_kwargs_no_forbidden_keys(cls, value: list[dict[str, Any]]) -> list[dict[str, Any]]:
        for index, item in enumerate(value):
            forbidden = item.keys() & FORBIDDEN_METADATA_KEYS
            if forbidden:
                raise ValueError(
                    f"kwargs[{index}] must not carry security- or privacy-critical known shapes: "
                    f"{sorted(forbidden)[0]}"
                )
        return value

    @model_validator(mode="after")
    def _validate_benchmark_specific_keys(self) -> TaskMetadata:
        forbidden = self.benchmark_specific.keys() & FORBIDDEN_METADATA_KEYS
        if forbidden:
            raise ValueError(
                "benchmark_specific must not carry security- or privacy-critical known shapes: "
                f"{sorted(forbidden)[0]}"
            )
        return self


class AggregateMetadata(BaseModel):
    """Typed metadata for Aggregate results."""
    model_config = ConfigDict(extra="ignore")

    suite_version: str = ""
    operator_version: str = ""
    test_timestamp: str = ""
    environment_info: dict[str, Any] = Field(default_factory=dict)
