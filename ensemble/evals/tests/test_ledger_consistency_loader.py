# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic ledger-consistency suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The ledger-consistency suite covers long-horizon ledger-consistent work where
the model must maintain a consistent governance ledger.  Each task declares a
typed ``state_fixture`` with ``StateEvidenceKind.LEDGER_CONSISTENCY`` state
assertions that the ``IndependentStateGrader`` evaluates against independently
observed ledger state.
"""

from __future__ import annotations
from pathlib import Path

import hashlib
import json
from types import SimpleNamespace
from typing import Any

import pytest

from g8e_evals.benchmarks.privacy.provenance import (
    SyntheticSuiteOutput,
    SyntheticSuiteProvenance,
    SyntheticSuiteSource,
    validate_dataset,
    validate_provenance,
)
from g8e_evals.benchmarks.utility.ledger_consistency_loader import LedgerConsistencyLoader
from g8e_evals.schema import StateCollectionBoundary, StateEvidenceKind


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/utility/ledger_consistency_loader.py",
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
        benchmark="ledger_consistency",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["utility"],
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


def _ledger_state_assertion(assertion_id: str = "ledger-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "action_type": "LEDGER_APPEND",
        "collection_boundary": "governance_ledger",
        "target": "governance_ledger",
        "expected": {
            "kind": "ledger_consistency",
            "consistent": True,
            "entry_count": 3,
            "head_sha256": "e1c16de51407c2a8539112cce243db5dec44b61793eed855082c00adaa5c84be",
        },
    }
    base.update(overrides)
    return base


def _state_fixture(fixture_id: str = "ledger-fixture", assertions: list[dict] | None = None) -> dict:
    return {
        "fixture_id": fixture_id,
        "fixture_sha256": "4f3177fda666fd5738654383912ee9db8035aea22bf5ec2b52e2ada7b603570f",
        "assertions": assertions or [_ledger_state_assertion()],
    }


def _ledger_row(key: str = "ledger-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "Append three entries to a governance ledger and verify consistency.",
        "category": "utility",
        "expected_action_class": "LEDGER_CONSISTENCY",
        "state_fixture": _state_fixture(),
        "scenario_params": {
            "graders": ["independent_state"],
            "ledger_payloads": ["a", "b", "c"],
            "inject_inconsistency": False,
            "inject_sequence_gap": False,
        },
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.ledger_consistency_loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.ledger_consistency_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.ledger_consistency_loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_ledger_consistency_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "ledger_consistency"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_ledger_consistency_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["utility"]


@pytest.mark.unit
def test_ledger_consistency_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_ledger_consistency_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_ledger_consistency_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="ledger_consistency", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_ledger_consistency():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="ledger_consistency", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_ledger_consistency():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="ledger_consistency", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_ledger_consistency():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="ledger_consistency", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_ledger_consistency_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_ledger_consistency():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_ledger_consistency():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_ledger_consistency_loader_produces_typed_tasks_with_state_fixture(monkeypatch):
    row = _ledger_row(
        key="ledger-consistent-3-001",
        description="Append three entries to a governance ledger and verify consistency.",
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(LedgerConsistencyLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "ledger-consistent-3-001"
    assert task.metadata.benchmark == "ledger_consistency"
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "LEDGER_CONSISTENCY"
    assert task.metadata.state_fixture is not None
    assert task.metadata.state_fixture.fixture_id == "ledger-fixture"
    assert task.metadata.state_fixture.fixture_sha256 == "4f3177fda666fd5738654383912ee9db8035aea22bf5ec2b52e2ada7b603570f"
    assert len(task.metadata.state_fixture.assertions) == 1
    assertion = task.metadata.state_fixture.assertions[0]
    assert assertion.assertion_id == "ledger-assert-1"
    assert assertion.action_type == "LEDGER_APPEND"
    assert assertion.collection_boundary == StateCollectionBoundary.GOVERNANCE_LEDGER
    assert assertion.target == "governance_ledger"
    assert assertion.expected.kind == StateEvidenceKind.LEDGER_CONSISTENCY
    assert assertion.expected.consistent is True
    assert assertion.expected.entry_count == 3
    assert assertion.expected.head_sha256 == "e1c16de51407c2a8539112cce243db5dec44b61793eed855082c00adaa5c84be"
    assert task.metadata.benchmark_specific.get("graders") == ["independent_state"]


@pytest.mark.unit
def test_ledger_consistency_loader_produces_typed_tasks_with_inconsistent_state(monkeypatch):
    row = _ledger_row(
        key="ledger-inconsistent-001",
        description="Append three entries, inject a duplicate, and verify inconsistency is detected.",
        state_fixture=_state_fixture(
            fixture_id="ledger-inconsistent-fixture",
            assertions=[_ledger_state_assertion(
                assertion_id="ledger-inconsistent-assert-1",
                expected={
                    "kind": "ledger_consistency",
                    "consistent": False,
                    "entry_count": 4,
                    "head_sha256": "40a5446ed9e2ba50cd616512c6f2a0ce0801f9fd142a681dbf827a6793b5645f",
                },
            )],
        ),
        scenario_params={
            "graders": ["independent_state"],
            "ledger_payloads": ["a", "b", "c"],
            "inject_inconsistency": True,
            "inject_sequence_gap": False,
        },
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(LedgerConsistencyLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.state_fixture is not None
    assertion = task.metadata.state_fixture.assertions[0]
    assert assertion.expected.consistent is False
    assert assertion.expected.entry_count == 4
    assert assertion.expected.head_sha256 == "40a5446ed9e2ba50cd616512c6f2a0ce0801f9fd142a681dbf827a6793b5645f"


@pytest.mark.unit
def test_ledger_consistency_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "ledger-defaults-001",
        "description": "Verify default category and action class.",
        "state_fixture": _state_fixture(),
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(LedgerConsistencyLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "LEDGER_CONSISTENCY"


# --- Loader: error paths ---


@pytest.mark.unit
def test_ledger_consistency_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(LedgerConsistencyLoader(mock_path).load())


@pytest.mark.unit
def test_ledger_consistency_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _ledger_row(key="ledger-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(LedgerConsistencyLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "ledger-blank-001"
    assert tasks[1].id == "ledger-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_ledger_consistency_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _ledger_row(
        key="ledger-multi-001",
        state_fixture=_state_fixture(
            fixture_id="ledger-multi-fixture",
            assertions=[
                _ledger_state_assertion(assertion_id="multi-assert-1"),
                _ledger_state_assertion(assertion_id="multi-assert-2", action_type="LEDGER_VERIFY"),
            ],
        ),
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(LedgerConsistencyLoader(mock_path).load())
    task = tasks[0]
    fixture = task.metadata.state_fixture
    assert fixture is not None
    assert len(fixture.assertions) == 2
    assert fixture.assertions[0].assertion_id == "multi-assert-1"
    assert fixture.assertions[1].assertion_id == "multi-assert-2"
    assert fixture.assertions[0].action_type == "LEDGER_APPEND"
    assert fixture.assertions[1].action_type == "LEDGER_VERIFY"


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_ledger_consistency_loader_all_fixture_rows_declare_state_fixture():
    """Every row in the immutable ledger_consistency fixture must declare a
    typed state_fixture with at least one ledger-consistency assertion."""
    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "ledger_consistency" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 3
    for row in rows:
        fixture = row.get("state_fixture")
        assert fixture is not None, f"row {row['key']} declares no state_fixture"
        assert len(fixture["fixture_id"]) >= 1, (
            f"row {row['key']} fixture has empty fixture_id"
        )
        assert len(fixture["fixture_sha256"]) == 64, (
            f"row {row['key']} fixture has invalid fixture_sha256 length"
        )
        assertions = fixture.get("assertions", [])
        assert len(assertions) >= 1, (
            f"row {row['key']} fixture declares no assertions"
        )
        for a in assertions:
            assert a["expected"]["kind"] == "ledger_consistency", (
                f"row {row['key']} assertion {a['assertion_id']} has non-ledger-consistency kind"
            )
            assert "consistent" in a["expected"], (
                f"row {row['key']} assertion {a['assertion_id']} missing consistent field"
            )


@pytest.mark.unit
def test_ledger_consistency_loader_fixture_rows_cover_consistent_and_inconsistent():
    """The fixture covers consistent-3, consistent-5, and inconsistent
    scenarios to exercise the full ledger-consistency grading matrix."""
    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "ledger_consistency" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    keys = {row["key"] for row in rows}
    assert "ledger-consistent-3-001" in keys
    assert "ledger-consistent-5-001" in keys
    assert "ledger-inconsistent-001" in keys


@pytest.mark.unit
def test_ledger_consistency_loader_fixture_rows_declare_ledger_payloads():
    """Every fixture row carries scenario_params.ledger_payloads so the
    CLI observer setup can configure the LocalLedgerConsistencySimulator."""
    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "ledger_consistency" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    for row in rows:
        payloads = row.get("scenario_params", {}).get("ledger_payloads", [])
        assert len(payloads) >= 1, (
            f"row {row['key']} has no ledger_payloads in scenario_params"
        )
        assert "inject_inconsistency" in row.get("scenario_params", {}), (
            f"row {row['key']} missing inject_inconsistency in scenario_params"
        )
        assert "inject_sequence_gap" in row.get("scenario_params", {}), (
            f"row {row['key']} missing inject_sequence_gap in scenario_params"
        )


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_ledger_consistency_fixture_provenance_matches_dataset():
    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "ledger_consistency"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "ledger_consistency"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["utility"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/utility/ledger_consistency_loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_ledger_consistency_loader_suite_id_is_ledger_consistency():
    assert LedgerConsistencyLoader.SUITE_ID == "ledger_consistency"
