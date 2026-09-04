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

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.schema import (
    CanaryScrubbingAssertion,
    ArtifactLeakageAssertion,
    ExfiltrationAttemptAssertion,
    FinalStateAssertion,
    PolicyOutcome,
    RehydrationAssertion,
    RejectionLayer,
    ReplayAttemptAssertion,
    SecretDetectionAssertion,
    StateFixtureDefinition,
    TokenStorePersistenceAssertion,
    TokenTTLExpiryAssertion,
    TokenPersistenceFailureAssertion,
    UnauthorizedMutationAssertion,
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
    # IFEval-specific fields
    instruction_id_list: list[str] = Field(default_factory=list)
    kwargs: list[dict[str, Any]] = Field(default_factory=list)
    # Other benchmark-specific data
    benchmark_specific: dict[str, Any] = Field(default_factory=dict)


class AggregateMetadata(BaseModel):
    """Typed metadata for Aggregate results."""
    model_config = ConfigDict(extra="ignore")

    suite_version: str = ""
    operator_version: str = ""
    test_timestamp: str = ""
    environment_info: dict[str, Any] = Field(default_factory=dict)
