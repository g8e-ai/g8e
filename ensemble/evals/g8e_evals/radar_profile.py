# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Radar profile and score family summary models for publication schema v5.

The v5 publication schema extends v4 with a radar profile that lets the
evaluation dimensions speak for themselves rather than collapsing to a
single composite score. Each radar dimension is a named score family
(task accuracy, tool reliability, instruction fidelity, security,
privacy, escalation quality, recovery, repeatability, token efficiency,
latency efficiency) with a value in [0.0, 1.0] and the source metric IDs
that fed it so anyone can inspect why a stack received that score.

The score family summary models (tool scorecard, escalation, security
events, correlated errors, cold-start vs warm inference tradeoff) are
typed projections derived from the existing metric observations and
record types. They carry only aggregate proportions and counts; no raw
prompts, outputs, or private configuration cross the projection
boundary.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
import math
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


class RadarDimensionName(StrEnum):
    """The 10 radar profile dimensions.

    Each dimension is a named score family. The radar profile publishes
    all 10 so the dimensions speak for themselves; no single composite
    score is published initially.

    ``TASK_ACCURACY``: Task accuracy, instruction following, reasoning, tool selection.
    ``TOOL_RELIABILITY``: Tool calling scorecard aggregate across all 10 dimensions.
    ``INSTRUCTION_FIDELITY``: Instruction adherence (IFEval and REAL_SYSTEM instruction suites).
    ``SECURITY``: Policy enforcement, authorization, audit completeness.
    ``PRIVACY``: Sensitive data exposure, redaction, unnecessary disclosure.
    ``ESCALATION_QUALITY``: Correct autonomous completion, correct escalation, false/missed escalation.
    ``RECOVERY``: Tool failure, malformed response, unavailable resource recovery.
    ``REPEATABILITY``: Consistency across repetitions (binomial pass/fail variance).
    ``TOKEN_EFFICIENCY``: Tokens per task, output token economy.
    ``LATENCY_EFFICIENCY``: End-to-end latency, time to first token, generation duration.
    """

    TASK_ACCURACY = "task_accuracy"
    TOOL_RELIABILITY = "tool_reliability"
    INSTRUCTION_FIDELITY = "instruction_fidelity"
    SECURITY = "security"
    PRIVACY = "privacy"
    ESCALATION_QUALITY = "escalation_quality"
    RECOVERY = "recovery"
    REPEATABILITY = "repeatability"
    TOKEN_EFFICIENCY = "token_efficiency"
    LATENCY_EFFICIENCY = "latency_efficiency"


_RADAR_DIMENSION_NAMES = frozenset(d.value for d in RadarDimensionName)


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class RadarDimension(BaseModel):
    """One dimension of the radar profile.

    Binds a named score family to its aggregate value and the source
    metric IDs that fed it. The value is a proportion in [0.0, 1.0].
    The source metric IDs are sorted and unique so the dimension is
    reproducible from the underlying metric observations.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    name: RadarDimensionName = Field(description="Radar dimension name.")
    value: float = Field(ge=0.0, le=1.0, description="Aggregate proportion for this dimension in [0.0, 1.0].")
    source_metric_ids: list[str] = Field(
        min_length=1,
        description="Sorted unique metric IDs that fed this dimension.",
    )

    @model_validator(mode="after")
    def _validate_dimension(self) -> Self:
        if not math.isfinite(self.value):
            raise ValueError(f"non-finite radar dimension value: {self.value!r}")
        if len(self.source_metric_ids) != len(set(self.source_metric_ids)):
            raise ValueError(f"duplicate source metric IDs in radar dimension {self.name!r}")
        if list(self.source_metric_ids) != sorted(self.source_metric_ids):
            raise ValueError(f"source metric IDs must be sorted in radar dimension {self.name!r}")
        return self


class RadarProfile(BaseModel):
    """Radar profile with one dimension per score family.

    Carries exactly one ``RadarDimension`` per ``RadarDimensionName``.
    The profile is the primary public surface for the v5 publication:
    it lets the dimensions speak for themselves rather than collapsing
    to a single composite score.

    The ``content_hash`` is SHA-256 over canonical JSON of the profile
    (sorted dimensions by name). Changing any field changes the hash
    and invalidates downstream publication validation.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    dimensions: list[RadarDimension] = Field(
        min_length=1,
        description="Radar dimensions, one per RadarDimensionName, sorted by name.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the profile.",
    )

    @model_validator(mode="after")
    def _validate_profile(self) -> Self:
        names = [d.name for d in self.dimensions]
        if len(names) != len(set(names)):
            raise ValueError(f"duplicate radar dimension names: {names!r}")
        expected_names = sorted(RadarDimensionName, key=lambda n: n.value)
        if sorted(names, key=lambda n: n.value) != expected_names:
            missing = set(expected_names) - set(names)
            extra = set(names) - set(expected_names)
            raise ValueError(
                f"radar profile must have exactly one dimension per RadarDimensionName: "
                f"missing={sorted(missing, key=lambda n: n.value)!r}, extra={sorted(extra, key=lambda n: n.value)!r}"
            )
        if [d.name for d in self.dimensions] != sorted(names, key=lambda n: n.value):
            raise ValueError("radar dimensions must be sorted by name")
        expected_hash = compute_radar_profile_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            dimensions=self.dimensions,
        )
        if self.content_hash != expected_hash:
            raise ValueError(
                f"radar profile content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected_hash!r}"
            )
        return self


def compute_radar_profile_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    dimensions: list[RadarDimension],
) -> str:
    """Compute the content hash for a radar profile without constructing the full model."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_revision": campaign_revision,
            "dimensions": [
                json.loads(d.model_dump_json())
                for d in sorted(dimensions, key=lambda d: d.name.value)
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class ToolScorecardDimensionSummary(BaseModel):
    """Aggregate summary for one tool-call scorecard dimension.

    Binds the dimension name to its pass rate (proportion of tool calls
    where the dimension passed) and the total tool call count that fed
    the proportion.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    dimension: str = Field(min_length=1, description="Scorecard dimension name.")
    pass_rate: float = Field(ge=0.0, le=1.0, description="Proportion of tool calls where the dimension passed.")
    tool_call_count: int = Field(ge=0, description="Total tool calls that fed this dimension.")

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        if not math.isfinite(self.pass_rate):
            raise ValueError(f"non-finite pass_rate: {self.pass_rate!r}")
        return self


class ToolScorecardSummary(BaseModel):
    """Summary of the tool calling scorecard across all 10 dimensions.

    Aggregates per-tool-call scorecard records into one summary per
    dimension. Each dimension's pass rate is the proportion of tool
    calls where that dimension passed. The summary is bound to the
    campaign identity and the total tool call count.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    total_tool_calls: int = Field(ge=0, description="Total tool calls across all attempts.")
    dimensions: list[ToolScorecardDimensionSummary] = Field(
        min_length=1,
        description="Per-dimension summaries, sorted by dimension name.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the summary.",
    )

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        dim_names = [d.dimension for d in self.dimensions]
        if len(dim_names) != len(set(dim_names)):
            raise ValueError(f"duplicate tool scorecard dimension names: {dim_names!r}")
        if list(dim_names) != sorted(dim_names):
            raise ValueError("tool scorecard dimensions must be sorted by dimension name")
        expected_hash = compute_tool_scorecard_summary_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            total_tool_calls=self.total_tool_calls,
            dimensions=self.dimensions,
        )
        if self.content_hash != expected_hash:
            raise ValueError(
                f"tool scorecard summary content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected_hash!r}"
            )
        return self


def compute_tool_scorecard_summary_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    total_tool_calls: int,
    dimensions: list[ToolScorecardDimensionSummary],
) -> str:
    """Compute the content hash for a tool scorecard summary without constructing the full model."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_revision": campaign_revision,
            "total_tool_calls": total_tool_calls,
            "dimensions": [
                json.loads(d.model_dump_json())
                for d in sorted(dimensions, key=lambda d: d.dimension)
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class EscalationSummary(BaseModel):
    """Summary of escalation outcomes across all scenario-model pairs.

    Aggregates per-scenario escalation records into outcome counts and
    rates. The ``escalation_efficiency`` rate is the proportion of
    scenarios where the routing decision was correct (autonomous
    completion or correct escalation) out of all scenarios.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    total_records: int = Field(ge=0, description="Total escalation records.")
    correct_autonomous_count: int = Field(ge=0, description="Count of correct autonomous completions.")
    correct_escalation_count: int = Field(ge=0, description="Count of correct escalations.")
    false_escalation_count: int = Field(ge=0, description="Count of false escalations.")
    missed_escalation_count: int = Field(ge=0, description="Count of missed escalations.")
    escalation_efficiency: float = Field(
        ge=0.0, le=1.0,
        description="Proportion of correct routing decisions (autonomous + correct escalation) out of total records.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the summary.",
    )

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        if not math.isfinite(self.escalation_efficiency):
            raise ValueError(f"non-finite escalation_efficiency: {self.escalation_efficiency!r}")
        expected_hash = compute_escalation_summary_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            total_records=self.total_records,
            correct_autonomous_count=self.correct_autonomous_count,
            correct_escalation_count=self.correct_escalation_count,
            false_escalation_count=self.false_escalation_count,
            missed_escalation_count=self.missed_escalation_count,
            escalation_efficiency=self.escalation_efficiency,
        )
        if self.content_hash != expected_hash:
            raise ValueError(
                f"escalation summary content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected_hash!r}"
            )
        return self


def compute_escalation_summary_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    total_records: int,
    correct_autonomous_count: int,
    correct_escalation_count: int,
    false_escalation_count: int,
    missed_escalation_count: int,
    escalation_efficiency: float,
) -> str:
    """Compute the content hash for an escalation summary without constructing the full model."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_revision": campaign_revision,
            "total_records": total_records,
            "correct_autonomous_count": correct_autonomous_count,
            "correct_escalation_count": correct_escalation_count,
            "false_escalation_count": false_escalation_count,
            "missed_escalation_count": missed_escalation_count,
            "escalation_efficiency": escalation_efficiency,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class SecurityEventSummary(BaseModel):
    """Summary of security and privacy events across all scenario-model pairs.

    Aggregates per-scenario security event records into event counts and
    rates. Each event rate is the proportion of records where the event
    was True. The summary decomposes the eventual Privacy/Security score
    into disclosed components so anyone can inspect why a stack received
    that score.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    total_records: int = Field(ge=0, description="Total security event records.")
    sensitive_data_present_rate: float = Field(ge=0.0, le=1.0)
    sensitive_data_required_rate: float = Field(ge=0.0, le=1.0)
    sensitive_data_sent_externally_rate: float = Field(ge=0.0, le=1.0)
    unnecessary_data_sent_externally_rate: float = Field(ge=0.0, le=1.0)
    policy_prevented_disclosure_rate: float = Field(ge=0.0, le=1.0)
    model_attempted_unauthorized_access_rate: float = Field(ge=0.0, le=1.0)
    tool_attempted_unauthorized_operation_rate: float = Field(ge=0.0, le=1.0)
    authorization_correctly_enforced_rate: float = Field(ge=0.0, le=1.0)
    audit_record_complete_rate: float = Field(ge=0.0, le=1.0)
    audit_record_tampered_rate: float = Field(ge=0.0, le=1.0)
    secret_redaction_successful_rate: float = Field(ge=0.0, le=1.0)
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the summary.",
    )

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        for rate in (
            self.sensitive_data_present_rate,
            self.sensitive_data_required_rate,
            self.sensitive_data_sent_externally_rate,
            self.unnecessary_data_sent_externally_rate,
            self.policy_prevented_disclosure_rate,
            self.model_attempted_unauthorized_access_rate,
            self.tool_attempted_unauthorized_operation_rate,
            self.authorization_correctly_enforced_rate,
            self.audit_record_complete_rate,
            self.audit_record_tampered_rate,
            self.secret_redaction_successful_rate,
        ):
            if not math.isfinite(rate):
                raise ValueError(f"non-finite security event rate: {rate!r}")
        expected_hash = compute_security_event_summary_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            total_records=self.total_records,
            sensitive_data_present_rate=self.sensitive_data_present_rate,
            sensitive_data_required_rate=self.sensitive_data_required_rate,
            sensitive_data_sent_externally_rate=self.sensitive_data_sent_externally_rate,
            unnecessary_data_sent_externally_rate=self.unnecessary_data_sent_externally_rate,
            policy_prevented_disclosure_rate=self.policy_prevented_disclosure_rate,
            model_attempted_unauthorized_access_rate=self.model_attempted_unauthorized_access_rate,
            tool_attempted_unauthorized_operation_rate=self.tool_attempted_unauthorized_operation_rate,
            authorization_correctly_enforced_rate=self.authorization_correctly_enforced_rate,
            audit_record_complete_rate=self.audit_record_complete_rate,
            audit_record_tampered_rate=self.audit_record_tampered_rate,
            secret_redaction_successful_rate=self.secret_redaction_successful_rate,
        )
        if self.content_hash != expected_hash:
            raise ValueError(
                f"security event summary content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected_hash!r}"
            )
        return self


def compute_security_event_summary_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    total_records: int,
    sensitive_data_present_rate: float,
    sensitive_data_required_rate: float,
    sensitive_data_sent_externally_rate: float,
    unnecessary_data_sent_externally_rate: float,
    policy_prevented_disclosure_rate: float,
    model_attempted_unauthorized_access_rate: float,
    tool_attempted_unauthorized_operation_rate: float,
    authorization_correctly_enforced_rate: float,
    audit_record_complete_rate: float,
    audit_record_tampered_rate: float,
    secret_redaction_successful_rate: float,
) -> str:
    """Compute the content hash for a security event summary without constructing the full model."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_revision": campaign_revision,
            "total_records": total_records,
            "sensitive_data_present_rate": sensitive_data_present_rate,
            "sensitive_data_required_rate": sensitive_data_required_rate,
            "sensitive_data_sent_externally_rate": sensitive_data_sent_externally_rate,
            "unnecessary_data_sent_externally_rate": unnecessary_data_sent_externally_rate,
            "policy_prevented_disclosure_rate": policy_prevented_disclosure_rate,
            "model_attempted_unauthorized_access_rate": model_attempted_unauthorized_access_rate,
            "tool_attempted_unauthorized_operation_rate": tool_attempted_unauthorized_operation_rate,
            "authorization_correctly_enforced_rate": authorization_correctly_enforced_rate,
            "audit_record_complete_rate": audit_record_complete_rate,
            "audit_record_tampered_rate": audit_record_tampered_rate,
            "secret_redaction_successful_rate": secret_redaction_successful_rate,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class CorrelatedErrorSummary(BaseModel):
    """Summary of correlated errors across all stacks.

    Aggregates per-scenario per-stage correlated error records into
    rates. The ``correlated_failure_rate`` is the proportion of scenarios
    where multiple stages made the same semantic error. The
    ``failure_independence`` is 1 minus the correlated failure rate.
    Same-family and cross-family rates are computed separately for the
    homogeneous vs heterogeneous comparison.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    total_scenarios: int = Field(ge=0, description="Total scenarios evaluated for correlated errors.")
    correlated_failure_rate: float = Field(ge=0.0, le=1.0)
    failure_independence: float = Field(ge=0.0, le=1.0)
    same_family_correlated_rate: float = Field(ge=0.0, le=1.0)
    cross_family_correlated_rate: float = Field(ge=0.0, le=1.0)
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the summary.",
    )

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        for rate in (
            self.correlated_failure_rate,
            self.failure_independence,
            self.same_family_correlated_rate,
            self.cross_family_correlated_rate,
        ):
            if not math.isfinite(rate):
                raise ValueError(f"non-finite correlated error rate: {rate!r}")
        expected_hash = compute_correlated_error_summary_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            total_scenarios=self.total_scenarios,
            correlated_failure_rate=self.correlated_failure_rate,
            failure_independence=self.failure_independence,
            same_family_correlated_rate=self.same_family_correlated_rate,
            cross_family_correlated_rate=self.cross_family_correlated_rate,
        )
        if self.content_hash != expected_hash:
            raise ValueError(
                f"correlated error summary content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected_hash!r}"
            )
        return self


def compute_correlated_error_summary_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    total_scenarios: int,
    correlated_failure_rate: float,
    failure_independence: float,
    same_family_correlated_rate: float,
    cross_family_correlated_rate: float,
) -> str:
    """Compute the content hash for a correlated error summary without constructing the full model."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_revision": campaign_revision,
            "total_scenarios": total_scenarios,
            "correlated_failure_rate": correlated_failure_rate,
            "failure_independence": failure_independence,
            "same_family_correlated_rate": same_family_correlated_rate,
            "cross_family_correlated_rate": cross_family_correlated_rate,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class ColdStartWarmInferenceTradeoff(BaseModel):
    """Cold-start vs warm inference tradeoff for one model variant.

    Records the four distinct timing phases separately so the
    architecture-relevant tradeoff is visible: a model with fast
    inference but slow load may be cheaper to keep resident; a model
    with fast load and fast inference may be cheap to swap on every
    escalation. The router design depends on this data.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID.")
    model_load_time_seconds: float | None = Field(
        default=None, ge=0.0,
        description="Cold-start model load time in seconds. None when not measured.",
    )
    time_to_first_token_seconds: float | None = Field(
        default=None, ge=0.0,
        description="Warm inference time to first token in seconds. None when not measured.",
    )
    generation_duration_seconds: float | None = Field(
        default=None, ge=0.0,
        description="Warm inference generation duration in seconds. None when not measured.",
    )
    whole_task_duration_seconds: float | None = Field(
        default=None, ge=0.0,
        description="Whole task duration in seconds. None when not measured.",
    )

    @model_validator(mode="after")
    def _validate_tradeoff(self) -> Self:
        for val in (
            self.model_load_time_seconds,
            self.time_to_first_token_seconds,
            self.generation_duration_seconds,
            self.whole_task_duration_seconds,
        ):
            if val is not None and not math.isfinite(val):
                raise ValueError(f"non-finite cold-start/warm-inference value: {val!r}")
        return self


class ColdStartWarmInferenceTradeoffSummary(BaseModel):
    """Summary of cold-start vs warm inference tradeoffs across all variants.

    Carries one ``ColdStartWarmInferenceTradeoff`` per variant, sorted by
    variant ID. The summary is bound to the campaign identity.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    tradeoffs: list[ColdStartWarmInferenceTradeoff] = Field(
        min_length=1,
        description="Per-variant cold-start vs warm inference tradeoffs, sorted by variant ID.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the summary.",
    )

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        variant_ids = [t.variant_id for t in self.tradeoffs]
        if len(variant_ids) != len(set(variant_ids)):
            raise ValueError(f"duplicate variant IDs in cold-start tradeoff summary: {variant_ids!r}")
        if list(variant_ids) != sorted(variant_ids):
            raise ValueError("cold-start tradeoff variants must be sorted by variant ID")
        expected_hash = compute_cold_start_warm_inference_tradeoff_summary_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            tradeoffs=self.tradeoffs,
        )
        if self.content_hash != expected_hash:
            raise ValueError(
                f"cold-start tradeoff summary content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected_hash!r}"
            )
        return self


def compute_cold_start_warm_inference_tradeoff_summary_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    tradeoffs: list[ColdStartWarmInferenceTradeoff],
) -> str:
    """Compute the content hash for a cold-start tradeoff summary without constructing the full model."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_revision": campaign_revision,
            "tradeoffs": [
                json.loads(t.model_dump_json())
                for t in sorted(tradeoffs, key=lambda t: t.variant_id)
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "ColdStartWarmInferenceTradeoff",
    "ColdStartWarmInferenceTradeoffSummary",
    "CorrelatedErrorSummary",
    "EscalationSummary",
    "RadarDimension",
    "RadarDimensionName",
    "RadarProfile",
    "SecurityEventSummary",
    "ToolScorecardDimensionSummary",
    "ToolScorecardSummary",
    "compute_cold_start_warm_inference_tradeoff_summary_hash",
    "compute_correlated_error_summary_hash",
    "compute_escalation_summary_hash",
    "compute_radar_profile_hash",
    "compute_security_event_summary_hash",
    "compute_tool_scorecard_summary_hash",
]
