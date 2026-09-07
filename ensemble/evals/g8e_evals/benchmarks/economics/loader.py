# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic economics and performance eval suite.

Each task in the suite declares an economics or performance measurement
(``PerformanceMetricKind``) with an expected value, tolerance window,
task complexity stratum, and action class. The loader reads the JSONL
dataset, validates it against the provenance manifest, and yields
``Task`` objects with typed ``TaskMetadata``.

The scenario parameters (per-assertion observed values) are carried in
the ``benchmark_specific`` scenario params so the CLI observer setup can
configure the ``LocalEconomicsPerformanceSimulator`` without embedding
measurement-critical known shapes in free-form metadata. The observed
values are simulator inputs, not security- or privacy-critical known
shapes, so they remain in ``benchmark_specific`` alongside the grader
list.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import load_provenance, validate_dataset, validate_provenance
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata
from g8e_evals.schema import (
    EconomicsPerformanceAssertion,
    PerformanceMetricKind,
    StateCollectionBoundary,
    TaskComplexity,
)


class EconomicsPerformanceLoader:
    """Loads tasks for the synthetic economics and performance suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    ``economics_performance_assertions`` list, and scenario parameters
    that the CLI uses to set up the
    ``LocalEconomicsPerformanceSimulator``.
    """

    SUITE_ID = "economics_performance"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"economics_performance gold set not found at {self.gold_set_path}"
            )

        provenance = load_provenance(self.gold_set_path.with_name("provenance.json"))
        trusted_root = self.gold_set_path.parent.parent.parent
        validate_provenance(provenance, suite_id=self.SUITE_ID, trusted_root=trusted_root)
        validate_dataset(self.gold_set_path, provenance)
        rows = [
            json.loads(line)
            for line in self.gold_set_path.read_text().splitlines()
            if line.strip()
        ]

        for data in rows:
            yield Task(
                id=str(data["key"]),
                prompt=data["description"],
                metadata=TaskMetadata(
                    benchmark=self.SUITE_ID,
                    category=data.get("category", "economics_performance"),
                    expected_action_class=data.get("expected_action_class", "ECONOMICS"),
                    economics_performance_assertions=[
                        EconomicsPerformanceAssertion(
                            assertion_id=a["assertion_id"],
                            metric_kind=PerformanceMetricKind(a["metric_kind"]),
                            role=a.get("role", ""),
                            action_class=a["action_class"],
                            task_complexity=TaskComplexity(a["task_complexity"]),
                            expected_value=float(a["expected_value"]),
                            tolerance=float(a["tolerance"]),
                            unit=a["unit"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                        )
                        for a in data.get("economics_performance_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
