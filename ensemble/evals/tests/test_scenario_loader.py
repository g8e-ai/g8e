# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Integration tests for the ScenarioLoader.

Verifies that the loader reads gold-set JSONL files, validates
provenance, yields Task objects with typed ScenarioMetadata in the
benchmark_specific field, and that the loader factory binds to the
correct suite ID.

Uses real gold-set files from the evals package.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.benchmarks.scenarios.loader import (
    ScenarioLoader,
    make_scenario_loader_factory,
)
from g8e_evals.benchmarks.scenarios.metadata import ScenarioMetadata


pytestmark = pytest.mark.integration

_GOLD_SETS_DIR = Path(__file__).resolve().parents[1] / "gold_sets"

_SCENARIO_SUITES: list[tuple[str, int]] = [
    ("tool_selection", 4),
    ("tool_arguments", 3),
    ("technical_analysis", 4),
    ("routing_delegation", 3),
    ("verification", 2),
    ("security_policy", 2),
    ("recovery", 2),
    ("final_response", 1),
]


@pytest.fixture(params=_SCENARIO_SUITES)
def suite_info(request):
    return request.param


class TestScenarioLoaderFactory:
    def test_factory_returns_scenario_loader(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        assert isinstance(loader, ScenarioLoader)

    def test_factory_binds_to_correct_suite_id(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        assert loader._suite_id == suite_id

    def test_factory_name_includes_suite_id(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        assert suite_id in factory.__name__


class TestScenarioLoaderLoad:
    def test_loader_yields_correct_task_count(self, suite_info):
        suite_id, expected_count = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        tasks = list(loader.load())
        assert len(tasks) == expected_count

    def test_loader_yields_tasks_with_ids(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        tasks = list(loader.load())
        for task in tasks:
            assert task.id
            assert isinstance(task.id, str)

    def test_loader_yields_tasks_with_prompts(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        tasks = list(loader.load())
        for task in tasks:
            assert task.prompt
            assert isinstance(task.prompt, str)

    def test_loader_yields_tasks_with_benchmark_specific_metadata(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        tasks = list(loader.load())
        for task in tasks:
            assert task.metadata.benchmark_specific is not None
            meta = ScenarioMetadata.model_validate(task.metadata.benchmark_specific)
            assert meta.category
            assert meta.complexity
            assert meta.expected_role
            assert len(meta.criteria) >= 1

    def test_loader_sets_benchmark_field_to_suite_id(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory(suite_id)
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        tasks = list(loader.load())
        for task in tasks:
            assert task.metadata.benchmark == suite_id

    def test_loader_raises_on_missing_gold_set(self):
        factory = make_scenario_loader_factory("tool_selection")
        loader = factory(Path("/nonexistent/gold_set.jsonl"))
        with pytest.raises(FileNotFoundError):
            list(loader.load())


class TestScenarioLoaderProvenanceValidation:
    def test_loader_validates_provenance_benchmark_matches_suite_id(self, suite_info):
        suite_id, _ = suite_info
        factory = make_scenario_loader_factory("wrong_suite_id")
        loader = factory(_GOLD_SETS_DIR / suite_id / "input_data.jsonl")
        with pytest.raises(ValueError, match="provenance benchmark mismatch"):
            list(loader.load())
