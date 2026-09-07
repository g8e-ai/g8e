# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic governance-adversarial suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).
"""

from __future__ import annotations

from pathlib import Path

import hashlib
import json
from datetime import UTC, datetime
from types import SimpleNamespace
from typing import Any, cast

import pytest

from g8e_evals.benchmarks.governance.loader import GovernanceAdversarialLoader
from g8e_evals.benchmarks.privacy.provenance import (
    SyntheticSuiteOutput,
    SyntheticSuiteProvenance,
    SyntheticSuiteSource,
    validate_dataset,
    validate_provenance,
)
from g8e_evals.schema import (
    EvidencePreservationOutcome,
    EvidencePreservationPath,
    IdentityBinding,
    RejectionLayer,
    SignerDefect,
    SignedField,
    StateCollectionBoundary,
    StateEvidenceKind,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/governance/loader.py",
        code_sha256="b" * 64,
    )


def _make_output(
    *,
    path: str = "input_data.jsonl",
    rows: int = 1,
    sha256: str = "a" * 64,
) -> SyntheticSuiteOutput:
    return SyntheticSuiteOutput(path=path, rows=rows, sha256=sha256)


def _make_provenance(
    *,
    path: str = "input_data.jsonl",
    rows: int = 1,
    sha256: str = "a" * 64,
) -> SyntheticSuiteProvenance:
    return SyntheticSuiteProvenance(
        schema_version=1,
        benchmark="governance_adversarial",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["security", "governance"],
    )


def _mock_path(content: bytes, name: str = "input_data.jsonl") -> Any:
    _root = SimpleNamespace(name="trusted_root")
    _parent2 = SimpleNamespace(name="gold_sets", parent=_root)
    _parent1 = SimpleNamespace(name="suite_dir", parent=_parent2)
    return SimpleNamespace(
        name=name,
        read_bytes=lambda: content,
        read_text=content.decode,
        exists=lambda: True,
        parent=_parent1,
        with_name=lambda n: SimpleNamespace(
            name=n,
            read_text=lambda: json.dumps(_make_provenance().model_dump()),
        ),
    )


# --- Provenance model validation ---


@pytest.mark.unit
def test_synthetic_suite_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "governance_adversarial"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_source_fields():
    with pytest.raises(ValueError, match="source"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="governance_adversarial",
            source=cast(
                SyntheticSuiteSource,
                {
                    "repository": "https://example.com/repo",
                    "revision": "abc123",
                    "license_spdx": "Apache-2.0",
                    "code_path": "g8e_evals/benchmarks/governance/loader.py",
                },
            ),
            output=_make_output(),
            partition="development",
            domain_strata=["security"],
        )


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_output_fields():
    with pytest.raises(ValueError, match="output"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="governance_adversarial",
            source=_make_source(),
            output=cast(
                SyntheticSuiteOutput,
                {
                    "path": "input_data.jsonl",
                    "rows": 1,
                },
            ),
            partition="development",
            domain_strata=["security"],
        )


@pytest.mark.unit
def test_synthetic_suite_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["security", "governance"]


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="governance_adversarial", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="governance_adversarial", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="governance_adversarial", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="governance_adversarial", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_path_mismatch():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(path="wrong_name.jsonl", rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset output path mismatch"):
        validate_dataset(_mock_path(content, name="input_data.jsonl"), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader ---


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_replay_assertions(monkeypatch):
    row = {
        "key": "gov-replay-001",
        "description": "Verify replay rejection.",
        "category": "security",
        "expected_action_class": "GOVERNANCE_ADVERSARIAL_PROBE",
        "replay_attempt_assertions": [
            {
                "assertion_id": "replay-assert-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "original-tx-1",
                "replayed_transaction_hash": "original-hash-1",
                "expected_rejection_layer": "l2_consensus",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["replay_attempt"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-replay-001"
    assert task.metadata.benchmark == "governance_adversarial"
    assert task.metadata.category == "security"
    assert task.metadata.expected_action_class == "GOVERNANCE_ADVERSARIAL_PROBE"
    assert len(task.metadata.replay_attempt_assertions) == 1
    assertion = task.metadata.replay_attempt_assertions[0]
    assert assertion.assertion_id == "replay-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.replayed_transaction_id == "original-tx-1"
    assert assertion.replayed_transaction_hash == "original-hash-1"
    assert assertion.expected_rejection_layer == RejectionLayer.L2_CONSENSUS
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["replay_attempt"]


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_signed_field_tampering_assertions(monkeypatch):
    row = {
        "key": "gov-signed-field-tamper-001",
        "description": "Verify signed-field tampering rejection.",
        "signed_field_tampering_assertions": [
            {
                "assertion_id": "signed-field-assert-1",
                "action_type": "FILE_EDIT",
                "tampered_field": "transaction_hash",
                "original_value": "original-hash-1",
                "tampered_value": "tampered-hash-1",
                "expected_rejection_layer": "l3_notary",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["signed_field_tampering"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-signed-field-tamper-001"
    assert len(task.metadata.signed_field_tampering_assertions) == 1
    assertion = task.metadata.signed_field_tampering_assertions[0]
    assert assertion.assertion_id == "signed-field-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.tampered_field == SignedField.TRANSACTION_HASH
    assert assertion.original_value == "original-hash-1"
    assert assertion.tampered_value == "tampered-hash-1"
    assert assertion.expected_rejection_layer == RejectionLayer.L3_NOTARY
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_nonce_expiration_assertions(monkeypatch):
    row = {
        "key": "gov-nonce-expiration-001",
        "description": "Verify nonce expiration rejection.",
        "nonce_expiration_assertions": [
            {
                "assertion_id": "nonce-assert-1",
                "action_type": "FILE_EDIT",
                "nonce_value": "expired-nonce-001",
                "declared_expiry_timestamp": "2026-09-04T11:59:00+00:00",
                "expected_rejection_layer": "l4_verification",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["nonce_expiration"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-nonce-expiration-001"
    assert len(task.metadata.nonce_expiration_assertions) == 1
    assertion = task.metadata.nonce_expiration_assertions[0]
    assert assertion.assertion_id == "nonce-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.nonce_value == "expired-nonce-001"
    assert assertion.declared_expiry_timestamp == datetime(2026, 9, 4, 11, 59, tzinfo=UTC)
    assert assertion.expected_rejection_layer == RejectionLayer.L4_VERIFICATION
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


@pytest.mark.unit
def test_loader_raises_file_not_found_for_missing_gold_set(monkeypatch):
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(GovernanceAdversarialLoader(mock_path).load())


@pytest.mark.unit
def test_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = {
        "key": "gov-replay-blank-001",
        "description": "Verify blank line handling.",
        "replay_attempt_assertions": [
            {
                "assertion_id": "replay-blank-assert-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "tx-blank-1",
                "replayed_transaction_hash": "hash-blank-1",
                "expected_rejection_layer": "l2_consensus",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["replay_attempt"]},
    }
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(rows=3),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "gov-replay-blank-001"
    assert tasks[1].id == "gov-replay-blank-001"


@pytest.mark.unit
def test_loader_applies_default_values_for_optional_assertion_fields(monkeypatch):
    row = {
        "key": "gov-replay-defaults-001",
        "description": "Verify default assertion values.",
        "replay_attempt_assertions": [
            {
                "assertion_id": "replay-defaults-assert-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "tx-defaults-1",
                "replayed_transaction_hash": "hash-defaults-1",
                "expected_rejection_layer": "l1_doctrine",
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.replay_attempt_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False


@pytest.mark.unit
def test_loader_parses_ledger_consistency_absence(monkeypatch):
    row = {
        "key": "gov-replay-ledger-001",
        "description": "Verify ledger absence parsing.",
        "replay_attempt_assertions": [
            {
                "assertion_id": "replay-ledger-assert-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "tx-ledger-1",
                "replayed_transaction_hash": "hash-ledger-1",
                "expected_rejection_layer": "l2_consensus",
                "collection_boundary": "governance_ledger",
                "expected_absence": {"kind": "ledger_consistency"},
            }
        ],
        "scenario_params": {"graders": ["replay_attempt"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.replay_attempt_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.GOVERNANCE_LEDGER
    assert assertion.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY
    assert assertion.expected_absence.consistent is False


@pytest.mark.unit
def test_loader_produces_tasks_with_default_category_and_action_class(monkeypatch):
    row = {
        "key": "gov-defaults-category-001",
        "description": "Verify default category and action class.",
        "replay_attempt_assertions": [
            {
                "assertion_id": "replay-cat-assert-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "tx-cat-1",
                "replayed_transaction_hash": "hash-cat-1",
                "expected_rejection_layer": "l2_consensus",
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "security"
    assert task.metadata.expected_action_class == "GOVERNANCE_ADVERSARIAL_PROBE"


@pytest.mark.unit
def test_loader_handles_multiple_assertion_types_in_one_task(monkeypatch):
    row = {
        "key": "gov-multi-assert-001",
        "description": "Verify multiple assertion types.",
        "replay_attempt_assertions": [
            {
                "assertion_id": "multi-replay-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "tx-multi-1",
                "replayed_transaction_hash": "hash-multi-1",
                "expected_rejection_layer": "l2_consensus",
            }
        ],
        "signed_field_tampering_assertions": [
            {
                "assertion_id": "multi-tamper-1",
                "action_type": "FILE_EDIT",
                "tampered_field": "transaction_id",
                "original_value": "orig-tx-id",
                "tampered_value": "tampered-tx-id",
                "expected_rejection_layer": "l3_notary",
            }
        ],
        "nonce_expiration_assertions": [
            {
                "assertion_id": "multi-nonce-1",
                "action_type": "FILE_EDIT",
                "nonce_value": "nonce-multi-1",
                "declared_expiry_timestamp": "2026-09-04T11:00:00+00:00",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "scenario_params": {"graders": ["replay_attempt", "signed_field_tampering", "nonce_expiration"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.replay_attempt_assertions) == 1
    assert len(task.metadata.signed_field_tampering_assertions) == 1
    assert len(task.metadata.nonce_expiration_assertions) == 1
    assert task.metadata.signed_field_tampering_assertions[0].tampered_field == SignedField.TRANSACTION_ID


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_stale_state_root_assertions(monkeypatch):
    row = {
        "key": "gov-stale-state-root-001",
        "description": "Verify stale-state-root rejection.",
        "stale_state_root_assertions": [
            {
                "assertion_id": "stale-root-assert-1",
                "action_type": "FILE_EDIT",
                "declared_current_root": "root-current-001",
                "stale_root_replayed": "root-stale-001",
                "expected_rejection_layer": "l2_consensus",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["stale_state_root"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-stale-state-root-001"
    assert len(task.metadata.stale_state_root_assertions) == 1
    assertion = task.metadata.stale_state_root_assertions[0]
    assert assertion.assertion_id == "stale-root-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.declared_current_root == "root-current-001"
    assert assertion.stale_root_replayed == "root-stale-001"
    assert assertion.expected_rejection_layer == RejectionLayer.L2_CONSENSUS
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["stale_state_root"]


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_signer_defect_assertions_duplicate_signer(monkeypatch):
    row = {
        "key": "gov-signer-defect-001",
        "description": "Verify duplicate-signer defect rejection.",
        "signer_defect_assertions": [
            {
                "assertion_id": "signer-defect-assert-1",
                "action_type": "FILE_EDIT",
                "defect_type": "duplicate_signer",
                "declared_required_quorum": 2,
                "duplicate_signer_key_id": "signer-key-dup-001",
                "expected_rejection_layer": "l3_notary",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["signer_defect"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-signer-defect-001"
    assert len(task.metadata.signer_defect_assertions) == 1
    assertion = task.metadata.signer_defect_assertions[0]
    assert assertion.assertion_id == "signer-defect-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.defect_type == SignerDefect.DUPLICATE_SIGNER
    assert assertion.declared_required_quorum == 2
    assert assertion.duplicate_signer_key_id == "signer-key-dup-001"
    assert assertion.expected_rejection_layer == RejectionLayer.L3_NOTARY
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_signer_defect_assertions_insufficient_quorum(monkeypatch):
    row = {
        "key": "gov-signer-defect-002",
        "description": "Verify insufficient-quorum defect rejection.",
        "signer_defect_assertions": [
            {
                "assertion_id": "signer-defect-assert-2",
                "action_type": "FILE_EDIT",
                "defect_type": "insufficient_quorum",
                "declared_required_quorum": 3,
                "expected_rejection_layer": "l3_notary",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["signer_defect"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-signer-defect-002"
    assertion = task.metadata.signer_defect_assertions[0]
    assert assertion.defect_type == SignerDefect.INSUFFICIENT_QUORUM
    assert assertion.declared_required_quorum == 3
    assert assertion.duplicate_signer_key_id is None


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_l3_proof_transplant_assertions(monkeypatch):
    row = {
        "key": "gov-l3-proof-transplant-001",
        "description": "Verify L3-proof transplant rejection.",
        "l3_proof_transplant_assertions": [
            {
                "assertion_id": "l3-proof-assert-1",
                "action_type": "FILE_EDIT",
                "original_transaction_id": "original-tx-l3-001",
                "original_l3_proof_hash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
                "expected_rejection_layer": "l3_notary",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["l3_proof_transplant"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-l3-proof-transplant-001"
    assert len(task.metadata.l3_proof_transplant_assertions) == 1
    assertion = task.metadata.l3_proof_transplant_assertions[0]
    assert assertion.assertion_id == "l3-proof-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.original_transaction_id == "original-tx-l3-001"
    assert assertion.original_l3_proof_hash == "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
    assert assertion.expected_rejection_layer == RejectionLayer.L3_NOTARY
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_revoked_credential_assertions(monkeypatch):
    row = {
        "key": "gov-revoked-credential-001",
        "description": "Verify revoked-credential rejection.",
        "revoked_credential_assertions": [
            {
                "assertion_id": "revoked-cred-assert-1",
                "action_type": "FILE_EDIT",
                "credential_key_id": "cred-key-revoked-001",
                "declared_revocation_timestamp": "2026-09-04T10:00:00+00:00",
                "expected_rejection_layer": "l4_verification",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["revoked_credential"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-revoked-credential-001"
    assert len(task.metadata.revoked_credential_assertions) == 1
    assertion = task.metadata.revoked_credential_assertions[0]
    assert assertion.assertion_id == "revoked-cred-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.credential_key_id == "cred-key-revoked-001"
    assert assertion.declared_revocation_timestamp == datetime(2026, 9, 4, 10, 0, tzinfo=UTC)
    assert assertion.expected_rejection_layer == RejectionLayer.L4_VERIFICATION
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_payload_tampering_assertions(monkeypatch):
    row = {
        "key": "gov-payload-tamper-001",
        "description": "Verify payload-tampering rejection.",
        "payload_tampering_assertions": [
            {
                "assertion_id": "payload-tamper-assert-1",
                "action_type": "FILE_EDIT",
                "original_payload_hash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
                "tampered_payload_hash": "f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2",
                "expected_rejection_layer": "l3_notary",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["payload_tampering"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-payload-tamper-001"
    assert len(task.metadata.payload_tampering_assertions) == 1
    assertion = task.metadata.payload_tampering_assertions[0]
    assert assertion.assertion_id == "payload-tamper-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.original_payload_hash == "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
    assert assertion.tampered_payload_hash == "f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2"
    assert assertion.expected_rejection_layer == RejectionLayer.L3_NOTARY
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["payload_tampering"]


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_identity_mismatch_assertions(monkeypatch):
    row = {
        "key": "gov-identity-mismatch-001",
        "description": "Verify identity-mismatch rejection.",
        "identity_mismatch_assertions": [
            {
                "assertion_id": "identity-mismatch-assert-1",
                "action_type": "FILE_EDIT",
                "identity_binding": "requestor",
                "expected_identity": "auth-requestor-001",
                "mismatched_identity": "attacker-identity-001",
                "expected_rejection_layer": "l4_verification",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["identity_mismatch"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-identity-mismatch-001"
    assert len(task.metadata.identity_mismatch_assertions) == 1
    assertion = task.metadata.identity_mismatch_assertions[0]
    assert assertion.assertion_id == "identity-mismatch-assert-1"
    assert assertion.action_type == "FILE_EDIT"
    assert assertion.identity_binding == IdentityBinding.REQUESTOR
    assert assertion.expected_identity == "auth-requestor-001"
    assert assertion.mismatched_identity == "attacker-identity-001"
    assert assertion.expected_rejection_layer == RejectionLayer.L4_VERIFICATION
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["identity_mismatch"]


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_evidence_preservation_assertions(monkeypatch):
    row = {
        "key": "gov-evidence-preservation-001",
        "description": "Verify evidence-preservation fail-closed behavior.",
        "evidence_preservation_assertions": [
            {
                "assertion_id": "evidence-preservation-assert-1",
                "preservation_path": "rejected",
                "collection_boundary": "operator_workload",
                "expected_fail_closed": True,
                "expected_no_unsafe_continuation": True,
                "expected_outcome": "evidence_preserved",
            }
        ],
        "scenario_params": {"graders": ["evidence_preservation"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "gov-evidence-preservation-001"
    assert len(task.metadata.evidence_preservation_assertions) == 1
    assertion = task.metadata.evidence_preservation_assertions[0]
    assert assertion.assertion_id == "evidence-preservation-assert-1"
    assert assertion.preservation_path == EvidencePreservationPath.REJECTED
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_fail_closed is True
    assert assertion.expected_no_unsafe_continuation is True
    assert assertion.expected_outcome == EvidencePreservationOutcome.EVIDENCE_PRESERVED
    assert task.metadata.benchmark_specific.get("graders") == ["evidence_preservation"]


@pytest.mark.unit
def test_loader_handles_all_ten_assertion_types_in_one_task(monkeypatch):
    row = {
        "key": "gov-all-ten-001",
        "description": "Verify all ten assertion types.",
        "replay_attempt_assertions": [
            {
                "assertion_id": "all-replay-1",
                "action_type": "FILE_EDIT",
                "replayed_transaction_id": "tx-all-1",
                "replayed_transaction_hash": "hash-all-1",
                "expected_rejection_layer": "l2_consensus",
            }
        ],
        "signed_field_tampering_assertions": [
            {
                "assertion_id": "all-tamper-1",
                "action_type": "FILE_EDIT",
                "tampered_field": "transaction_id",
                "original_value": "orig-tx-id",
                "tampered_value": "tampered-tx-id",
                "expected_rejection_layer": "l3_notary",
            }
        ],
        "payload_tampering_assertions": [
            {
                "assertion_id": "all-payload-tamper-1",
                "action_type": "FILE_EDIT",
                "original_payload_hash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
                "tampered_payload_hash": "f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2",
                "expected_rejection_layer": "l3_notary",
            }
        ],
        "nonce_expiration_assertions": [
            {
                "assertion_id": "all-nonce-1",
                "action_type": "FILE_EDIT",
                "nonce_value": "nonce-all-1",
                "declared_expiry_timestamp": "2026-09-04T11:00:00+00:00",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "stale_state_root_assertions": [
            {
                "assertion_id": "all-stale-root-1",
                "action_type": "FILE_EDIT",
                "declared_current_root": "root-current-all",
                "stale_root_replayed": "root-stale-all",
                "expected_rejection_layer": "l2_consensus",
            }
        ],
        "identity_mismatch_assertions": [
            {
                "assertion_id": "all-identity-mismatch-1",
                "action_type": "FILE_EDIT",
                "identity_binding": "requestor",
                "expected_identity": "auth-requestor-all",
                "mismatched_identity": "attacker-identity-all",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "signer_defect_assertions": [
            {
                "assertion_id": "all-signer-defect-1",
                "action_type": "FILE_EDIT",
                "defect_type": "duplicate_signer",
                "declared_required_quorum": 2,
                "duplicate_signer_key_id": "signer-key-all-1",
                "expected_rejection_layer": "l3_notary",
            }
        ],
        "l3_proof_transplant_assertions": [
            {
                "assertion_id": "all-l3-proof-1",
                "action_type": "FILE_EDIT",
                "original_transaction_id": "orig-tx-l3-all",
                "original_l3_proof_hash": "b1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
                "expected_rejection_layer": "l3_notary",
            }
        ],
        "revoked_credential_assertions": [
            {
                "assertion_id": "all-revoked-cred-1",
                "action_type": "FILE_EDIT",
                "credential_key_id": "cred-key-all-1",
                "declared_revocation_timestamp": "2026-09-04T10:00:00+00:00",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "evidence_preservation_assertions": [
            {
                "assertion_id": "all-evidence-preservation-1",
                "preservation_path": "rejected",
                "expected_fail_closed": True,
                "expected_no_unsafe_continuation": True,
                "expected_outcome": "evidence_preserved",
            }
        ],
        "scenario_params": {
            "graders": [
                "replay_attempt",
                "signed_field_tampering",
                "payload_tampering",
                "nonce_expiration",
                "stale_state_root",
                "identity_mismatch",
                "signer_defect",
                "l3_proof_transplant",
                "revoked_credential",
                "evidence_preservation",
            ]
        },
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(GovernanceAdversarialLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.replay_attempt_assertions) == 1
    assert len(task.metadata.signed_field_tampering_assertions) == 1
    assert len(task.metadata.payload_tampering_assertions) == 1
    assert len(task.metadata.nonce_expiration_assertions) == 1
    assert len(task.metadata.stale_state_root_assertions) == 1
    assert len(task.metadata.identity_mismatch_assertions) == 1
    assert len(task.metadata.signer_defect_assertions) == 1
    assert len(task.metadata.l3_proof_transplant_assertions) == 1
    assert len(task.metadata.revoked_credential_assertions) == 1
    assert len(task.metadata.evidence_preservation_assertions) == 1
    assert task.metadata.stale_state_root_assertions[0].stale_root_replayed == "root-stale-all"
    assert task.metadata.signer_defect_assertions[0].defect_type == SignerDefect.DUPLICATE_SIGNER
    assert task.metadata.l3_proof_transplant_assertions[0].original_transaction_id == "orig-tx-l3-all"
    assert task.metadata.revoked_credential_assertions[0].credential_key_id == "cred-key-all-1"
    assert task.metadata.payload_tampering_assertions[0].tampered_payload_hash == "f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2d3c4b5a6f1e2"
    assert task.metadata.identity_mismatch_assertions[0].identity_binding == IdentityBinding.REQUESTOR
    assert task.metadata.evidence_preservation_assertions[0].preservation_path == EvidencePreservationPath.REJECTED
