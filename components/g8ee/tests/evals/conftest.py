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

"""Fixtures for AI accuracy evaluation tests.

Provides eval_results_collector fixture that collects all evaluation results
and displays them in a summary at the end of the test run.
"""

import asyncio
import logging
import os

import pytest
import pytest_asyncio
from typing import Any

from app.llm.factory import get_llm_settings, clear_provider_cache
from app.models.settings import G8eeUserSettings, SearchSettings, EvalJudgeSettings
from app.services.service_factory import ServiceFactory

logger = logging.getLogger(__name__)


@pytest_asyncio.fixture(scope="session", autouse=True, loop_scope="session")
async def clear_llm_provider_cache():
    """Clear LLM provider cache at session end to prevent unclosed client session warnings."""
    yield
    await clear_provider_cache()


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def all_services(cache_aside_service, test_settings):
    """Fixture that returns all g8ee services.

    Benchmark and eval tests use fake operators (documents only, no real process).
    Use auto_approve_pending helper to approve pending approvals during tests.
    """
    services = ServiceFactory.create_all_services(test_settings, cache_aside_service)

    yield services

    chat_task_manager = services.get('chat_task_manager')
    if chat_task_manager is not None:
        await chat_task_manager.wait_all(timeout=5.0)

    await ServiceFactory.stop_services(services)


@pytest.fixture(scope="function")
def tool_service(all_services):
    """Returns the AIToolService from all_services."""
    return all_services['tool_service']


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def cleanup(cache_aside_service, all_services):
    """Autouse-friendly cleanup tracker for integration tests.

    Track documents created during a test via ``cleanup.track_investigation(id)``
    etc. All tracked documents are deleted after the test, even on failure.
    """
    from tests.integration.cleanup import IntegrationCleanupTracker

    tracker = IntegrationCleanupTracker(cache_aside_service)
    yield tracker

    chat_task_manager = all_services.get('chat_task_manager')
    if chat_task_manager is not None:
        await chat_task_manager.wait_all(timeout=5.0)

    await tracker.cleanup()


@pytest.fixture(scope="session")
def test_user_settings():
    """Session-scoped fixture providing G8eeUserSettings for integration eval tests.

    Constructs user settings with LLM configuration from environment variables
    (via get_llm_settings()) and search disabled by default. Tests that require
    search enabled can override the search field on the returned object.

    Eliminates duplication across test_agent_benchmark.py, test_agent_accuracy.py,
    and provider-specific accuracy tests.
    """
    llm_settings = get_llm_settings()
    if not llm_settings or not llm_settings.primary_model:
        pytest.skip("LLM provider is not configured")

    search_settings = SearchSettings(enabled=False)
    eval_judge_settings = EvalJudgeSettings(model=llm_settings.primary_model)
    return G8eeUserSettings(llm=llm_settings, search=search_settings, eval_judge=eval_judge_settings)


@pytest.fixture(scope="session")
def eval_results_collector(request):
    """Collects and displays all eval results in a summary at the end.

    Tests should append their AccuracyTestResult to this collector's results list.
    The fixture automatically prints a summary when the session ends.
    """
    class EvalResultsCollector:
        def __init__(self):
            self.results: list[dict[str, Any]] = []

        def add_result(self, result: dict[str, Any]):
            self.results.append(result)

    collector = EvalResultsCollector()

    def print_summary():
        if not collector.results:
            return

        print("\n")
        print("=" * 80)
        print("EVALUATION RESULTS SUMMARY")
        print("=" * 80)
        print()

        for result in collector.results:
            print("=" * 60)
            print(f"[EVAL_RESULT] Scenario: {result['scenario_id']}")
            print(f"[EVAL_RESULT] Score: {result['score']}/5")
            print(f"[EVAL_RESULT] Passed: {result['passed']}")
            print(f"[EVAL_RESULT] Execution Time: {result['execution_time_ms']:.1f}ms")
            print(f"[EVAL_RESULT] Reasoning: {result['reasoning']}")
            print("=" * 60)
            print()

        total = len(collector.results)
        passed = sum(1 for r in collector.results if r['passed'])
        avg_score = sum(r['score'] for r in collector.results) / total if total > 0 else 0

        print("=" * 80)
        print(f"Total Scenarios: {total}")
        print(f"Passed: {passed}/{total}")
        print(f"Average Score: {avg_score:.2f}/5")
        print("=" * 80)

    request.addfinalizer(print_summary)
    return collector


@pytest.fixture(scope="session")
def benchmark_results_collector(request):
    """Collects benchmark results and displays a binary pass/fail summary with aggregate percentage.

    Includes Tribunal delta statistics when available.
    """
    class BenchmarkResultsCollector:
        def __init__(self):
            self.results: list[dict[str, Any]] = []

        def add_result(self, result: dict[str, Any]):
            self.results.append(result)

    collector = BenchmarkResultsCollector()

    def print_summary():
        if not collector.results:
            return

        print("\n")
        print("=" * 80)
        print("BENCHMARK RESULTS SUMMARY")
        print("=" * 80)
        print()

        categories: dict[str, list[dict[str, Any]]] = {}
        for result in collector.results:
            cat = result.get("category", "general")
            categories.setdefault(cat, []).append(result)

        for cat, results in sorted(categories.items()):
            cat_passed = sum(1 for r in results if r["passed"])
            cat_total = len(results)
            cat_pct = (cat_passed / cat_total * 100) if cat_total > 0 else 0
            print(f"  [{cat}] {cat_passed}/{cat_total} ({cat_pct:.0f}%)")
            for result in results:
                status = "PASS" if result["passed"] else "FAIL"
                print(f"    {status} {result['scenario_id']}")
                if not result["passed"] and result.get("failures"):
                    for failure in result["failures"][:3]:
                        print(f"         - {failure[:120]}")
            print()

        total = len(collector.results)
        passed = sum(1 for r in collector.results if r["passed"])
        pct = (passed / total * 100) if total > 0 else 0

        print("-" * 80)
        print(f"AGGREGATE: {passed}/{total} scenarios passed ({pct:.1f}%)")
        print("-" * 80)

        continue_results = [
            r for r in collector.results if r.get("agent_continue_approvals", 0) > 0
        ]
        if continue_results:
            print()
            print("AGENT_CONTINUE APPROVALS (scenarios that exceeded AGENT_MAX_TOOL_TURNS):")
            total_continues = sum(r["agent_continue_approvals"] for r in continue_results)
            print(
                f"  {len(continue_results)}/{total} scenarios required continuation "
                f"(total AGENT_CONTINUE approvals={total_continues})"
            )
            for r in continue_results:
                print(
                    f"    {r['scenario_id']}: "
                    f"agent_continue={r['agent_continue_approvals']} "
                    f"all_approvals={r.get('approvals_by_type') or {}}"
                )
            print("-" * 80)

        tribunal_results = [r for r in collector.results if r.get("tribunal_outcome")]
        if tribunal_results:
            print()
            print("TRIBUNAL DELTA:")
            t_total = len(tribunal_results)
            t_improved = sum(1 for r in tribunal_results if r.get("tribunal_improved"))
            t_accuracy = sum(
                1 for r in tribunal_results
                if r.get("tribunal_improved") and r.get("passed") and r.get("tribunal_pre_score") is False
            )
            t_rate = (t_accuracy / t_total * 100) if t_total > 0 else 0

            print(f"  Scenarios with Tribunal: {t_total}")
            print(f"  Tribunal changed command: {t_improved}/{t_total}")
            print(f"  Tribunal improved accuracy: {t_accuracy}/{t_total} ({t_rate:.1f}%)")

            for r in tribunal_results:
                if r.get("tribunal_improved"):
                    print(f"    {r['scenario_id']}:")
                    print(f"      BEFORE: {r.get('tribunal_original_command', 'N/A')[:100]}")
                    print(f"      AFTER:  {r.get('tribunal_final_command', 'N/A')[:100]}")
            print("-" * 80)

        print("=" * 80)

    request.addfinalizer(print_summary)
    return collector
