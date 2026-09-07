# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic citation-backed answer eval suite.

Each task in the suite defines a citation that the model must produce to
back its answer, with typed ``CitationBackedAssertion`` records.  The
loader reads the JSONL dataset, validates it against the provenance
manifest, and yields ``Task`` objects with typed ``TaskMetadata``.

The scenario parameters (expected citation, match type, observed citation)
are carried in the typed assertion fields and the ``benchmark_specific``
scenario params so the CLI observer setup can configure the
``LocalCitationBackedSimulator`` without embedding utility-critical known
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
    CitationBackedAssertion,
    CitationMatchType,
    StateCollectionBoundary,
)


class CitationBackedLoader:
    """Loads tasks for the synthetic citation-backed suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    ``citation_backed_assertions`` list, and scenario parameters that the
    CLI uses to set up the ``LocalCitationBackedSimulator``.
    """

    SUITE_ID = "citation_backed"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"citation-backed gold set not found at {self.gold_set_path}"
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
                    expected_action_class=data.get("expected_action_class", "CITATION_BACKED"),
                    citation_backed_assertions=[
                        CitationBackedAssertion(
                            assertion_id=a["assertion_id"],
                            expected_citation=a["expected_citation"],
                            match_type=CitationMatchType(a["match_type"]),
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                        )
                        for a in data.get("citation_backed_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
