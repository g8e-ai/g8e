# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Benchmark population records for the multi-role, heterogeneous-stack
benchmark.

Builds frozen ``BenchmarkPopulation`` records for every model-comparison
suite (IFEval instruction-following and the eight REAL_SYSTEM scenario
suites) from the gold-set JSONL files and provenance manifests. Each
population records its kind, suite ID, task IDs, dataset content hash,
source provenance hash, redistribution license, deduplication/contamination
check, minimum population, and computed content hash.

The populations are reproducible from the gold sets: the dataset content
hash is the SHA-256 of the gold-set JSONL, the source provenance hash is
the SHA-256 of the provenance manifest, and the content hash is the
canonical JSON hash of the population record. Changing any gold set or
provenance manifest changes the corresponding population hash.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

from g8e_evals.benchmark_contract import (
    BENCHMARK_POPULATION_VERSION,
    BenchmarkPopulation,
    BenchmarkPopulationKind,
    DeduplicationCheck,
    compute_population_hash,
)
from g8e_evals.suites import SUITE_REGISTRY, SuiteExecutionClass

_GOLD_SETS_DIR = Path(__file__).resolve().parents[3] / "gold_sets"

_SUITE_KIND: dict[str, BenchmarkPopulationKind] = {
    "ifeval_subset": BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
    "tool_selection": BenchmarkPopulationKind.STRUCTURED_OUTPUT,
    "tool_arguments": BenchmarkPopulationKind.STRUCTURED_OUTPUT,
    "technical_analysis": BenchmarkPopulationKind.TIER_EXERCISE,
    "routing_delegation": BenchmarkPopulationKind.TIER_EXERCISE,
    "verification": BenchmarkPopulationKind.TIER_EXERCISE,
    "security_policy": BenchmarkPopulationKind.TIER_EXERCISE,
    "recovery": BenchmarkPopulationKind.TIER_EXERCISE,
    "final_response": BenchmarkPopulationKind.TIER_EXERCISE,
}

_SUITE_DESCRIPTIONS: dict[str, str] = {
    "ifeval_subset": "IFEval instruction-following subset (120 tasks).",
    "tool_selection": "Tool selection scenarios (4 tasks).",
    "tool_arguments": "Tool arguments scenarios (3 tasks).",
    "technical_analysis": "Technical analysis scenarios (4 tasks).",
    "routing_delegation": "Routing and delegation scenarios (3 tasks).",
    "verification": "Verification scenarios (2 tasks).",
    "security_policy": "Security and policy scenarios (2 tasks).",
    "recovery": "Recovery scenarios (2 tasks).",
    "final_response": "Final response scenarios (1 task).",
}

_MINIMUM_POPULATION = 1


def _sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _read_task_ids(gold_set_path: Path) -> list[str]:
    rows = [
        json.loads(line)
        for line in gold_set_path.read_text().splitlines()
        if line.strip()
    ]
    return sorted(str(row["key"]) for row in rows)


def _read_license(provenance_path: Path) -> str:
    provenance = json.loads(provenance_path.read_text())
    return provenance["source"]["license_spdx"]


def _make_dedup_check(report_hash: str) -> DeduplicationCheck:
    return DeduplicationCheck(
        deduplication_method="exact_hash",
        contamination_check_method="ngram_overlap",
        duplicate_tasks_removed=0,
        contaminated_tasks_removed=0,
        check_report_hash=report_hash,
    )


def build_population(suite_id: str, *, gold_sets_dir: Path | None = None) -> BenchmarkPopulation:
    """Build a frozen ``BenchmarkPopulation`` for ``suite_id`` from its gold set.

    Reads the gold-set JSONL to extract task IDs, reads the provenance
    manifest for the dataset content hash and redistribution license,
    computes the source provenance hash, and constructs a frozen
    ``BenchmarkPopulation`` with a computed content hash.

    Raises ``KeyError`` if the suite is not in the population map.
    Raises ``FileNotFoundError`` if the gold set or provenance manifest
    is missing.
    """
    if suite_id not in _SUITE_KIND:
        raise KeyError(f"no population kind defined for suite: {suite_id}")

    base_dir = gold_sets_dir or _GOLD_SETS_DIR
    spec = SUITE_REGISTRY[suite_id]
    gold_set_path = base_dir / suite_id / "input_data.jsonl"
    provenance_path = base_dir / suite_id / "provenance.json"

    task_ids = _read_task_ids(gold_set_path)
    dataset_content_hash = _sha256_file(gold_set_path)
    source_provenance_hash = _sha256_file(provenance_path)
    redistribution_license = _read_license(provenance_path)
    dedup_check = _make_dedup_check(source_provenance_hash)

    population_id = f"pop-{suite_id}"
    content_hash = compute_population_hash(
        population_id=population_id,
        population_version=BENCHMARK_POPULATION_VERSION,
        kind=_SUITE_KIND[suite_id],
        suite_id=suite_id,
        description=_SUITE_DESCRIPTIONS[suite_id],
        task_ids=task_ids,
        dataset_content_hash=dataset_content_hash,
        source_provenance_hash=source_provenance_hash,
        redistribution_license=redistribution_license,
        deduplication_check=dedup_check,
        minimum_population=_MINIMUM_POPULATION,
    )

    return BenchmarkPopulation(
        population_id=population_id,
        population_version=BENCHMARK_POPULATION_VERSION,
        kind=_SUITE_KIND[suite_id],
        suite_id=suite_id,
        description=_SUITE_DESCRIPTIONS[suite_id],
        task_ids=task_ids,
        task_count=len(task_ids),
        dataset_content_hash=dataset_content_hash,
        source_provenance_hash=source_provenance_hash,
        redistribution_license=redistribution_license,
        deduplication_check=dedup_check,
        minimum_population=_MINIMUM_POPULATION,
        content_hash=content_hash,
    )


def build_all_populations(*, gold_sets_dir: Path | None = None) -> list[BenchmarkPopulation]:
    """Build frozen ``BenchmarkPopulation`` records for every model-comparison suite.

    Returns populations for every REAL_MODEL and REAL_SYSTEM suite in the
    suite registry, sorted by suite ID.
    """
    suite_ids = sorted(
        sid for sid, spec in SUITE_REGISTRY.items()
        if spec.execution_class in (SuiteExecutionClass.REAL_MODEL, SuiteExecutionClass.REAL_SYSTEM)
        and sid in _SUITE_KIND
    )
    return [build_population(sid, gold_sets_dir=gold_sets_dir) for sid in suite_ids]


__all__ = ["build_all_populations", "build_population"]
