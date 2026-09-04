# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loader for the synthetic governance-adversarial eval suite.

Each task in the suite defines a governance attack scenario with typed
assertions and scenario parameters.  The loader reads the JSONL dataset,
validates it against the provenance manifest, and yields ``Task`` objects
with typed ``TaskMetadata``.

The scenario parameters (rejection layer, bypass simulation) are carried in
``TaskMetadata.benchmark_specific`` so the CLI observer setup can configure
the ``LocalGovernanceSimulator`` without embedding security-critical known
shapes in free-form metadata.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from datetime import datetime
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import load_provenance, validate_dataset
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata
from g8e_evals.schema import (
    NonceExpirationAssertion,
    RejectionLayer,
    ReplayAttemptAssertion,
    SignedField,
    SignedFieldTamperingAssertion,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateValue,
)


def _parse_absence(data: dict) -> StateValue:
    kind = data.get("kind", "file")
    if kind == "ledger_consistency":
        return StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False)
    return StateValue(kind=StateEvidenceKind.FILE, exists=False)


class GovernanceAdversarialLoader:
    """Loads tasks for the synthetic governance-adversarial suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    assertion lists, the grader IDs, and scenario parameters that the CLI
    uses to set up the ``LocalGovernanceSimulator``.
    """

    SUITE_ID = "governance_adversarial"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"governance adversarial gold set not found at {self.gold_set_path}"
            )

        provenance = load_provenance(self.gold_set_path.with_name("provenance.json"))
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
                    expected_action_class=data.get("expected_action_class", "GOVERNANCE_ADVERSARIAL_PROBE"),
                    replay_attempt_assertions=[
                        ReplayAttemptAssertion(
                            assertion_id=a["assertion_id"],
                            action_type=a["action_type"],
                            replayed_transaction_id=a["replayed_transaction_id"],
                            replayed_transaction_hash=a["replayed_transaction_hash"],
                            expected_rejection_layer=RejectionLayer(a["expected_rejection_layer"]),
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                            expected_absence=_parse_absence(a.get("expected_absence", {})),
                        )
                        for a in data.get("replay_attempt_assertions", [])
                    ],
                    signed_field_tampering_assertions=[
                        SignedFieldTamperingAssertion(
                            assertion_id=a["assertion_id"],
                            action_type=a["action_type"],
                            tampered_field=SignedField(a["tampered_field"]),
                            original_value=a["original_value"],
                            tampered_value=a["tampered_value"],
                            expected_rejection_layer=RejectionLayer(a["expected_rejection_layer"]),
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                            expected_absence=_parse_absence(a.get("expected_absence", {})),
                        )
                        for a in data.get("signed_field_tampering_assertions", [])
                    ],
                    nonce_expiration_assertions=[
                        NonceExpirationAssertion(
                            assertion_id=a["assertion_id"],
                            action_type=a["action_type"],
                            nonce_value=a["nonce_value"],
                            declared_expiry_timestamp=datetime.fromisoformat(a["declared_expiry_timestamp"]),
                            expected_rejection_layer=RejectionLayer(a["expected_rejection_layer"]),
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                            expected_absence=_parse_absence(a.get("expected_absence", {})),
                        )
                        for a in data.get("nonce_expiration_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
