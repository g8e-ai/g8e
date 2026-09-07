# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic ledger-consistency eval suite.

Each task in the suite declares a ``state_fixture`` with
``StateEvidenceKind.LEDGER_CONSISTENCY`` state assertions.  The loader
reads the JSONL dataset, validates it against the provenance manifest,
and yields ``Task`` objects with typed ``TaskMetadata`` carrying the
``state_fixture`` and ``initial_state_fixture_hash``.

The scenario parameters (ledger payloads, optional inconsistency
injection) are carried in the ``benchmark_specific`` scenario params so
the CLI observer setup can configure the
``LocalLedgerConsistencySimulator`` without embedding utility-critical
known shapes in free-form metadata.  The ledger payloads are simulator
inputs, not security- or privacy-critical known shapes, so they remain
in ``benchmark_specific`` alongside the grader list.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import load_provenance, validate_dataset, validate_provenance
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata
from g8e_evals.schema import (
    StateAssertion,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateFixtureDefinition,
    StateValue,
)


class LedgerConsistencyLoader:
    """Loads tasks for the synthetic ledger-consistency suite.

    Each JSONL row contains a ``key``, a ``description``, a typed
    ``state_fixture`` with ledger-consistency assertions, and scenario
    parameters that the CLI uses to set up the
    ``LocalLedgerConsistencySimulator``.
    """

    SUITE_ID = "ledger_consistency"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"ledger-consistency gold set not found at {self.gold_set_path}"
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
            fixture_data = data.get("state_fixture")
            state_fixture = (
                self._parse_state_fixture(fixture_data)
                if fixture_data is not None
                else None
            )
            yield Task(
                id=str(data["key"]),
                prompt=data["description"],
                metadata=TaskMetadata(
                    benchmark=self.SUITE_ID,
                    category=data.get("category", "utility"),
                    expected_action_class=data.get("expected_action_class", "LEDGER_CONSISTENCY"),
                    state_fixture=state_fixture,
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )

    @staticmethod
    def _parse_state_fixture(fixture_data: dict) -> StateFixtureDefinition:
        assertions = [
            StateAssertion(
                assertion_id=a["assertion_id"],
                action_type=a["action_type"],
                collection_boundary=StateCollectionBoundary(a["collection_boundary"]),
                target=a["target"],
                expected=StateValue(
                    kind=StateEvidenceKind(a["expected"]["kind"]),
                    consistent=a["expected"].get("consistent"),
                    entry_count=a["expected"].get("entry_count"),
                    head_sha256=a["expected"].get("head_sha256"),
                ),
            )
            for a in fixture_data.get("assertions", [])
        ]
        return StateFixtureDefinition(
            fixture_id=fixture_data["fixture_id"],
            fixture_sha256=fixture_data["fixture_sha256"],
            assertions=assertions,
        )
