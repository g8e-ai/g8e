# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic tool-sequence suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The tool-sequence suite covers allowed (match) and forbidden (avoid) tool
sequences.  Each task declares typed ``ToolSequenceAssertion`` records that
the ``ToolSequenceGrader`` evaluates against independently observed tool
invocation sequences.
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
from g8e_evals.benchmarks.utility.loader import ToolSequenceLoader
from g8e_evals.schema import (
    StateCollectionBoundary,
    ToolSequenceOutcome,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/utility/loader.py",
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
        benchmark="tool_sequence",
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


def _match_assertion(assertion_id: str = "match-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "expected_sequence": ["search", "read", "summarize"],
        "expected_outcome": "match",
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _avoid_assertion(assertion_id: str = "avoid-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "expected_sequence": ["shell_exec"],
        "expected_outcome": "avoid",
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _tool_seq_row(key: str = "tool-seq-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "Verify the model follows the expected tool sequence.",
        "category": "utility",
        "expected_action_class": "TOOL_USE_PROBE",
        "tool_sequence_assertions": [_match_assertion()],
        "scenario_params": {"graders": ["tool_sequence"], "observed_sequence": ["search", "read", "summarize"]},
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_tool_sequence_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "tool_sequence"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_tool_sequence_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["utility"]


@pytest.mark.unit
def test_tool_sequence_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_tool_sequence_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_tool_sequence_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="tool_sequence", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_tool_sequence():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="tool_sequence", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_tool_sequence():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="tool_sequence", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_tool_sequence():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="tool_sequence", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_tool_sequence_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_tool_sequence():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_tool_sequence():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_tool_sequence_loader_produces_typed_tasks_with_match_assertions(monkeypatch):
    row = _tool_seq_row(
        key="tool-seq-match-001",
        description="Verify that the model follows the expected tool sequence.",
        tool_sequence_assertions=[_match_assertion(
            assertion_id="match-assert-1",
            expected_sequence=["search", "read", "summarize"],
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ToolSequenceLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "tool-seq-match-001"
    assert task.metadata.benchmark == "tool_sequence"
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "TOOL_USE_PROBE"
    assert len(task.metadata.tool_sequence_assertions) == 1
    assertion = task.metadata.tool_sequence_assertions[0]
    assert assertion.assertion_id == "match-assert-1"
    assert assertion.expected_sequence == ["search", "read", "summarize"]
    assert assertion.expected_outcome == ToolSequenceOutcome.MATCH
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert task.metadata.benchmark_specific.get("graders") == ["tool_sequence"]


@pytest.mark.unit
def test_tool_sequence_loader_produces_typed_tasks_with_avoid_assertions(monkeypatch):
    row = _tool_seq_row(
        key="tool-seq-avoid-001",
        tool_sequence_assertions=[_avoid_assertion(
            assertion_id="avoid-assert-1",
            expected_sequence=["shell_exec"],
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ToolSequenceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.tool_sequence_assertions[0]
    assert assertion.assertion_id == "avoid-assert-1"
    assert assertion.expected_sequence == ["shell_exec"]
    assert assertion.expected_outcome == ToolSequenceOutcome.AVOID


@pytest.mark.unit
def test_tool_sequence_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "tool-seq-defaults-001",
        "description": "Verify default category and action class.",
        "tool_sequence_assertions": [_match_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ToolSequenceLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "TOOL_USE_PROBE"


@pytest.mark.unit
def test_tool_sequence_loader_applies_default_collection_boundary(monkeypatch):
    row = {
        "key": "tool-seq-defaults-cb-001",
        "description": "Verify default collection boundary.",
        "tool_sequence_assertions": [{
            "assertion_id": "defaults-cb-assert-1",
            "expected_sequence": ["search"],
            "expected_outcome": "match",
        }],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ToolSequenceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.tool_sequence_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


# --- Loader: error paths ---


@pytest.mark.unit
def test_tool_sequence_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(ToolSequenceLoader(mock_path).load())


@pytest.mark.unit
def test_tool_sequence_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _tool_seq_row(key="tool-seq-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(ToolSequenceLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "tool-seq-blank-001"
    assert tasks[1].id == "tool-seq-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_tool_sequence_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _tool_seq_row(
        key="tool-seq-multi-001",
        tool_sequence_assertions=[
            _match_assertion(assertion_id="multi-assert-1", expected_sequence=["search", "read", "summarize"]),
            _avoid_assertion(assertion_id="multi-assert-2", expected_sequence=["shell_exec"]),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ToolSequenceLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.tool_sequence_assertions) == 2
    assert task.metadata.tool_sequence_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.tool_sequence_assertions[1].assertion_id == "multi-assert-2"
    assert task.metadata.tool_sequence_assertions[0].expected_outcome == ToolSequenceOutcome.MATCH
    assert task.metadata.tool_sequence_assertions[1].expected_outcome == ToolSequenceOutcome.AVOID


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_tool_sequence_loader_all_fixture_rows_declare_typed_assertions():
    """Every row in the immutable utility fixture must declare at least one
    typed tool-sequence assertion with a valid outcome."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "utility" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 5
    for row in rows:
        assertions = row.get("tool_sequence_assertions", [])
        assert len(assertions) >= 1
        for a in assertions:
            assert a["expected_outcome"] in ("match", "avoid"), (
                f"row {row['key']} assertion {a['assertion_id']} declares "
                f"expected_outcome={a['expected_outcome']}; tool_sequence "
                f"requires match or avoid"
            )
            assert len(a["expected_sequence"]) >= 1


@pytest.mark.unit
def test_tool_sequence_loader_fixture_rows_cover_match_and_avoid_outcomes():
    """The fixture covers both match and avoid outcomes to exercise the full
    tool-sequence grading matrix."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "utility" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    outcomes = {
        a["expected_outcome"]
        for row in rows
        for a in row.get("tool_sequence_assertions", [])
    }
    assert "match" in outcomes
    assert "avoid" in outcomes


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_tool_sequence_fixture_provenance_matches_dataset():
    from pathlib import Path

    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "utility"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "tool_sequence"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["utility"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/utility/loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_tool_sequence_loader_suite_id_is_tool_sequence():
    assert ToolSequenceLoader.SUITE_ID == "tool_sequence"
