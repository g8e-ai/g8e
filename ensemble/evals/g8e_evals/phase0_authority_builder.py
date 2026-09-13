from __future__ import annotations

from pathlib import Path

from pydantic import BaseModel, ConfigDict

from g8e_evals.live_operations_authority import (
    ArtifactBinding,
    BudgetAuthority,
    BudgetAuthoritySet,
    EF7TransportDisposition,
    GovernedInferenceSmokeAuthority,
    build_budget_authority,
    build_budget_authority_set,
    build_ef7_transport_disposition,
    build_governed_inference_smoke_authority,
    render_authority_json,
)


class Phase0AuthorityPacket(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    ef7_transport_disposition: EF7TransportDisposition
    governed_inference_smoke: GovernedInferenceSmokeAuthority
    budget_authorities: BudgetAuthoritySet


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
    return Phase0AuthorityPacket(
        ef7_transport_disposition=build_ef7_transport_disposition(ef7_bindings),
        governed_inference_smoke=build_governed_inference_smoke_authority(smoke_bindings),
        budget_authorities=build_phase0_budget_authorities(),
    )


def compare_rendered_authority(authority: BaseModel, path: Path) -> bool:
    return path.read_bytes() == render_authority_json(authority)
