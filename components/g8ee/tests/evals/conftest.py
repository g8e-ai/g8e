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

Provides unified_metrics_collector fixture that collects all evaluation results
across accuracy, safety, and privacy dimensions using fake operators (documents only,
no real process), and displays them in a summary at the end of the test run with
persisted artifacts.

Real-operator evals are being migrated to a new host-driven framework.
See docs/benchmarking/evals.md for the new design.
"""

import logging

import pytest
import pytest_asyncio
from typing import Any

from app.constants.paths import PATHS
from app.llm.factory import get_llm_settings, clear_provider_cache
from app.models.settings import G8eeUserSettings, SearchSettings, EvalJudgeSettings
from app.services.service_factory import ServiceFactory
from tests.evals.metrics import EvalRow, FullReport
from tests.evals.reporter import compute_summaries, persist_report, render_text_table

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

    Injects a real WebSearchProvider if TEST_WEB_SEARCH_* env vars are set,
    ensuring the g8e_web_search tool is registered for eval scenarios that expect it.
    """
    import os
    from app.services.ai.grounding.web_search_provider import WebSearchProvider

    # Check if web search credentials are available via env vars
    web_search_provider = None
    project_id = os.environ.get("TEST_WEB_SEARCH_PROJECT_ID", "").strip()
    engine_id = os.environ.get("TEST_WEB_SEARCH_ENGINE_ID", "").strip()
    api_key = os.environ.get("TEST_WEB_SEARCH_API_KEY", "").strip()
    location = os.environ.get("TEST_WEB_SEARCH_LOCATION", "").strip() or "global"

    if project_id and engine_id and api_key:
        web_search_provider = WebSearchProvider(
            project_id=project_id,
            engine_id=engine_id,
            api_key=api_key,
            location=location,
        )
        logger.info(
            "[EVAL-FIXTURE] Injecting real WebSearchProvider from env vars: project_id=%s engine_id=%s",
            project_id, engine_id
        )

    services = ServiceFactory.create_all_services(
        test_settings,
        cache_aside_service,
        web_search_provider=web_search_provider,
    )

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
def unified_metrics_collector(request):
    """Unified collector for all eval results across accuracy, safety, and privacy dimensions.

    Tests should call add_row(EvalRow) to record results. The fixture automatically
    prints a text summary to stdout and persists artifacts (report.txt, results.csv,
    summary.json) to components/g8ee/reports/evals/<timestamp>/ at session end.
    """
    class UnifiedMetricsCollector:
        def __init__(self):
            self.rows: list[EvalRow] = []

        def add_row(self, row: EvalRow):
            self.rows.append(row)

    collector = UnifiedMetricsCollector()

    def finalize_session():
        if not collector.rows:
            return

        from datetime import datetime, UTC

        llm_settings = get_llm_settings()
        llm_config = {}
        if llm_settings:
            llm_config = {
                "primary_provider": llm_settings.primary_provider.value if llm_settings.primary_provider else "unknown",
                "primary_model": llm_settings.primary_model or "unknown",
                "assistant_provider": llm_settings.assistant_provider.value if llm_settings.assistant_provider else "none",
                "assistant_model": llm_settings.assistant_model or "none",
            }

        report = FullReport(
            rows=collector.rows,
            summaries=compute_summaries(collector.rows),
            finished_at=datetime.now(UTC),
            llm_config=llm_config,
        )

        print("\n")
        print(render_text_table(report))

        reports_dir = PATHS["g8ee"]["evals"]["reports_dir"]
        try:
            artifacts = persist_report(report, reports_dir)
            print(f"\nArtifacts persisted to: {artifacts['run_dir']}")
        except Exception as e:
            logger.error("Failed to persist eval artifacts: %s", e)

    request.addfinalizer(finalize_session)
    return collector


@pytest.fixture(scope="session")
def eval_results_collector(unified_metrics_collector):
    """Backwards-compatible shim for accuracy eval results.

    Delegates to unified_metrics_collector with dimension="accuracy".
    Tests using this fixture should call add_result(dict) as before.
    """
    class EvalResultsCollectorShim:
        def __init__(self, unified):
            self.unified = unified

        def add_result(self, result: dict[str, Any]):
            dimension = result.get("dimension", "accuracy")
            row = EvalRow(
                dimension=dimension,
                suite=result.get("suite", "agent_accuracy"),
                scenario_id=result["scenario_id"],
                category=result.get("category", ""),
                passed=result["passed"],
                score=result.get("score"),
                score_max=5 if result.get("score") is not None else None,
                latency_ms=result.get("execution_time_ms", 0),
                error=result.get("error"),
                details={
                    "reasoning": result.get("reasoning", ""),
                    "response_text": result.get("response_text", "")[:500],
                },
            )
            self.unified.add_row(row)

    return EvalResultsCollectorShim(unified_metrics_collector)


@pytest.fixture(scope="session")
def benchmark_results_collector(unified_metrics_collector):
    """Backwards-compatible shim for benchmark results.

    Delegates to unified_metrics_collector with dimension="accuracy" for most
    benchmarks, or "safety" for security_refusal category.
    """
    class BenchmarkResultsCollectorShim:
        def __init__(self, unified):
            self.unified = unified

        def add_result(self, result: dict[str, Any]):
            category = result.get("category", "general")
            if category == "security_refusal":
                dimension = "safety"
            else:
                dimension = "accuracy"

            row = EvalRow(
                dimension=dimension,
                suite=result.get("suite", "agent_benchmark"),
                scenario_id=result["scenario_id"],
                category=category,
                passed=result["passed"],
                score=None,
                score_max=None,
                latency_ms=result.get("execution_time_ms", 0),
                error=result.get("error"),
                details={
                    "tool_called": result.get("tool_called", False),
                    "matchers_total": result.get("matchers_total", 0),
                    "matchers_passed": result.get("matchers_passed", 0),
                    "failures": result.get("failures", []),
                    "tribunal_final_command": result.get("tribunal_final_command"),
                    "tribunal_outcome": result.get("tribunal_outcome"),
                    "agent_continue_approvals": result.get("agent_continue_approvals", 0),
                    "approvals_by_type": result.get("approvals_by_type", {}),
                },
            )
            self.unified.add_row(row)

    return BenchmarkResultsCollectorShim(unified_metrics_collector)
