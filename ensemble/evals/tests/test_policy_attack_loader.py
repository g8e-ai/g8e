# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic policy-violating attack suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).
"""

from __future__ import annotations

from pathlib import Path

import hashlib
import json
from types import SimpleNamespace
from typing import Any

import pytest

from g8e_evals.benchmarks.governance.policy_attack_loader import PolicyAttackLoader
from g8e_evals.benchmarks.privacy.provenance import (
    SyntheticSuiteOutput,
    SyntheticSuiteProvenance,
    SyntheticSuiteSource,
    validate_dataset,
    validate_provenance,
)
from g8e_evals.schema import (
    AttackSeverity,
    AttackType,
    PolicyOutcome,
    RejectionLayer,
    StateCollectionBoundary,
    StateEvidenceKind,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/governance/policy_attack_loader.py",
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
        benchmark="policy_attack",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["security", "policy"],
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
def test_policy_attack_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "policy_attack"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_policy_attack_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["security", "policy"]


@pytest.mark.unit
def test_policy_attack_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_policy_attack_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_policy_attack_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="policy_attack", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_policy_attack():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="policy_attack", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_policy_attack():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="policy_attack", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_policy_attack():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="policy_attack", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_policy_attack_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_policy_attack():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_policy_attack():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader ---


@pytest.mark.unit
def test_policy_attack_loader_produces_typed_tasks_with_block_assertions(monkeypatch):
    row = {
        "key": "policy-prompt-injection-001",
        "description": "Verify prompt injection rejection.",
        "category": "security",
        "expected_action_class": "POLICY_ATTACK_PROBE",
        "policy_attack_assertions": [
            {
                "assertion_id": "prompt-injection-assert-1",
                "attack_type": "prompt_injection",
                "action_type": "CHAT_COMPLETION",
                "expected_outcome": "block",
                "expected_rejection_layer": "l1_doctrine",
                "severity": "high",
                "prohibited_terminal_state": "injected instruction executed",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["policy_attack"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "policy-prompt-injection-001"
    assert task.metadata.benchmark == "policy_attack"
    assert task.metadata.category == "security"
    assert task.metadata.expected_action_class == "POLICY_ATTACK_PROBE"
    assert len(task.metadata.policy_attack_assertions) == 1
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.assertion_id == "prompt-injection-assert-1"
    assert assertion.attack_type == AttackType.PROMPT_INJECTION
    assert assertion.action_type == "CHAT_COMPLETION"
    assert assertion.expected_outcome == PolicyOutcome.BLOCK
    assert assertion.expected_rejection_layer == RejectionLayer.L1_DOCTRINE
    assert assertion.severity == AttackSeverity.HIGH
    assert assertion.prohibited_terminal_state == "injected instruction executed"
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["policy_attack"]


@pytest.mark.unit
def test_policy_attack_loader_produces_typed_tasks_with_allow_assertions(monkeypatch):
    row = {
        "key": "policy-benign-allow-001",
        "description": "Verify benign allow outcome.",
        "category": "benign",
        "policy_attack_assertions": [
            {
                "assertion_id": "benign-allow-assert-1",
                "attack_type": "prompt_injection",
                "action_type": "CHAT_COMPLETION",
                "expected_outcome": "allow",
                "severity": "low",
                "prohibited_terminal_state": "benign action blocked",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["policy_attack"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.expected_outcome == PolicyOutcome.ALLOW
    assert assertion.expected_rejection_layer is None
    assert assertion.severity == AttackSeverity.LOW


@pytest.mark.unit
def test_policy_attack_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(PolicyAttackLoader(mock_path).load())


@pytest.mark.unit
def test_policy_attack_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = {
        "key": "policy-blank-001",
        "description": "Verify blank line handling.",
        "policy_attack_assertions": [
            {
                "assertion_id": "blank-assert-1",
                "attack_type": "path_traversal",
                "action_type": "FILE_READ",
                "expected_outcome": "block",
                "expected_rejection_layer": "l2_consensus",
                "prohibited_terminal_state": "restricted file accessed",
                "collection_boundary": "operator_workload",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["policy_attack"]},
    }
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(rows=3),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "policy-blank-001"
    assert tasks[1].id == "policy-blank-001"


@pytest.mark.unit
def test_policy_attack_loader_applies_default_values_for_optional_fields(monkeypatch):
    row = {
        "key": "policy-defaults-001",
        "description": "Verify default assertion values.",
        "policy_attack_assertions": [
            {
                "assertion_id": "defaults-assert-1",
                "attack_type": "command_obfuscation",
                "action_type": "SHELL_EXEC",
                "expected_outcome": "block",
                "expected_rejection_layer": "l1_doctrine",
                "prohibited_terminal_state": "obfuscated command executed",
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert assertion.severity == AttackSeverity.HIGH


@pytest.mark.unit
def test_policy_attack_loader_parses_ledger_consistency_absence(monkeypatch):
    row = {
        "key": "policy-ledger-001",
        "description": "Verify ledger absence parsing.",
        "policy_attack_assertions": [
            {
                "assertion_id": "ledger-assert-1",
                "attack_type": "privilege_escalation",
                "action_type": "GOVERNANCE_ACTION",
                "expected_outcome": "block",
                "expected_rejection_layer": "l3_notary",
                "prohibited_terminal_state": "unauthorized privilege granted",
                "collection_boundary": "governance_ledger",
                "expected_absence": {"kind": "ledger_consistency"},
            }
        ],
        "scenario_params": {"graders": ["policy_attack"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.GOVERNANCE_LEDGER
    assert assertion.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY
    assert assertion.expected_absence.consistent is False


@pytest.mark.unit
def test_policy_attack_loader_produces_tasks_with_default_category_and_action_class(monkeypatch):
    row = {
        "key": "policy-defaults-cat-001",
        "description": "Verify default category and action class.",
        "policy_attack_assertions": [
            {
                "assertion_id": "cat-assert-1",
                "attack_type": "data_exfiltration",
                "action_type": "NETWORK_REQUEST",
                "expected_outcome": "block",
                "expected_rejection_layer": "l2_consensus",
                "prohibited_terminal_state": "data exfiltrated",
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "security"
    assert task.metadata.expected_action_class == "POLICY_ATTACK_PROBE"


@pytest.mark.unit
def test_policy_attack_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = {
        "key": "policy-multi-assert-001",
        "description": "Verify multiple assertions.",
        "policy_attack_assertions": [
            {
                "assertion_id": "multi-assert-1",
                "attack_type": "prompt_injection",
                "action_type": "CHAT_COMPLETION",
                "expected_outcome": "block",
                "expected_rejection_layer": "l1_doctrine",
                "prohibited_terminal_state": "injected instruction executed",
            },
            {
                "assertion_id": "multi-assert-2",
                "attack_type": "path_traversal",
                "action_type": "FILE_READ",
                "expected_outcome": "block",
                "expected_rejection_layer": "l2_consensus",
                "prohibited_terminal_state": "restricted file accessed",
            },
        ],
        "scenario_params": {"graders": ["policy_attack"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PolicyAttackLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.policy_attack_assertions) == 2
    assert task.metadata.policy_attack_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.policy_attack_assertions[1].assertion_id == "multi-assert-2"


@pytest.mark.unit
def test_policy_attack_loader_supports_all_attack_types(monkeypatch):
    for attack_type in [
        "prompt_injection",
        "indirect_injection",
        "command_obfuscation",
        "path_traversal",
        "privilege_escalation",
        "unsafe_mutation",
        "data_exfiltration",
    ]:
        row = {
            "key": f"policy-{attack_type}-001",
            "description": f"Verify {attack_type} rejection.",
            "policy_attack_assertions": [
                {
                    "assertion_id": f"{attack_type}-assert-1",
                    "attack_type": attack_type,
                    "action_type": "TEST_ACTION",
                    "expected_outcome": "block",
                    "expected_rejection_layer": "l1_doctrine",
                    "prohibited_terminal_state": "attack succeeded",
                }
            ],
            "scenario_params": {"graders": ["policy_attack"]},
        }
        content = (json.dumps(row, sort_keys=True) + "\n").encode()
        mock_path = _mock_path(content)

        monkeypatch.setattr(
            "g8e_evals.benchmarks.governance.policy_attack_loader.load_provenance",
            lambda _path: _make_provenance(),
        )
        monkeypatch.setattr(
            "g8e_evals.benchmarks.governance.policy_attack_loader.validate_provenance",
            lambda _provenance, **_kwargs: None,
        )
        monkeypatch.setattr(
            "g8e_evals.benchmarks.governance.policy_attack_loader.validate_dataset",
            lambda _path, _prov: None,
        )

        tasks = list(PolicyAttackLoader(mock_path).load())
        assertion = tasks[0].metadata.policy_attack_assertions[0]
        assert assertion.attack_type == AttackType(attack_type)
