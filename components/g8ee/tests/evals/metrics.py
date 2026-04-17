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

"""Unified metrics dataclasses for AI evals reporting across accuracy, safety, and privacy dimensions."""

from dataclasses import dataclass, field
from datetime import datetime, UTC
from typing import Any


@dataclass
class EvalRow:
    """Single evaluation result row.

    Attributes:
        dimension: One of "accuracy", "safety", or "privacy".
        suite: Test suite identifier (e.g., "agent_accuracy", "agent_benchmark", "agent_privacy").
        scenario_id: Unique scenario identifier from gold set.
        category: Optional category string for grouping (e.g., "multi_step_execution", "email").
        passed: Whether the scenario passed the evaluation criteria.
        score: Numeric score if applicable (e.g., 1-5 for accuracy), None for binary pass/fail.
        score_max: Maximum possible score if score is not None.
        latency_ms: Execution time in milliseconds.
        error: Error message if the test failed due to an exception.
        details: Arbitrary structured details for debugging and analysis.
    """
    dimension: str
    suite: str
    scenario_id: str
    category: str = ""
    passed: bool = False
    score: int | None = None
    score_max: int | None = None
    latency_ms: float = 0.0
    error: str | None = None
    details: dict[str, Any] = field(default_factory=dict)


@dataclass
class DimensionSummary:
    """Summary statistics for a single dimension (accuracy/safety/privacy).

    Attributes:
        dimension: Dimension name.
        total: Total number of scenarios in this dimension.
        passed: Number of scenarios that passed.
        failed: Number of scenarios that failed.
        pass_pct: Pass percentage (0-100).
        avg_score: Average score across all scored scenarios, None if no scores.
        per_category: Dict mapping category name to (passed, total, pct) tuples.
    """
    dimension: str
    total: int = 0
    passed: int = 0
    failed: int = 0
    pass_pct: float = 0.0
    avg_score: float | None = None
    per_category: dict[str, tuple[int, int, float]] = field(default_factory=dict)


@dataclass
class FullReport:
    """Complete evaluation report with all rows, summaries, and metadata.

    Attributes:
        rows: All evaluation rows collected during the test session.
        summaries: Per-dimension summaries keyed by dimension name.
        started_at: UTC timestamp when the first test started.
        finished_at: UTC timestamp when the last test finished.
        llm_config: Dict containing LLM provider and model configuration.
    """
    rows: list[EvalRow] = field(default_factory=list)
    summaries: dict[str, DimensionSummary] = field(default_factory=dict)
    started_at: datetime = field(default_factory=lambda: datetime.now(UTC))
    finished_at: datetime = field(default_factory=lambda: datetime.now(UTC))
    llm_config: dict[str, Any] = field(default_factory=dict)
