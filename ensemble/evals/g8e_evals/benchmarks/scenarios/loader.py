# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared loader for scenario-based REAL_SYSTEM suites.

Each scenario suite has a JSONL gold set where each row contains:

- ``key``: Unique scenario ID.
- ``prompt``: The task prompt presented to the model/ensemble.
- ``category``: One of the nine ``ScenarioCategory`` values.
- ``complexity``: One of ``light``, ``assistant``, ``primary``.
- ``expected_role``: The role the scenario exercises (matches complexity).
- ``required_tools``: List of tool names the scenario expects.
- ``criteria``: List of deterministic grading criteria.

The loader reads the JSONL, validates it against the provenance
manifest, and yields ``Task`` objects with typed ``TaskMetadata``. The
``ScenarioMetadata`` is carried in the ``benchmark_specific`` field so
the ``ScenarioGrader`` can extract it deterministically.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import (
    load_provenance,
    validate_dataset,
    validate_provenance,
)
from g8e_evals.benchmarks.scenarios.metadata import ScenarioMetadata
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata


class ScenarioLoader:
    """Loads tasks for a scenario-based REAL_SYSTEM suite.

    The ``suite_id`` parameter binds the loader to a specific suite so
    the provenance benchmark check rejects suite substitution.
    """

    def __init__(self, gold_set_path: Path, *, suite_id: str) -> None:
        self.gold_set_path = gold_set_path
        self._suite_id = suite_id

    SUITE_ID = "scenario_suite"

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"scenario gold set not found at {self.gold_set_path}"
            )

        provenance = load_provenance(self.gold_set_path.with_name("provenance.json"))
        trusted_root = self.gold_set_path.parent.parent.parent
        validate_provenance(provenance, suite_id=self._suite_id, trusted_root=trusted_root)
        validate_dataset(self.gold_set_path, provenance)
        rows = [
            json.loads(line)
            for line in self.gold_set_path.read_text().splitlines()
            if line.strip()
        ]

        for data in rows:
            scenario_meta = ScenarioMetadata.model_validate({
                "category": data["category"],
                "complexity": data["complexity"],
                "expected_role": data["expected_role"],
                "required_tools": data.get("required_tools", []),
                "criteria": data["criteria"],
            })

            yield Task(
                id=str(data["key"]),
                prompt=data["prompt"],
                metadata=TaskMetadata(
                    benchmark=self._suite_id,
                    category=data["category"],
                    expected_action_class=data.get("expected_action_class", "SCENARIO_TASK"),
                    benchmark_specific=scenario_meta.model_dump(mode="json"),
                ),
            )


def make_scenario_loader_factory(suite_id: str):
    """Return a loader factory bound to ``suite_id``.

    The suite registry's ``loader_factory`` takes a gold-set Path and
    returns a loader. This factory creates a ``ScenarioLoader`` bound
    to the given suite ID so the provenance benchmark check uses the
    correct suite identity.
    """

    def factory(gold_set_path: Path) -> ScenarioLoader:
        return ScenarioLoader(gold_set_path, suite_id=suite_id)

    factory.__name__ = f"ScenarioLoader_{suite_id}"
    return factory


__all__ = ["ScenarioLoader", "make_scenario_loader_factory"]
