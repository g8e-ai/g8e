# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Integration tests for benchmark population construction.

Verifies that ``build_population`` and ``build_all_populations``
construct frozen ``BenchmarkPopulation`` records for every
model-comparison suite from the gold-set files and provenance
manifests. Each population must have correct task IDs, dataset
content hash, source provenance hash, redistribution license, and
computed content hash.

Uses real gold-set files from the evals package.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

from g8e_evals.benchmark_contract import (
    BENCHMARK_POPULATION_VERSION,
    BenchmarkPopulation,
    BenchmarkPopulationKind,
    compute_population_hash,
)
from g8e_evals.benchmarks.scenarios.populations import (
    build_all_populations,
    build_population,
)


pytestmark = pytest.mark.integration

_GOLD_SETS_DIR = Path(__file__).resolve().parents[1] / "gold_sets"

_SUITE_EXPECTATIONS: list[tuple[str, BenchmarkPopulationKind, int]] = [
    ("ifeval_subset", BenchmarkPopulationKind.INSTRUCTION_FOLLOWING, 120),
    ("tool_selection", BenchmarkPopulationKind.STRUCTURED_OUTPUT, 4),
    ("tool_arguments", BenchmarkPopulationKind.STRUCTURED_OUTPUT, 3),
    ("technical_analysis", BenchmarkPopulationKind.TIER_EXERCISE, 4),
    ("routing_delegation", BenchmarkPopulationKind.TIER_EXERCISE, 3),
    ("verification", BenchmarkPopulationKind.TIER_EXERCISE, 2),
    ("security_policy", BenchmarkPopulationKind.TIER_EXERCISE, 2),
    ("recovery", BenchmarkPopulationKind.TIER_EXERCISE, 2),
    ("final_response", BenchmarkPopulationKind.TIER_EXERCISE, 1),
]


@pytest.fixture(params=_SUITE_EXPECTATIONS)
def suite_expectation(request):
    return request.param


class TestBuildPopulation:
    def test_population_has_correct_suite_id(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        assert pop.suite_id == suite_id

    def test_population_has_correct_kind(self, suite_expectation):
        suite_id, expected_kind, _ = suite_expectation
        pop = build_population(suite_id)
        assert pop.kind == expected_kind

    def test_population_has_correct_task_count(self, suite_expectation):
        suite_id, _, expected_count = suite_expectation
        pop = build_population(suite_id)
        assert pop.task_count == expected_count
        assert len(pop.task_ids) == expected_count

    def test_population_task_ids_are_sorted(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        assert pop.task_ids == sorted(pop.task_ids)

    def test_population_dataset_hash_matches_gold_set(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        gold_set = _GOLD_SETS_DIR / suite_id / "input_data.jsonl"
        expected_hash = hashlib.sha256(gold_set.read_bytes()).hexdigest()
        assert pop.dataset_content_hash == expected_hash

    def test_population_source_provenance_hash_matches_manifest(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        provenance = _GOLD_SETS_DIR / suite_id / "provenance.json"
        expected_hash = hashlib.sha256(provenance.read_bytes()).hexdigest()
        assert pop.source_provenance_hash == expected_hash

    def test_population_redistribution_license_from_provenance(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        provenance = json.loads((_GOLD_SETS_DIR / suite_id / "provenance.json").read_text())
        assert pop.redistribution_license == provenance["source"]["license_spdx"]

    def test_population_content_hash_is_valid_sha256(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        assert len(pop.content_hash) == 64
        int(pop.content_hash, 16)

    def test_population_content_hash_is_reproducible(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop1 = build_population(suite_id)
        pop2 = build_population(suite_id)
        assert pop1.content_hash == pop2.content_hash

    def test_population_is_frozen(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        with pytest.raises(Exception):
            pop.suite_id = "modified"

    def test_population_population_id_uses_suite_id(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        assert pop.population_id == f"pop-{suite_id}"

    def test_population_version_matches_contract(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        assert pop.population_version == BENCHMARK_POPULATION_VERSION

    def test_population_dedup_check_has_valid_report_hash(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        assert len(pop.deduplication_check.check_report_hash) == 64
        int(pop.deduplication_check.check_report_hash, 16)

    def test_population_round_trip_serialization(self, suite_expectation):
        suite_id, _, _ = suite_expectation
        pop = build_population(suite_id)
        data = json.loads(pop.model_dump_json())
        restored = BenchmarkPopulation.model_validate(data)
        assert restored.content_hash == pop.content_hash
        assert restored.task_ids == pop.task_ids

    def test_population_raises_keyerror_for_unknown_suite(self):
        with pytest.raises(KeyError, match="no population kind"):
            build_population("nonexistent_suite")


class TestBuildAllPopulations:
    def test_builds_all_nine_populations(self):
        pops = build_all_populations()
        assert len(pops) == 9

    def test_populations_are_sorted_by_suite_id(self):
        pops = build_all_populations()
        suite_ids = [p.suite_id for p in pops]
        assert suite_ids == sorted(suite_ids)

    def test_all_populations_have_valid_content_hashes(self):
        pops = build_all_populations()
        for pop in pops:
            assert len(pop.content_hash) == 64
            int(pop.content_hash, 16)

    def test_all_populations_cover_25_scenarios_plus_ifeval(self):
        pops = build_all_populations()
        total_tasks = sum(p.task_count for p in pops)
        assert total_tasks == 120 + 4 + 3 + 4 + 3 + 2 + 2 + 2 + 1

    def test_population_kinds_match_expected(self):
        pops = build_all_populations()
        for pop in pops:
            if pop.suite_id == "ifeval_subset":
                assert pop.kind == BenchmarkPopulationKind.INSTRUCTION_FOLLOWING
            elif pop.suite_id in ("tool_selection", "tool_arguments"):
                assert pop.kind == BenchmarkPopulationKind.STRUCTURED_OUTPUT
            else:
                assert pop.kind == BenchmarkPopulationKind.TIER_EXERCISE
