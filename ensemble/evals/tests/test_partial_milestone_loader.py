# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic partial-milestone suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The partial-milestone suite covers long-horizon tasks where the model must
reach intermediate milestones at declared order indices.  Each task declares
typed ``PartialMilestoneAssertion`` records that the
``PartialMilestoneGrader`` evaluates against independently observed milestone
records.
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
from g8e_evals.benchmarks.utility.partial_milestone_loader import PartialMilestoneLoader
from g8e_evals.schema import StateCollectionBoundary


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/utility/partial_milestone_loader.py",
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
        benchmark="partial_milestone",
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


def _milestone_assertion(assertion_id: str = "partial-milestone-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "expected_label": "literature review",
        "expected_order": 0,
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _partial_milestone_row(key: str = "partial-milestone-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "Complete a multi-step research task. Reach all three milestones.",
        "category": "utility",
        "expected_action_class": "PARTIAL_MILESTONE",
        "partial_milestone_assertions": [_milestone_assertion()],
        "scenario_params": {
            "graders": ["partial_milestone"],
            "observed_milestones": [{"label": "literature review", "order": 0}],
        },
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.partial_milestone_loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.partial_milestone_loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.utility.partial_milestone_loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_partial_milestone_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "partial_milestone"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_partial_milestone_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["utility"]


@pytest.mark.unit
def test_partial_milestone_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_partial_milestone_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_partial_milestone_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="partial_milestone", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_partial_milestone():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="partial_milestone", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_partial_milestone():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="partial_milestone", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_partial_milestone():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="partial_milestone", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_partial_milestone_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_partial_milestone():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_partial_milestone():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_partial_milestone_loader_produces_typed_tasks_with_milestone_assertions(monkeypatch):
    row = _partial_milestone_row(
        key="partial-milestone-all-reached-001",
        description="Complete a multi-step research task. Reach all three milestones.",
        partial_milestone_assertions=[
            _milestone_assertion(assertion_id="partial-milestone-all-reached-assert-1", expected_label="literature review", expected_order=0),
            _milestone_assertion(assertion_id="partial-milestone-all-reached-assert-2", expected_label="data collection", expected_order=1),
            _milestone_assertion(assertion_id="partial-milestone-all-reached-assert-3", expected_label="analysis", expected_order=2),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(PartialMilestoneLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "partial-milestone-all-reached-001"
    assert task.metadata.benchmark == "partial_milestone"
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "PARTIAL_MILESTONE"
    assert len(task.metadata.partial_milestone_assertions) == 3
    assertion = task.metadata.partial_milestone_assertions[0]
    assert assertion.assertion_id == "partial-milestone-all-reached-assert-1"
    assert assertion.expected_label == "literature review"
    assert assertion.expected_order == 0
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert task.metadata.benchmark_specific.get("graders") == ["partial_milestone"]


@pytest.mark.unit
def test_partial_milestone_loader_produces_typed_tasks_with_single_milestone(monkeypatch):
    row = _partial_milestone_row(
        key="partial-milestone-single-001",
        description="Complete a focused writing task. Reach the single milestone.",
        partial_milestone_assertions=[
            _milestone_assertion(assertion_id="partial-milestone-single-assert-1", expected_label="draft completion", expected_order=0),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(PartialMilestoneLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.partial_milestone_assertions) == 1
    assertion = task.metadata.partial_milestone_assertions[0]
    assert assertion.assertion_id == "partial-milestone-single-assert-1"
    assert assertion.expected_label == "draft completion"
    assert assertion.expected_order == 0


@pytest.mark.unit
def test_partial_milestone_loader_produces_typed_tasks_with_four_milestones(monkeypatch):
    row = _partial_milestone_row(
        key="partial-milestone-partial-completion-001",
        description="Complete a multi-phase migration task. Reach all four milestones.",
        partial_milestone_assertions=[
            _milestone_assertion(assertion_id="assert-1", expected_label="assessment", expected_order=0),
            _milestone_assertion(assertion_id="assert-2", expected_label="planning", expected_order=1),
            _milestone_assertion(assertion_id="assert-3", expected_label="execution", expected_order=2),
            _milestone_assertion(assertion_id="assert-4", expected_label="verification", expected_order=3),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(PartialMilestoneLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.partial_milestone_assertions) == 4
    assert task.metadata.partial_milestone_assertions[3].expected_order == 3


@pytest.mark.unit
def test_partial_milestone_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "partial-milestone-defaults-001",
        "description": "Verify default category and action class.",
        "partial_milestone_assertions": [_milestone_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(PartialMilestoneLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "utility"
    assert task.metadata.expected_action_class == "PARTIAL_MILESTONE"


@pytest.mark.unit
def test_partial_milestone_loader_applies_default_collection_boundary(monkeypatch):
    row = {
        "key": "partial-milestone-defaults-cb-001",
        "description": "Verify default collection boundary.",
        "partial_milestone_assertions": [{
            "assertion_id": "defaults-cb-assert-1",
            "expected_label": "literature review",
            "expected_order": 0,
        }],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(PartialMilestoneLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.partial_milestone_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


# --- Loader: error paths ---


@pytest.mark.unit
def test_partial_milestone_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(PartialMilestoneLoader(mock_path).load())


@pytest.mark.unit
def test_partial_milestone_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _partial_milestone_row(key="partial-milestone-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(PartialMilestoneLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "partial-milestone-blank-001"
    assert tasks[1].id == "partial-milestone-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_partial_milestone_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _partial_milestone_row(
        key="partial-milestone-multi-001",
        partial_milestone_assertions=[
            _milestone_assertion(assertion_id="multi-assert-1", expected_label="design", expected_order=0),
            _milestone_assertion(assertion_id="multi-assert-2", expected_label="implementation", expected_order=1),
            _milestone_assertion(assertion_id="multi-assert-3", expected_label="testing", expected_order=2),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(PartialMilestoneLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.partial_milestone_assertions) == 3
    assert task.metadata.partial_milestone_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.partial_milestone_assertions[1].assertion_id == "multi-assert-2"
    assert task.metadata.partial_milestone_assertions[2].assertion_id == "multi-assert-3"
    assert task.metadata.partial_milestone_assertions[0].expected_order == 0
    assert task.metadata.partial_milestone_assertions[1].expected_order == 1
    assert task.metadata.partial_milestone_assertions[2].expected_order == 2


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_partial_milestone_loader_all_fixture_rows_declare_typed_assertions():
    """Every row in the immutable partial_milestone fixture must declare at least one
    typed partial-milestone assertion with a valid expected_order."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "partial_milestone" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 5
    for row in rows:
        assertions = row.get("partial_milestone_assertions", [])
        assert len(assertions) >= 1, f"row {row['key']} declares no partial_milestone_assertions"
        for a in assertions:
            assert len(a["assertion_id"]) >= 1, (
                f"row {row['key']} assertion has empty assertion_id"
            )
            assert len(a["expected_label"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty expected_label"
            )
            assert a["expected_order"] >= 0, (
                f"row {row['key']} assertion {a['assertion_id']} declares "
                f"expected_order={a['expected_order']}; partial_milestone "
                f"requires expected_order >= 0"
            )


@pytest.mark.unit
def test_partial_milestone_loader_fixture_rows_cover_all_reached_skipped_out_of_order():
    """The fixture covers all-reached, skipped, out-of-order, single, and
    partial-completion scenarios to exercise the full partial-milestone
    grading matrix."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "partial_milestone" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    keys = {row["key"] for row in rows}
    assert "partial-milestone-all-reached-001" in keys
    assert "partial-milestone-skipped-001" in keys
    assert "partial-milestone-out-of-order-001" in keys
    assert "partial-milestone-single-001" in keys
    assert "partial-milestone-partial-completion-001" in keys


@pytest.mark.unit
def test_partial_milestone_loader_fixture_rows_declare_observed_milestones():
    """Every fixture row carries scenario_params.observed_milestones so the
    CLI observer setup can configure the LocalPartialMilestoneSimulator."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "partial_milestone" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    for row in rows:
        observed = row.get("scenario_params", {}).get("observed_milestones", [])
        assert len(observed) >= 1, (
            f"row {row['key']} has no observed_milestones in scenario_params"
        )
        for m in observed:
            assert "label" in m, (
                f"row {row['key']} observed milestone missing label"
            )
            assert "order" in m, (
                f"row {row['key']} observed milestone missing order"
            )


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_partial_milestone_fixture_provenance_matches_dataset():
    from pathlib import Path

    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "partial_milestone"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "partial_milestone"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["utility"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/utility/partial_milestone_loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_partial_milestone_loader_suite_id_is_partial_milestone():
    assert PartialMilestoneLoader.SUITE_ID == "partial_milestone"
