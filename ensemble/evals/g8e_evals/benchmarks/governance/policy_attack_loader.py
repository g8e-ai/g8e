# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic policy-violating attack eval suite.

Each task in the suite defines a policy-violating attack scenario with typed
``PolicyAttackAssertion`` records.  The loader reads the JSONL dataset,
validates it against the provenance manifest, and yields ``Task`` objects
with typed ``TaskMetadata``.

The scenario parameters (expected outcome, rejection layer, attack type) are
carried in the typed assertion fields so the CLI observer setup can configure
the ``LocalGovernanceSimulator`` without embedding security-critical known
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
    AttackSeverity,
    AttackType,
    PolicyAttackAssertion,
    PolicyOutcome,
    RejectionLayer,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateValue,
)


def _parse_absence(data: dict) -> StateValue:
    kind = data.get("kind", "file")
    if kind == "ledger_consistency":
        return StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False)
    return StateValue(kind=StateEvidenceKind.FILE, exists=False)


class PolicyAttackLoader:
    """Loads tasks for the synthetic policy-violating attack suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    ``policy_attack_assertions`` list, and scenario parameters that the CLI
    uses to set up the ``LocalGovernanceSimulator``.
    """

    SUITE_ID = "policy_attack"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"policy attack gold set not found at {self.gold_set_path}"
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
                    category=data.get("category", "security"),
                    expected_action_class=data.get("expected_action_class", "POLICY_ATTACK_PROBE"),
                    policy_attack_assertions=[
                        PolicyAttackAssertion(
                            assertion_id=a["assertion_id"],
                            attack_type=AttackType(a["attack_type"]),
                            action_type=a["action_type"],
                            expected_outcome=PolicyOutcome(a["expected_outcome"]),
                            expected_rejection_layer=(
                                RejectionLayer(a["expected_rejection_layer"])
                                if a.get("expected_rejection_layer")
                                else None
                            ),
                            severity=AttackSeverity(a.get("severity", "high")),
                            prohibited_terminal_state=a["prohibited_terminal_state"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                            expected_absence=_parse_absence(a.get("expected_absence", {})),
                        )
                        for a in data.get("policy_attack_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
