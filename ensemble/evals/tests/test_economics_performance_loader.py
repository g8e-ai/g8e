# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic economics and performance suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem. File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The economics and performance suite covers all six declared
``PerformanceMetricKind`` values (provider charge, per-role calls,
per-role tokens, stage latency, local resource metadata, and human wait
time) stratified by all three ``TaskComplexity`` values (simple,
moderate, complex), plus two measured-failure cases (out-of-tolerance
and no-measurement). Each task declares typed
``EconomicsPerformanceAssertion`` records that the
``EconomicsPerformanceGrader`` evaluates against independently observed
measurement records.
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
from g8e_evals.benchmarks.economics.loader import EconomicsPerformanceLoader
from g8e_evals.schema import (
    PerformanceMetricKind,
    StateCollectionBoundary,
    TaskComplexity,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/economics/loader.py",
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
        benchmark="economics_performance",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["economics", "performance"],
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


def _economics_assertion(assertion_id: str = "economics-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "metric_kind": "provider_charge",
        "role": "",
        "action_class": "CHAT_COMPLETION",
        "task_complexity": "simple",
        "expected_value": 0.002,
        "tolerance": 0.001,
        "unit": "usd",
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _economics_row(key: str = "econ-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "A simple single-call task where the provider charges a measured amount. The system must record the measured provider charge within the declared tolerance.",
        "category": "economics_performance",
        "expected_action_class": "ECONOMICS",
        "economics_performance_assertions": [_economics_assertion()],
        "scenario_params": {
            "graders": ["economics_performance"],
            "economics_params": {
                "economics-assert-1": {
                    "observed_value": 0.002,
                },
            },
        },
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.economics.loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.economics.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.economics.loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_economics_performance_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "economics_performance"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_economics_performance_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["economics", "performance"]


@pytest.mark.unit
def test_economics_performance_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_economics_performance_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_economics_performance_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="economics_performance", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_economics_performance():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="economics_performance", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_economics_performance():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="economics_performance", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_economics_performance():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="economics_performance", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_suite_substitution_for_economics_performance():
    provenance = _make_provenance()
    with pytest.raises(ValueError, match="benchmark"):
        validate_provenance(provenance, suite_id="wrong_suite", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_economics_performance_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_economics_performance():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_economics_performance():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_provider_charge_assertion(monkeypatch):
    row = _economics_row(
        key="econ-provider-charge-simple-001",
        description="A simple single-call task where the provider charges a measured amount.",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-provider-charge-simple-assert-1",
                metric_kind="provider_charge",
                role="",
                action_class="CHAT_COMPLETION",
                task_complexity="simple",
                expected_value=0.002,
                tolerance=0.001,
                unit="usd",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "econ-provider-charge-simple-001"
    assert task.metadata.benchmark == "economics_performance"
    assert task.metadata.category == "economics_performance"
    assert task.metadata.expected_action_class == "ECONOMICS"
    assert len(task.metadata.economics_performance_assertions) == 1
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.assertion_id == "econ-provider-charge-simple-assert-1"
    assert assertion.metric_kind == PerformanceMetricKind.PROVIDER_CHARGE
    assert assertion.role == ""
    assert assertion.action_class == "CHAT_COMPLETION"
    assert assertion.task_complexity == TaskComplexity.SIMPLE
    assert assertion.expected_value == 0.002
    assert assertion.tolerance == 0.001
    assert assertion.unit == "usd"
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert task.metadata.benchmark_specific.get("graders") == ["economics_performance"]


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_per_role_calls_assertion(monkeypatch):
    row = _economics_row(
        key="econ-per-role-calls-moderate-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-per-role-calls-moderate-assert-1",
                metric_kind="per_role_calls",
                role="primary",
                action_class="CHAT_COMPLETION",
                task_complexity="moderate",
                expected_value=3.0,
                tolerance=1.0,
                unit="calls",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.metric_kind == PerformanceMetricKind.PER_ROLE_CALLS
    assert assertion.role == "primary"
    assert assertion.task_complexity == TaskComplexity.MODERATE
    assert assertion.unit == "calls"


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_per_role_tokens_assertion(monkeypatch):
    row = _economics_row(
        key="econ-per-role-tokens-moderate-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-per-role-tokens-moderate-assert-1",
                metric_kind="per_role_tokens",
                role="assistant",
                action_class="CHAT_COMPLETION",
                task_complexity="moderate",
                expected_value=1500.0,
                tolerance=200.0,
                unit="tokens",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.metric_kind == PerformanceMetricKind.PER_ROLE_TOKENS
    assert assertion.role == "assistant"
    assert assertion.unit == "tokens"


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_stage_latency_assertion(monkeypatch):
    row = _economics_row(
        key="econ-stage-latency-complex-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-stage-latency-complex-assert-1",
                metric_kind="stage_latency",
                role="",
                action_class="GOVERNANCE_ACTION",
                task_complexity="complex",
                expected_value=50.0,
                tolerance=15.0,
                unit="ms",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.metric_kind == PerformanceMetricKind.STAGE_LATENCY
    assert assertion.action_class == "GOVERNANCE_ACTION"
    assert assertion.task_complexity == TaskComplexity.COMPLEX
    assert assertion.unit == "ms"


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_local_resource_metadata_assertion(monkeypatch):
    row = _economics_row(
        key="econ-local-resource-complex-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-local-resource-complex-assert-1",
                metric_kind="local_resource_metadata",
                role="",
                action_class="TOOL_USE",
                task_complexity="complex",
                expected_value=1048576.0,
                tolerance=100000.0,
                unit="bytes",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.metric_kind == PerformanceMetricKind.LOCAL_RESOURCE_METADATA
    assert assertion.action_class == "TOOL_USE"
    assert assertion.unit == "bytes"


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_human_wait_time_assertion(monkeypatch):
    row = _economics_row(
        key="econ-human-wait-moderate-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-human-wait-moderate-assert-1",
                metric_kind="human_wait_time",
                role="",
                action_class="CHAT_COMPLETION",
                task_complexity="moderate",
                expected_value=12.0,
                tolerance=3.0,
                unit="seconds",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.metric_kind == PerformanceMetricKind.HUMAN_WAIT_TIME
    assert assertion.unit == "seconds"


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_out_of_tolerance_measured_failure(monkeypatch):
    row = _economics_row(
        key="econ-out-of-tolerance-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-out-of-tolerance-assert-1",
                metric_kind="provider_charge",
                expected_value=0.002,
                tolerance=0.001,
            ),
        ],
        scenario_params={
            "graders": ["economics_performance"],
            "economics_params": {
                "econ-out-of-tolerance-assert-1": {
                    "observed_value": 0.01,
                },
            },
        },
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.expected_value == 0.002
    params = task.metadata.benchmark_specific.get("economics_params", {})
    assert params["econ-out-of-tolerance-assert-1"]["observed_value"] == 0.01


@pytest.mark.unit
def test_economics_performance_loader_produces_typed_tasks_with_no_measurement_measured_failure(monkeypatch):
    row = _economics_row(
        key="econ-no-measurement-001",
        economics_performance_assertions=[
            _economics_assertion(
                assertion_id="econ-no-measurement-assert-1",
                metric_kind="provider_charge",
            ),
        ],
        scenario_params={
            "graders": ["economics_performance"],
            "economics_params": {
                "econ-no-measurement-assert-1": {
                    "observed_value": None,
                },
            },
        },
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    params = task.metadata.benchmark_specific.get("economics_params", {})
    assert params["econ-no-measurement-assert-1"]["observed_value"] is None


@pytest.mark.unit
def test_economics_performance_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "econ-defaults-001",
        "description": "Verify default category and action class.",
        "economics_performance_assertions": [_economics_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "economics_performance"
    assert task.metadata.expected_action_class == "ECONOMICS"


@pytest.mark.unit
def test_economics_performance_loader_applies_default_collection_boundary(monkeypatch):
    row = {
        "key": "econ-defaults-cb-001",
        "description": "Verify default collection boundary.",
        "economics_performance_assertions": [{
            "assertion_id": "defaults-cb-assert-1",
            "metric_kind": "provider_charge",
            "role": "",
            "action_class": "CHAT_COMPLETION",
            "task_complexity": "simple",
            "expected_value": 0.002,
            "tolerance": 0.001,
            "unit": "usd",
        }],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.economics_performance_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


# --- Loader: error paths ---


@pytest.mark.unit
def test_economics_performance_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(EconomicsPerformanceLoader(mock_path).load())


@pytest.mark.unit
def test_economics_performance_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _economics_row(key="econ-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "econ-blank-001"
    assert tasks[1].id == "econ-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_economics_performance_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _economics_row(
        key="econ-multi-001",
        economics_performance_assertions=[
            _economics_assertion(assertion_id="multi-assert-1", metric_kind="provider_charge", task_complexity="simple"),
            _economics_assertion(assertion_id="multi-assert-2", metric_kind="per_role_calls", role="primary", task_complexity="moderate", expected_value=3.0, tolerance=1.0, unit="calls"),
            _economics_assertion(assertion_id="multi-assert-3", metric_kind="stage_latency", action_class="GOVERNANCE_ACTION", task_complexity="complex", expected_value=50.0, tolerance=15.0, unit="ms"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(EconomicsPerformanceLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.economics_performance_assertions) == 3
    assert task.metadata.economics_performance_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.economics_performance_assertions[1].assertion_id == "multi-assert-2"
    assert task.metadata.economics_performance_assertions[2].assertion_id == "multi-assert-3"
    assert task.metadata.economics_performance_assertions[0].metric_kind == PerformanceMetricKind.PROVIDER_CHARGE
    assert task.metadata.economics_performance_assertions[1].metric_kind == PerformanceMetricKind.PER_ROLE_CALLS
    assert task.metadata.economics_performance_assertions[2].metric_kind == PerformanceMetricKind.STAGE_LATENCY


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_economics_performance_loader_all_fixture_rows_declare_typed_assertions():
    """Every row in the immutable economics_performance fixture must declare at least one
    typed economics-performance assertion with a valid metric kind and task complexity."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "economics_performance" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 8
    for row in rows:
        assertions = row.get("economics_performance_assertions", [])
        assert len(assertions) >= 1, f"row {row['key']} declares no economics_performance_assertions"
        for a in assertions:
            assert len(a["assertion_id"]) >= 1, (
                f"row {row['key']} assertion has empty assertion_id"
            )
            assert len(a["metric_kind"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty metric_kind"
            )
            assert len(a["action_class"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty action_class"
            )
            assert len(a["task_complexity"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty task_complexity"
            )
            assert len(a["unit"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty unit"
            )


@pytest.mark.unit
def test_economics_performance_loader_fixture_rows_cover_all_metric_kinds():
    """The fixture covers all six declared PerformanceMetricKind values,
    plus two measured-failure cases."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "economics_performance" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    metric_kinds = set()
    for row in rows:
        for a in row.get("economics_performance_assertions", []):
            metric_kinds.add(a["metric_kind"])
    assert "provider_charge" in metric_kinds
    assert "per_role_calls" in metric_kinds
    assert "per_role_tokens" in metric_kinds
    assert "stage_latency" in metric_kinds
    assert "local_resource_metadata" in metric_kinds
    assert "human_wait_time" in metric_kinds
    assert len(metric_kinds) == 6


@pytest.mark.unit
def test_economics_performance_loader_fixture_rows_cover_all_complexity_strata():
    """The fixture covers all three declared TaskComplexity values."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "economics_performance" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    complexity_strata = set()
    for row in rows:
        for a in row.get("economics_performance_assertions", []):
            complexity_strata.add(a["task_complexity"])
    assert "simple" in complexity_strata
    assert "moderate" in complexity_strata
    assert "complex" in complexity_strata
    assert len(complexity_strata) == 3


@pytest.mark.unit
def test_economics_performance_loader_fixture_rows_declare_economics_params():
    """Every fixture row carries scenario_params.economics_params so the
    CLI observer setup can configure the LocalEconomicsPerformanceSimulator."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "economics_performance" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    for row in rows:
        params = row.get("scenario_params", {}).get("economics_params", {})
        assert len(params) >= 1, (
            f"row {row['key']} has no economics_params in scenario_params"
        )
        for assertion_id, p in params.items():
            assert "observed_value" in p, (
                f"row {row['key']} economics_params for {assertion_id} missing observed_value"
            )


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_economics_performance_fixture_provenance_matches_dataset():
    from pathlib import Path

    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "economics_performance"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "economics_performance"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["economics", "performance"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/economics/loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_economics_performance_loader_suite_id_is_economics_performance():
    assert EconomicsPerformanceLoader.SUITE_ID == "economics_performance"
