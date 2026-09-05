# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic tool-sequence eval suite.

Each task in the suite defines an allowed or forbidden tool sequence
with typed ``ToolSequenceAssertion`` records.  The loader reads the
JSONL dataset, validates it against the provenance manifest, and yields
``Task`` objects with typed ``TaskMetadata``.

The scenario parameters (expected sequence, outcome, tool list) are
carried in the typed assertion fields and the ``benchmark_specific``
scenario params so the CLI observer setup can configure the
``LocalToolUseSimulator`` without embedding utility-critical known
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
    StateCollectionBoundary,
    ToolSequenceAssertion,
    ToolSequenceOutcome,
)


class ToolSequenceLoader:
    """Loads tasks for the synthetic tool-sequence suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    ``tool_sequence_assertions`` list, and scenario parameters that the
    CLI uses to set up the ``LocalToolUseSimulator``.
    """

    SUITE_ID = "tool_sequence"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"tool-sequence gold set not found at {self.gold_set_path}"
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
                    category=data.get("category", "utility"),
                    expected_action_class=data.get("expected_action_class", "TOOL_USE_PROBE"),
                    tool_sequence_assertions=[
                        ToolSequenceAssertion(
                            assertion_id=a["assertion_id"],
                            expected_sequence=a["expected_sequence"],
                            expected_outcome=ToolSequenceOutcome(a["expected_outcome"]),
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                        )
                        for a in data.get("tool_sequence_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
