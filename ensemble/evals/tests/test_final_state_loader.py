# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic final-state suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The final-state suite covers independently observed final-state transitions
where the model must produce a state-root change (or non-change) matching a
declared predicate.  Each task declares typed ``FinalStateAssertion`` records
that the ``FinalStateAssertionGrader`` evaluates against independently observed
final-state observations bound to signed receipts.
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
from g8e_evals.benchmarks.utility.final_state_loader import FinalStateLoader
from g8e_evals.schema import StateAssertionPredicate


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/utility/final_state_loader.py",
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
        benchmark="final_state",
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


def _final_state_assertion(assertion_id: str = "final-state-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "predicate": "state_root_changed",
        "action_type": "FILE_WRITE",
    }
    base.update(overrides)
    return base


def _final_state_row(key: str = "final-state-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "Execute a file write action that changes the state root.",
        "category": "utility",
        "expected_action_class": "FINAL_STATE",
        "expected_final_state_assertions": [_final_state_assertion()],
        "scenario_params": {
            "graders": ["final_state_assertions"],
            "final_state_params": {
                "final-state-assert-1": {
                    "state_root_before": "root-v1",
                    "state_root_after": "root-v2",
                },
            },
        },
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.final_state_loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.final_state_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.final_state_loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_final_state_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "final_state"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_final_state_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["utility"]


@pytest.mark.unit
def test_final_state_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_final_state_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_final_state_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="final_state", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_final_state():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="final_state", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_final_state():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="final_state", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_final_state():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="final_state", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_final_state_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_final_state():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_final_state():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_final_state_loader_produces_typed_tasks_with_final_state_assertions(monkeypatch):
    row = _final_state_row(
        key="final-state-changed-001",
        description="Execute a file write action that changes the state root.",
        expected_final_state_assertions=[
            _final_state_assertion(assertion_id="final-state-changed-assert-1", predicate="state_root_changed", action_type="FILE_WRITE"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FinalStateLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "final-state-changed-001"
    assert task.metadata.benchmark == "final_state"
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "FINAL_STATE"
    assert len(task.metadata.expected_final_state_assertions) == 1
    assertion = task.metadata.expected_final_state_assertions[0]
    assert assertion.assertion_id == "final-state-changed-assert-1"
    assert assertion.predicate == StateAssertionPredicate.STATE_ROOT_CHANGED
    assert assertion.action_type == "FILE_WRITE"
    assert task.metadata.benchmark_specific.get("graders") == ["final_state_assertions"]


@pytest.mark.unit
def test_final_state_loader_produces_typed_tasks_with_unchanged_predicate(monkeypatch):
    row = _final_state_row(
        key="final-state-unchanged-001",
        description="Execute a file read action that does not change the state root.",
        expected_final_state_assertions=[
            _final_state_assertion(assertion_id="final-state-unchanged-assert-1", predicate="state_root_unchanged", action_type="FILE_READ"),
        ],
        scenario_params={
            "graders": ["final_state_assertions"],
            "final_state_params": {
                "final-state-unchanged-assert-1": {
                    "state_root_before": "root-v1",
                    "state_root_after": "root-v1",
                },
            },
        },
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FinalStateLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.expected_final_state_assertions) == 1
    assertion = task.metadata.expected_final_state_assertions[0]
    assert assertion.predicate == StateAssertionPredicate.STATE_ROOT_UNCHANGED
    assert assertion.action_type == "FILE_READ"


@pytest.mark.unit
def test_final_state_loader_produces_typed_tasks_with_mixed_predicates(monkeypatch):
    row = _final_state_row(
        key="final-state-mixed-001",
        description="Execute two actions: one that changes the state root and one that does not.",
        expected_final_state_assertions=[
            _final_state_assertion(assertion_id="final-state-mixed-assert-1", predicate="state_root_changed", action_type="FILE_WRITE"),
            _final_state_assertion(assertion_id="final-state-mixed-assert-2", predicate="state_root_unchanged", action_type="FILE_READ"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FinalStateLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.expected_final_state_assertions) == 2
    assert task.metadata.expected_final_state_assertions[0].predicate == StateAssertionPredicate.STATE_ROOT_CHANGED
    assert task.metadata.expected_final_state_assertions[1].predicate == StateAssertionPredicate.STATE_ROOT_UNCHANGED


@pytest.mark.unit
def test_final_state_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "final-state-defaults-001",
        "description": "Verify default category and action class.",
        "expected_final_state_assertions": [_final_state_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FinalStateLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "FINAL_STATE"


# --- Loader: error paths ---


@pytest.mark.unit
def test_final_state_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(FinalStateLoader(mock_path).load())


@pytest.mark.unit
def test_final_state_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _final_state_row(key="final-state-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(FinalStateLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "final-state-blank-001"
    assert tasks[1].id == "final-state-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_final_state_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _final_state_row(
        key="final-state-multi-001",
        expected_final_state_assertions=[
            _final_state_assertion(assertion_id="multi-assert-1", predicate="state_root_changed", action_type="FILE_WRITE"),
            _final_state_assertion(assertion_id="multi-assert-2", predicate="state_root_unchanged", action_type="FILE_READ"),
            _final_state_assertion(assertion_id="multi-assert-3", predicate="state_root_changed", action_type="TOOL_USE"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FinalStateLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.expected_final_state_assertions) == 3
    assert task.metadata.expected_final_state_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.expected_final_state_assertions[1].assertion_id == "multi-assert-2"
    assert task.metadata.expected_final_state_assertions[2].assertion_id == "multi-assert-3"
    assert task.metadata.expected_final_state_assertions[0].action_type == "FILE_WRITE"
    assert task.metadata.expected_final_state_assertions[1].action_type == "FILE_READ"
    assert task.metadata.expected_final_state_assertions[2].action_type == "TOOL_USE"


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_final_state_loader_all_fixture_rows_declare_typed_assertions():
    """Every row in the immutable final_state fixture must declare at least one
    typed final-state assertion with a valid predicate and action type."""
    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "final_state" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 3
    valid_predicates = {"state_root_changed", "state_root_unchanged"}
    for row in rows:
        assertions = row.get("expected_final_state_assertions", [])
        assert len(assertions) >= 1, f"row {row['key']} declares no expected_final_state_assertions"
        for a in assertions:
            assert len(a["assertion_id"]) >= 1, (
                f"row {row['key']} assertion has empty assertion_id"
            )
            assert a["predicate"] in valid_predicates, (
                f"row {row['key']} assertion {a['assertion_id']} has invalid predicate {a['predicate']}"
            )
            assert len(a["action_type"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty action_type"
            )


@pytest.mark.unit
def test_final_state_loader_fixture_rows_cover_changed_unchanged_and_mixed():
    """The fixture covers changed, unchanged, and mixed predicate scenarios to
    exercise the full final-state grading matrix."""
    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "final_state" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    keys = {row["key"] for row in rows}
    assert "final-state-changed-001" in keys
    assert "final-state-unchanged-001" in keys
    assert "final-state-mixed-001" in keys


@pytest.mark.unit
def test_final_state_loader_fixture_rows_declare_final_state_params():
    """Every fixture row carries scenario_params.final_state_params so the
    CLI observer setup can configure the LocalFinalStateSimulator."""
    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "final_state" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    for row in rows:
        params = row.get("scenario_params", {}).get("final_state_params", {})
        assert len(params) >= 1, (
            f"row {row['key']} has no final_state_params in scenario_params"
        )
        for assertion_id, p in params.items():
            assert "state_root_before" in p, (
                f"row {row['key']} final_state_params[{assertion_id}] missing state_root_before"
            )
            assert "state_root_after" in p, (
                f"row {row['key']} final_state_params[{assertion_id}] missing state_root_after"
            )


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_final_state_fixture_provenance_matches_dataset():
    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "final_state"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "final_state"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["utility"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/utility/final_state_loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_final_state_loader_suite_id_is_final_state():
    assert FinalStateLoader.SUITE_ID == "final_state"
