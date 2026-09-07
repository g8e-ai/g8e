# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic factual/domain-QA suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The factual-QA suite covers exact-match, normalized-match, and contains
match types.  Each task declares typed ``FactualQAAssertion`` records that
the ``FactualQAGrader`` evaluates against independently observed answer
strings.
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
from g8e_evals.benchmarks.utility.factual_qa_loader import FactualQALoader
from g8e_evals.schema import (
    FactualQAMatchType,
    StateCollectionBoundary,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/utility/factual_qa_loader.py",
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
        benchmark="factual_qa",
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


def _exact_assertion(assertion_id: str = "factual-qa-exact-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "expected_answer": "Paris",
        "match_type": "exact_match",
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _normalized_assertion(assertion_id: str = "factual-qa-normalized-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "expected_answer": "Jupiter is the largest planet",
        "match_type": "normalized_match",
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _contains_assertion(assertion_id: str = "factual-qa-contains-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "expected_answer": "Einstein",
        "match_type": "contains",
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _factual_qa_row(key: str = "factual-qa-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "What is the capital of France? Provide the exact answer.",
        "category": "utility",
        "expected_action_class": "FACTUAL_QA",
        "factual_qa_assertions": [_exact_assertion()],
        "scenario_params": {"graders": ["factual_qa"], "observed_answer": "Paris"},
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.factual_qa_loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.factual_qa_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.factual_qa_loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_factual_qa_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "factual_qa"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_factual_qa_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["utility"]


@pytest.mark.unit
def test_factual_qa_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_factual_qa_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_factual_qa_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="factual_qa", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_factual_qa():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="factual_qa", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_factual_qa():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="factual_qa", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_factual_qa():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="factual_qa", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_factual_qa_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_factual_qa():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_factual_qa():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_factual_qa_loader_produces_typed_tasks_with_exact_match_assertions(monkeypatch):
    row = _factual_qa_row(
        key="factual-qa-exact-001",
        description="What is the capital of France? Provide the exact answer.",
        factual_qa_assertions=[_exact_assertion(
            assertion_id="factual-qa-exact-assert-1",
            expected_answer="Paris",
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FactualQALoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "factual-qa-exact-001"
    assert task.metadata.benchmark == "factual_qa"
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "FACTUAL_QA"
    assert len(task.metadata.factual_qa_assertions) == 1
    assertion = task.metadata.factual_qa_assertions[0]
    assert assertion.assertion_id == "factual-qa-exact-assert-1"
    assert assertion.expected_answer == "Paris"
    assert assertion.match_type == FactualQAMatchType.EXACT_MATCH
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert task.metadata.benchmark_specific.get("graders") == ["factual_qa"]


@pytest.mark.unit
def test_factual_qa_loader_produces_typed_tasks_with_normalized_match_assertions(monkeypatch):
    row = _factual_qa_row(
        key="factual-qa-normalized-001",
        factual_qa_assertions=[_normalized_assertion(
            assertion_id="factual-qa-normalized-assert-1",
            expected_answer="Jupiter is the largest planet",
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FactualQALoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.factual_qa_assertions[0]
    assert assertion.assertion_id == "factual-qa-normalized-assert-1"
    assert assertion.expected_answer == "Jupiter is the largest planet"
    assert assertion.match_type == FactualQAMatchType.NORMALIZED_MATCH


@pytest.mark.unit
def test_factual_qa_loader_produces_typed_tasks_with_contains_assertions(monkeypatch):
    row = _factual_qa_row(
        key="factual-qa-contains-001",
        factual_qa_assertions=[_contains_assertion(
            assertion_id="factual-qa-contains-assert-1",
            expected_answer="Einstein",
        )],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FactualQALoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.factual_qa_assertions[0]
    assert assertion.assertion_id == "factual-qa-contains-assert-1"
    assert assertion.expected_answer == "Einstein"
    assert assertion.match_type == FactualQAMatchType.CONTAINS


@pytest.mark.unit
def test_factual_qa_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "factual-qa-defaults-001",
        "description": "Verify default category and action class.",
        "factual_qa_assertions": [_exact_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FactualQALoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "FACTUAL_QA"


@pytest.mark.unit
def test_factual_qa_loader_applies_default_collection_boundary(monkeypatch):
    row = {
        "key": "factual-qa-defaults-cb-001",
        "description": "Verify default collection boundary.",
        "factual_qa_assertions": [{
            "assertion_id": "defaults-cb-assert-1",
            "expected_answer": "Paris",
            "match_type": "exact_match",
        }],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FactualQALoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.factual_qa_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


# --- Loader: error paths ---


@pytest.mark.unit
def test_factual_qa_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(FactualQALoader(mock_path).load())


@pytest.mark.unit
def test_factual_qa_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _factual_qa_row(key="factual-qa-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(FactualQALoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "factual-qa-blank-001"
    assert tasks[1].id == "factual-qa-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_factual_qa_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _factual_qa_row(
        key="factual-qa-multi-001",
        factual_qa_assertions=[
            _exact_assertion(assertion_id="multi-assert-1", expected_answer="299792458"),
            _contains_assertion(assertion_id="multi-assert-2", expected_answer="meters per second"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(FactualQALoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.factual_qa_assertions) == 2
    assert task.metadata.factual_qa_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.factual_qa_assertions[1].assertion_id == "multi-assert-2"
    assert task.metadata.factual_qa_assertions[0].match_type == FactualQAMatchType.EXACT_MATCH
    assert task.metadata.factual_qa_assertions[1].match_type == FactualQAMatchType.CONTAINS


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_factual_qa_loader_all_fixture_rows_declare_typed_assertions():
    """Every row in the immutable factual_qa fixture must declare at least one
    typed factual-QA assertion with a valid match type."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "factual_qa" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 5
    for row in rows:
        assertions = row.get("factual_qa_assertions", [])
        assert len(assertions) >= 1
        for a in assertions:
            assert a["match_type"] in ("exact_match", "normalized_match", "contains"), (
                f"row {row['key']} assertion {a['assertion_id']} declares "
                f"match_type={a['match_type']}; factual_qa "
                f"requires exact_match, normalized_match, or contains"
            )
            assert len(a["expected_answer"]) >= 1


@pytest.mark.unit
def test_factual_qa_loader_fixture_rows_cover_all_match_types():
    """The fixture covers all three match types to exercise the full
    factual-QA grading matrix."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "factual_qa" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    match_types = {
        a["match_type"]
        for row in rows
        for a in row.get("factual_qa_assertions", [])
    }
    assert "exact_match" in match_types
    assert "normalized_match" in match_types
    assert "contains" in match_types


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_factual_qa_fixture_provenance_matches_dataset():
    from pathlib import Path

    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "factual_qa"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "factual_qa"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["utility"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/utility/factual_qa_loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_factual_qa_loader_suite_id_is_factual_qa():
    assert FactualQALoader.SUITE_ID == "factual_qa"
