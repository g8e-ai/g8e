# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import binascii
import hashlib
import json
import logging
import os
import platform
import sys
import uuid
from pathlib import Path
from datetime import datetime, UTC

import click
from rich.console import Console

from app.constants import LLMProvider
from app.llm.factory import get_llm_provider
from app.models.settings import LLMSettings
from app.services.ai.eval_judge import EvalJudge, EvalJudgeError
from g8e.constants import PORTS
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_OUTCOME_FAILED,
)
from g8e.receipts import (
    decode_ed25519_public_key,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)
from g8e_evals.arms import ALL_ARMS, GOVERNED_ARMS, Arm, GovernancePosture
from g8e_evals.auth_bridge import AuthBridgeError, load_cli_auth_context
from g8e_evals.graders import (
    DeterministicGradingContext,
    grade_deterministically,
    observe_receipt_final_state,
)
from g8e_evals.evidence import EvidenceEncryptionKey, encrypt_evidence_artifact, load_evidence_encryption_key
from g8e_evals.harness import BindingType, LLMRoleConfig, ReceiptEvidence, RowResult, SUTConfig
from g8e_evals.stages import EvidenceArtifact, normalize_attempt_evidence
from g8e_evals.schema import (
    ArmManifestEntry,
    ArtifactLeakageObservation,
    AttemptRecord,
    ContentHash,
    ExfiltrationAttemptObservation,
    FinalStateObservation,
    GraderClass,
    GraderReference,
    MetricObservation,
    ModelIdentity,
    PolicyOutcome,
    PostureObservation,
    PayloadTamperingObservation,
    ReceiptObservation,
    RehydrationObservation,
    ReplayAttemptObservation,
    RoleToModelMapping,
    RunManifest,
    SecretDetectionObservation,
    SignedFieldTamperingObservation,
    StaleStateRootObservation,
    IdentityMismatchObservation,
    NonceExpirationObservation,
    SignerDefectObservation,
    L3ProofTransplantObservation,
    RevokedCredentialObservation,
    EvidencePreservationObservation,
    StackEnvironment,
    StateObservation,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    UnauthorizedMutationObservation,
    TokenStorePersistenceObservation,
    TokenTTLExpiryObservation,
    TokenPersistenceFailureObservation,
    VerificationStatus,
)
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.sut.direct_provider import DirectProviderSUT
from g8e_evals.sut.g8ee_chat import ChatEvaluationReceipt, G8eeChatSUT, AuthenticationError
from g8e_evals.posture import observe_gateway_posture
from g8e_evals.transport import AuthContext
from g8e_evals.agent_trail_renderer import TurnRenderer
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.provenance import load_provenance
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.receipts.collector import ReceiptCollector
from g8e_evals.receipts.verify import receipt_action_type
from g8e_evals.report.aggregate import aggregate_results
from g8e_evals.report.cli_renderer import render_summary
from g8e_evals.models import ScoreDetails

console = Console()
logger = logging.getLogger(__name__)

_PROVIDER_CHOICES = ["openai", "anthropic", "gemini", "ollama", "llamacpp", "fake"]
_KEYLESS_PROVIDERS = frozenset({"ollama", "llamacpp", "fake"})
_IFEVAL_GRADER_ID = "ifeval_subset_verifier"
_EVAL_JUDGE_GRADER_ID = "eval_judge"
_RECEIPT_INTEGRITY_GRADER_ID = "receipt_integrity"
_PROTOCOL_CHAIN_GRADER_ID = "protocol_chain"
_CANARY_SCRUBBING_GRADER_ID = "canary_scrubbing"
_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID = "model_boundary_raw_secret_rate"
_EXACT_LOCAL_REHYDRATION_GRADER_ID = "exact_local_rehydration"
_SECRET_DETECTION_PRECISION_GRADER_ID = "secret_detection_precision"
_SECRET_DETECTION_RECALL_GRADER_ID = "secret_detection_recall"
_FINAL_STATE_GRADER_ID = "final_state_assertions"
_FINAL_STATE_METRIC_ID = "final_state_accuracy"
_INDEPENDENT_STATE_GRADER_ID = "independent_state"
_INDEPENDENT_STATE_METRIC_ID = "independent_state_accuracy"
_POLICY_OUTCOME_GRADER_ID = "policy_outcome"
_UNAUTHORIZED_MUTATION_GRADER_ID = "unauthorized_mutation"
_TOKEN_STORE_PERSISTENCE_GRADER_ID = "token_store_persistence"
_TOKEN_TTL_EXPIRY_GRADER_ID = "token_ttl_expiry"
_TOKEN_PERSISTENCE_FAILURE_GRADER_ID = "token_persistence_failure"
_EXFILTRATION_ATTEMPT_GRADER_ID = "exfiltration_attempt"
_ARTIFACT_LEAKAGE_GRADER_ID = "artifact_leakage"
_REPLAY_ATTEMPT_GRADER_ID = "replay_attempt"
_SIGNED_FIELD_TAMPERING_GRADER_ID = "signed_field_tampering"
_PAYLOAD_TAMPERING_GRADER_ID = "payload_tampering"
_STALE_STATE_ROOT_GRADER_ID = "stale_state_root"
_IDENTITY_MISMATCH_GRADER_ID = "identity_mismatch"
_NONCE_EXPIRATION_GRADER_ID = "nonce_expiration"
_SIGNER_DEFECT_GRADER_ID = "signer_defect"
_L3_PROOF_TRANSPLANT_GRADER_ID = "l3_proof_transplant"
_REVOKED_CREDENTIAL_GRADER_ID = "revoked_credential"
_EVIDENCE_PRESERVATION_GRADER_ID = "evidence_preservation"
_GRADER_VERSION = "1.0.0"


class EvaluationRunError(Exception):
    pass


def _create_eval_judge(config: LLMRoleConfig) -> EvalJudge:
    if not config.provider or not config.model:
        raise EvaluationRunError("eval judge requires both provider and model")
    settings = LLMSettings(
        llm_lite_provider=LLMProvider(config.provider),
        llm_lite_model=config.model,
        lite_api_key=config.api_key,
        lite_endpoint=config.endpoint,
    )
    return EvalJudge(
        provider=get_llm_provider(settings, is_lite=True),
        model=config.model,
    )


@click.group()
def main():
    """g8e High-Fidelity AI Evaluation Harness"""
    handler = logging.StreamHandler()
    handler.setFormatter(logging.Formatter("%(asctime)s - %(name)s - %(levelname)s - %(message)s"))
    root_logger = logging.getLogger()
    root_logger.setLevel(logging.INFO)
    root_logger.addHandler(handler)

    # Silence noisy loggers
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)

    logger.info("evals CLI initialized")

@main.command()
@click.option("--suite", type=click.Choice(["ifeval_subset"]), required=True)
@click.option("--provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_PRIMARY_PROVIDER", help="Primary LLM provider")
@click.option("--model", envvar="G8E_TEST_LLM_PRIMARY_MODEL", help="Primary model name (e.g., gpt-4o)")
@click.option("--assistant-provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_ASSISTANT_PROVIDER", help="Assistant LLM provider")
@click.option("--assistant-model", envvar="G8E_TEST_LLM_ASSISTANT_MODEL", help="Assistant model name")
@click.option("--lite-provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_LITE_PROVIDER", help="Lite LLM provider")
@click.option("--lite-model", envvar="G8E_TEST_LLM_LITE_MODEL", help="Lite model name")
@click.option("--judge-provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_JUDGE_PROVIDER", help="Secondary Eval Judge provider")
@click.option("--judge-model", envvar="G8E_TEST_LLM_JUDGE_MODEL", help="Secondary Eval Judge model")
@click.option("--headless/--no-headless", default=False,
              help="Approve correlated command requests from the authenticated SSE stream")
@click.option("--verbose-text/--no-verbose-text", default=False,
              help="Stream the agent's response text inline as chunks arrive")
@click.option("--idle-timeout", type=float, default=10.0,
              help="Seconds without an SSE event before declaring a task idle")
@click.option("--g8ee-url", envvar="G8E_G8EE_URL", required=True,
              help="URL of the g8ee application endpoint.")
@click.option("--operator-url", default=f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}")
@click.option("--operator-session-id", envvar="G8E_OPERATOR_SESSION_ID",
              help="Operator session id override. The default comes from the canonical CLI identity.")
@click.option("--g8e-cli", default="./g8e", envvar="G8E_CLI_BIN", show_default=True,
              help="Path to the g8e CLI used to load the canonical authentication context.")
@click.option("--auth-project-root", type=click.Path(path_type=Path, file_okay=False),
              envvar="G8E_AUTH_PROJECT_ROOT", required=True,
              help="Project root containing the canonical CLI runtime identity.")
@click.option("--arm", type=click.Choice([a.value for a in ALL_ARMS]), default=Arm.ENSEMBLE_UNGOVERNED.value,
              help="Experiment arm: direct (raw provider call), ensemble_ungoverned (g8ee without governance), doctrine (L1), consensus (L1+L2), notary (L1+L2+L3)")
@click.option("--state-root", default="test-state-root-v1")
@click.option("--output-dir", type=click.Path(path_type=Path), default=Path("reports"))
@click.option("--evidence-key-file", type=click.Path(path_type=Path, dir_okay=False), envvar="G8E_EVIDENCE_KEY_FILE",
              help='Owner-only JSON key file: {"version":1,"key_id":"...","key_b64":"<32-byte-base64>"}.')
@click.option("--gold-set", type=click.Path(exists=True, path_type=Path))
@click.option("--limit", type=int, help="Limit number of tasks to run")
@click.option("--l2-key", help="L2 private key hex")
@click.option("--l2-key-id", help="L2 key ID")
@click.option("--primary-api-key", envvar="G8E_TEST_LLM_PRIMARY_API_KEY", help="API key for the primary provider")
@click.option("--primary-endpoint", envvar="G8E_TEST_LLM_PRIMARY_ENDPOINT", help="Endpoint URL for the primary provider")
@click.option("--assistant-api-key", envvar="G8E_TEST_LLM_ASSISTANT_API_KEY", help="API key for the assistant provider")
@click.option("--assistant-endpoint", envvar="G8E_TEST_LLM_ASSISTANT_ENDPOINT", help="Endpoint URL for the assistant provider")
@click.option("--lite-api-key", envvar="G8E_TEST_LLM_LITE_API_KEY", help="API key for the lite provider")
@click.option("--lite-endpoint", envvar="G8E_TEST_LLM_LITE_ENDPOINT", help="Endpoint URL for the lite provider")
@click.option("--judge-api-key", envvar="G8E_TEST_LLM_JUDGE_API_KEY", help="API key for the Eval Judge provider")
@click.option("--judge-endpoint", envvar="G8E_TEST_LLM_JUDGE_ENDPOINT", help="Endpoint URL for the Eval Judge provider")
@click.option("--web-search-project", envvar="G8E_WEB_SEARCH_PROJECT", help="Web search project ID")
@click.option("--web-search-app", envvar="G8E_WEB_SEARCH_APP", help="Web search app ID")
@click.option("--web-search-api-key", envvar="G8E_WEB_SEARCH_API_KEY", help="Web search API key")
def run(suite, model, provider, assistant_model, assistant_provider, lite_model, lite_provider, judge_model, judge_provider, headless, verbose_text, idle_timeout, g8ee_url, operator_url, operator_session_id, g8e_cli, auth_project_root, arm, state_root, output_dir, evidence_key_file, gold_set, limit, l2_key, l2_key_id, primary_api_key, primary_endpoint, assistant_api_key, assistant_endpoint, lite_api_key, lite_endpoint, judge_api_key, judge_endpoint, web_search_project, web_search_app, web_search_api_key):
    """Run a benchmark suite"""
    # Reject the well-known footgun: passing the operator_id UUID as
    # --operator-session-id silently 401s downstream because the Gateway
    # has no session matching that id. Fail fast with an actionable hint
    # rather than warning-and-continuing with an invalid token.
    operator_id_env = (os.environ.get("G8E_OPERATOR_ID") or "").strip()
    if operator_session_id and operator_id_env and operator_session_id == operator_id_env:
        raise click.UsageError(
            "--operator-session-id was given the operator_id UUID, not a session id. "
            "Drop the flag so `./g8e auth context` can load the canonical session."
        )

    try:
        auth_context = load_cli_auth_context(g8e_cli, str(auth_project_root.resolve()))
    except AuthBridgeError as error:
        raise click.UsageError(
            f"Could not load the canonical CLI identity: {error}. "
            "Run `./g8e auth enroll user` or `./g8e auth refresh`, then retry."
        ) from error

    if evidence_key_file is None:
        raise click.UsageError(
            "--evidence-key-file or G8E_EVIDENCE_KEY_FILE is required to encrypt restricted evidence."
        )
    try:
        evidence_key = load_evidence_encryption_key(evidence_key_file)
    except ValueError as error:
        raise click.UsageError(f"Could not load the evidence encryption key: {error}") from error

    selected_arm = Arm(arm)

    config = SUTConfig(
        g8ee_url=g8ee_url,
        primary=LLMRoleConfig(provider=provider, model=model, api_key=primary_api_key, endpoint=primary_endpoint),
        assistant=LLMRoleConfig(provider=assistant_provider, model=assistant_model, api_key=assistant_api_key, endpoint=assistant_endpoint),
        lite=LLMRoleConfig(provider=lite_provider, model=lite_model, api_key=lite_api_key, endpoint=lite_endpoint),
        judge=LLMRoleConfig(provider=judge_provider, model=judge_model, api_key=judge_api_key, endpoint=judge_endpoint),
        operator_url=operator_url,
        operator_session_id=operator_session_id or auth_context.operator_session_id,
        auth_context=auth_context,
        state_root=state_root,
        l2_private_key=l2_key,
        l2_key_id=l2_key_id,
        arm=selected_arm,
        headless=headless,
    )

    try:
        asyncio.run(_run_suite(suite, config, gold_set, output_dir, limit, verbose_text=verbose_text, idle_timeout=idle_timeout, evidence_key=evidence_key))
    except EvaluationRunError as error:
        raise click.ClickException(str(error)) from error

async def _run_suite(suite: str, config: SUTConfig, gold_set: Path | None, output_dir: Path, limit: int | None = None, verbose_text: bool = False, idle_timeout: float = 180.0, evidence_key: EvidenceEncryptionKey | None = None):
    # 1. Load benchmark
    if suite == "ifeval_subset":
        if not gold_set:
            gold_set = Path("gold_sets/ifeval_subset/input_data.jsonl")
        loader = IFEvalLoader(gold_set)
        tasks = list(loader.load())
        if limit:
            tasks = tasks[:limit]
        verifier = IFEvalVerifier()
        provenance = load_provenance(gold_set.with_name("provenance.json"))
        suite_id = provenance.benchmark
        suite_version = provenance.output.sha256[:12]
        dataset_hash = provenance.output.sha256
    else:
        raise EvaluationRunError(f"unknown suite: {suite}")

    # 2. Apply G8E_TEST_LLM_* env vars as fallbacks (uniform with integration tests)
    # Priority: CLI flags > G8E_TEST_LLM_* env vars > g8ee settings
    if not config.primary.provider:
        config.primary.provider = os.environ.get("G8E_TEST_LLM_PRIMARY_PROVIDER", "").strip() or None
    if not config.primary.model:
        config.primary.model = os.environ.get("G8E_TEST_LLM_PRIMARY_MODEL", "").strip() or None
    if not config.primary.api_key:
        config.primary.api_key = os.environ.get("G8E_TEST_LLM_PRIMARY_API_KEY", "").strip() or None
    if not config.primary.endpoint:
        config.primary.endpoint = os.environ.get("G8E_TEST_LLM_PRIMARY_ENDPOINT_URL", "").strip() or None

    if not config.assistant.provider:
        config.assistant.provider = os.environ.get("G8E_TEST_LLM_ASSISTANT_PROVIDER", "").strip() or None
    if not config.assistant.model:
        config.assistant.model = os.environ.get("G8E_TEST_LLM_ASSISTANT_MODEL", "").strip() or None
    if not config.assistant.api_key:
        config.assistant.api_key = os.environ.get("G8E_TEST_LLM_ASSISTANT_API_KEY", "").strip() or None
    if not config.assistant.endpoint:
        config.assistant.endpoint = os.environ.get("G8E_TEST_LLM_ASSISTANT_ENDPOINT_URL", "").strip() or None

    if not config.lite.provider:
        config.lite.provider = os.environ.get("G8E_TEST_LLM_LITE_PROVIDER", "").strip() or None
    if not config.lite.model:
        config.lite.model = os.environ.get("G8E_TEST_LLM_LITE_MODEL", "").strip() or None
    if not config.lite.api_key:
        config.lite.api_key = os.environ.get("G8E_TEST_LLM_LITE_API_KEY", "").strip() or None
    if not config.lite.endpoint:
        config.lite.endpoint = os.environ.get("G8E_TEST_LLM_LITE_ENDPOINT_URL", "").strip() or None

    arm_def = config.arm_definition

    # 3. Build model identities. The manifest refuses to start when a
    #    required model identity is unavailable. The direct arm requires
    #    only the primary role; g8ee arms require at least one model
    #    (primary, assistant, or lite) either from CLI flags or g8ee
    #    settings (resolved during preflight).
    def _model_identity(role: str) -> ModelIdentity | None:
        rc = getattr(config, role)
        if not rc.provider or not rc.model:
            return None
        return ModelIdentity(
            role=role,
            provider=rc.provider,
            model=rc.model,
            endpoint=rc.endpoint,
            endpoint_class="local" if rc.provider in _KEYLESS_PROVIDERS or (rc.endpoint or "").startswith(("http://localhost", "http://127.")) else "remote",
            api_key_present=bool(rc.api_key),
        )

    primary_identity = _model_identity("primary")
    assistant_identity = _model_identity("assistant")
    lite_identity = _model_identity("lite")
    judge_identity = _model_identity("judge")

    if bool(config.judge.provider) != bool(config.judge.model):
        raise EvaluationRunError("eval judge requires both provider and model")
    if (
        judge_identity is not None
        and config.judge.provider not in _KEYLESS_PROVIDERS
        and not config.judge.api_key
    ):
        raise EvaluationRunError(
            f"Missing API key for judge provider '{config.judge.provider}'"
        )

    if arm_def.arm_id == Arm.DIRECT:
        if primary_identity is None:
            raise EvaluationRunError(
                "direct arm requires a primary model identity (provider and model). "
                "Provide them via --provider/--model or G8E_TEST_LLM_PRIMARY_*."
            )

    # 4. Initialize SUT and run preflight BEFORE writing the manifest.
    #    Preflight failures (auth, provider config) must not create a
    #    report directory; the diagnostic surface is the command output.
    current_renderer: dict[str, TurnRenderer | None] = {"r": None}

    async def _on_event(event_type: str, payload: dict) -> None:
        r = current_renderer["r"]
        if r is not None:
            r.render(event_type, payload)

    llm_settings = None
    remote_settings = None

    if arm_def.arm_id == Arm.DIRECT:
        sut = DirectProviderSUT(config)
    else:
        try:
            sut = G8eeChatSUT(
                config,
                on_event=_on_event,
                idle_timeout_s=idle_timeout,
            )
            remote_settings = await sut.check_settings()
        except AuthenticationError as e:
            console.print("[bold red]Authentication Error:[/bold red]")
            console.print(f"  {e}")
            console.print("\n[yellow]Run ./g8e auth enroll user or ./g8e auth refresh.[/yellow]")
            raise EvaluationRunError(f"preflight authentication failed: {e}") from e

        llm_settings = remote_settings.llm if remote_settings else None

        errors = []
        for role_name in ["primary", "assistant", "lite", "judge"]:
            role_config = getattr(config, role_name)
            if not role_config or not role_config.provider:
                continue

            if role_config.provider in _KEYLESS_PROVIDERS:
                continue

            if role_config.api_key:
                continue

            if role_name == "judge":
                errors.append(
                    f"Missing API key for judge provider '{role_config.provider}'"
                )
                continue

            if not llm_settings:
                errors.append(f"Missing API key for {role_name} provider '{role_config.provider}' (could not fetch remote settings)")
                continue

            provider_key_map = {
                "openai": "openai_api_key",
                "anthropic": "anthropic_api_key",
                "gemini": "gemini_api_key",
            }
            remote_key_field = provider_key_map.get(role_config.provider)
            if remote_key_field and getattr(llm_settings, remote_key_field, None):
                continue

            if getattr(llm_settings, f"{role_name}_api_key", None):
                continue

            errors.append(f"Missing API key for {role_name} provider '{role_config.provider}'")

        if errors:
            console.print("[bold red]Pre-flight validation failed:[/bold red]")
            for err in errors:
                console.print(f"  - {err}")
            console.print("\n[yellow]Provide keys via --primary-api-key, etc. or configure them in g8ee settings.[/yellow]")
            raise EvaluationRunError("provider preflight failed: " + "; ".join(errors))

        has_cli_primary = bool(config.primary.provider and config.primary.model)
        has_cli_assistant = bool(config.assistant.provider and config.assistant.model)
        has_cli_lite = bool(config.lite.provider and config.lite.model)

        if llm_settings is None and not (has_cli_primary or has_cli_assistant or has_cli_lite):
            console.print("[bold red]Pre-flight validation failed:[/bold red]")
            console.print("  - Could not fetch LLM settings from g8ee and no CLI model provided")
            console.print("\n[yellow]To configure LLM models:[/yellow]")
            console.print("  1. Run: ./g8e platform settings")
            console.print("  2. Set primary_model and/or assistant_model in the LLM section")
            console.print("  3. Or set G8E_TEST_LLM_* environment variables (uniform with integration tests):")
            console.print("     - G8E_TEST_LLM_PRIMARY_PROVIDER (e.g., 'openai')")
            console.print("     - G8E_TEST_LLM_PRIMARY_MODEL (e.g., 'gpt-4o')")
            console.print("     - G8E_TEST_LLM_PRIMARY_API_KEY (your API key)")
            console.print("     - G8E_TEST_LLM_PRIMARY_ENDPOINT_URL (if using custom endpoint)")
            console.print("  4. Restart g8ee: ./g8e apps restart g8ee")
            console.print("\n[yellow]Alternatively, use CLI flags:[/yellow]")
            console.print("  ./g8e evals bench --suite ifeval_subset --provider openai --model gpt-4o")
            raise EvaluationRunError("provider preflight failed: no model configured")

        if llm_settings:
            has_settings_primary = bool(llm_settings.primary_model)
            has_settings_assistant = bool(llm_settings.assistant_model)
            has_settings_lite = bool(llm_settings.lite_model)

            if not (has_cli_primary or has_cli_assistant or has_cli_lite or
                    has_settings_primary or has_settings_assistant or has_settings_lite):
                console.print("[bold red]Pre-flight validation failed:[/bold red]")
                console.print("  - No LLM model configured in g8ee settings")
                console.print("\n[yellow]To configure LLM models:[/yellow]")
                console.print("  1. Run: ./g8e platform settings")
                console.print("  2. Set primary_model and/or assistant_model in the LLM section")
                console.print("  3. Or set G8E_TEST_LLM_* environment variables (uniform with integration tests):")
                console.print("     - G8E_TEST_LLM_PRIMARY_PROVIDER (e.g., 'openai')")
                console.print("     - G8E_TEST_LLM_PRIMARY_MODEL (e.g., 'gpt-4o')")
                console.print("     - G8E_TEST_LLM_PRIMARY_API_KEY (your API key)")
                console.print("     - G8E_TEST_LLM_PRIMARY_ENDPOINT_URL (if using custom endpoint)")
                console.print("  4. Restart g8ee: ./g8e apps restart g8ee")
                console.print("\n[yellow]Alternatively, use CLI flags:[/yellow]")
                console.print("  ./g8e evals bench --suite ifeval_subset --provider openai --model gpt-4o")
                raise EvaluationRunError("provider preflight failed: no model configured")

    eval_judge = _create_eval_judge(config.judge) if judge_identity is not None else None

    if evidence_key is None:
        raise EvaluationRunError("restricted evidence encryption key is required")

    # 5. Build the run manifest. Required hashes: dataset hash (always),
    #    grader bundle hash (computed from verifier source), prompt bundle
    #    hash (computed from task prompts). The manifest refuses to start
    #    when a required hash or model identity is unavailable.
    run_id = str(uuid.uuid4())

    prompt_bundle_content = "\n".join(t.prompt for t in tasks).encode()
    prompt_bundle_hash = hashlib.sha256(prompt_bundle_content).hexdigest()

    grader_bundle_content = (
        f"{suite}:{suite_version}:"
        f"{_IFEVAL_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_RECEIPT_INTEGRITY_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_PROTOCOL_CHAIN_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_CANARY_SCRUBBING_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_EXACT_LOCAL_REHYDRATION_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_SECRET_DETECTION_PRECISION_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_SECRET_DETECTION_RECALL_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_FINAL_STATE_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_INDEPENDENT_STATE_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_POLICY_OUTCOME_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_EVAL_JUDGE_GRADER_ID}@{_GRADER_VERSION}:"
        f"{config.judge.provider or ''}:{config.judge.model or ''}"
    ).encode()
    grader_bundle_hash = hashlib.sha256(grader_bundle_content).hexdigest()

    content_hashes = [
        ContentHash(name="dataset", sha256=dataset_hash, byte_length=gold_set.stat().st_size if gold_set.exists() else 0),
        ContentHash(name="prompt_bundle", sha256=prompt_bundle_hash, byte_length=len(prompt_bundle_content)),
        ContentHash(name="grader_bundle", sha256=grader_bundle_hash, byte_length=len(grader_bundle_content)),
    ]

    arm_entry = ArmManifestEntry(
        arm_id=arm_def.arm_id,
        requested_posture=arm_def.requested_posture,
        uses_g8ee=arm_def.uses_g8ee,
        uses_gateway=arm_def.uses_gateway,
        receipt_binding=arm_def.receipt_binding,
        is_production_posture=arm_def.is_production_posture,
    )

    manifest = RunManifest(
        run_id=run_id,
        suite_id=suite_id,
        suite_version=suite_version,
        arms=[arm_entry],
        content_hashes=content_hashes,
        role_to_model=RoleToModelMapping(
            primary=primary_identity,
            assistant=assistant_identity,
            lite=lite_identity,
            judge=judge_identity,
        ),
        stack_environment=StackEnvironment(
            os=platform.platform(),
            arch=platform.machine(),
            runtime_version=platform.python_version(),
        ),
    )

    # 6. Create report directory and write manifest BEFORE execution.
    ts = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
    report_dir = output_dir / f"{suite}-{ts}"
    report_dir.mkdir(parents=True, exist_ok=True)

    manifest_path = report_dir / "manifest.json"
    manifest_path.write_text(manifest.model_dump_json(indent=2))

    collector = ReceiptCollector(config.operator_url, cli_context=config.auth_context)

    # Load warden pub key for verification
    warden_pub_path = Path(os.environ.get("G8E_GATEWAY_PKI_DIR", ".g8e/pki")) / "warden_pub.pem"
    warden_pub = ""
    if warden_pub_path.exists():
        warden_pub = warden_pub_path.read_text()

    # Observe the gateway's configured posture for governed arms. The
    # gateway is the posture authority; the runner never infers posture
    # from the CLI argument alone. For ungoverned arms, the observed
    # posture is NONE because the task does not route through the gateway.
    observed_posture: GovernancePosture | None = GovernancePosture.NONE
    posture_source = "arm_definition"
    if arm_def.uses_gateway:
        gw_env = AuthContext.from_env(
            operator_session_id=config.operator_session_id,
            g8ee_url=config.g8ee_url,
            operator_url=config.operator_url,
            cli_context=config.auth_context,
        )
        observed_posture = await observe_gateway_posture(gw_env)
        posture_source = "gateway_health_endpoint"
        if observed_posture is None:
            console.print("[bold yellow]Warning: could not observe gateway posture; recording as unobserved[/bold yellow]")
        else:
            console.print(f"  [dim]Observed gateway posture: {observed_posture.value}[/dim]")

    results = []

    display_model = f"{config.primary.provider}:{config.primary.model}" if config.primary.provider and config.primary.model else (config.primary.model or "openai:gpt-4")
    console.print(f"[bold blue]Running {suite} [{arm_def.arm_id.value}] with {display_model}...[/bold blue]")

    # 7. Write task definitions before execution.
    task_defs = [
        TaskDefinition(
            task_id=t.id,
            suite_id=suite_id,
            suite_version=suite_version,
            category=t.metadata.category or "instruction_following",
            expected_action_class=t.metadata.expected_action_class,
            compatible_arms=list(
                GOVERNED_ARMS
                if (
                    t.metadata.expected_action_class
                    or t.metadata.state_fixture
                    or t.metadata.expected_final_state_assertions
                    or t.metadata.expected_allow_block_outcome
                    or t.metadata.unauthorized_mutation_assertions
                    or t.metadata.token_store_persistence_assertions
                    or t.metadata.token_ttl_expiry_assertions
                    or t.metadata.token_persistence_failure_assertions
                    or t.metadata.exfiltration_attempt_assertions
                    or t.metadata.artifact_leakage_assertions
                    or t.metadata.replay_attempt_assertions
                    or t.metadata.signed_field_tampering_assertions
                    or t.metadata.payload_tampering_assertions
                    or t.metadata.stale_state_root_assertions
                    or t.metadata.identity_mismatch_assertions
                    or t.metadata.nonce_expiration_assertions
                    or t.metadata.signer_defect_assertions
                    or t.metadata.l3_proof_transplant_assertions
                    or t.metadata.revoked_credential_assertions
                    or t.metadata.evidence_preservation_assertions
                )
                else ALL_ARMS
            ),
            prompt_hash=hashlib.sha256(t.prompt.encode()).hexdigest(),
            prompt_length=len(t.prompt),
            initial_state_fixture_hash=(
                t.metadata.state_fixture.fixture_sha256 if t.metadata.state_fixture else None
            ),
            state_fixture=t.metadata.state_fixture,
            expected_final_state_assertions=t.metadata.expected_final_state_assertions,
            expected_allow_block_outcome=t.metadata.expected_allow_block_outcome,
            expected_rejection_layer=t.metadata.expected_rejection_layer,
            sensitive_canary_annotations=t.metadata.sensitive_canary_annotations,
            rehydration_assertions=t.metadata.rehydration_assertions,
            secret_detection_assertions=t.metadata.secret_detection_assertions,
            unauthorized_mutation_assertions=t.metadata.unauthorized_mutation_assertions,
            token_store_persistence_assertions=t.metadata.token_store_persistence_assertions,
            token_ttl_expiry_assertions=t.metadata.token_ttl_expiry_assertions,
            token_persistence_failure_assertions=t.metadata.token_persistence_failure_assertions,
            exfiltration_attempt_assertions=t.metadata.exfiltration_attempt_assertions,
            artifact_leakage_assertions=t.metadata.artifact_leakage_assertions,
            replay_attempt_assertions=t.metadata.replay_attempt_assertions,
            signed_field_tampering_assertions=t.metadata.signed_field_tampering_assertions,
            payload_tampering_assertions=t.metadata.payload_tampering_assertions,
            stale_state_root_assertions=t.metadata.stale_state_root_assertions,
            identity_mismatch_assertions=t.metadata.identity_mismatch_assertions,
            nonce_expiration_assertions=t.metadata.nonce_expiration_assertions,
            signer_defect_assertions=t.metadata.signer_defect_assertions,
            l3_proof_transplant_assertions=t.metadata.l3_proof_transplant_assertions,
            revoked_credential_assertions=t.metadata.revoked_credential_assertions,
            evidence_preservation_assertions=t.metadata.evidence_preservation_assertions,
            graders=[
                GraderReference(
                    grader_id=_IFEVAL_GRADER_ID,
                    grader_version=_GRADER_VERSION,
                    grader_class=GraderClass.DETERMINISTIC,
                ),
                *([
                    GraderReference(
                        grader_id=_EVAL_JUDGE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.LLM_JUDGE,
                    )
                ] if eval_judge else []),
                *([
                    GraderReference(
                        grader_id=_RECEIPT_INTEGRITY_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_action_class else []),
                *([
                    GraderReference(
                        grader_id=_PROTOCOL_CHAIN_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_action_class else []),
                *([
                    GraderReference(
                        grader_id=_CANARY_SCRUBBING_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.sensitive_canary_annotations else []),
                *([
                    GraderReference(
                        grader_id=_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.sensitive_canary_annotations else []),
                *([
                    GraderReference(
                        grader_id=_EXACT_LOCAL_REHYDRATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.rehydration_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SECRET_DETECTION_PRECISION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.secret_detection_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SECRET_DETECTION_RECALL_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.secret_detection_assertions else []),
                *([
                    GraderReference(
                        grader_id=_FINAL_STATE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_final_state_assertions else []),
                *([
                    GraderReference(
                        grader_id=_INDEPENDENT_STATE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.state_fixture else []),
                *([
                    GraderReference(
                        grader_id=_POLICY_OUTCOME_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_allow_block_outcome else []),
                *([
                    GraderReference(
                        grader_id=_UNAUTHORIZED_MUTATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.unauthorized_mutation_assertions else []),
                *([
                    GraderReference(
                        grader_id=_TOKEN_STORE_PERSISTENCE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.token_store_persistence_assertions else []),
                *([
                    GraderReference(
                        grader_id=_TOKEN_TTL_EXPIRY_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.token_ttl_expiry_assertions else []),
                *([
                    GraderReference(
                        grader_id=_TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.token_persistence_failure_assertions else []),
                *([
                    GraderReference(
                        grader_id=_EXFILTRATION_ATTEMPT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.exfiltration_attempt_assertions else []),
                *([
                    GraderReference(
                        grader_id=_ARTIFACT_LEAKAGE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.artifact_leakage_assertions else []),
                *([
                    GraderReference(
                        grader_id=_REPLAY_ATTEMPT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.replay_attempt_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SIGNED_FIELD_TAMPERING_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.signed_field_tampering_assertions else []),
                *([
                    GraderReference(
                        grader_id=_PAYLOAD_TAMPERING_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.payload_tampering_assertions else []),
                *([
                    GraderReference(
                        grader_id=_STALE_STATE_ROOT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.stale_state_root_assertions else []),
                *([
                    GraderReference(
                        grader_id=_IDENTITY_MISMATCH_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.identity_mismatch_assertions else []),
                *([
                    GraderReference(
                        grader_id=_NONCE_EXPIRATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.nonce_expiration_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SIGNER_DEFECT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.signer_defect_assertions else []),
                *([
                    GraderReference(
                        grader_id=_L3_PROOF_TRANSPLANT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.l3_proof_transplant_assertions else []),
                *([
                    GraderReference(
                        grader_id=_REVOKED_CREDENTIAL_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.revoked_credential_assertions else []),
                *([
                    GraderReference(
                        grader_id=_EVIDENCE_PRESERVATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.evidence_preservation_assertions else []),
            ],
            metadata={"instruction_id_list": t.metadata.instruction_id_list},
        )
        for t in tasks
    ]
    task_defs_by_id = {task.task_id: task for task in task_defs}
    with open(report_dir / "tasks.jsonl", "w") as f:
        for td in task_defs:
            f.write(td.model_dump_json() + "\n")

    # 8. Execution loop
    attempt_records: list[AttemptRecord] = []
    stage_records: list[StageObservation] = []
    metric_records: list[MetricObservation] = []
    receipt_records: list[ReceiptObservation] = []
    final_state_records: list[FinalStateObservation] = []
    state_records: list[StateObservation] = []
    rehydration_records: list[RehydrationObservation] = []
    secret_detection_records: list[SecretDetectionObservation] = []
    unauthorized_mutation_records: list[UnauthorizedMutationObservation] = []
    token_store_persistence_records: list[TokenStorePersistenceObservation] = []
    token_ttl_expiry_records: list[TokenTTLExpiryObservation] = []
    token_persistence_failure_records: list[TokenPersistenceFailureObservation] = []
    exfiltration_attempt_records: list[ExfiltrationAttemptObservation] = []
    artifact_leakage_records: list[ArtifactLeakageObservation] = []
    replay_attempt_records: list[ReplayAttemptObservation] = []
    signed_field_tampering_records: list[SignedFieldTamperingObservation] = []
    payload_tampering_records: list[PayloadTamperingObservation] = []
    stale_state_root_records: list[StaleStateRootObservation] = []
    identity_mismatch_records: list[IdentityMismatchObservation] = []
    nonce_expiration_records: list[NonceExpirationObservation] = []
    signer_defect_records: list[SignerDefectObservation] = []
    l3_proof_transplant_records: list[L3ProofTransplantObservation] = []
    revoked_credential_records: list[RevokedCredentialObservation] = []
    evidence_preservation_records: list[EvidencePreservationObservation] = []
    evidence_artifacts: list[EvidenceArtifact] = []
    for task in tasks:
        intent = ""
        if suite == "ifeval_subset" and task.metadata.instruction_id_list:
            constraints = [instruction_id.split(":")[-1] for instruction_id in task.metadata.instruction_id_list]
            intent = f" [dim][{', '.join(constraints)}][/dim]"

        prompt_preview = task.prompt.replace("\n", " ")[:50]
        if len(task.prompt) > 50:
            prompt_preview += "..."

        console.print(f"  [cyan]{task.id:>4}[/cyan]: {prompt_preview}{intent}")

        renderer = TurnRenderer(console, task_id=str(task.id), verbose_text=verbose_text)
        current_renderer["r"] = renderer

        started_at = datetime.now(UTC)
        response = await sut.get_answer(task)
        current_renderer["r"] = None
        ended_at = datetime.now(UTC)

        terminal_event = response.chat_evidence.terminal_event if response.chat_evidence else None

        renderer.finish(
            terminal_event=terminal_event,
            answer_chars=len(response.answer or ""),
        )

        if arm_def.receipt_binding:
            collected_receipts = []
            collected_transaction_ids: set[str] = set()
            for transaction_id in response.transaction_ids:
                receipt = await collector.collect_receipt(transaction_id)
                if receipt is not None and receipt.transaction_id not in collected_transaction_ids:
                    collected_receipts.append(receipt)
                    collected_transaction_ids.add(receipt.transaction_id)

            correlated_action_types = [
                action_type
                for action_type in [
                    task.metadata.expected_action_class,
                    *(
                        assertion.action_type
                        for assertion in task.metadata.expected_final_state_assertions
                    ),
                    *(
                        assertion.action_type
                        for assertion in (
                            task.metadata.state_fixture.assertions
                            if task.metadata.state_fixture
                            else []
                        )
                    ),
                    *response.governed_action_types,
                ]
                if action_type
            ]
            if isinstance(response.chat_evidence, ChatEvaluationReceipt):
                for action_type in dict.fromkeys(correlated_action_types):
                    if any(
                        receipt_action_type(receipt) == action_type
                        for receipt in collected_receipts
                    ):
                        continue
                    receipt = await collector.collect_receipt_for_investigation(
                        response.chat_evidence.investigation_id,
                        action_type,
                    )
                    if receipt is not None and receipt.transaction_id not in collected_transaction_ids:
                        collected_receipts.append(receipt)
                        collected_transaction_ids.add(receipt.transaction_id)

            response.receipts = [
                ReceiptEvidence(
                    action_receipt=receipt,
                    verified=bool(
                        warden_pub
                        and verify_action_receipt_signature(receipt, warden_pub)
                        and verify_receipt_persistence_attestation(receipt, warden_pub)
                    ),
                )
                for receipt in collected_receipts
            ]
            if task.metadata.expected_action_class:
                matching_receipts = [
                    receipt.action_receipt.transaction_id
                    for receipt in response.receipts
                    if receipt_action_type(receipt.action_receipt)
                    == task.metadata.expected_action_class
                ]
                if len(matching_receipts) == 1:
                    response.primary_transaction_id = matching_receipts[0]
            elif response.receipts:
                response.primary_transaction_id = response.receipts[0].action_receipt.transaction_id

            if response.primary_transaction_id:
                response.binding = BindingType.RECEIPT_BOUND
                response.unbound_reason = None
            elif response.receipts:
                response.binding = BindingType.UNBOUND
                response.unbound_reason = "declared-action receipt was not uniquely identified"

        # Score
        if suite == "ifeval_subset":
            score = verifier.verify(
                task.id,
                task.prompt,
                response.answer,
                task.metadata.instruction_id_list,
                task.metadata.kwargs
            )

        judge_metric_value: float | None = None
        judge_metric_status = VerificationStatus.NOT_APPLICABLE
        if eval_judge is not None:
            try:
                judge_grade = await eval_judge.grade_turn(
                    user_query=task.prompt,
                    interaction_trace=response.answer,
                    expected_behavior="Satisfy every declared benchmark instruction.",
                    required_concepts=task.metadata.instruction_id_list,
                )
                score.model_calls.extend(judge_grade.model_calls)
                judge_metric_value = float(judge_grade.score)
                judge_metric_status = VerificationStatus.VERIFIED
                score.details.benchmark_specific["eval_judge"] = {
                    "status": "completed",
                    "score": judge_grade.score,
                    "passed": judge_grade.passed,
                }
            except EvalJudgeError as error:
                score.model_calls.extend(error.model_calls)
                judge_metric_status = VerificationStatus.FAILED
                score.details.benchmark_specific["eval_judge"] = {
                    "status": "failed",
                    "error_type": type(error).__name__,
                }

        res = RowResult(task=task, response=response, score=score, arm=arm_def.arm_id)
        results.append(res)

        # Build the attempt record with requested and observed posture.
        # The observed posture comes from the gateway health endpoint for
        # governed arms, not from the CLI argument. posture_match is None
        # when the posture could not be independently observed.
        if observed_posture is not None and arm_def.uses_gateway:
            posture_match = observed_posture == arm_def.requested_posture
        elif not arm_def.uses_gateway:
            posture_match = True
        else:
            posture_match = None

        posture = PostureObservation(
            requested_posture=arm_def.requested_posture,
            observed_posture=observed_posture,
            observation_source=posture_source,
            observation_timestamp=ended_at,
            posture_match=posture_match,
        )

        if "HTTP 403" in (response.unbound_reason or "") and "Governance verification failed" in (response.unbound_reason or ""):
            terminal_status = TerminalStatus.GOVERNANCE_REJECTED
        elif "HTTP 401" in (response.unbound_reason or "") or "HTTP 403" in (response.unbound_reason or ""):
            terminal_status = TerminalStatus.INFRASTRUCTURE_FAILED
        elif not response.answer or not terminal_event or terminal_event.endswith(("failed", "stopped", "dead.lettered")):
            terminal_status = TerminalStatus.MODEL_FAILED
        else:
            terminal_status = TerminalStatus.COMPLETED

        attempt_id = f"{run_id}:{task.id}:{arm_def.arm_id.value}:1"
        attempt_receipts = [
            ReceiptObservation(
                receipt_id=f"{attempt_id}:receipt:{receipt.action_receipt.transaction_id}",
                attempt_id=attempt_id,
                run_id=run_id,
                transaction_id=receipt.action_receipt.transaction_id,
                action_type=receipt_action_type(receipt.action_receipt),
                primary=receipt.action_receipt.transaction_id == response.primary_transaction_id,
                verified=receipt.verified,
                action_receipt=receipt.action_receipt,
            )
            for receipt in response.receipts
        ]
        receipt_records.extend(attempt_receipts)
        if any(
            receipt.primary
            and receipt.verified
            and any(
                stage.kind == DETERMINISTIC_STAGE_KIND_L4_VERIFICATION
                and stage.outcome == DETERMINISTIC_STAGE_OUTCOME_FAILED
                for stage in receipt.action_receipt.deterministic_stage_evidence
            )
            for receipt in attempt_receipts
        ):
            terminal_status = TerminalStatus.GOVERNANCE_REJECTED
        normalized = (
            normalize_attempt_evidence(
                response.chat_evidence,
                run_id,
                attempt_id,
                receipts=response.receipts,
                grading_model_calls=score.model_calls,
            )
            if response.chat_evidence
            else None
        )
        evidence_refs: list[str] = []
        attempt_stages = normalized.stages if normalized else []
        if normalized:
            stage_records.extend(attempt_stages)
            if normalized.raw_evidence:
                evidence_artifacts.append(normalized.raw_evidence)
                evidence_refs.append(normalized.raw_evidence.index.artifact_id)
            usage_metric = MetricObservation(
                metric_id="stage_usage_reconciled",
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=float(normalized.usage.reconciled),
                unit="boolean",
                verification_status=VerificationStatus.VERIFIED,
                evidence_refs=evidence_refs,
            )
            DEFAULT_METRIC_REGISTRY.validate(usage_metric)
            metric_records.append(usage_metric)

        primary_receipt = next(
            (receipt for receipt in attempt_receipts if receipt.primary),
            None,
        )
        attempt = AttemptRecord(
            attempt_id=attempt_id,
            run_id=run_id,
            task_id=task.id,
            arm_id=arm_def.arm_id,
            state_snapshot_hash=(
                task.metadata.state_fixture.fixture_sha256
                if task.metadata.state_fixture
                else config.state_root
            ),
            replicate_id="1",
            assignment_order=len(attempt_records),
            started_at=started_at,
            ended_at=ended_at,
            terminal_status=terminal_status,
            posture=posture,
            state_root_before=(
                primary_receipt.action_receipt.state_root_before or None
                if primary_receipt
                else None
            ),
            state_root_after=(
                primary_receipt.action_receipt.state_root_after or None
                if primary_receipt
                else None
            ),
            correlation_ids={
                "transaction_id": response.primary_transaction_id or "",
            },
            receipt_refs=[receipt.receipt_id for receipt in attempt_receipts],
            missingness_or_failure=response.unbound_reason if terminal_status != TerminalStatus.COMPLETED else None,
            usage_reconciliation=normalized.usage if normalized else None,
        )
        final_state_observations = observe_receipt_final_state(
            task_defs_by_id[task.id],
            attempt,
            attempt_receipts,
        )
        final_state_records.extend(final_state_observations)
        attempt.final_state_observation_refs = [
            observation.observation_id for observation in final_state_observations
        ]
        state_observations = (
            await config.state_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.state_fixture and config.state_observer is not None
            else []
        )
        state_records.extend(state_observations)
        attempt.state_observation_refs = [
            observation.observation_id for observation in state_observations
        ]
        rehydration_observations = (
            await config.rehydration_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.rehydration_assertions and config.rehydration_observer is not None
            else []
        )
        rehydration_records.extend(rehydration_observations)
        attempt.rehydration_observation_refs = [
            observation.observation_id for observation in rehydration_observations
        ]
        secret_detection_observations = (
            await config.secret_detection_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.secret_detection_assertions and config.secret_detection_observer is not None
            else []
        )
        secret_detection_records.extend(secret_detection_observations)
        attempt.secret_detection_observation_refs = [
            observation.observation_id for observation in secret_detection_observations
        ]
        unauthorized_mutation_observations = (
            await config.unauthorized_mutation_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.unauthorized_mutation_assertions and config.unauthorized_mutation_observer is not None
            else []
        )
        unauthorized_mutation_records.extend(unauthorized_mutation_observations)
        attempt.unauthorized_mutation_observation_refs = [
            observation.observation_id for observation in unauthorized_mutation_observations
        ]
        token_store_persistence_observations = (
            await config.token_store_persistence_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.token_store_persistence_assertions and config.token_store_persistence_observer is not None
            else []
        )
        token_store_persistence_records.extend(token_store_persistence_observations)
        attempt.token_store_persistence_observation_refs = [
            observation.observation_id for observation in token_store_persistence_observations
        ]
        token_ttl_expiry_observations = (
            await config.token_ttl_expiry_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.token_ttl_expiry_assertions and config.token_ttl_expiry_observer is not None
            else []
        )
        token_ttl_expiry_records.extend(token_ttl_expiry_observations)
        attempt.token_ttl_expiry_observation_refs = [
            observation.observation_id for observation in token_ttl_expiry_observations
        ]
        token_persistence_failure_observations = (
            await config.token_persistence_failure_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.token_persistence_failure_assertions and config.token_persistence_failure_observer is not None
            else []
        )
        token_persistence_failure_records.extend(token_persistence_failure_observations)
        attempt.token_persistence_failure_observation_refs = [
            observation.observation_id for observation in token_persistence_failure_observations
        ]
        exfiltration_attempt_observations = (
            await config.exfiltration_attempt_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.exfiltration_attempt_assertions and config.exfiltration_attempt_observer is not None
            else []
        )
        exfiltration_attempt_records.extend(exfiltration_attempt_observations)
        attempt.exfiltration_attempt_observation_refs = [
            observation.observation_id for observation in exfiltration_attempt_observations
        ]
        artifact_leakage_observations = (
            await config.artifact_leakage_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.artifact_leakage_assertions and config.artifact_leakage_observer is not None
            else []
        )
        artifact_leakage_records.extend(artifact_leakage_observations)
        attempt.artifact_leakage_observation_refs = [
            observation.observation_id for observation in artifact_leakage_observations
        ]
        replay_attempt_observations = (
            await config.replay_attempt_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.replay_attempt_assertions and config.replay_attempt_observer is not None
            else []
        )
        replay_attempt_records.extend(replay_attempt_observations)
        attempt.replay_attempt_observation_refs = [
            observation.observation_id for observation in replay_attempt_observations
        ]
        signed_field_tampering_observations = (
            await config.signed_field_tampering_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.signed_field_tampering_assertions and config.signed_field_tampering_observer is not None
            else []
        )
        signed_field_tampering_records.extend(signed_field_tampering_observations)
        attempt.signed_field_tampering_observation_refs = [
            observation.observation_id for observation in signed_field_tampering_observations
        ]
        payload_tampering_observations = (
            await config.payload_tampering_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.payload_tampering_assertions and config.payload_tampering_observer is not None
            else []
        )
        payload_tampering_records.extend(payload_tampering_observations)
        attempt.payload_tampering_observation_refs = [
            observation.observation_id for observation in payload_tampering_observations
        ]
        stale_state_root_observations = (
            await config.stale_state_root_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.stale_state_root_assertions and config.stale_state_root_observer is not None
            else []
        )
        stale_state_root_records.extend(stale_state_root_observations)
        attempt.stale_state_root_observation_refs = [
            observation.observation_id for observation in stale_state_root_observations
        ]
        identity_mismatch_observations = (
            await config.identity_mismatch_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.identity_mismatch_assertions and config.identity_mismatch_observer is not None
            else []
        )
        identity_mismatch_records.extend(identity_mismatch_observations)
        attempt.identity_mismatch_observation_refs = [
            observation.observation_id for observation in identity_mismatch_observations
        ]
        nonce_expiration_observations = (
            await config.nonce_expiration_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.nonce_expiration_assertions and config.nonce_expiration_observer is not None
            else []
        )
        nonce_expiration_records.extend(nonce_expiration_observations)
        attempt.nonce_expiration_observation_refs = [
            observation.observation_id for observation in nonce_expiration_observations
        ]
        signer_defect_observations = (
            await config.signer_defect_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.signer_defect_assertions and config.signer_defect_observer is not None
            else []
        )
        signer_defect_records.extend(signer_defect_observations)
        attempt.signer_defect_observation_refs = [
            observation.observation_id for observation in signer_defect_observations
        ]
        l3_proof_transplant_observations = (
            await config.l3_proof_transplant_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.l3_proof_transplant_assertions and config.l3_proof_transplant_observer is not None
            else []
        )
        l3_proof_transplant_records.extend(l3_proof_transplant_observations)
        attempt.l3_proof_transplant_observation_refs = [
            observation.observation_id for observation in l3_proof_transplant_observations
        ]
        revoked_credential_observations = (
            await config.revoked_credential_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.revoked_credential_assertions and config.revoked_credential_observer is not None
            else []
        )
        revoked_credential_records.extend(revoked_credential_observations)
        attempt.revoked_credential_observation_refs = [
            observation.observation_id for observation in revoked_credential_observations
        ]
        evidence_preservation_observations = (
            await config.evidence_preservation_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.evidence_preservation_assertions and config.evidence_preservation_observer is not None
            else []
        )
        evidence_preservation_records.extend(evidence_preservation_observations)
        attempt.evidence_preservation_observation_refs = [
            observation.observation_id for observation in evidence_preservation_observations
        ]
        grade_metrics = [
            MetricObservation(
                metric_id=_IFEVAL_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=float(score.passed),
                unit="boolean",
                verification_status=VerificationStatus.VERIFIED,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=evidence_refs,
            )
        ]
        if eval_judge is not None:
            grade_metrics.append(MetricObservation(
                metric_id=_EVAL_JUDGE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=judge_metric_value,
                unit="score_1_to_5",
                eligible=judge_metric_status == VerificationStatus.VERIFIED,
                denominator_contribution=int(judge_metric_status == VerificationStatus.VERIFIED),
                verification_status=judge_metric_status,
                grader_class=GraderClass.LLM_JUDGE,
                evidence_refs=evidence_refs,
            ))
        if task.metadata.expected_action_class:
            receipt_grade = grade_deterministically(
                _RECEIPT_INTEGRITY_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_RECEIPT_INTEGRITY_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=receipt_grade.value,
                unit="boolean",
                verification_status=receipt_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=receipt_grade.evidence_refs,
            ))
            protocol_grade = grade_deterministically(
                _PROTOCOL_CHAIN_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_PROTOCOL_CHAIN_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=protocol_grade.value,
                unit="boolean",
                verification_status=protocol_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=protocol_grade.evidence_refs,
            ))
        if task.metadata.sensitive_canary_annotations:
            canary_grade = grade_deterministically(
                _CANARY_SCRUBBING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_CANARY_SCRUBBING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=canary_grade.value,
                unit="proportion",
                denominator_contribution=len(task.metadata.sensitive_canary_annotations),
                verification_status=canary_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=canary_grade.evidence_refs,
            ))
            model_boundary_grade = grade_deterministically(
                _MODEL_BOUNDARY_RAW_SECRET_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=model_boundary_grade.value,
                unit="raw_occurrences_per_injected_canary",
                denominator_contribution=sum(
                    assertion.expected_occurrences
                    for assertion in task.metadata.sensitive_canary_annotations
                ),
                verification_status=model_boundary_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=model_boundary_grade.evidence_refs,
            ))
        if task.metadata.rehydration_assertions:
            rehydration_grade = grade_deterministically(
                _EXACT_LOCAL_REHYDRATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    rehydration_observations=rehydration_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EXACT_LOCAL_REHYDRATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=rehydration_grade.value,
                unit="proportion",
                eligible=rehydration_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=rehydration_grade.denominator_contribution,
                verification_status=rehydration_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=rehydration_grade.evidence_refs,
            ))
        if task.metadata.secret_detection_assertions:
            for grader_id in (
                _SECRET_DETECTION_PRECISION_GRADER_ID,
                _SECRET_DETECTION_RECALL_GRADER_ID,
            ):
                secret_detection_grade = grade_deterministically(
                    grader_id,
                    _GRADER_VERSION,
                    DeterministicGradingContext(
                        task=task_defs_by_id[task.id],
                        attempt=attempt,
                        receipts=attempt_receipts,
                        stages=attempt_stages,
                        secret_detection_observations=secret_detection_observations,
                    ),
                )
                grade_metrics.append(MetricObservation(
                    metric_id=grader_id,
                    attempt_id=attempt_id,
                    run_id=run_id,
                    arm_id=arm_def.arm_id,
                    task_id=task.id,
                    value=secret_detection_grade.value,
                    unit="proportion",
                    eligible=(
                        secret_detection_grade.verification_status
                        == VerificationStatus.VERIFIED
                    ),
                    denominator_contribution=(
                        secret_detection_grade.denominator_contribution
                    ),
                    verification_status=secret_detection_grade.verification_status,
                    grader_class=GraderClass.DETERMINISTIC,
                    evidence_refs=secret_detection_grade.evidence_refs,
                ))
        if task.metadata.expected_final_state_assertions:
            final_state_grade = grade_deterministically(
                _FINAL_STATE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_FINAL_STATE_METRIC_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=final_state_grade.value,
                unit="proportion",
                denominator_contribution=len(task.metadata.expected_final_state_assertions),
                verification_status=final_state_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=final_state_grade.evidence_refs,
            ))
        if task.metadata.state_fixture:
            independent_state_grade = grade_deterministically(
                _INDEPENDENT_STATE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    state_observations=state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_INDEPENDENT_STATE_METRIC_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=independent_state_grade.value,
                unit="proportion",
                denominator_contribution=len(task.metadata.state_fixture.assertions),
                verification_status=independent_state_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=independent_state_grade.evidence_refs,
            ))
        if task.metadata.expected_allow_block_outcome:
            policy_grade = grade_deterministically(
                _POLICY_OUTCOME_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_POLICY_OUTCOME_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=policy_grade.value,
                unit="boolean",
                verification_status=policy_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=policy_grade.evidence_refs,
            ))
        if task.metadata.unauthorized_mutation_assertions:
            unauthorized_mutation_grade = grade_deterministically(
                _UNAUTHORIZED_MUTATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    unauthorized_mutation_observations=unauthorized_mutation_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_UNAUTHORIZED_MUTATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=unauthorized_mutation_grade.value,
                unit="proportion",
                eligible=unauthorized_mutation_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=unauthorized_mutation_grade.denominator_contribution,
                verification_status=unauthorized_mutation_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=unauthorized_mutation_grade.evidence_refs,
            ))
        if task.metadata.token_store_persistence_assertions:
            token_store_persistence_grade = grade_deterministically(
                _TOKEN_STORE_PERSISTENCE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    token_store_persistence_observations=token_store_persistence_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_STORE_PERSISTENCE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=token_store_persistence_grade.value,
                unit="proportion",
                eligible=token_store_persistence_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=token_store_persistence_grade.denominator_contribution,
                verification_status=token_store_persistence_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=token_store_persistence_grade.evidence_refs,
            ))
        if task.metadata.token_ttl_expiry_assertions:
            token_ttl_expiry_grade = grade_deterministically(
                _TOKEN_TTL_EXPIRY_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    token_ttl_expiry_observations=token_ttl_expiry_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_TTL_EXPIRY_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=token_ttl_expiry_grade.value,
                unit="proportion",
                eligible=token_ttl_expiry_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=token_ttl_expiry_grade.denominator_contribution,
                verification_status=token_ttl_expiry_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=token_ttl_expiry_grade.evidence_refs,
            ))
        if task.metadata.token_persistence_failure_assertions:
            token_persistence_failure_grade = grade_deterministically(
                _TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    token_persistence_failure_observations=token_persistence_failure_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=token_persistence_failure_grade.value,
                unit="proportion",
                eligible=token_persistence_failure_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=token_persistence_failure_grade.denominator_contribution,
                verification_status=token_persistence_failure_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=token_persistence_failure_grade.evidence_refs,
            ))
        if task.metadata.exfiltration_attempt_assertions:
            exfiltration_attempt_grade = grade_deterministically(
                _EXFILTRATION_ATTEMPT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    exfiltration_attempt_observations=exfiltration_attempt_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EXFILTRATION_ATTEMPT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=exfiltration_attempt_grade.value,
                unit="proportion",
                eligible=exfiltration_attempt_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=exfiltration_attempt_grade.denominator_contribution,
                verification_status=exfiltration_attempt_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=exfiltration_attempt_grade.evidence_refs,
            ))
        if task.metadata.artifact_leakage_assertions:
            artifact_leakage_grade = grade_deterministically(
                _ARTIFACT_LEAKAGE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    artifact_leakage_observations=artifact_leakage_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_ARTIFACT_LEAKAGE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=artifact_leakage_grade.value,
                unit="proportion",
                eligible=artifact_leakage_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=artifact_leakage_grade.denominator_contribution,
                verification_status=artifact_leakage_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=artifact_leakage_grade.evidence_refs,
            ))
        if task.metadata.replay_attempt_assertions:
            replay_attempt_grade = grade_deterministically(
                _REPLAY_ATTEMPT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    replay_attempt_observations=replay_attempt_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_REPLAY_ATTEMPT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=replay_attempt_grade.value,
                unit="proportion",
                eligible=replay_attempt_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=replay_attempt_grade.denominator_contribution,
                verification_status=replay_attempt_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=replay_attempt_grade.evidence_refs,
            ))
        if task.metadata.signed_field_tampering_assertions:
            signed_field_tampering_grade = grade_deterministically(
                _SIGNED_FIELD_TAMPERING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    signed_field_tampering_observations=signed_field_tampering_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_SIGNED_FIELD_TAMPERING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=signed_field_tampering_grade.value,
                unit="proportion",
                eligible=signed_field_tampering_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=signed_field_tampering_grade.denominator_contribution,
                verification_status=signed_field_tampering_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=signed_field_tampering_grade.evidence_refs,
            ))
        if task.metadata.payload_tampering_assertions:
            payload_tampering_grade = grade_deterministically(
                _PAYLOAD_TAMPERING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    payload_tampering_observations=payload_tampering_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_PAYLOAD_TAMPERING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=payload_tampering_grade.value,
                unit="proportion",
                eligible=payload_tampering_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=payload_tampering_grade.denominator_contribution,
                verification_status=payload_tampering_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=payload_tampering_grade.evidence_refs,
            ))
        if task.metadata.stale_state_root_assertions:
            stale_state_root_grade = grade_deterministically(
                _STALE_STATE_ROOT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    stale_state_root_observations=stale_state_root_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_STALE_STATE_ROOT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=stale_state_root_grade.value,
                unit="proportion",
                eligible=stale_state_root_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=stale_state_root_grade.denominator_contribution,
                verification_status=stale_state_root_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=stale_state_root_grade.evidence_refs,
            ))
        if task.metadata.identity_mismatch_assertions:
            identity_mismatch_grade = grade_deterministically(
                _IDENTITY_MISMATCH_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    identity_mismatch_observations=identity_mismatch_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_IDENTITY_MISMATCH_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=identity_mismatch_grade.value,
                unit="proportion",
                eligible=identity_mismatch_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=identity_mismatch_grade.denominator_contribution,
                verification_status=identity_mismatch_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=identity_mismatch_grade.evidence_refs,
            ))
        if task.metadata.nonce_expiration_assertions:
            nonce_expiration_grade = grade_deterministically(
                _NONCE_EXPIRATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    nonce_expiration_observations=nonce_expiration_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_NONCE_EXPIRATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=nonce_expiration_grade.value,
                unit="proportion",
                eligible=nonce_expiration_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=nonce_expiration_grade.denominator_contribution,
                verification_status=nonce_expiration_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=nonce_expiration_grade.evidence_refs,
            ))
        if task.metadata.signer_defect_assertions:
            signer_defect_grade = grade_deterministically(
                _SIGNER_DEFECT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    signer_defect_observations=signer_defect_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_SIGNER_DEFECT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=signer_defect_grade.value,
                unit="proportion",
                eligible=signer_defect_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=signer_defect_grade.denominator_contribution,
                verification_status=signer_defect_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=signer_defect_grade.evidence_refs,
            ))
        if task.metadata.l3_proof_transplant_assertions:
            l3_proof_transplant_grade = grade_deterministically(
                _L3_PROOF_TRANSPLANT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    l3_proof_transplant_observations=l3_proof_transplant_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_L3_PROOF_TRANSPLANT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=l3_proof_transplant_grade.value,
                unit="proportion",
                eligible=l3_proof_transplant_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=l3_proof_transplant_grade.denominator_contribution,
                verification_status=l3_proof_transplant_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=l3_proof_transplant_grade.evidence_refs,
            ))
        if task.metadata.revoked_credential_assertions:
            revoked_credential_grade = grade_deterministically(
                _REVOKED_CREDENTIAL_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    revoked_credential_observations=revoked_credential_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_REVOKED_CREDENTIAL_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=revoked_credential_grade.value,
                unit="proportion",
                eligible=revoked_credential_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=revoked_credential_grade.denominator_contribution,
                verification_status=revoked_credential_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=revoked_credential_grade.evidence_refs,
            ))
        if task.metadata.evidence_preservation_assertions:
            evidence_preservation_grade = grade_deterministically(
                _EVIDENCE_PRESERVATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    evidence_preservation_observations=evidence_preservation_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EVIDENCE_PRESERVATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=evidence_preservation_grade.value,
                unit="proportion",
                eligible=evidence_preservation_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=evidence_preservation_grade.denominator_contribution,
                verification_status=evidence_preservation_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=evidence_preservation_grade.evidence_refs,
            ))
        for metric in grade_metrics:
            DEFAULT_METRIC_REGISTRY.validate(metric)
        metric_records.extend(grade_metrics)
        attempt.grade_refs = [metric.metric_id for metric in grade_metrics]
        attempt_records.append(attempt)

        status_color = "green" if score.passed else "red"
        receipt_status = ""
        if response.binding == BindingType.RECEIPT_BOUND:
            if response.receipts_verified:
                receipt_status = f" [cyan]({len(response.receipts)} receipts verified)[/cyan]"
            else:
                receipt_status = f" [yellow]({len(response.receipts)} receipts not fully verified)[/yellow]"
        elif response.unbound_reason:
            receipt_status = f" [yellow]({response.unbound_reason})[/yellow]"

        event_count = response.chat_evidence.event_count if response.chat_evidence else 0

        console.print(
            f"  [dim]{task.id}[/dim] [{status_color}]{'PASS' if score.passed else 'FAIL'}[/{status_color}]"
            f" answer_chars={len(response.answer or '')}"
            f" agent_events={event_count}{receipt_status}"
        )

    # 9. Write attempts.jsonl
    with open(report_dir / "attempts.jsonl", "w") as f:
        for ar in attempt_records:
            f.write(ar.model_dump_json() + "\n")

    with open(report_dir / "receipts.jsonl", "w") as f:
        for receipt in receipt_records:
            f.write(receipt.model_dump_json() + "\n")

    with open(report_dir / "final-state-observations.jsonl", "w") as f:
        for observation in final_state_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "state-observations.jsonl", "w") as f:
        for observation in state_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "rehydration-observations.jsonl", "w") as f:
        for observation in rehydration_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "secret-detection-observations.jsonl", "w") as f:
        for observation in secret_detection_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "unauthorized-mutation-observations.jsonl", "w") as f:
        for observation in unauthorized_mutation_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "token-store-persistence-observations.jsonl", "w") as f:
        for observation in token_store_persistence_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "token-ttl-expiry-observations.jsonl", "w") as f:
        for observation in token_ttl_expiry_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "token-persistence-failure-observations.jsonl", "w") as f:
        for observation in token_persistence_failure_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "exfiltration-attempt-observations.jsonl", "w") as f:
        for observation in exfiltration_attempt_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "artifact-leakage-observations.jsonl", "w") as f:
        for observation in artifact_leakage_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "replay-attempt-observations.jsonl", "w") as f:
        for observation in replay_attempt_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "signed-field-tampering-observations.jsonl", "w") as f:
        for observation in signed_field_tampering_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "payload-tampering-observations.jsonl", "w") as f:
        for observation in payload_tampering_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "stale-state-root-observations.jsonl", "w") as f:
        for observation in stale_state_root_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "identity-mismatch-observations.jsonl", "w") as f:
        for observation in identity_mismatch_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "nonce-expiration-observations.jsonl", "w") as f:
        for observation in nonce_expiration_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "signer-defect-observations.jsonl", "w") as f:
        for observation in signer_defect_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "l3-proof-transplant-observations.jsonl", "w") as f:
        for observation in l3_proof_transplant_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "revoked-credential-observations.jsonl", "w") as f:
        for observation in revoked_credential_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "evidence-preservation-observations.jsonl", "w") as f:
        for observation in evidence_preservation_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "stages.jsonl", "w") as f:
        for stage in stage_records:
            f.write(stage.model_dump_json() + "\n")

    with open(report_dir / "metrics.jsonl", "w") as f:
        for metric in metric_records:
            f.write(metric.model_dump_json() + "\n")

    encrypted_evidence_artifacts = [
        encrypt_evidence_artifact(artifact, evidence_key)
        for artifact in evidence_artifacts
    ]
    with open(report_dir / "evidence-index.jsonl", "w") as f:
        for artifact in encrypted_evidence_artifacts:
            f.write(artifact.index.model_dump_json() + "\n")

    for artifact in encrypted_evidence_artifacts:
        artifact_path = report_dir / artifact.index.storage_location
        artifact_path.parent.mkdir(parents=True, exist_ok=True)
        artifact_path.write_text(artifact.envelope_json)

    # 10. Aggregate & Report
    agg = aggregate_results(suite, results)
    render_summary(agg, arm=config.arm)

    # 11. Save legacy artifacts (results.jsonl, summary.json)
    evidence_index_by_attempt = {
        artifact.index.attempt_id: artifact.index
        for artifact in encrypted_evidence_artifacts
        if artifact.index.attempt_id
    }

    receipt_records_by_attempt = {
        attempt.attempt_id: [
            receipt
            for receipt in receipt_records
            if receipt.attempt_id == attempt.attempt_id
        ]
        for attempt in attempt_records
    }

    def row_to_dict(r: RowResult, attempt: AttemptRecord):
        evidence_index = evidence_index_by_attempt.get(attempt.attempt_id)
        attempt_receipts = receipt_records_by_attempt[attempt.attempt_id]

        details_data = None
        if r.score.details:
            if isinstance(r.score.details, ScoreDetails):
                details_data = r.score.details.model_dump()
            else:
                details_data = r.score.details  # Already a dict

        prompt_bytes = r.task.prompt.encode()
        answer_bytes = (r.response.answer or "").encode()
        return {
            "task_id": r.task.id,
            "prompt_sha256": hashlib.sha256(prompt_bytes).hexdigest(),
            "prompt_length": len(prompt_bytes),
            "answer_sha256": hashlib.sha256(answer_bytes).hexdigest(),
            "answer_length": len(answer_bytes),
            "primary_transaction_id": r.response.primary_transaction_id,
            "chat_evidence_ref": evidence_index.artifact_id if evidence_index else None,
            "chat_evidence_sha256": evidence_index.sha256 if evidence_index else None,
            "receipts": [receipt.model_dump(mode="json") for receipt in attempt_receipts],
            "receipts_verified": r.response.receipts_verified,
            "passed": r.score.passed,
            "details": details_data,
            "timestamp": r.timestamp.isoformat()
        }

    with open(report_dir / "results.jsonl", "w") as f:
        for result, attempt in zip(results, attempt_records, strict=True):
            f.write(json.dumps(row_to_dict(result, attempt)) + "\n")

    with open(report_dir / "summary.json", "w") as f:
        f.write(json.dumps(agg.__dict__, indent=2))

    console.print(f"\n[bold green]Report saved to {report_dir}[/bold green]")

    policy_metrics_by_attempt = {
        metric.attempt_id: metric
        for metric in metric_records
        if metric.metric_id == _POLICY_OUTCOME_GRADER_ID
    }
    invalid_results = []
    for result, attempt in zip(results, attempt_records, strict=True):
        policy_metric = policy_metrics_by_attempt.get(attempt.attempt_id)
        verified_expected_block = (
            result.task.metadata.expected_allow_block_outcome == PolicyOutcome.BLOCK
            and attempt.terminal_status == TerminalStatus.GOVERNANCE_REJECTED
            and policy_metric is not None
            and policy_metric.value == 1.0
            and policy_metric.verification_status == VerificationStatus.VERIFIED
        )
        if (
            not result.response.chat_evidence
            or not result.response.chat_evidence.terminal_event
            or (
                not verified_expected_block
                and (
                    not result.response.answer
                    or result.response.chat_evidence.terminal_event.endswith(
                        ("failed", "stopped", "dead.lettered")
                    )
                    or "HTTP 401" in (result.response.unbound_reason or "")
                    or "HTTP 403" in (result.response.unbound_reason or "")
                )
            )
        ):
            invalid_results.append(result)
    if invalid_results:
        failed_ids = ", ".join(str(result.task.id) for result in invalid_results)
        raise EvaluationRunError(
            f"run produced invalid evidence for task(s) {failed_ids}; diagnostic report retained at {report_dir}"
        )

@main.command()
@click.argument("report_dir", type=click.Path(exists=True, path_type=Path))
@click.option("--pki-dir", type=click.Path(exists=True, path_type=Path))
def verify_receipts(report_dir, pki_dir):
    """Re-verify all receipts in a report directory offline.

    Loads every ``*Actuator_pub.pem`` file in the PKI directory, derives the
    key_id from each PEM, and matches each receipt to its ``signer_key_id``.
    Unified-stack runs produce receipts signed by two distinct actuators
    (gateway and operator), so the verifier needs both keys. A receipt whose
    ``signer_key_id`` has no matching key fails verification.
    """
    if not pki_dir:
        pki_dir = Path(os.environ.get("G8E_GATEWAY_PKI_DIR", ".g8e/pki"))

    keys: dict[str, str] = {}
    for pem_path in sorted(pki_dir.glob("*Actuator_pub.pem")):
        pem = pem_path.read_text()
        key_id = binascii.hexlify(decode_ed25519_public_key(pem)).decode()
        keys[key_id] = pem
        console.print(f"  loaded key {key_id[:16]}... from {pem_path.name}")

    if not keys:
        console.print(
            f"[bold red]Error:[/bold red] No actuator public keys "
            f"(*Actuator_pub.pem) found in {pki_dir}"
        )
        sys.exit(1)

    receipts_path = report_dir / "receipts.jsonl"
    if not receipts_path.exists():
        console.print(f"[bold red]Error:[/bold red] receipts.jsonl not found in {report_dir}")
        return

    console.print(f"[bold blue]Verifying receipts in {report_dir}...[/bold blue]")

    total = 0
    verified = 0
    failed = 0
    no_key = 0

    with open(receipts_path) as f:
        for line in f:
            total += 1
            try:
                observation = ReceiptObservation.model_validate_json(line)
            except ValueError:
                failed += 1
                console.print("  [red]FAILED:[/red] Could not parse typed receipt observation")
                continue
            receipt = observation.action_receipt
            public_key = keys.get(receipt.signer_key_id)
            if public_key is None:
                no_key += 1
                failed += 1
                console.print(
                    f"  [red]FAILED:[/red] No key for signer_key_id "
                    f"{receipt.signer_key_id[:16]}... "
                    f"(attempt {observation.attempt_id}, TX: {observation.transaction_id})"
                )
                continue
            if (
                verify_action_receipt_signature(receipt, public_key)
                and verify_receipt_persistence_attestation(receipt, public_key)
            ):
                verified += 1
            else:
                failed += 1
                console.print(
                    f"  [red]FAILED:[/red] Receipt for attempt {observation.attempt_id} "
                    f"(TX: {observation.transaction_id})"
                )

    if total == 0:
        console.print("[yellow]No bound receipts found in report.[/yellow]")
    else:
        status = "green" if failed == 0 else "red"
        console.print(f"\n[{status}]Re-verification complete:[/{status}]")
        console.print(f"  Total receipts: {total}")
        console.print(f"  Verified: {verified}")
        console.print(f"  Failed: {failed}")
        console.print(f"  Keys loaded: {len(keys)}")
        console.print(f"  No-key receipts: {no_key}")

        if failed > 0:
            sys.exit(1)

if __name__ == "__main__":
    main()
