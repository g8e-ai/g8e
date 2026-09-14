from __future__ import annotations

import hashlib
import json
from datetime import datetime
from enum import StrEnum
from typing import Literal, Self
from urllib.parse import urlparse

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

AUTHORITY_SCHEMA_VERSION = "1.0.0"
_REQUIRED_EF7_BINDINGS = frozenset(
    {
        "ef7_authority",
        "model_registry",
        "final_response_profile",
        "final_response_instrumentation_policy",
        "final_response_expected_record_policy",
        "final_response_preregistration",
        "replacement_manifest_rule",
    }
)
_REQUIRED_EXPLORATORY_BASELINE_BINDINGS = frozenset(
    {
        "baseline_manifest",
        "ifeval_dataset",
        "ifeval_provenance",
        "ifeval_profile",
        "ifeval_preregistration",
        "model_registry",
    }
)


class TransportPath(StrEnum):
    ENSEMBLE_UNGOVERNED = "ensemble_ungoverned"


class ModelRole(StrEnum):
    PRIMARY = "primary"
    ASSISTANT = "assistant"
    LITE = "lite"


class EvidenceRequirement(StrEnum):
    TARGET_MODEL_EQUALITY = "target_model_equality"
    TRANSACTION_IDENTITY = "transaction_identity"
    FINAL_RECEIPT = "final_receipt"
    RESULT_DIGEST = "result_digest"
    AUDIT_RECORD = "audit_record"
    COMMITMENT = "commitment"
    PERSISTENCE_ATTESTATION = "persistence_attestation"


class OperationKind(StrEnum):
    EMBEDDED_AUTHORITY_DIAGNOSTIC = "embedded_authority_diagnostic"
    GOVERNED_INFERENCE_SMOKE = "governed_inference_smoke"
    EXPLORATORY_BASELINE = "exploratory_baseline"
    EF7_REPLACEMENT = "ef7_replacement"
    P12_CHILD = "p12_child"
    EF7_PHASE_A = "ef7_phase_a"
    PHASE_C = "phase_c"
    PHASE_D = "phase_d"
    O7_CYCLE = "o7_cycle"


class CommandFamily(StrEnum):
    CAMPAIGN_RUN = "campaign_run"
    GOVERNED_INFERENCE_SMOKE = "governed_inference_smoke"
    CONTROLLER_RUN = "controller_run"


class LeaseStatus(StrEnum):
    DRAFT = "draft"
    ACTIVE = "active"
    COMPLETED = "completed"
    EXPIRED = "expired"
    STOPPED = "stopped"


class LeaseStopCondition(StrEnum):
    COMMAND_COMPLETED = "command_completed"
    CANDIDATE_DRIFT = "candidate_drift"
    AUTHORITY_DRIFT = "authority_drift"
    MODEL_INVENTORY_DRIFT = "model_inventory_drift"
    BUDGET_EXHAUSTED = "budget_exhausted"
    DEADLINE_EXPIRED = "deadline_expired"
    PROVIDER_CALL_UNCOUNTED = "provider_call_uncounted"
    IDENTITY_DRIFT = "identity_drift"
    RECEIPT_OR_EVIDENCE_MISMATCH = "receipt_or_evidence_mismatch"


class RoleModelExpectation(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    role: ModelRole
    model: str = Field(min_length=1)


def _canonical_payload(model: BaseModel) -> bytes:
    data = model.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    return json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode()


def _content_hash(model: BaseModel) -> str:
    return hashlib.sha256(_canonical_payload(model)).hexdigest()


def render_authority_json(model: BaseModel) -> bytes:
    return (
        json.dumps(
            model.model_dump(mode="json", by_alias=True),
            allow_nan=False,
            ensure_ascii=False,
            indent=2,
            sort_keys=True,
        )
        + "\n"
    ).encode()


class ArtifactBinding(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    name: str = Field(min_length=1)
    path: str = Field(min_length=1)
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")

    @field_validator("path")
    @classmethod
    def _validate_path(cls, value: str) -> str:
        parts = value.split("/")
        if value.startswith("/") or ".." in parts or "" in parts:
            raise ValueError("artifact path must be a normalized repository-relative path")
        return value


class EF7TransportDisposition(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    authority_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    transport_path: TransportPath
    provider_adapter: Literal["g8e_evals.sut.g8ee_chat.G8eeChatSUT"]
    timeout_seconds: int = Field(gt=0)
    max_retries: int = Field(ge=0)
    concurrency: Literal[1]
    bindings: list[ArtifactBinding] = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_disposition(self) -> Self:
        if self.transport_path != TransportPath.ENSEMBLE_UNGOVERNED:
            raise ValueError("EF7 transport_path must remain ensemble_ungoverned")
        names = [binding.name for binding in self.bindings]
        if len(names) != len(set(names)):
            raise ValueError("EF7 transport bindings must have unique names")
        missing = sorted(_REQUIRED_EF7_BINDINGS.difference(names))
        if missing:
            raise ValueError(f"EF7 transport bindings are incomplete: {missing}")
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"EF7 transport disposition content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class ExploratoryBaselineAuthority(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    authority_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    operation_kind: Literal[OperationKind.EXPLORATORY_BASELINE]
    benchmark: Literal["ifeval_subset"]
    transport_path: Literal[TransportPath.ENSEMBLE_UNGOVERNED]
    assignment_count: Literal[180]
    repetitions: Literal[5]
    publication_eligible: Literal[False]
    bindings: list[ArtifactBinding] = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_authority(self) -> Self:
        names = [binding.name for binding in self.bindings]
        if len(names) != len(set(names)):
            raise ValueError("exploratory baseline bindings must have unique names")
        missing = sorted(_REQUIRED_EXPLORATORY_BASELINE_BINDINGS.difference(names))
        if missing:
            raise ValueError(f"exploratory baseline bindings are incomplete: {missing}")
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"exploratory baseline authority content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class GovernedInferenceSmokeAuthority(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    authority_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    roles: list[ModelRole] = Field(min_length=1)
    model_expectations: list[RoleModelExpectation] = Field(min_length=1)
    max_provider_calls: int = Field(gt=0)
    automatic_retry: Literal[False]
    target_operator_session_required: Literal[True]
    required_evidence: list[EvidenceRequirement] = Field(min_length=1)
    bindings: list[ArtifactBinding] = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_smoke(self) -> Self:
        if self.roles != list(ModelRole):
            raise ValueError(f"roles must be ordered as {[role.value for role in ModelRole]}")
        if [expectation.role for expectation in self.model_expectations] != self.roles:
            raise ValueError(
                "model_expectations must bind one exact model to each role in canonical order"
            )
        if self.max_provider_calls != len(self.roles):
            raise ValueError("max_provider_calls must equal the role count")
        if self.required_evidence != list(EvidenceRequirement):
            raise ValueError(
                "required_evidence must contain every governed inference acceptance requirement in canonical order"
            )
        names = [binding.name for binding in self.bindings]
        if len(names) != len(set(names)):
            raise ValueError("governed inference smoke bindings must have unique names")
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"governed inference smoke authority content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class BudgetAuthority(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    max_requests: int = Field(gt=0)
    max_tokens_per_request: int = Field(gt=0)
    max_tokens: int = Field(gt=0)
    max_usd: float = Field(ge=0)
    concurrency: Literal[1]
    min_free_disk_bytes: int = Field(gt=0)
    max_duration_seconds: int = Field(gt=0)
    max_retries: int = Field(ge=0)
    max_replacements: int = Field(ge=0)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_hash(self) -> Self:
        if self.max_tokens > self.max_requests * self.max_tokens_per_request:
            raise ValueError(
                "aggregate max_tokens cannot exceed max_requests times max_tokens_per_request"
            )
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"budget authority content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class BudgetAuthoritySet(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    authority_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    budgets: list[BudgetAuthority] = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_set(self) -> Self:
        operation_ids = [budget.operation_id for budget in self.budgets]
        if len(operation_ids) != len(set(operation_ids)):
            raise ValueError("budget operation_id values must be unique")
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"budget authority set content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class LiveOperationLeaseTemplate(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    template_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    operation_kind: OperationKind
    operation_authority_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    budget: BudgetAuthority
    endpoint: str = Field(min_length=1)
    endpoint_class: Literal["remote"]
    permitted_command_family: CommandFamily
    required_runtime_authority_names: list[str] = Field(min_length=1)
    inventory_check_required: Literal[True]
    fresh_candidate_identity_required: Literal[True]
    fresh_report_root_required: Literal[True]
    stop_conditions: list[LeaseStopCondition] = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @field_validator("endpoint")
    @classmethod
    def _validate_endpoint(cls, value: str) -> str:
        parsed = urlparse(value)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("endpoint must be an absolute HTTP URL")
        return value.rstrip("/")

    @model_validator(mode="after")
    def _validate_template(self) -> Self:
        if len(self.required_runtime_authority_names) != len(
            set(self.required_runtime_authority_names)
        ):
            raise ValueError("required runtime authority names must be unique")
        if any(not name for name in self.required_runtime_authority_names):
            raise ValueError("required runtime authority names must be non-empty")
        if self.stop_conditions != list(LeaseStopCondition):
            raise ValueError(
                "stop_conditions must contain every lease stop condition in canonical order"
            )
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"lease template content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class LiveOperationLeaseTemplateSet(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    authority_id: str = Field(min_length=1)
    authority_version: str = Field(default=AUTHORITY_SCHEMA_VERSION)
    templates: list[LiveOperationLeaseTemplate] = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_set(self) -> Self:
        template_ids = [template.template_id for template in self.templates]
        if len(template_ids) != len(set(template_ids)):
            raise ValueError("lease template_id values must be unique")
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"lease template set content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class CandidateIdentity(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    source_tree_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    execution_source_manifest_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    binary_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    image_ids: list[str] = Field(min_length=1)

    @field_validator("image_ids")
    @classmethod
    def _validate_image_ids(cls, values: list[str]) -> list[str]:
        if len(values) != len(set(values)) or any(
            len(value) != 71 or not value.startswith("sha256:") for value in values
        ):
            raise ValueError("image_ids must be unique sha256 image identities")
        return values


class LiveOperationLease(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    lease_id: str = Field(min_length=1)
    template: LiveOperationLeaseTemplate
    candidate: CandidateIdentity
    model_inventory_digest: str = Field(pattern=r"^[0-9a-f]{64}$")
    operation_identity: str = Field(min_length=1)
    runtime_authorities: list[ArtifactBinding] = Field(min_length=1)
    permitted_command: str = Field(min_length=1)
    command_family: CommandFamily
    report_root: str = Field(min_length=1)
    app_identity: str = Field(min_length=1)
    operator_session_identity: str = Field(min_length=1)
    endpoint: str = Field(min_length=1)
    budget: BudgetAuthority
    issued_at: datetime
    start_deadline: datetime
    expires_at: datetime
    status: LeaseStatus
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_lease(self) -> Self:
        if not self.issued_at < self.start_deadline < self.expires_at:
            raise ValueError("issued_at must precede start_deadline and expires_at")
        if self.endpoint != self.template.endpoint:
            raise ValueError("lease endpoint must match the template")
        if self.command_family != self.template.permitted_command_family:
            raise ValueError("lease command family must match the template")
        runtime_authority_names = [authority.name for authority in self.runtime_authorities]
        if runtime_authority_names != self.template.required_runtime_authority_names:
            raise ValueError(
                "lease runtime authorities must exactly match the template requirements in canonical order"
            )
        if self.budget.content_hash != self.template.budget.content_hash:
            raise ValueError("lease budget must match the template")
        expected = _content_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"live operation lease content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class LiveOperationLeaseSet(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    leases: list[LiveOperationLease]

    @model_validator(mode="after")
    def _validate_active_count(self) -> Self:
        if sum(lease.status == LeaseStatus.ACTIVE for lease in self.leases) > 1:
            raise ValueError("only one active lease is permitted")
        return self


def build_ef7_transport_disposition(bindings: list[ArtifactBinding]) -> EF7TransportDisposition:
    fields = {
        "authority_id": "ef7-v2.1.8-transport-disposition-v1",
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "transport_path": TransportPath.ENSEMBLE_UNGOVERNED,
        "provider_adapter": "g8e_evals.sut.g8ee_chat.G8eeChatSUT",
        "timeout_seconds": 120,
        "max_retries": 1,
        "concurrency": 1,
        "bindings": bindings,
    }
    provisional = EF7TransportDisposition.model_construct(**fields, content_hash="0" * 64)
    return EF7TransportDisposition(**fields, content_hash=_content_hash(provisional))


def build_exploratory_baseline_authority(
    bindings: list[ArtifactBinding],
) -> ExploratoryBaselineAuthority:
    fields = {
        "authority_id": "v2.1.8-exploratory-overnight-ifeval-baseline-v1",
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "operation_kind": OperationKind.EXPLORATORY_BASELINE,
        "benchmark": "ifeval_subset",
        "transport_path": TransportPath.ENSEMBLE_UNGOVERNED,
        "assignment_count": 180,
        "repetitions": 5,
        "publication_eligible": False,
        "bindings": bindings,
    }
    provisional = ExploratoryBaselineAuthority.model_construct(**fields, content_hash="0" * 64)
    return ExploratoryBaselineAuthority(**fields, content_hash=_content_hash(provisional))


def build_governed_inference_smoke_authority(
    bindings: list[ArtifactBinding],
) -> GovernedInferenceSmokeAuthority:
    fields = {
        "authority_id": "governed-inference-smoke-v1",
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "roles": list(ModelRole),
        "model_expectations": [
            RoleModelExpectation(role=ModelRole.PRIMARY, model="qwen3:4b"),
            RoleModelExpectation(role=ModelRole.ASSISTANT, model="qwen3:1.7b"),
            RoleModelExpectation(role=ModelRole.LITE, model="qwen3:0.6b"),
        ],
        "max_provider_calls": len(ModelRole),
        "automatic_retry": False,
        "target_operator_session_required": True,
        "required_evidence": list(EvidenceRequirement),
        "bindings": bindings,
    }
    provisional = GovernedInferenceSmokeAuthority.model_construct(**fields, content_hash="0" * 64)
    return GovernedInferenceSmokeAuthority(**fields, content_hash=_content_hash(provisional))


def build_budget_authority(
    *,
    operation_id: str,
    max_requests: int,
    max_tokens_per_request: int,
    max_tokens: int,
    max_usd: float,
    concurrency: Literal[1],
    min_free_disk_bytes: int,
    max_duration_seconds: int,
    max_retries: int,
    max_replacements: int,
) -> BudgetAuthority:
    fields = {
        "operation_id": operation_id,
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "max_requests": max_requests,
        "max_tokens_per_request": max_tokens_per_request,
        "max_tokens": max_tokens,
        "max_usd": max_usd,
        "concurrency": concurrency,
        "min_free_disk_bytes": min_free_disk_bytes,
        "max_duration_seconds": max_duration_seconds,
        "max_retries": max_retries,
        "max_replacements": max_replacements,
    }
    provisional = BudgetAuthority.model_construct(**fields, content_hash="0" * 64)
    return BudgetAuthority(**fields, content_hash=_content_hash(provisional))


def build_budget_authority_set(
    authority_id: str, budgets: list[BudgetAuthority]
) -> BudgetAuthoritySet:
    fields = {
        "authority_id": authority_id,
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "budgets": budgets,
    }
    provisional = BudgetAuthoritySet.model_construct(**fields, content_hash="0" * 64)
    return BudgetAuthoritySet(**fields, content_hash=_content_hash(provisional))


def build_live_operation_lease_template(
    *,
    template_id: str,
    operation_kind: OperationKind,
    operation_authority_hash: str,
    budget: BudgetAuthority,
    endpoint: str,
    permitted_command_family: CommandFamily,
    required_runtime_authority_names: list[str],
) -> LiveOperationLeaseTemplate:
    fields = {
        "template_id": template_id,
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "operation_kind": operation_kind,
        "operation_authority_hash": operation_authority_hash,
        "budget": budget,
        "endpoint": endpoint,
        "endpoint_class": "remote",
        "permitted_command_family": permitted_command_family,
        "required_runtime_authority_names": required_runtime_authority_names,
        "inventory_check_required": True,
        "fresh_candidate_identity_required": True,
        "fresh_report_root_required": True,
        "stop_conditions": list(LeaseStopCondition),
    }
    provisional = LiveOperationLeaseTemplate.model_construct(**fields, content_hash="0" * 64)
    return LiveOperationLeaseTemplate(**fields, content_hash=_content_hash(provisional))


def build_live_operation_lease_template_set(
    authority_id: str,
    templates: list[LiveOperationLeaseTemplate],
) -> LiveOperationLeaseTemplateSet:
    fields = {
        "authority_id": authority_id,
        "authority_version": AUTHORITY_SCHEMA_VERSION,
        "templates": templates,
    }
    provisional = LiveOperationLeaseTemplateSet.model_construct(**fields, content_hash="0" * 64)
    return LiveOperationLeaseTemplateSet(**fields, content_hash=_content_hash(provisional))


def build_live_operation_lease(
    *,
    lease_id: str,
    template: LiveOperationLeaseTemplate,
    candidate: CandidateIdentity,
    model_inventory_digest: str,
    operation_identity: str,
    runtime_authorities: list[ArtifactBinding],
    permitted_command: str,
    command_family: CommandFamily,
    report_root: str,
    app_identity: str,
    operator_session_identity: str,
    issued_at: datetime,
    start_deadline: datetime,
    expires_at: datetime,
    status: LeaseStatus,
) -> LiveOperationLease:
    fields = {
        "lease_id": lease_id,
        "template": template,
        "candidate": candidate,
        "model_inventory_digest": model_inventory_digest,
        "operation_identity": operation_identity,
        "runtime_authorities": runtime_authorities,
        "permitted_command": permitted_command,
        "command_family": command_family,
        "report_root": report_root,
        "app_identity": app_identity,
        "operator_session_identity": operator_session_identity,
        "endpoint": template.endpoint,
        "budget": template.budget,
        "issued_at": issued_at,
        "start_deadline": start_deadline,
        "expires_at": expires_at,
        "status": status,
    }
    provisional = LiveOperationLease.model_construct(**fields, content_hash="0" * 64)
    return LiveOperationLease(**fields, content_hash=_content_hash(provisional))
