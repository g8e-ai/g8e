# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Suite loaders for the synthetic privacy eval suites.

Each task in a suite defines a privacy scenario with typed assertions
and scenario parameters.  The loader reads the JSONL dataset, validates
it against the provenance manifest, and yields ``Task`` objects with
typed ``TaskMetadata``.

The scenario parameters (token definitions, clock advancement, failure
injection, exfiltration destinations, artifact leak types, rehydration
tokens) are carried in ``TaskMetadata.benchmark_specific`` so the CLI
observer setup can configure the local systems under test without
embedding security-critical known shapes in free-form metadata.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.privacy.provenance import load_provenance, validate_dataset, validate_provenance
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata
from g8e_evals.schema import (
    ArtifactCollectionBoundary,
    ArtifactLeakageAssertion,
    ExfiltrationAttemptAssertion,
    RejectionLayer,
    RehydrationAssertion,
    SensitiveArtifactContentType,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateValue,
    TokenPersistenceFailureAssertion,
    TokenPersistenceFailureOutcome,
    TokenStorePersistenceAssertion,
    TokenTTLExpiryAssertion,
)


class PrivacyTokenLifecycleLoader:
    """Loads tasks for the synthetic privacy token lifecycle suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    assertion lists, the grader IDs, and scenario parameters that the
    CLI uses to set up the ``LocalEncryptedTokenStore``.
    """

    SUITE_ID = "privacy_token_lifecycle"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"privacy token lifecycle gold set not found at {self.gold_set_path}"
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
                    category=data.get("category", "privacy"),
                    expected_action_class=data.get("expected_action_class", "TOKEN_STORE_PRIVACY_PROBE"),
                    token_store_persistence_assertions=[
                        TokenStorePersistenceAssertion(
                            assertion_id=a["assertion_id"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "encrypted_token_store")
                            ),
                            expected_encryption_at_rest=a.get("expected_encryption_at_rest", True),
                            expected_fail_closed_on_lock=a.get("expected_fail_closed_on_lock", True),
                            expected_persistence_across_restart=a.get("expected_persistence_across_restart", True),
                            expected_ttl_seconds=a["expected_ttl_seconds"],
                            expected_restored_token_count=a["expected_restored_token_count"],
                        )
                        for a in data.get("token_store_persistence_assertions", [])
                    ],
                    token_ttl_expiry_assertions=[
                        TokenTTLExpiryAssertion(
                            assertion_id=a["assertion_id"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "encrypted_token_store")
                            ),
                            expected_ttl_seconds=a["expected_ttl_seconds"],
                            expected_visible_before_expiry=a.get("expected_visible_before_expiry", True),
                            expected_invisible_after_expiry=a.get("expected_invisible_after_expiry", True),
                            expected_expiry_tolerance_seconds=a.get("expected_expiry_tolerance_seconds", 0),
                        )
                        for a in data.get("token_ttl_expiry_assertions", [])
                    ],
                    token_persistence_failure_assertions=[
                        TokenPersistenceFailureAssertion(
                            assertion_id=a["assertion_id"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "encrypted_token_store")
                            ),
                            expected_fail_closed=a.get("expected_fail_closed", True),
                            expected_rollback=a.get("expected_rollback", True),
                            expected_no_sensitive_leak=a.get("expected_no_sensitive_leak", True),
                            expected_no_unsafe_continuation=a.get("expected_no_unsafe_continuation", True),
                            expected_failure_outcome=TokenPersistenceFailureOutcome(
                                a.get("expected_failure_outcome", "storage_failure")
                            ),
                        )
                        for a in data.get("token_persistence_failure_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )


def _parse_absence(data: dict) -> StateValue:
    kind = data.get("kind", "file")
    if kind == "ledger_consistency":
        return StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False)
    return StateValue(kind=StateEvidenceKind.FILE, exists=False)


class PrivacyBoundaryLeakageLoader:
    """Loads tasks for the synthetic privacy boundary leakage suite.

    Each JSONL row contains a ``key``, a ``description``, the typed
    assertion lists (exfiltration attempt, artifact leakage, rehydration),
    the grader IDs, and scenario parameters that the CLI uses to set up the
    ``LocalExfiltrationSimulator``, ``LocalArtifactEmitter``, and
    ``LocalRehydrationArtifact``.
    """

    SUITE_ID = "privacy_boundary_leakage"

    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        if not self.gold_set_path.exists():
            raise FileNotFoundError(
                f"privacy boundary leakage gold set not found at {self.gold_set_path}"
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
                    category=data.get("category", "privacy"),
                    expected_action_class=data.get(
                        "expected_action_class", "PRIVACY_BOUNDARY_LEAKAGE_PROBE"
                    ),
                    exfiltration_attempt_assertions=[
                        ExfiltrationAttemptAssertion(
                            assertion_id=a["assertion_id"],
                            action_type=a["action_type"],
                            source=a["source"],
                            destination=a["destination"],
                            collection_boundary=StateCollectionBoundary(
                                a.get("collection_boundary", "operator_workload")
                            ),
                            expected_rejection_layer=RejectionLayer(a["expected_rejection_layer"]),
                            expected_absence=_parse_absence(a.get("expected_absence", {})),
                        )
                        for a in data.get("exfiltration_attempt_assertions", [])
                    ],
                    artifact_leakage_assertions=[
                        ArtifactLeakageAssertion(
                            assertion_id=a["assertion_id"],
                            artifact_class=a["artifact_class"],
                            collection_boundary=ArtifactCollectionBoundary(
                                a.get("collection_boundary", "report_directory")
                            ),
                            expected_absent_sensitive_types=[
                                SensitiveArtifactContentType(t)
                                for t in a.get("expected_absent_sensitive_types", [])
                            ],
                            expected_artifact_present=a.get("expected_artifact_present", True),
                        )
                        for a in data.get("artifact_leakage_assertions", [])
                    ],
                    rehydration_assertions=[
                        RehydrationAssertion(
                            assertion_id=a["assertion_id"],
                            source=a["source"],
                            input_artifact_sha256=a["input_artifact_sha256"],
                            expected_output_artifact_sha256=a["expected_output_artifact_sha256"],
                            expected_token_count=a["expected_token_count"],
                            expected_sensitive_types=a.get("expected_sensitive_types", []),
                        )
                        for a in data.get("rehydration_assertions", [])
                    ],
                    benchmark_specific=data.get("scenario_params", {}),
                ),
            )
