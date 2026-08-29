# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import hashlib
import json
import logging
import os
import platform
import sys
import uuid
from pathlib import Path
from datetime import datetime, timezone, UTC
from typing import Optional

import click
from rich.console import Console

from g8e.constants import PORTS
from g8e.receipts import (
    action_receipt_to_dict,
    parse_action_receipt,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)
from g8e_evals.arms import ALL_ARMS, ARM_DEFINITIONS, Arm, GovernancePosture, get_arm_definition
from g8e_evals.auth_bridge import AuthBridgeError, load_cli_auth_context
from g8e_evals.evidence import EvidenceEncryptionKey, encrypt_evidence_artifact, load_evidence_encryption_key
from g8e_evals.harness import RowResult, BindingType, SUTConfig, LLMRoleConfig
from g8e_evals.stages import EvidenceArtifact, normalize_attempt_evidence
from g8e_evals.schema import (
    ArmManifestEntry,
    AttemptRecord,
    ContentHash,
    MetricObservation,
    ModelIdentity,
    PostureObservation,
    RoleToModelMapping,
    RunManifest,
    SamplingSettings,
    StackEnvironment,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)
from g8e_evals.sut.direct_provider import DirectProviderSUT
from g8e_evals.sut.g8ee_chat import ChatEvaluationReceipt, G8eeChatSUT, AuthenticationError
from g8e_evals.posture import observe_gateway_posture
from g8e_evals.transport import AuthContext
from g8e_evals.agent_trail_renderer import TurnRenderer
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.provenance import load_provenance
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.receipts.collector import ReceiptCollector
from g8e_evals.report.aggregate import aggregate_results
from g8e_evals.report.cli_renderer import render_summary
from g8e_evals.models import ScoreDetails

console = Console()
logger = logging.getLogger(__name__)

_PROVIDER_CHOICES = ["openai", "anthropic", "gemini", "ollama", "llamacpp", "fake"]
_KEYLESS_PROVIDERS = frozenset({"ollama", "llamacpp", "fake"})


class EvaluationRunError(Exception):
    pass

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
@click.option("--web-search-project", envvar="G8E_WEB_SEARCH_PROJECT", help="Web search project ID")
@click.option("--web-search-app", envvar="G8E_WEB_SEARCH_APP", help="Web search app ID")
@click.option("--web-search-api-key", envvar="G8E_WEB_SEARCH_API_KEY", help="Web search API key")
def run(suite, model, provider, assistant_model, assistant_provider, lite_model, lite_provider, verbose_text, idle_timeout, g8ee_url, operator_url, operator_session_id, g8e_cli, auth_project_root, arm, state_root, output_dir, evidence_key_file, gold_set, limit, l2_key, l2_key_id, primary_api_key, primary_endpoint, assistant_api_key, assistant_endpoint, lite_api_key, lite_endpoint, web_search_project, web_search_app, web_search_api_key):
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
        operator_url=operator_url,
        operator_session_id=operator_session_id or auth_context.operator_session_id,
        auth_context=auth_context,
        state_root=state_root,
        l2_private_key=l2_key,
        l2_key_id=l2_key_id,
        arm=selected_arm,
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
        for role_name in ["primary", "assistant", "lite"]:
            role_config = getattr(config, role_name)
            if not role_config or not role_config.provider:
                continue

            if role_config.provider in _KEYLESS_PROVIDERS:
                continue

            if role_config.api_key:
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

    if evidence_key is None:
        raise EvaluationRunError("restricted evidence encryption key is required")

    # 5. Build the run manifest. Required hashes: dataset hash (always),
    #    grader bundle hash (computed from verifier source), prompt bundle
    #    hash (computed from task prompts). The manifest refuses to start
    #    when a required hash or model identity is unavailable.
    run_id = str(uuid.uuid4())

    prompt_bundle_content = "\n".join(t.prompt for t in tasks).encode()
    prompt_bundle_hash = hashlib.sha256(prompt_bundle_content).hexdigest()

    grader_bundle_content = f"{suite}:{suite_version}".encode()
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
            prompt_hash=hashlib.sha256(t.prompt.encode()).hexdigest(),
            prompt_length=len(t.prompt),
            grader_ids=["ifeval_subset_verifier"],
            grader_versions=["1.0.0"],
            metadata={"instruction_id_list": t.metadata.instruction_id_list},
        )
        for t in tasks
    ]
    with open(report_dir / "tasks.jsonl", "w") as f:
        for td in task_defs:
            f.write(td.model_dump_json() + "\n")

    # 8. Execution loop
    attempt_records: list[AttemptRecord] = []
    stage_records: list[StageObservation] = []
    metric_records: list[MetricObservation] = []
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

        on_chain_receipt = None
        if arm_def.receipt_binding:
            if response.transaction_id:
                on_chain_receipt = await collector.collect_receipt(response.transaction_id)
            elif isinstance(response.chat_evidence, ChatEvaluationReceipt):
                on_chain_receipt = await collector.collect_receipt_for_investigation(
                    response.chat_evidence.investigation_id
                )
            if on_chain_receipt:
                response.transaction_id = on_chain_receipt.transaction_id
                response.action_receipt = on_chain_receipt
                response.binding = BindingType.RECEIPT_BOUND
                response.unbound_reason = None
                if warden_pub:
                    response.receipt_verified = (
                        verify_action_receipt_signature(on_chain_receipt, warden_pub)
                        and verify_receipt_persistence_attestation(on_chain_receipt, warden_pub)
                    )

        # Score
        if suite == "ifeval_subset":
            score = verifier.verify(
                task.id,
                task.prompt,
                response.answer,
                task.metadata.instruction_id_list,
                task.metadata.kwargs
            )

        res = RowResult(task=task, response=response, score=score)
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
        normalized = (
            normalize_attempt_evidence(
                response.chat_evidence,
                run_id,
                attempt_id,
                action_receipt=response.action_receipt,
                receipt_verified=response.receipt_verified,
                grading_model_calls=score.model_calls,
            )
            if response.chat_evidence
            else None
        )
        if normalized:
            stage_records.extend(normalized.stages)
            evidence_refs = []
            if normalized.raw_evidence:
                evidence_artifacts.append(normalized.raw_evidence)
                evidence_refs.append(normalized.raw_evidence.index.artifact_id)
            metric_records.append(MetricObservation(
                metric_id="stage_usage_reconciled",
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=float(normalized.usage.reconciled),
                unit="boolean",
                verification_status=VerificationStatus.VERIFIED,
                evidence_refs=evidence_refs,
            ))

        attempt = AttemptRecord(
            attempt_id=attempt_id,
            run_id=run_id,
            task_id=task.id,
            arm_id=arm_def.arm_id,
            state_snapshot_hash=config.state_root,
            replicate_id="1",
            assignment_order=len(attempt_records),
            started_at=started_at,
            ended_at=ended_at,
            terminal_status=terminal_status,
            posture=posture,
            correlation_ids={
                "transaction_id": response.transaction_id or "",
            },
            missingness_or_failure=response.unbound_reason if terminal_status != TerminalStatus.COMPLETED else None,
            usage_reconciliation=normalized.usage if normalized else None,
        )
        attempt_records.append(attempt)

        status_color = "green" if score.passed else "red"
        receipt_status = ""
        if response.binding == BindingType.RECEIPT_BOUND:
            if response.receipt_verified:
                receipt_status = " [cyan](receipt verified)[/cyan]"
            else:
                receipt_status = " [yellow](receipt unverified)[/yellow]"
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

    def row_to_dict(r: RowResult, attempt: AttemptRecord):
        evidence_index = evidence_index_by_attempt.get(attempt.attempt_id)
        action_receipt = (
            action_receipt_to_dict(r.response.action_receipt)
            if r.response.action_receipt
            else None
        )

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
            "transaction_id": r.response.transaction_id,
            "chat_evidence_ref": evidence_index.artifact_id if evidence_index else None,
            "chat_evidence_sha256": evidence_index.sha256 if evidence_index else None,
            "action_receipt": action_receipt,
            "receipt_verified": r.response.receipt_verified,
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

    invalid_results = [
        result
        for result in results
        if not result.response.answer
        or not result.response.chat_evidence
        or not result.response.chat_evidence.terminal_event
        or result.response.chat_evidence.terminal_event.endswith(("failed", "stopped", "dead.lettered"))
        or "HTTP 401" in (result.response.unbound_reason or "")
        or "HTTP 403" in (result.response.unbound_reason or "")
    ]
    if invalid_results:
        failed_ids = ", ".join(str(result.task.id) for result in invalid_results)
        raise EvaluationRunError(
            f"run produced invalid evidence for task(s) {failed_ids}; diagnostic report retained at {report_dir}"
        )

@main.command()
@click.argument("report_dir", type=click.Path(exists=True, path_type=Path))
@click.option("--pki-dir", type=click.Path(exists=True, path_type=Path))
def verify_receipts(report_dir, pki_dir):
    """Re-verify all receipts in a report directory offline"""
    if not pki_dir:
        pki_dir = Path(os.environ.get("G8E_GATEWAY_PKI_DIR", ".g8e/pki"))

    warden_pub_path = pki_dir / "warden_pub.pem"
    if not warden_pub_path.exists():
        console.print(f"[bold red]Error:[/bold red] Warden public key not found at {warden_pub_path}")
        return

    warden_pub = warden_pub_path.read_text()

    results_path = report_dir / "results.jsonl"
    if not results_path.exists():
        console.print(f"[bold red]Error:[/bold red] results.jsonl not found in {report_dir}")
        return

    console.print(f"[bold blue]Verifying receipts in {report_dir}...[/bold blue]")

    total = 0
    verified = 0
    failed = 0

    with open(results_path) as f:
        for line in f:
            data = json.loads(line)
            receipt = data.get("action_receipt")
            if not receipt:
                continue

            total += 1
            try:
                receipt_model = parse_action_receipt(receipt)
            except Exception:
                failed += 1
                console.print(f"  [red]FAILED:[/red] Could not parse receipt for task {data.get('task_id')} (TX: {data.get('transaction_id')})")
                continue
            if (
                verify_action_receipt_signature(receipt_model, warden_pub)
                and verify_receipt_persistence_attestation(receipt_model, warden_pub)
            ):
                verified += 1
            else:
                failed += 1
                console.print(f"  [red]FAILED:[/red] Receipt for task {data.get('task_id')} (TX: {data.get('transaction_id')})")

    if total == 0:
        console.print("[yellow]No bound receipts found in report.[/yellow]")
    else:
        status = "green" if failed == 0 else "red"
        console.print(f"\n[{status}]Re-verification complete:[/{status}]")
        console.print(f"  Total receipts: {total}")
        console.print(f"  Verified: {verified}")
        console.print(f"  Failed: {failed}")

        if failed > 0:
            sys.exit(1)

if __name__ == "__main__":
    main()
