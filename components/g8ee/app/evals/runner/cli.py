# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""CLI entrypoint for evals runner.

Invoked as ``python -m app.evals.runner.cli`` from the g8ee component
root (``components/g8ee``) so that ``app.*`` resolves through the standard
package hierarchy without any runtime ``sys.path`` mutation.
"""

import argparse
import asyncio
import json
import sys
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from app.constants import LLMProvider
from app.llm.factory import get_llm_provider
from app.models.settings import LLMSettings
from app.services.ai.eval_judge import EvalJudge

from .client import G8edClient
from .fleet import FleetManager
from .metrics import EvalRow, FullReport
from .reporter import compute_summaries, persist_report, render_text_table
from .scorer import (
    score_accuracy_scenario_llm,
    score_benchmark_scenario,
    score_privacy_scenario,
)

# components/g8ee/ — anchors for sibling resources (evals fleet, reports/).
_G8EE_ROOT = Path(__file__).resolve().parent.parent.parent.parent
_COMPOSE_FILE = _G8EE_ROOT / "evals" / "docker-compose.evals.yml"
_REPORTS_DIR = _G8EE_ROOT / "reports" / "evals"
_GOLD_SETS_DIR = _G8EE_ROOT / "evals" / "gold_sets"


def get_available_gold_sets() -> dict[str, Path]:
    """Auto-discover available gold sets from the gold_sets directory.
    
    Returns:
        Dict mapping short names (e.g., 'benchmark') to full paths.
    """
    gold_sets = {}
    if _GOLD_SETS_DIR.exists():
        for json_file in _GOLD_SETS_DIR.glob("*.json"):
            name = json_file.stem  # e.g., 'benchmark' from 'benchmark.json'
            gold_sets[name] = json_file
    return gold_sets


def resolve_gold_set_path(gold_set_arg: str | None) -> Path | None:
    """Resolve a gold set argument to a full path.
    
    Args:
        gold_set_arg: Either a short name ('benchmark') or full path.
        
    Returns:
        Path to the gold set file, or None if not found.
    """
    if not gold_set_arg:
        return None
    
    path = Path(gold_set_arg)
    if path.exists():
        return path.resolve()
    
    # Try as a short name
    gold_sets = get_available_gold_sets()
    if gold_set_arg in gold_sets:
        return gold_sets[gold_set_arg]
    
    return None


async def run_dry_run(device_token: str, g8ed_url: str = "https://g8e.local") -> None:
    """Dry run: bring up fleet, run one canned chat, print response, tear down.

    Args:
        device_token: Device link token for operator authentication
        g8ed_url: g8ed API base URL
    """
    fleet = FleetManager(_COMPOSE_FILE)

    try:
        print("[evals] Bringing up fleet...")
        fleet.up(nodes=3, device_token=device_token)

        print("[evals] Waiting for operators to bind...")
        await fleet.wait_bound(timeout=60)

        print("[evals] Fleet is up. Running canned chat: 'uname -a'")

        async with G8edClient(g8ed_url) as client:
            investigation = await client.create_investigation(
                operator_session_id=device_token
            )
            print(f"[evals] Created investigation: {investigation['id']}")

            response_text = ""
            async for event in client.send_chat_message(
                investigation_id=investigation["id"],
                message="run `uname -a`",
                operator_session_id=device_token,
            ):
                if event.get("type") == "text_chunk":
                    response_text += event.get("data", "")

        print(f"[evals] Response: {response_text}")

    finally:
        print("[evals] Tearing down fleet...")
        fleet.down()
        print("[evals] Done.")


async def run_scenario(
    scenario: dict[str, Any],
    device_token: str,
    g8ed_url: str,
    fleet: FleetManager,
    node_id: str,
    judge: EvalJudge,
) -> EvalRow:
    """Run a single scenario against an eval node.

    Args:
        scenario: Gold set scenario dict
        device_token: Device link token for operator authentication
        g8ed_url: g8ed API base URL
        fleet: FleetManager instance
        node_id: Container name to use
        judge: EvalJudge instance for accuracy scoring

    Returns:
        EvalRow with results
    """
    start_time = datetime.now(UTC)
    scenario_id = scenario["id"]
    user_query = scenario["user_query"]
    dimension = scenario.get("dimension", "accuracy")

    try:
        async with G8edClient(g8ed_url) as client:
            investigation = await client.create_investigation(
                operator_session_id=device_token
            )

            response_text = ""
            tool_calls = []
            approval_events = []

            async for event in client.send_chat_message(
                investigation_id=investigation["id"],
                message=user_query,
                operator_session_id=device_token,
            ):
                if event.get("type") == "text_chunk":
                    response_text += event.get("data", "")
                elif event.get("type") == "tool_call":
                    # Tool calls from g8ed might have slightly different shape than SDK
                    # Extract data from the event
                    tool_data = event.get("data", {})
                    tool_calls.append(tool_data)
                elif event.get("type") == "approval_required":
                    approval_events.append(event)
                    approval_id = event.get("approval_id")
                    if approval_id:
                        await client.approve_request(approval_id, device_token)

        latency_ms = (datetime.now(UTC) - start_time).total_seconds() * 1000

        # Select scoring path based on scenario content
        if scenario.get("expected_payload"):
            passed, failures = score_benchmark_scenario(scenario, tool_calls)
            return EvalRow(
                dimension=dimension,
                suite="evals_runner",
                scenario_id=scenario_id,
                category=scenario.get("category", ""),
                passed=passed,
                score=None,
                latency_ms=latency_ms,
                error="; ".join(failures) if failures else None,
                details={"tool_calls": tool_calls, "failures": failures},
            )
        
        if scenario.get("secret"):
            passed, details = score_privacy_scenario(scenario, response_text, []) # egress_events empty for now
            return EvalRow(
                dimension="privacy",
                suite="evals_runner",
                scenario_id=scenario_id,
                category=scenario.get("category", ""),
                passed=passed,
                score=None,
                latency_ms=latency_ms,
                details=details,
            )

        if scenario.get("expected_behavior") or scenario.get("required_concepts"):
            passed, score, reasoning = await score_accuracy_scenario_llm(
                scenario, response_text, tool_calls, judge
            )
            return EvalRow(
                dimension="accuracy",
                suite="evals_runner",
                scenario_id=scenario_id,
                category=scenario.get("category", ""),
                passed=passed,
                score=score,
                latency_ms=latency_ms,
                error=reasoning if not passed else None,
                details={"reasoning": reasoning},
            )

        return EvalRow(
            dimension=dimension,
            suite="evals_runner",
            scenario_id=scenario_id,
            category=scenario.get("category", ""),
            passed=True,
            score=None,
            latency_ms=latency_ms,
            details={"response_text": response_text[:500]},
        )

    except Exception as e:
        latency_ms = (datetime.now(UTC) - start_time).total_seconds() * 1000
        return EvalRow(
            dimension=scenario.get("dimension", "accuracy"),
            suite="evals_runner",
            scenario_id=scenario_id,
            category=scenario.get("category", ""),
            passed=False,
            score=None,
            latency_ms=latency_ms,
            error=str(e),
            details={},
        )


async def run_full_eval(
    device_token: str | None,
    gold_set_path: str,
    g8ed_url: str = "https://g8e.local",
    nodes: int = 3,
    parallel: int = 1,
    judge_model: str = "gemini-2.5-pro",
    llm_provider: str | None = None,
    llm_primary_model: str | None = None,
    llm_assistant_model: str | None = None,
    llm_lite_model: str | None = None,
    llm_endpoint: str | None = None,
    llm_api_key: str | None = None,
) -> None:
    """Run full eval suite against gold set.

    Args:
        device_token: Device link token for operator authentication (optional if fleet is running)
        gold_set_path: Path to gold set JSON file
        g8ed_url: g8ed API base URL
        nodes: Number of eval nodes to use
        parallel: Number of scenarios to run in parallel
        judge_model: Eval judge model name
        llm_primary_model: Primary LLM model
        llm_assistant_model: Assistant LLM model
        llm_lite_model: Lite LLM model
        llm_endpoint: LLM provider endpoint URL
        llm_api_key: LLM provider API key
    """
    fleet = FleetManager(_COMPOSE_FILE)

    if device_token is None:
        if fleet.is_running():
            device_token = fleet.get_device_token()
            if device_token:
                print("[evals] Using device token from running fleet")
            else:
                print("[evals] Error: Fleet is running but device token not found")
                print("[evals] Run './g8e evals down' then './g8e evals up --device-token <token>'")
                sys.exit(1)
        else:
            print("[evals] Error: Fleet is not running and no device token provided")
            print("[evals] Run './g8e evals up --device-token <token>' first")
            sys.exit(1)

    fleet_was_running = fleet.is_running()

    with open(gold_set_path) as f:
        scenarios = json.load(f)

    operator_bound_scenarios = [s for s in scenarios if s.get("agent_mode") == "OPERATOR_BOUND"]
    print(f"[evals] Found {len(operator_bound_scenarios)} OPERATOR_BOUND scenarios")

    # Configure LLM settings for the judge
    settings = LLMSettings()
    if llm_provider:
        settings.primary_provider = LLMProvider(llm_provider)
        settings.assistant_provider = LLMProvider(llm_provider)
        settings.lite_provider = LLMProvider(llm_provider)
    
    if llm_primary_model:
        settings.primary_model = llm_primary_model
    if llm_assistant_model:
        settings.assistant_model = llm_assistant_model
    if llm_lite_model:
        settings.lite_model = llm_lite_model
    
    if settings.primary_provider == LLMProvider.GEMINI:
        settings.gemini_api_key = llm_api_key
    elif settings.primary_provider == LLMProvider.OPENAI:
        settings.openai_api_key = llm_api_key
        if llm_endpoint:
            settings.openai_endpoint = llm_endpoint
    elif settings.primary_provider == LLMProvider.ANTHROPIC:
        settings.anthropic_api_key = llm_api_key
        if llm_endpoint:
            settings.anthropic_endpoint = llm_endpoint
    elif settings.primary_provider == LLMProvider.OLLAMA:
        settings.ollama_api_key = llm_api_key
        if llm_endpoint:
            settings.ollama_endpoint = llm_endpoint
    elif settings.primary_provider == LLMProvider.LLAMACPP:
        settings.llamacpp_api_key = llm_api_key
        if llm_endpoint:
            settings.llamacpp_endpoint = llm_endpoint

    provider = get_llm_provider(settings)
    judge = EvalJudge(provider=provider, model=judge_model)

    try:
        if not fleet_was_running:
            print("[evals] Bringing up fleet...")
            fleet.up(nodes=nodes, device_token=device_token)

            print("[evals] Waiting for operators to bind...")
            await fleet.wait_bound(timeout=60)
        else:
            print("[evals] Using existing running fleet")

        print(f"[evals] Running {len(operator_bound_scenarios)} scenarios...")
        rows = []

        for i, scenario in enumerate(operator_bound_scenarios):
            node_id = f"evals-eval-node-{(i % nodes) + 1}"
            print(f"[evals] [{i+1}/{len(operator_bound_scenarios)}] Running {scenario['id']} on {node_id}")

            row = await run_scenario(scenario, device_token, g8ed_url, fleet, node_id, judge)
            rows.append(row)

            if row.passed:
                print(f"[evals]   PASSED ({row.latency_ms:.0f}ms)")
            else:
                print(f"[evals]   FAILED: {row.error}")

            fleet.restart(node_id)

        report = FullReport(
            rows=rows,
            summaries=compute_summaries(rows),
            finished_at=datetime.now(UTC),
        )

        print("\n" + "=" * 60)
        print("EVALS REPORT")
        print("=" * 60)
        print(render_text_table(report))

        artifacts = persist_report(report, _REPORTS_DIR)
        print(f"\nArtifacts persisted to: {artifacts['run_dir']}")

    finally:
        if not fleet_was_running:
            print("[evals] Tearing down fleet...")
            fleet.down()
        print("[evals] Done.")


def main() -> None:
    """CLI entrypoint."""
    parser = argparse.ArgumentParser(description="g8e evals runner")
    subparsers = parser.add_subparsers(dest="command", required=True)

    # 'run' subcommand
    run_parser = subparsers.add_parser("run", help="Run evals against a gold set")
    run_parser.add_argument(
        "--device-token",
        required=False,
        help="Device link token for operator authentication (optional if fleet is running)",
    )
    run_parser.add_argument(
        "--g8ed-url",
        default="https://g8e.local",
        help="g8ed API base URL (default: https://g8e.local)",
    )
    run_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Run one canned chat and tear down (for testing)",
    )
    run_parser.add_argument(
        "--gold-set",
        default="benchmark",
        help="Gold set name or path (e.g., 'benchmark', 'accuracy', 'privacy', or full path). Default: 'benchmark'",
    )
    run_parser.add_argument(
        "--nodes",
        type=int,
        default=3,
        help="Number of eval nodes to use (default: 3)",
    )
    run_parser.add_argument(
        "--parallel",
        type=int,
        default=1,
        help="Number of scenarios to run in parallel (default: 1)",
    )
    run_parser.add_argument(
        "-p", "--llm-provider",
    )
    run_parser.add_argument(
        "-m", "--primary-model",
        help="Primary LLM model",
    )
    run_parser.add_argument(
        "-a", "--assistant-model",
        help="Assistant LLM model",
    )
    run_parser.add_argument(
        "-l", "--lite-model",
        help="Lite LLM model",
    )
    run_parser.add_argument(
        "-e", "--llm-endpoint-url",
        help="LLM provider endpoint URL",
    )
    run_parser.add_argument(
        "-k", "--llm-api-key",
        help="LLM provider API key",
    )
    run_parser.add_argument(
        "--judge-model",
        default="gemini-2.5-pro",
        help="Eval judge model name (default: gemini-2.5-pro)",
    )

    # 'list' subcommand
    list_parser = subparsers.add_parser("list", help="List available gold sets")

    args = parser.parse_args()

    if args.command == "list":
        gold_sets = get_available_gold_sets()
        print("[evals] Available gold sets:")
        for name, path in sorted(gold_sets.items()):
            print(f"  {name}: {path}")
        sys.exit(0)

    if args.command == "run":
        if args.dry_run:
            if not args.device_token:
                print("[evals] Error: --device-token is required for --dry-run")
                sys.exit(1)
            asyncio.run(run_dry_run(args.device_token, args.g8ed_url))
        else:
            gold_set_path = resolve_gold_set_path(args.gold_set)
            if not gold_set_path:
                print(f"[evals] Gold set not found: {args.gold_set}")
                print("[evals] Available gold sets:")
                for name, path in sorted(get_available_gold_sets().items()):
                    print(f"  {name}: {path}")
                sys.exit(1)
            asyncio.run(run_full_eval(
                args.device_token,
                str(gold_set_path),
                args.g8ed_url,
                args.nodes,
                args.parallel,
                args.judge_model,
                args.llm_provider,
                args.primary_model,
                args.assistant_model,
                args.lite_model,
                args.llm_endpoint_url,
                args.llm_api_key,
            ))


if __name__ == "__main__":
    main()
