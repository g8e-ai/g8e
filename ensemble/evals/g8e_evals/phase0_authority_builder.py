from __future__ import annotations

from pathlib import Path

from pydantic import BaseModel, ConfigDict

from g8e_evals.live_operations_authority import (
    ArtifactBinding,
    BudgetAuthority,
    BudgetAuthoritySet,
    CommandFamily,
    EF7TransportDisposition,
    GovernedInferenceSmokeAuthority,
    LiveOperationLeaseTemplate,
    LiveOperationLeaseTemplateSet,
    OperationKind,
    build_budget_authority,
    build_budget_authority_set,
    build_ef7_transport_disposition,
    build_governed_inference_smoke_authority,
    build_live_operation_lease_template,
    build_live_operation_lease_template_set,
    render_authority_json,
)


_REMOTE_OLLAMA_ENDPOINT = "http://192.168.1.2:11434"
_P12_CAMPAIGN_SET_PLAN_HASH = "2e7cb65fd86d52560fe065fecf28d33a7aed4cd2664a8c6d0c7295dd2f163ebb"
_REPEATABILITY_POLICY_HASH = "461dc160764859bcd3ebae78a69f19f7e7e4ff7d1f8f3253b34d5e0b83f53159"
_STACK_POLICY_HASH = "27b6534af9fc8bbd98720305d3bfcf7de193c14d53788e9a52613a82ffce9130"
_P12_CHILD_IDS = (
    "a1a0287e1df76273afe1715dab1b7b61d4561c87660f714f8f75f3f4cda89231",
    "e49f8e81119bb61e44d5facf2d2fd6177f5a7ba0f06f1a75c52547c7af07d1b9",
    "5491db99c40772a84a88efbc1d35e648f1ac532fa506a640f526c62b979764f4",
    "d81cf52d96b9aeec1cf0aa8218c50a82e1f8e74b0e9217a1ae8bbf6784f362ec",
)
_EF7_SUITE_OPERATION_IDS = (
    "ef7-final-response",
    "ef7-recovery",
    "ef7-routing-delegation",
    "ef7-security-policy",
    "ef7-technical-analysis",
    "ef7-tool-arguments",
    "ef7-tool-selection",
    "ef7-verification",
)


class Phase0AuthorityPacket(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    ef7_transport_disposition: EF7TransportDisposition
    governed_inference_smoke: GovernedInferenceSmokeAuthority
    budget_authorities: BudgetAuthoritySet
    lease_templates: LiveOperationLeaseTemplateSet


def _budget(operation_id: str, max_requests: int, max_duration_seconds: int, max_retries: int, max_replacements: int) -> BudgetAuthority:
    return build_budget_authority(
        operation_id=operation_id,
        max_requests=max_requests,
        max_tokens_per_request=16_384,
        max_tokens=max_requests * 16_384,
        max_usd=0.0,
        concurrency=1,
        min_free_disk_bytes=1_073_741_824,
        max_duration_seconds=max_duration_seconds,
        max_retries=max_retries,
        max_replacements=max_replacements,
    )


def build_phase0_budget_authorities() -> BudgetAuthoritySet:
    return build_budget_authority_set(
        "v2.1.8-live-operation-budgets-v1",
        [
            _budget("embedded-authority-diagnostic", 6, 1_800, 1, 0),
            _budget("governed-inference-smoke", 3, 1_800, 0, 0),
            _budget("ef7-final-response-replacement", 186, 28_800, 1, 3),
            _budget("p12-child", 5_580, 86_400, 1, 3),
            _budget("ef7-final-response", 186, 28_800, 1, 0),
            _budget("ef7-recovery", 372, 28_800, 1, 0),
            _budget("ef7-routing-delegation", 558, 28_800, 1, 0),
            _budget("ef7-security-policy", 372, 28_800, 1, 0),
            _budget("ef7-technical-analysis", 744, 28_800, 1, 0),
            _budget("ef7-tool-arguments", 558, 28_800, 1, 0),
            _budget("ef7-tool-selection", 744, 28_800, 1, 0),
            _budget("ef7-verification", 372, 28_800, 1, 0),
            _budget("phase-c-repeatability", 11_250, 259_200, 1, 3),
            _budget("phase-d-stack-evaluation", 1_350, 172_800, 1, 3),
            _budget("o7-two-cycle-rehearsal", 900, 259_200, 1, 1),
        ],
    )


def _budget_by_id(budgets: BudgetAuthoritySet, operation_id: str) -> BudgetAuthority:
    matches = [budget for budget in budgets.budgets if budget.operation_id == operation_id]
    if len(matches) != 1:
        raise ValueError(f"expected exactly one budget for {operation_id!r}")
    return matches[0]


def _lease_template(
    *,
    template_id: str,
    operation_kind: OperationKind,
    operation_authority_hash: str,
    budget_authorities: BudgetAuthoritySet,
    budget_operation_id: str,
    command_family: CommandFamily,
    runtime_authority_names: list[str],
) -> LiveOperationLeaseTemplate:
    return build_live_operation_lease_template(
        template_id=template_id,
        operation_kind=operation_kind,
        operation_authority_hash=operation_authority_hash,
        budget=_budget_by_id(budget_authorities, budget_operation_id),
        endpoint=_REMOTE_OLLAMA_ENDPOINT,
        permitted_command_family=command_family,
        required_runtime_authority_names=runtime_authority_names,
    )


def build_phase0_lease_templates(
    ef7_transport_disposition: EF7TransportDisposition,
    governed_inference_smoke: GovernedInferenceSmokeAuthority,
    budget_authorities: BudgetAuthoritySet,
) -> LiveOperationLeaseTemplateSet:
    templates = [
        _lease_template(
            template_id="embedded-authority-diagnostic-v1",
            operation_kind=OperationKind.EMBEDDED_AUTHORITY_DIAGNOSTIC,
            operation_authority_hash=ef7_transport_disposition.content_hash,
            budget_authorities=budget_authorities,
            budget_operation_id="embedded-authority-diagnostic",
            command_family=CommandFamily.CAMPAIGN_RUN,
            runtime_authority_names=["embedded_diagnostic_manifest"],
        ),
        _lease_template(
            template_id="governed-inference-smoke-v1",
            operation_kind=OperationKind.GOVERNED_INFERENCE_SMOKE,
            operation_authority_hash=governed_inference_smoke.content_hash,
            budget_authorities=budget_authorities,
            budget_operation_id="governed-inference-smoke",
            command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
            runtime_authority_names=["governed_inference_smoke"],
        ),
        _lease_template(
            template_id="ef7-final-response-replacement-v1",
            operation_kind=OperationKind.EF7_REPLACEMENT,
            operation_authority_hash=ef7_transport_disposition.content_hash,
            budget_authorities=budget_authorities,
            budget_operation_id="ef7-final-response-replacement",
            command_family=CommandFamily.CAMPAIGN_RUN,
            runtime_authority_names=["ef7_final_response_replacement_manifest"],
        ),
    ]
    templates.extend(
        _lease_template(
            template_id=f"{operation_id}-v1",
            operation_kind=OperationKind.EF7_PHASE_A,
            operation_authority_hash=ef7_transport_disposition.content_hash,
            budget_authorities=budget_authorities,
            budget_operation_id=operation_id,
            command_family=CommandFamily.CAMPAIGN_RUN,
            runtime_authority_names=[f"{operation_id.replace('-', '_')}_campaign_manifest"],
        )
        for operation_id in _EF7_SUITE_OPERATION_IDS
    )
    templates.extend(
        _lease_template(
            template_id=f"p12-child-{child_id}-v1",
            operation_kind=OperationKind.P12_CHILD,
            operation_authority_hash=_P12_CAMPAIGN_SET_PLAN_HASH,
            budget_authorities=budget_authorities,
            budget_operation_id="p12-child",
            command_family=CommandFamily.CAMPAIGN_RUN,
            runtime_authority_names=["p12_child_campaign_manifest"],
        )
        for child_id in _P12_CHILD_IDS
    )
    templates.extend([
        _lease_template(
            template_id="phase-c-repeatability-v1",
            operation_kind=OperationKind.PHASE_C,
            operation_authority_hash=_REPEATABILITY_POLICY_HASH,
            budget_authorities=budget_authorities,
            budget_operation_id="phase-c-repeatability",
            command_family=CommandFamily.CAMPAIGN_RUN,
            runtime_authority_names=["phase_b_role_candidate_manifest", "d16_population_selection", "phase_c_campaign_manifest"],
        ),
        _lease_template(
            template_id="phase-d-stack-evaluation-v1",
            operation_kind=OperationKind.PHASE_D,
            operation_authority_hash=_STACK_POLICY_HASH,
            budget_authorities=budget_authorities,
            budget_operation_id="phase-d-stack-evaluation",
            command_family=CommandFamily.CAMPAIGN_RUN,
            runtime_authority_names=["phase_b_role_candidate_manifest", "d16_population_selection", "phase_d_stack_manifest"],
        ),
        _lease_template(
            template_id="o7-two-cycle-rehearsal-v1",
            operation_kind=OperationKind.O7_CYCLE,
            operation_authority_hash=_budget_by_id(budget_authorities, "o7-two-cycle-rehearsal").content_hash,
            budget_authorities=budget_authorities,
            budget_operation_id="o7-two-cycle-rehearsal",
            command_family=CommandFamily.CONTROLLER_RUN,
            runtime_authority_names=["o7_rehearsal_manifest", "public_contract_pack", "mirror_trust_authority", "safety_stop_matrix"],
        ),
    ])
    return build_live_operation_lease_template_set("v2.1.8-live-operation-lease-templates-v1", templates)


def build_phase0_authority_packet() -> Phase0AuthorityPacket:
    ef7_bindings = [
        ArtifactBinding(
            name="ef7_authority",
            path=".local.dev/campaign/live-packet/ef7/ef7-authority.json",
            sha256="72c9762c9762fa9119f9ad6827038f2e94791c4055284bfa9b036517e0dc33f1",
        ),
        ArtifactBinding(
            name="model_registry",
            path=".local.dev/campaign/model-registry.json",
            sha256="ada35b8d25596627e480a08ca5dd810f2c6ed16815b106b1b07aef45731d55bf",
        ),
        ArtifactBinding(
            name="final_response_profile",
            path=".local.dev/campaign/live-packet/ef7/framework/final_response-profile.json",
            sha256="11673e78f8ae28f7c8d0f809a0b64d50582847ca5ec69b3e52d3107b74b13e20",
        ),
        ArtifactBinding(
            name="final_response_instrumentation_policy",
            path=".local.dev/campaign/live-packet/ef7/framework/final_response-instrumentation-policy.json",
            sha256="afa7b1fae12fcfc49ce7c586d420dcc593fecb826401606744b32e6a0c1beb6b",
        ),
        ArtifactBinding(
            name="final_response_expected_record_policy",
            path=".local.dev/campaign/live-packet/ef7/framework/final_response-expected-record-policy.json",
            sha256="924aca12bf17fa0cada568d2a91958c8af0f45122b29efc47d0b7b9cfabd15be",
        ),
        ArtifactBinding(
            name="final_response_preregistration",
            path=".local.dev/campaign/live-packet/ef7/framework/final_response-preregistration.json",
            sha256="80782fe58027edf95bc48d2b8a40766ba331b8b544b5aab853d9f9824ecb3773",
        ),
        ArtifactBinding(
            name="replacement_manifest_rule",
            path=".local.dev/campaign/live-packet/replacement-manifest-rule.json",
            sha256="7fce09a30f34af4c22c6c6e89feb4155ce7333595089f3de88c40b9643c63e6b",
        ),
    ]
    smoke_bindings = [
        ArtifactBinding(
            name="model_registry",
            path=".local.dev/campaign/model-registry.json",
            sha256="ada35b8d25596627e480a08ca5dd810f2c6ed16815b106b1b07aef45731d55bf",
        ),
    ]
    ef7_transport_disposition = build_ef7_transport_disposition(ef7_bindings)
    governed_inference_smoke = build_governed_inference_smoke_authority(smoke_bindings)
    budget_authorities = build_phase0_budget_authorities()
    return Phase0AuthorityPacket(
        ef7_transport_disposition=ef7_transport_disposition,
        governed_inference_smoke=governed_inference_smoke,
        budget_authorities=budget_authorities,
        lease_templates=build_phase0_lease_templates(
            ef7_transport_disposition,
            governed_inference_smoke,
            budget_authorities,
        ),
    )


def compare_rendered_authority(authority: BaseModel, path: Path) -> bool:
    return path.read_bytes() == render_authority_json(authority)
