# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Shared utilities for AI accuracy evaluation test suites.

Provides ``AccuracyTestResult`` and ``load_and_validate_gold_set`` used by
the agent, Gemini, and Ollama accuracy tests.
"""

import json
import logging
import os
from dataclasses import dataclass
from typing import Any

from app.constants import AgentMode, OperatorStatus
from app.models.http_context import BoundOperator

logger = logging.getLogger(__name__)

REQUIRED_SCENARIO_FIELDS = {"id", "user_query", "agent_mode", "expected_behavior", "required_concepts"}
REQUIRED_BENCHMARK_FIELDS = {"id", "user_query", "agent_mode", "expected_tool", "expected_payload"}


@dataclass
class AccuracyTestResult:
    """Structured result for a single accuracy test scenario."""
    scenario_id: str
    passed: bool = False
    score: int = 0
    reasoning: str = ""
    response_text: str = ""
    execution_time_ms: float = 0.0
    error: str | None = None
    dimension: str = "accuracy"

    def to_dict(self) -> dict[str, Any]:
        return {
            "scenario_id": self.scenario_id,
            "passed": self.passed,
            "score": self.score,
            "reasoning": self.reasoning,
            "response_text": self.response_text[:500],
            "execution_time_ms": self.execution_time_ms,
            "error": self.error,
            "dimension": self.dimension,
        }


@dataclass
class BenchmarkTestResult:
    """Structured result for a single benchmark scenario (binary pass/fail)."""
    scenario_id: str
    category: str = ""
    passed: bool = False
    tool_called: bool = False
    matchers_total: int = 0
    matchers_passed: int = 0
    failures: list[str] | None = None
    execution_time_ms: float = 0.0
    error: str | None = None
    tribunal_final_command: str | None = None
    tribunal_outcome: str | None = None
    agent_continue_approvals: int = 0
    approvals_by_type: dict[str, int] | None = None
    dimension: str = "accuracy"

    def to_dict(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "scenario_id": self.scenario_id,
            "category": self.category,
            "passed": self.passed,
            "tool_called": self.tool_called,
            "matchers_total": self.matchers_total,
            "matchers_passed": self.matchers_passed,
            "failures": self.failures or [],
            "execution_time_ms": self.execution_time_ms,
            "error": self.error,
            "agent_continue_approvals": self.agent_continue_approvals,
            "approvals_by_type": self.approvals_by_type or {},
            "dimension": self.dimension,
        }
        if self.tribunal_outcome is not None:
            result["tribunal_final_command"] = self.tribunal_final_command
            result["tribunal_outcome"] = self.tribunal_outcome
        return result


def load_and_validate_gold_set(
    gold_set_path: str,
    *,
    filter_operator_bound: bool = True,
    filter_expected_tools: bool = True,
) -> list[dict[str, Any]]:
    """Load, validate, and filter the gold set for parameterization.

    Args:
        gold_set_path: Absolute path to gold_set.json.
        filter_operator_bound: Drop ``operator_bound`` scenarios (requires a
            real operator process the test environment does not have).
        filter_expected_tools: Drop scenarios with ``expected_tools`` (model
            tool usage is non-deterministic in raw-model tests).

    Returns:
        List of valid, filtered scenario dicts.
    """
    if not os.path.exists(gold_set_path):
        logger.error("Gold set file not found: %s", gold_set_path)
        return []

    try:
        with open(gold_set_path, "r", encoding="utf-8") as f:
            scenarios = json.load(f)
    except json.JSONDecodeError as exc:
        logger.error("Failed to parse gold_set.json: %s", exc)
        return []

    valid: list[dict[str, Any]] = []
    for scenario in scenarios:
        missing = REQUIRED_SCENARIO_FIELDS - set(scenario.keys())
        if missing:
            logger.warning("Scenario %s missing fields: %s", scenario.get("id", "unknown"), missing)
            continue

        if filter_operator_bound and scenario.get("agent_mode") == "operator_bound":
            logger.info("Skipping %s: requires operator_bound mode", scenario["id"])
            continue

        if filter_expected_tools and scenario.get("expected_tools"):
            logger.info("Skipping %s: depends on expected_tools %s", scenario["id"], scenario["expected_tools"])
            continue

        valid.append(scenario)

    logger.info("Loaded %d valid scenarios from gold set (%s)", len(valid), gold_set_path)
    return valid


def load_and_validate_benchmark_set(benchmark_path: str) -> list[dict[str, Any]]:
    """Load and validate a benchmark gold set for parameterization.

    Unlike the accuracy gold set, benchmark scenarios require:
      - ``expected_tool``: the tool name the agent must call
      - ``expected_payload``: list of {field, pattern} matchers

    No filtering is applied -- all valid scenarios are returned.
    """
    if not os.path.exists(benchmark_path):
        logger.error("Benchmark file not found: %s", benchmark_path)
        return []

    try:
        with open(benchmark_path, "r", encoding="utf-8") as f:
            scenarios = json.load(f)
    except json.JSONDecodeError as exc:
        logger.error("Failed to parse benchmark JSON: %s", exc)
        return []

    valid: list[dict[str, Any]] = []
    for scenario in scenarios:
        missing = REQUIRED_BENCHMARK_FIELDS - set(scenario.keys())
        if missing:
            logger.warning(
                "Benchmark scenario %s missing fields: %s",
                scenario.get("id", "unknown"),
                missing,
            )
            continue

        if not isinstance(scenario.get("expected_payload"), list):
            logger.warning(
                "Benchmark scenario %s: expected_payload must be a list",
                scenario["id"],
            )
            continue

        valid.append(scenario)

    logger.info("Loaded %d valid benchmark scenarios (%s)", len(valid), benchmark_path)
    return valid


async def seed_operator_if_bound(
    agent_mode: AgentMode,
    operator_id: str,
    operator_data_service,
    cleanup,
    log_prefix: str,
) -> list[BoundOperator]:
    """Create and seed a realistic operator document for operator-bound eval/benchmark scenarios.

    Uses ``build_production_operator_document`` which reflects a production Operator
    environment (root user, uid=0, systemd init, bare-metal Linux). This gives
    the LLM and Tribunal accurate privilege context so they avoid injecting
    unnecessary ``sudo`` into commands.

    Args:
        agent_mode: The agent mode for the test scenario.
        operator_id: Unique operator ID.
        operator_data_service: Service for creating operator documents.
        cleanup: Cleanup tracker for operator teardown.
        log_prefix: Prefix for log messages (e.g., "[BENCH]" or "[EVAL]").

    Returns:
        List of bound operators (empty if not operator-bound mode).
    """
    if agent_mode != AgentMode.OPERATOR_BOUND:
        return []

    from tests.fakes.factories import build_bound_operator, build_production_operator_document

    operator_doc = build_production_operator_document(operator_id=operator_id)

    bound_operators = [build_bound_operator(
        operator_id=operator_doc.operator_id,
        operator_session_id=operator_doc.operator_session_id,
        status=OperatorStatus.BOUND,
    )]

    await operator_data_service.create_operator(operator_doc)
    logger.info("%s Seeded operator document: %s", log_prefix, operator_doc.operator_id)
    cleanup.track_operator(operator_doc.operator_id)

    return bound_operators
