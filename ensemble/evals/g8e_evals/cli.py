# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import json
import logging
import os
import sys
from pathlib import Path
from datetime import datetime
from typing import Optional

import click
from rich.console import Console

from g8e.constants import PORTS
from g8e.receipts import (
    action_receipt_to_dict,
    parse_action_receipt,
    verify_action_receipt_signature,
)
from g8e_evals.auth_bridge import AuthBridgeError, load_cli_auth_context
from g8e_evals.harness import RowResult, BindingType, SUTConfig, LLMRoleConfig
from g8e_evals.sut.g8ee_chat import G8eeChatSUT, AuthenticationError
from g8e_evals.agent_trail_renderer import TurnRenderer
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.receipts.collector import ReceiptCollector
from g8e_evals.report.aggregate import aggregate_results
from g8e_evals.report.cli_renderer import render_summary
from g8e_evals.models import ScoreDetails

console = Console()
logger = logging.getLogger(__name__)

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
@click.option("--provider", type=click.Choice(["openai", "anthropic", "gemini", "ollama", "llamacpp"]), envvar="G8E_TEST_LLM_PRIMARY_PROVIDER", help="Primary LLM provider")
@click.option("--model", envvar="G8E_TEST_LLM_PRIMARY_MODEL", help="Primary model name (e.g., gpt-4o)")
@click.option("--assistant-provider", type=click.Choice(["openai", "anthropic", "gemini", "ollama", "llamacpp"]), envvar="G8E_TEST_LLM_ASSISTANT_PROVIDER", help="Assistant LLM provider")
@click.option("--assistant-model", envvar="G8E_TEST_LLM_ASSISTANT_MODEL", help="Assistant model name")
@click.option("--lite-provider", type=click.Choice(["openai", "anthropic", "gemini", "ollama", "llamacpp"]), envvar="G8E_TEST_LLM_LITE_PROVIDER", help="Lite LLM provider")
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
@click.option("--mode", type=click.Choice(["receipt", "baseline"]), default="receipt",
              help="Receipt mode (default) verifies on-Gateway receipts; baseline mode runs without binding")
@click.option("--state-root", default="test-state-root-v1")
@click.option("--output-dir", type=click.Path(path_type=Path), default=Path("reports"))
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
def run(suite, model, provider, assistant_model, assistant_provider, lite_model, lite_provider, verbose_text, idle_timeout, g8ee_url, operator_url, operator_session_id, g8e_cli, auth_project_root, mode, state_root, output_dir, gold_set, limit, l2_key, l2_key_id, primary_api_key, primary_endpoint, assistant_api_key, assistant_endpoint, lite_api_key, lite_endpoint, web_search_project, web_search_app, web_search_api_key):
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
        mode=mode
    )

    asyncio.run(_run_suite(suite, config, gold_set, output_dir, limit, verbose_text=verbose_text, idle_timeout=idle_timeout))

async def _run_suite(suite: str, config: SUTConfig, gold_set: Path | None, output_dir: Path, limit: int | None = None, verbose_text: bool = False, idle_timeout: float = 180.0):
    # 1. Load benchmark
    if suite == "ifeval_subset":
        if not gold_set:
            gold_set = Path("gold_sets/ifeval_subset/input_data.jsonl")
        loader = IFEvalLoader(gold_set)
        tasks = list(loader.load())
        if limit:
            tasks = tasks[:limit]
        verifier = IFEvalVerifier()

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

    # 3. Initialize SUT (real g8ee chat pipeline by default).
    # The renderer is task-scoped, so the SUT-level callback delegates
    # to whichever TurnRenderer is active for the current task.
    current_renderer: dict[str, TurnRenderer | None] = {"r": None}

    async def _on_event(event_type: str, payload: dict) -> None:
        r = current_renderer["r"]
        if r is not None:
            r.render(event_type, payload)

    try:
        sut = G8eeChatSUT(
            config,
            on_event=_on_event,
            idle_timeout_s=idle_timeout,
        )
        # 3. Pre-flight validation: ensure we have API keys for active providers.
        remote_settings = await sut.check_settings()
    except AuthenticationError as e:
        console.print("[bold red]Authentication Error:[/bold red]")
        console.print(f"  {e}")
        console.print("\n[yellow]Run ./g8e auth enroll user or ./g8e auth refresh.[/yellow]")
        return

    llm_settings = remote_settings.llm if remote_settings else None

    errors = []
    for role_name in ["primary", "assistant", "lite"]:
        role_config = getattr(config, role_name)
        if not role_config or not role_config.provider:
            continue

        # Skip API key validation for providers that don't require authentication
        if role_config.provider in ("ollama", "llamacpp"):
            continue

        # Key provided via CLI flag?
        if role_config.api_key:
            continue

        if not llm_settings:
            errors.append(f"Missing API key for {role_name} provider '{role_config.provider}' (could not fetch remote settings)")
            continue

        # Key exists in remote settings for this provider?
        provider_key_map = {
            "openai": "openai_api_key",
            "anthropic": "anthropic_api_key",
            "gemini": "gemini_api_key",
        }
        remote_key_field = provider_key_map.get(role_config.provider)
        if remote_key_field and getattr(llm_settings, remote_key_field, None):
            continue

        # Role-specific override in remote settings?
        if getattr(llm_settings, f"{role_name}_api_key", None):
            continue

        errors.append(f"Missing API key for {role_name} provider '{role_config.provider}'")

    if errors:
        console.print("[bold red]Pre-flight validation failed:[/bold red]")
        for err in errors:
            console.print(f"  - {err}")
        console.print("\n[yellow]Provide keys via --primary-api-key, etc. or configure them in g8ee settings.[/yellow]")
        return

    # Validate that at least one LLM model is configured (either in g8ee settings or via CLI flags)
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
        return

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
            return

    collector = ReceiptCollector(config.operator_url, cli_context=config.auth_context)

    # Load warden pub key for verification
    warden_pub_path = Path(os.environ.get("G8E_GATEWAY_PKI_DIR", ".g8e/pki")) / "warden_pub.pem"
    warden_pub = ""
    if warden_pub_path.exists():
        warden_pub = warden_pub_path.read_text()

    results = []

    display_model = f"{config.primary.provider}:{config.primary.model}" if config.primary.provider and config.primary.model else (config.primary.model or "openai:gpt-4")
    console.print(f"[bold blue]Running {suite} with {display_model}...[/bold blue]")

    # 3. Execution loop
    for task in tasks:
        # Create a descriptive summary for the task
        intent = ""
        if suite == "ifeval_subset" and task.metadata.instruction_id_list:
            # Extract short names from instruction IDs (e.g., 'length:min_words' -> 'min_words')
            constraints = [instruction_id.split(":")[-1] for instruction_id in task.metadata.instruction_id_list]
            intent = f" [dim][{', '.join(constraints)}][/dim]"

        prompt_preview = task.prompt.replace("\n", " ")[:50]
        if len(task.prompt) > 50:
            prompt_preview += "..."

        console.print(f"  [cyan]{task.id:>4}[/cyan]: {prompt_preview}{intent}")

        # Per-task live renderer captures every agent stage.
        renderer = TurnRenderer(console, task_id=str(task.id), verbose_text=verbose_text)
        current_renderer["r"] = renderer

        # Get answer (drives the full g8ee chat pipeline end-to-end).
        response = await sut.get_answer(task)
        current_renderer["r"] = None

        terminal_event = response.chat_evidence.terminal_event if response.chat_evidence else None

        renderer.finish(
            terminal_event=terminal_event,
            answer_chars=len(response.answer or ""),
        )

        if response.binding == BindingType.RECEIPT_BOUND and response.transaction_id:
            on_chain_receipt = await collector.collect_receipt(response.transaction_id)
            if on_chain_receipt:
                response.action_receipt = on_chain_receipt
                if warden_pub:
                    response.receipt_verified = verify_action_receipt_signature(on_chain_receipt, warden_pub)

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

    # 4. Aggregate & Report
    agg = aggregate_results(suite, results)
    render_summary(agg, mode=config.mode)

    # 5. Save artifacts
    ts = datetime.now().strftime("%Y%m%d-%H%M%S")
    report_dir = output_dir / f"{suite}-{ts}"
    report_dir.mkdir(parents=True, exist_ok=True)

    def row_to_dict(r: RowResult):
        chat_evidence = r.response.chat_evidence.model_dump() if r.response.chat_evidence else None
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

        return {
            "task_id": r.task.id,
            "prompt": r.task.prompt,
            "answer": r.response.answer,
            "transaction_id": r.response.transaction_id,
            "chat_evidence": chat_evidence,
            "action_receipt": action_receipt,
            "receipt_verified": r.response.receipt_verified,
            "passed": r.score.passed,
            "details": details_data,
            "timestamp": r.timestamp.isoformat()
        }

    with open(report_dir / "results.jsonl", "w") as f:
        for r in results:
            f.write(json.dumps(row_to_dict(r)) + "\n")

    with open(report_dir / "summary.json", "w") as f:
        f.write(json.dumps(agg.__dict__, indent=2))

    console.print(f"\n[bold green]Report saved to {report_dir}[/bold green]")

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
            if verify_action_receipt_signature(receipt_model, warden_pub):
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
