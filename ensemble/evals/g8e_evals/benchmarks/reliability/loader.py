# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic reliability eval suite.

Each task in the suite declares a reliability failure scenario
(``ReliabilityScenarioType``) with an expected handling behavior
(``ReliabilityExpectedBehavior``) and evidence-preservation requirement.
The loader reads the JSONL dataset, validates it against the provenance
manifest, and yields ``Task`` objects with typed ``TaskMetadata``.

The scenario parameters (per-assertion observed behavior and
evidence-preservation flags) are carried in the ``benchmark_specific``
scenario params so the CLI observer setup can configure the
``LocalReliabilitySimulator`` without embedding reliability-critical
known shapes in free-form metadata. The observed behavior values are
simulator inputs, not security- or privacy-critical known shapes, so
they remain in ``benchmark_specific`` alongside the grader list.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import load_provenance, validate_dataset, validate_provenance
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata
from g8e_evals.schema import (
    ReliabilityAssertion,
    ReliabilityExpectedBehavior,
    ReliabilityScenarioType,
    StateCollectionBoundary,
)


class ReliabilityLoader:
    """Loads tasks for the synthetic reliability suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    ``reliability_assertions`` list, and scenario parameters that the
    CLI uses to set up the ``LocalReliabilitySimulator``.
    """

    SUITE_ID = "reliability"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"reliability gold set not found at {self.gold_set_path}"
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
                    category=data.get("category", "reliability"),
                    expected_action_class=data.get("expected_action_class", "RELIABILITY"),
                    reliability_assertions=[
                        ReliabilityAssertion(
                            assertion_id=a["assertion_id"],
                            scenario_type=ReliabilityScenarioType(a["scenario_type"]),
                            action_type=a["action_type"],
                            expected_behavior=ReliabilityExpectedBehavior(a["expected_behavior"]),
                            expected_evidence_preserved=a.get("expected_evidence_preserved", True),
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                        )
                        for a in data.get("reliability_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
