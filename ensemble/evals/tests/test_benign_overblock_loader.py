# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic benign-overblock suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The benign-overblock suite complements the policy-violating attack suite:
every assertion declares ``expected_outcome = allow`` and no
``expected_rejection_layer``, so the ``PolicyAttackGrader`` checks that the
governance path allows the action and does not produce a false-positive
block.  Together the two suites form the allow/block confusion matrix.
"""

from __future__ import annotations

import hashlib
import json
from types import SimpleNamespace
from typing import Any

import pytest

from g8e_evals.benchmarks.governance.benign_overblock_loader import BenignOverblockLoader
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
    StateCollectionBoundary,
    StateEvidenceKind,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/governance/benign_overblock_loader.py",
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
        benchmark="benign_overblock",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["security", "policy"],
    )


def _mock_path(content: bytes, name: str = "input_data.jsonl") -> Any:
    return SimpleNamespace(
        name=name,
        read_bytes=lambda: content,
        read_text=content.decode,
        exists=lambda: True,
        with_name=lambda n: SimpleNamespace(
            name=n,
            read_text=lambda: json.dumps(_make_provenance().model_dump()),
        ),
    )


def _allow_assertion(assertion_id: str = "benign-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "attack_type": "prompt_injection",
        "action_type": "CHAT_COMPLETION",
        "expected_outcome": "allow",
        "severity": "low",
        "prohibited_terminal_state": "benign action blocked",
        "collection_boundary": "operator_workload",
        "expected_absence": {"kind": "file", "exists": False},
    }
    base.update(overrides)
    return base


def _benign_row(key: str = "benign-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "Verify a benign action is allowed.",
        "category": "benign",
        "expected_action_class": "BENIGN_OVERBLOCK_PROBE",
        "policy_attack_assertions": [_allow_assertion()],
        "scenario_params": {"graders": ["policy_attack"]},
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.benign_overblock_loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.governance.benign_overblock_loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_benign_overblock_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "benign_overblock"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_benign_overblock_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["security", "policy"]


@pytest.mark.unit
def test_benign_overblock_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_benign_overblock_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_benign_overblock_manifest():
    validate_provenance(_make_provenance())


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_benign_overblock():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance)


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_benign_overblock():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance)


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_benign_overblock():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance)


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_benign_overblock_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_benign_overblock():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_benign_overblock():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_benign_overblock_loader_produces_typed_tasks_with_allow_assertions(monkeypatch):
    row = _benign_row(
        key="benign-chat-001",
        description="Verify that a benign chat completion is allowed.",
        policy_attack_assertions=[_allow_assertion(
            assertion_id="benign-chat-assert-1",
            attack_type="prompt_injection",
            action_type="CHAT_COMPLETION",
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(BenignOverblockLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "benign-chat-001"
    assert task.metadata.benchmark == "benign_overblock"
    assert task.metadata.category == "benign"
    assert task.metadata.expected_action_class == "BENIGN_OVERBLOCK_PROBE"
    assert len(task.metadata.policy_attack_assertions) == 1
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.assertion_id == "benign-chat-assert-1"
    assert assertion.attack_type == AttackType.PROMPT_INJECTION
    assert assertion.action_type == "CHAT_COMPLETION"
    assert assertion.expected_outcome == PolicyOutcome.ALLOW
    assert assertion.expected_rejection_layer is None
    assert assertion.severity == AttackSeverity.LOW
    assert assertion.prohibited_terminal_state == "benign action blocked"
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["policy_attack"]


@pytest.mark.unit
def test_benign_overblock_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "benign-defaults-cat-001",
        "description": "Verify default category and action class.",
        "policy_attack_assertions": [_allow_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(BenignOverblockLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "benign"
    assert task.metadata.expected_action_class == "BENIGN_OVERBLOCK_PROBE"


@pytest.mark.unit
def test_benign_overblock_loader_applies_default_severity_and_collection_boundary(monkeypatch):
    row = {
        "key": "benign-defaults-001",
        "description": "Verify default assertion values.",
        "policy_attack_assertions": [{
            "assertion_id": "defaults-assert-1",
            "attack_type": "command_obfuscation",
            "action_type": "SHELL_EXEC",
            "expected_outcome": "allow",
            "prohibited_terminal_state": "benign action blocked",
        }],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(BenignOverblockLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert assertion.severity == AttackSeverity.LOW


# --- Loader: error paths ---


@pytest.mark.unit
def test_benign_overblock_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(BenignOverblockLoader(mock_path).load())


@pytest.mark.unit
def test_benign_overblock_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _benign_row(key="benign-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(BenignOverblockLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "benign-blank-001"
    assert tasks[1].id == "benign-blank-001"


# --- Loader: multiple assertions and attack types ---


@pytest.mark.unit
def test_benign_overblock_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _benign_row(
        key="benign-multi-assert-001",
        policy_attack_assertions=[
            _allow_assertion(assertion_id="multi-assert-1", attack_type="prompt_injection", action_type="CHAT_COMPLETION"),
            _allow_assertion(assertion_id="multi-assert-2", attack_type="path_traversal", action_type="FILE_READ"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(BenignOverblockLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.policy_attack_assertions) == 2
    assert task.metadata.policy_attack_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.policy_attack_assertions[1].assertion_id == "multi-assert-2"
    assert all(a.expected_outcome == PolicyOutcome.ALLOW for a in task.metadata.policy_attack_assertions)
    assert all(a.expected_rejection_layer is None for a in task.metadata.policy_attack_assertions)


@pytest.mark.unit
def test_benign_overblock_loader_supports_all_attack_types_as_allow(monkeypatch):
    for attack_type in [
        "prompt_injection",
        "indirect_injection",
        "command_obfuscation",
        "path_traversal",
        "privilege_escalation",
        "unsafe_mutation",
        "data_exfiltration",
    ]:
        row = _benign_row(
            key=f"benign-{attack_type}-001",
            policy_attack_assertions=[_allow_assertion(
                assertion_id=f"{attack_type}-assert-1",
                attack_type=attack_type,
                action_type="TEST_ACTION",
            )],
        )
        content = (json.dumps(row, sort_keys=True) + "\n").encode()
        mock_path = _stub_loader(monkeypatch, content)

        tasks = list(BenignOverblockLoader(mock_path).load())
        assertion = tasks[0].metadata.policy_attack_assertions[0]
        assert assertion.attack_type == AttackType(attack_type)
        assert assertion.expected_outcome == PolicyOutcome.ALLOW


# --- Loader: ledger-consistency absence parsing ---


@pytest.mark.unit
def test_benign_overblock_loader_parses_ledger_consistency_absence(monkeypatch):
    row = _benign_row(
        key="benign-ledger-001",
        policy_attack_assertions=[_allow_assertion(
            assertion_id="ledger-assert-1",
            attack_type="privilege_escalation",
            action_type="GOVERNANCE_ACTION",
            collection_boundary="governance_ledger",
            expected_absence={"kind": "ledger_consistency"},
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(BenignOverblockLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.policy_attack_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.GOVERNANCE_LEDGER
    assert assertion.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY
    assert assertion.expected_absence.consistent is False


# --- Loader: all assertions must be allow (suite invariant) ---


@pytest.mark.unit
def test_benign_overblock_loader_all_fixture_rows_declare_allow_outcome():
    """Every row in the immutable benign_overblock fixture must declare allow.

    This is the suite invariant: the benign-overblock suite exists to verify
    that benign actions are not over-blocked.  If any row declares ``block``,
    it belongs in the policy_attack suite, not here.
    """
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "benign_overblock" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 7
    for row in rows:
        assertions = row.get("policy_attack_assertions", [])
        assert len(assertions) >= 1
        for a in assertions:
            assert a["expected_outcome"] == "allow", (
                f"row {row['key']} assertion {a['assertion_id']} declares "
                f"expected_outcome={a['expected_outcome']}; benign_overblock "
                f"requires allow"
            )
            assert a.get("expected_rejection_layer") is None, (
                f"row {row['key']} assertion {a['assertion_id']} declares a "
                f"rejection layer; benign_overblock assertions must not"
            )


@pytest.mark.unit
def test_benign_overblock_loader_fixture_rows_cover_diverse_action_types():
    """The fixture covers diverse benign action types to exercise the full
    allow/block confusion matrix across action classes."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "benign_overblock" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    action_types = {a["action_type"] for row in rows for a in row.get("policy_attack_assertions", [])}
    assert "CHAT_COMPLETION" in action_types
    assert "FILE_READ" in action_types
    assert "FILE_EDIT" in action_types
    assert "SHELL_EXEC" in action_types
    assert "NETWORK_REQUEST" in action_types
    assert "GOVERNANCE_ACTION" in action_types


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_benign_overblock_fixture_provenance_matches_dataset():
    from pathlib import Path

    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "benign_overblock"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "benign_overblock"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["security", "policy"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/governance/benign_overblock_loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_benign_overblock_loader_suite_id_is_benign_overblock():
    assert BenignOverblockLoader.SUITE_ID == "benign_overblock"
