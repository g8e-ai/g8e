from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest
from pydantic import ValidationError

from g8e_evals.phase0_authority_builder import build_phase0_authority_packet
from g8e_evals.live_operations_authority import (
    ArtifactBinding,
    BudgetAuthority,
    BudgetAuthoritySet,
    CandidateIdentity,
    CommandFamily,
    EF7TransportDisposition,
    EvidenceRequirement,
    ExploratoryBaselineAuthority,
    GovernedInferenceSmokeAuthority,
    LeaseStatus,
    LiveOperationLease,
    LiveOperationLeaseSet,
    LiveOperationLeaseTemplate,
    LiveOperationLeaseTemplateSet,
    LeaseStopCondition,
    ModelRole,
    OperationKind,
    TransportPath,
    build_budget_authority,
    build_budget_authority_set,
    build_ef7_transport_disposition,
    build_exploratory_baseline_authority,
    build_governed_inference_smoke_authority,
    build_live_operation_lease,
    build_live_operation_lease_template,
    build_live_operation_lease_template_set,
    render_authority_json,
)

pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_OTHER_HASH = "b" * 64


def _bindings() -> list[ArtifactBinding]:
    names = [
        "ef7_authority",
        "model_registry",
        "final_response_profile",
        "final_response_instrumentation_policy",
        "final_response_expected_record_policy",
        "final_response_preregistration",
        "replacement_manifest_rule",
    ]
    return [
        ArtifactBinding(
            name=name,
            path=f"live-packet/{name}.json",
            sha256=_VALID_HASH if index % 2 == 0 else _OTHER_HASH,
        )
        for index, name in enumerate(names)
    ]


def _budget(operation_id: str = "governed-inference-smoke") -> BudgetAuthority:
    return build_budget_authority(
        operation_id=operation_id,
        max_requests=3,
        max_tokens_per_request=16_384,
        max_tokens=49_152,
        max_usd=0.0,
        concurrency=1,
        min_free_disk_bytes=10_000_000_000,
        max_duration_seconds=1080,
        max_retries=0,
        max_replacements=0,
    )


def _template(budget: BudgetAuthority | None = None) -> LiveOperationLeaseTemplate:
    selected_budget = budget or _budget()
    return build_live_operation_lease_template(
        template_id="governed-inference-smoke-v1",
        operation_kind=OperationKind.GOVERNED_INFERENCE_SMOKE,
        operation_authority_hash=_VALID_HASH,
        budget=selected_budget,
        endpoint="http://192.168.1.2:11434",
        permitted_command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
        required_runtime_authority_names=["governed_inference_smoke"],
    )


def _candidate() -> CandidateIdentity:
    return CandidateIdentity(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="3" * 64,
        image_ids=["sha256:" + "4" * 64],
    )


def _lease(
    lease_id: str = "lease-1", status: LeaseStatus = LeaseStatus.ACTIVE
) -> LiveOperationLease:
    issued_at = datetime(2026, 9, 13, 12, 0, tzinfo=UTC)
    return build_live_operation_lease(
        lease_id=lease_id,
        template=_template(),
        candidate=_candidate(),
        model_inventory_digest="5" * 64,
        operation_identity="governed-inference-smoke-v1",
        runtime_authorities=[
            ArtifactBinding(
                name="governed_inference_smoke", path="live-packet/smoke.json", sha256=_VALID_HASH
            )
        ],
        permitted_command="g8e inference smoke --role primary --role assistant --role lite",
        command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
        report_root="reports/governed-inference-smoke-v1",
        app_identity="spiffe://g8e.local/app/g8ee",
        operator_session_identity="spiffe://g8e.local/operator/org/operator/session",
        issued_at=issued_at,
        start_deadline=issued_at + timedelta(minutes=5),
        expires_at=issued_at + timedelta(minutes=18),
        status=status,
    )


def test_ef7_transport_disposition_binds_existing_ungoverned_authorities() -> None:
    disposition = build_ef7_transport_disposition(_bindings())

    assert disposition.transport_path == TransportPath.ENSEMBLE_UNGOVERNED
    assert disposition.provider_adapter == "g8e_evals.sut.g8ee_chat.G8eeChatSUT"
    assert disposition.timeout_seconds == 120
    assert disposition.max_retries == 1
    assert disposition.concurrency == 1
    assert disposition.bindings == _bindings()
    assert len(disposition.content_hash) == 64


def test_ef7_transport_disposition_rejects_governed_transport() -> None:
    valid = build_ef7_transport_disposition(_bindings())

    with pytest.raises(ValidationError, match="ensemble_ungoverned"):
        EF7TransportDisposition.model_validate(valid.model_dump() | {"transport_path": "g8e"})


def test_governed_inference_smoke_binds_all_roles_and_receipt_evidence() -> None:
    authority = build_governed_inference_smoke_authority(_bindings())

    assert authority.roles == [ModelRole.PRIMARY, ModelRole.ASSISTANT, ModelRole.LITE]
    assert [expectation.model for expectation in authority.model_expectations] == [
        "qwen3:4b",
        "qwen3:1.7b",
        "qwen3:0.6b",
    ]
    assert authority.max_provider_calls == 3
    assert authority.automatic_retry is False
    assert authority.target_operator_session_required is True
    assert set(authority.required_evidence) == set(EvidenceRequirement)


def test_governed_inference_smoke_rejects_duplicate_roles() -> None:
    valid = build_governed_inference_smoke_authority(_bindings())

    with pytest.raises(ValidationError, match="roles"):
        GovernedInferenceSmokeAuthority.model_validate(
            valid.model_dump() | {"roles": ["primary", "primary", "lite"]}
        )


def test_exploratory_baseline_authority_binds_exact_ifeval_work() -> None:
    bindings = [
        ArtifactBinding(name=name, path=f"overnight/{name}.json", sha256=_VALID_HASH)
        for name in (
            "baseline_manifest",
            "ifeval_dataset",
            "ifeval_provenance",
            "ifeval_profile",
            "ifeval_preregistration",
            "model_registry",
        )
    ]

    authority = build_exploratory_baseline_authority(bindings)

    assert isinstance(authority, ExploratoryBaselineAuthority)
    assert authority.operation_kind == OperationKind.EXPLORATORY_BASELINE
    assert authority.transport_path == TransportPath.ENSEMBLE_UNGOVERNED
    assert authority.assignment_count == 180
    assert authority.repetitions == 5
    assert authority.publication_eligible is False


def test_budget_authority_requires_every_ceiling_and_serial_execution() -> None:
    budget = _budget()

    assert budget.max_requests == 3
    assert budget.max_tokens_per_request == 16_384
    assert budget.max_tokens == 49_152
    assert budget.max_usd == 0.0
    assert budget.concurrency == 1
    assert budget.min_free_disk_bytes == 10_000_000_000
    assert budget.max_duration_seconds == 1080


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("max_requests", 0),
        ("max_tokens_per_request", 0),
        ("max_tokens", 0),
        ("concurrency", 2),
        ("min_free_disk_bytes", 0),
        ("max_duration_seconds", 0),
    ],
)
def test_budget_authority_rejects_missing_or_nonserialized_ceiling(field: str, value: int) -> None:
    valid = _budget()

    with pytest.raises(ValidationError):
        BudgetAuthority.model_validate(valid.model_dump() | {field: value})


def test_budget_mutation_changes_content_hash() -> None:
    first = _budget()
    second = build_budget_authority(
        operation_id=first.operation_id,
        max_requests=4,
        max_tokens_per_request=first.max_tokens_per_request,
        max_tokens=first.max_tokens,
        max_usd=first.max_usd,
        concurrency=first.concurrency,
        min_free_disk_bytes=first.min_free_disk_bytes,
        max_duration_seconds=first.max_duration_seconds,
        max_retries=first.max_retries,
        max_replacements=first.max_replacements,
    )

    assert first.content_hash != second.content_hash


def test_budget_authority_set_rejects_duplicate_operation_ids() -> None:
    first = _budget()
    provisional = BudgetAuthoritySet.model_construct(
        authority_id="budgets-v1",
        budgets=[first, first],
        content_hash="0" * 64,
    )

    with pytest.raises(ValidationError, match="operation_id"):
        build_budget_authority_set(provisional.authority_id, provisional.budgets)


def test_lease_template_binds_budget_and_remote_endpoint() -> None:
    budget = _budget()
    template = _template(budget)

    assert template.budget == budget
    assert template.endpoint_class == "remote"
    assert template.endpoint == "http://192.168.1.2:11434"
    assert template.permitted_command_family == CommandFamily.GOVERNED_INFERENCE_SMOKE
    assert template.required_runtime_authority_names == ["governed_inference_smoke"]
    assert template.inventory_check_required is True
    assert template.fresh_report_root_required is True
    assert set(template.stop_conditions) == set(LeaseStopCondition)


def test_lease_template_rejects_duplicate_runtime_authority_names() -> None:
    with pytest.raises(ValidationError, match="runtime authority names"):
        build_live_operation_lease_template(
            template_id="governed-inference-smoke-v1",
            operation_kind=OperationKind.GOVERNED_INFERENCE_SMOKE,
            operation_authority_hash=_VALID_HASH,
            budget=_budget(),
            endpoint="http://192.168.1.2:11434",
            permitted_command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
            required_runtime_authority_names=["smoke", "smoke"],
        )


def test_lease_template_set_is_content_addressed_and_rejects_duplicate_ids() -> None:
    template = _template()
    template_set = build_live_operation_lease_template_set("lease-templates-v1", [template])

    assert isinstance(template_set, LiveOperationLeaseTemplateSet)
    assert len(template_set.content_hash) == 64
    with pytest.raises(ValidationError, match="template_id"):
        build_live_operation_lease_template_set("lease-templates-v1", [template, template])


def test_live_lease_binds_candidate_inventory_identity_and_ceiling() -> None:
    lease = _lease()

    assert lease.candidate == _candidate()
    assert lease.model_inventory_digest == "5" * 64
    assert lease.runtime_authorities[0].name == "governed_inference_smoke"
    assert (
        lease.permitted_command == "g8e inference smoke --role primary --role assistant --role lite"
    )
    assert lease.command_family == CommandFamily.GOVERNED_INFERENCE_SMOKE
    assert lease.budget.max_requests == 3
    assert lease.status == LeaseStatus.ACTIVE
    assert lease.issued_at < lease.start_deadline < lease.expires_at


def test_live_lease_rejects_missing_runtime_authority() -> None:
    valid = _lease()

    with pytest.raises(ValidationError, match="runtime_authorities"):
        LiveOperationLease.model_validate(valid.model_dump() | {"runtime_authorities": []})


def test_live_lease_rejects_command_family_mismatch() -> None:
    valid = _lease()

    with pytest.raises(ValidationError, match="command family"):
        LiveOperationLease.model_validate(
            valid.model_dump() | {"command_family": CommandFamily.CAMPAIGN_RUN}
        )


def test_live_lease_rejects_expiry_before_start_deadline() -> None:
    valid = _lease()

    with pytest.raises(ValidationError, match="expires_at"):
        LiveOperationLease.model_validate(valid.model_dump() | {"expires_at": valid.issued_at})


def test_lease_set_rejects_multiple_active_leases() -> None:
    with pytest.raises(ValidationError, match="one active lease"):
        LiveOperationLeaseSet(leases=[_lease("lease-1"), _lease("lease-2")])


def test_authority_rendering_is_byte_reproducible() -> None:
    first = render_authority_json(_budget())
    second = render_authority_json(_budget())

    assert first == second
    assert first.endswith(b"\n")


def test_phase0_builder_reproduces_reviewed_authority_bindings() -> None:
    first = build_phase0_authority_packet()
    second = build_phase0_authority_packet()

    assert first == second
    assert len(first.budget_authorities.budgets) == 15
    assert len(first.lease_templates.templates) == 18
    assert all(
        budget.max_tokens == budget.max_requests * 16_384
        for budget in first.budget_authorities.budgets
    )
    assert all(
        budget.min_free_disk_bytes == 1_073_741_824 for budget in first.budget_authorities.budgets
    )
    assert first.budget_authorities.budgets[-1].operation_id == "o7-two-cycle-rehearsal"
    assert first.budget_authorities.budgets[-1].max_requests == 900
    assert [template.template_id for template in first.lease_templates.templates[:3]] == [
        "embedded-authority-diagnostic-v1",
        "governed-inference-smoke-v1",
        "ef7-final-response-replacement-v1",
    ]
    assert [template.template_id for template in first.lease_templates.templates[11:15]] == [
        "p12-child-a1a0287e1df76273afe1715dab1b7b61d4561c87660f714f8f75f3f4cda89231-v1",
        "p12-child-e49f8e81119bb61e44d5facf2d2fd6177f5a7ba0f06f1a75c52547c7af07d1b9-v1",
        "p12-child-5491db99c40772a84a88efbc1d35e648f1ac532fa506a640f526c62b979764f4-v1",
        "p12-child-d81cf52d96b9aeec1cf0aa8218c50a82e1f8e74b0e9217a1ae8bbf6784f362ec-v1",
    ]
    assert first.lease_templates.templates[-1].required_runtime_authority_names == [
        "o7_rehearsal_manifest",
        "public_contract_pack",
        "mirror_trust_authority",
        "safety_stop_matrix",
    ]
    assert (
        first.ef7_transport_disposition.bindings[0].sha256
        == "72c9762c9762fa9119f9ad6827038f2e94791c4055284bfa9b036517e0dc33f1"
    )
    assert (
        first.governed_inference_smoke.bindings[0].sha256
        == "ada35b8d25596627e480a08ca5dd810f2c6ed16815b106b1b07aef45731d55bf"
    )
