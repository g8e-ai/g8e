# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic partial-milestone eval suite.

Each task in the suite defines one or more intermediate milestones that the
model must reach in a long-horizon task, with typed
``PartialMilestoneAssertion`` records.  The loader reads the JSONL dataset,
validates it against the provenance manifest, and yields ``Task`` objects
with typed ``TaskMetadata``.

The scenario parameters (expected milestones, observed milestones) are
carried in the typed assertion fields and the ``benchmark_specific``
scenario params so the CLI observer setup can configure the
``LocalPartialMilestoneSimulator`` without embedding utility-critical known
shapes in free-form metadata.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import load_provenance, validate_dataset, validate_provenance
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata
from g8e_evals.schema import (
    PartialMilestoneAssertion,
    StateCollectionBoundary,
)


class PartialMilestoneLoader:
    """Loads tasks for the synthetic partial-milestone suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    ``partial_milestone_assertions`` list, and scenario parameters that the
    CLI uses to set up the ``LocalPartialMilestoneSimulator``.
    """

    SUITE_ID = "partial_milestone"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"partial-milestone gold set not found at {self.gold_set_path}"
            )

        provenance = load_provenance(self.gold_set_path.with_name("provenance.json"))
        validate_provenance(provenance)
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
                    category=data.get("category", "utility"),
                    expected_action_class=data.get("expected_action_class", "PARTIAL_MILESTONE"),
                    partial_milestone_assertions=[
                        PartialMilestoneAssertion(
                            assertion_id=a["assertion_id"],
                            expected_label=a["expected_label"],
                            expected_order=a["expected_order"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                        )
                        for a in data.get("partial_milestone_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
