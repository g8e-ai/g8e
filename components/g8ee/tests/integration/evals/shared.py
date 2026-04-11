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

logger = logging.getLogger(__name__)

REQUIRED_SCENARIO_FIELDS = {"id", "user_query", "agent_mode", "expected_behavior", "required_concepts"}


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

    def to_dict(self) -> dict[str, Any]:
        return {
            "scenario_id": self.scenario_id,
            "passed": self.passed,
            "score": self.score,
            "reasoning": self.reasoning,
            "response_text": self.response_text[:500],
            "execution_time_ms": self.execution_time_ms,
            "error": self.error,
        }


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
