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

from app.llm.factory import get_llm_provider
from app.models.settings import LLMSettings
from app.services.ai.eval_judge import EvalJudge

from .client import G8edClient
from .fleet import FleetManager
from .metrics import EvalRow, FullReport
from .reporter import compute_summaries, persist_report, render_text_table
from .scorer import score_benchmark_scenario

# components/g8ee/ — anchors for sibling resources (evals fleet, reports/).
_G8EE_ROOT = Path(__file__).resolve().parent.parent.parent.parent
_COMPOSE_FILE = _G8EE_ROOT / "evals" / "docker-compose.evals.yml"
_REPORTS_DIR = _G8EE_ROOT / "reports" / "evals"


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
        await fleet.wait_bound(timeout=60, device_token=device_token)

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
) -> EvalRow:
    """Run a single scenario against an eval node.

    Args:
        scenario: Gold set scenario dict
        device_token: Device link token for operator authentication
        g8ed_url: g8ed API base URL
        fleet: FleetManager instance
        node_id: Container name to use

    Returns:
        EvalRow with results
    """
    start_time = datetime.now(UTC)
    scenario_id = scenario["id"]
    user_query = scenario["user_query"]

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
                    tool_calls.append(event)
                elif event.get("type") == "approval_required":
                    approval_events.append(event)
                    approval_id = event.get("approval_id")
                    if approval_id:
                        await client.approve_request(approval_id, device_token)

        latency_ms = (datetime.now(UTC) - start_time).total_seconds() * 1000

        if scenario.get("expected_payload"):
            passed, failures = score_benchmark_scenario(scenario, tool_calls)
            return EvalRow(
                dimension=scenario.get("dimension", "accuracy"),
                suite="evals_runner",
                scenario_id=scenario_id,
                category=scenario.get("category", ""),
                passed=passed,
                score=None,
                latency_ms=latency_ms,
                error="; ".join(failures) if failures else None,
                details={"tool_calls": tool_calls, "failures": failures},
            )
        return EvalRow(
            dimension=scenario.get("dimension", "accuracy"),
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
    device_token: str,
    gold_set_path: str,
    g8ed_url: str = "https://g8e.local",
    nodes: int = 3,
    parallel: int = 1,
    model: str = "gemini-2.5-pro",
) -> None:
    """Run full eval suite against gold set.

    Args:
        device_token: Device link token for operator authentication
        gold_set_path: Path to gold set JSON file
        g8ed_url: g8ed API base URL
        nodes: Number of eval nodes to use
        parallel: Number of scenarios to run in parallel
        model: Eval judge model name
    """
    fleet = FleetManager(_COMPOSE_FILE)

    with open(gold_set_path) as f:
        scenarios = json.load(f)

    operator_bound_scenarios = [s for s in scenarios if s.get("agent_mode") == "OPERATOR_BOUND"]
    print(f"[evals] Found {len(operator_bound_scenarios)} OPERATOR_BOUND scenarios")

    settings = LLMSettings()
    provider = get_llm_provider(settings)
    judge = EvalJudge(provider=provider, model=model)

    try:
        print("[evals] Bringing up fleet...")
        fleet.up(nodes=nodes, device_token=device_token)

        print("[evals] Waiting for operators to bind...")
        await fleet.wait_bound(timeout=60, device_token=device_token)

        print(f"[evals] Running {len(operator_bound_scenarios)} scenarios...")
        rows = []

        for i, scenario in enumerate(operator_bound_scenarios):
            node_id = f"eval-node-{(i % nodes) + 1:02d}"
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
        print("[evals] Tearing down fleet...")
        fleet.down()
        print("[evals] Done.")


def main() -> None:
    """CLI entrypoint."""
    parser = argparse.ArgumentParser(description="g8e evals runner")
    parser.add_argument(
        "--device-token",
        required=True,
        help="Device link token for operator authentication",
    )
    parser.add_argument(
        "--g8ed-url",
        default="https://g8e.local",
        help="g8ed API base URL (default: https://g8e.local)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Run one canned chat and tear down (for testing)",
    )
    parser.add_argument(
        "--gold-set",
        help="Path to gold set JSON file (e.g., gold_sets/benchmark.json)",
    )
    parser.add_argument(
        "--nodes",
        type=int,
        default=3,
        help="Number of eval nodes to use (default: 3)",
    )
    parser.add_argument(
        "--parallel",
        type=int,
        default=1,
        help="Number of scenarios to run in parallel (default: 1)",
    )

    args = parser.parse_args()

    if args.dry_run:
        asyncio.run(run_dry_run(args.device_token, args.g8ed_url))
    elif args.gold_set:
        gold_set_path = Path(args.gold_set)
        if not gold_set_path.exists():
            print(f"[evals] Gold set file not found: {args.gold_set}")
            sys.exit(1)
        asyncio.run(run_full_eval(
            args.device_token,
            str(gold_set_path),
            args.g8ed_url,
            args.nodes,
            args.parallel,
        ))
    else:
        print("[evals] Must specify --dry-run or --gold-set")
        sys.exit(1)


if __name__ == "__main__":
    main()
