# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic reliability suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem. File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).

The reliability suite covers all twelve declared reliability failure
scenarios (provider throttling, malformed structured output, interrupted
streams, duplicate events, delayed events, audit-store unavailability,
operator disconnect, operator restart, locked vault, signing failure,
concurrent state mutations, and replay races) plus two measured-failure
cases. Each task declares typed ``ReliabilityAssertion`` records that the
``ReliabilityGrader`` evaluates against independently observed
handling-behavior records.
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
from g8e_evals.benchmarks.reliability.loader import ReliabilityLoader
from g8e_evals.schema import (
    ReliabilityExpectedBehavior,
    ReliabilityScenarioType,
    StateCollectionBoundary,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/reliability/loader.py",
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
        benchmark="reliability",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["reliability"],
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


def _reliability_assertion(assertion_id: str = "reliability-assert-1", **overrides: Any) -> dict:
    base = {
        "assertion_id": assertion_id,
        "scenario_type": "provider_throttling",
        "action_type": "CHAT_COMPLETION",
        "expected_behavior": "retry_with_backoff",
        "expected_evidence_preserved": True,
        "collection_boundary": "operator_workload",
    }
    base.update(overrides)
    return base


def _reliability_row(key: str = "reliability-001", **overrides: Any) -> dict:
    base = {
        "key": key,
        "description": "The provider returns a 429 throttling error. The system must retry with exponential backoff.",
        "category": "reliability",
        "expected_action_class": "RELIABILITY",
        "reliability_assertions": [_reliability_assertion()],
        "scenario_params": {
            "graders": ["reliability"],
            "reliability_params": {
                "reliability-assert-1": {
                    "observed_behavior": "retry_with_backoff",
                    "evidence_preserved": True,
                },
            },
        },
    }
    base.update(overrides)
    return base


def _stub_loader(monkeypatch, content: bytes, provenance: SyntheticSuiteProvenance | None = None):
    mock_path = _mock_path(content)
    monkeypatch.setattr(
        "g8e_evals.benchmarks.reliability.loader.load_provenance",
        lambda _path: provenance or _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.reliability.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.reliability.loader.validate_dataset",
        lambda _path, _prov: None,
    )
    return mock_path


# --- Provenance model validation ---


@pytest.mark.unit
def test_reliability_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "reliability"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_reliability_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["reliability"]


@pytest.mark.unit
def test_reliability_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_reliability_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_reliability_manifest(monkeypatch):
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.provenance._verify_code_digest",
        lambda code_path, expected_sha256, trusted_root: None,
    )
    validate_provenance(_make_provenance(), suite_id="reliability", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version_for_reliability():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, suite_id="reliability", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition_for_reliability():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance, suite_id="reliability", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata_for_reliability():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance, suite_id="reliability", trusted_root=Path("."))


@pytest.mark.unit
def test_validate_provenance_rejects_suite_substitution_for_reliability():
    provenance = _make_provenance()
    with pytest.raises(ValueError, match="benchmark"):
        validate_provenance(provenance, suite_id="wrong_suite", trusted_root=Path("."))


# --- validate_dataset ---


@pytest.mark.unit
def test_validate_dataset_accepts_matching_reliability_content():
    content = b'{"key": "task-1"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_sha256_mismatch_for_reliability():
    content = b'{"key": "task-1"}\n'
    provenance = _make_provenance(rows=1, sha256="0" * 64)
    with pytest.raises(ValueError, match="dataset SHA-256 mismatch"):
        validate_dataset(_mock_path(content), provenance)


@pytest.mark.unit
def test_validate_dataset_rejects_row_count_mismatch_for_reliability():
    content = b'{"key": "task-1"}\n{"key": "task-2"}\n'
    sha = hashlib.sha256(content).hexdigest()
    provenance = _make_provenance(rows=1, sha256=sha)
    with pytest.raises(ValueError, match="dataset row count mismatch"):
        validate_dataset(_mock_path(content), provenance)


# --- Loader: basic typed task production ---


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_reliability_assertions(monkeypatch):
    row = _reliability_row(
        key="reliability-throttling-001",
        description="The provider returns a 429 throttling error. The system must retry with exponential backoff and preserve evidence.",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-throttling-assert-1",
                scenario_type="provider_throttling",
                action_type="CHAT_COMPLETION",
                expected_behavior="retry_with_backoff",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "reliability-throttling-001"
    assert task.metadata.benchmark == "reliability"
    assert task.metadata.category == "reliability"
    assert task.metadata.expected_action_class == "RELIABILITY"
    assert len(task.metadata.reliability_assertions) == 1
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.assertion_id == "reliability-throttling-assert-1"
    assert assertion.scenario_type == ReliabilityScenarioType.PROVIDER_THROTTLING
    assert assertion.action_type == "CHAT_COMPLETION"
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.RETRY_WITH_BACKOFF
    assert assertion.expected_evidence_preserved is True
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert task.metadata.benchmark_specific.get("graders") == ["reliability"]


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_malformed_output_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-malformed-output-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-malformed-output-assert-1",
                scenario_type="malformed_structured_output",
                action_type="STRUCTURED_OUTPUT",
                expected_behavior="fail_closed",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.MALFORMED_STRUCTURED_OUTPUT
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.FAIL_CLOSED


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_interrupted_stream_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-interrupted-stream-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-interrupted-stream-assert-1",
                scenario_type="interrupted_stream",
                action_type="STREAM_COMPLETION",
                expected_behavior="fail_closed",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.INTERRUPTED_STREAM


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_audit_store_unavailable_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-audit-store-unavailable-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-audit-store-assert-1",
                scenario_type="audit_store_unavailable",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="fail_closed",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.AUDIT_STORE_UNAVAILABLE


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_signing_failure_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-signing-failure-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-signing-failure-assert-1",
                scenario_type="signing_failure",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="fail_closed",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.SIGNING_FAILURE


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_wrong_behavior_measured_failure(monkeypatch):
    row = _reliability_row(
        key="reliability-wrong-behavior-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-wrong-behavior-assert-1",
                scenario_type="provider_throttling",
                action_type="CHAT_COMPLETION",
                expected_behavior="retry_with_backoff",
            ),
        ],
        scenario_params={
            "graders": ["reliability"],
            "reliability_params": {
                "reliability-wrong-behavior-assert-1": {
                    "observed_behavior": "degrade_gracefully",
                    "evidence_preserved": True,
                },
            },
        },
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.RETRY_WITH_BACKOFF
    params = task.metadata.benchmark_specific.get("reliability_params", {})
    assert params["reliability-wrong-behavior-assert-1"]["observed_behavior"] == "degrade_gracefully"


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_no_evidence_measured_failure(monkeypatch):
    row = _reliability_row(
        key="reliability-no-evidence-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-no-evidence-assert-1",
                scenario_type="provider_throttling",
                action_type="CHAT_COMPLETION",
                expected_behavior="retry_with_backoff",
            ),
        ],
        scenario_params={
            "graders": ["reliability"],
            "reliability_params": {
                "reliability-no-evidence-assert-1": {
                    "observed_behavior": "retry_with_backoff",
                    "evidence_preserved": False,
                },
            },
        },
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    params = task.metadata.benchmark_specific.get("reliability_params", {})
    assert params["reliability-no-evidence-assert-1"]["evidence_preserved"] is False


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_duplicate_event_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-duplicate-event-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-duplicate-event-assert-1",
                scenario_type="duplicate_event",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="deduplicate",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.DUPLICATE_EVENT
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.DEDUPLICATE


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_delayed_event_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-delayed-event-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-delayed-event-assert-1",
                scenario_type="delayed_event",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="detect_and_reject",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.DELAYED_EVENT
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.DETECT_AND_REJECT


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_operator_disconnect_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-operator-disconnect-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-operator-disconnect-assert-1",
                scenario_type="operator_disconnect",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="reconnect_recover",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.OPERATOR_DISCONNECT
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.RECONNECT_RECOVER


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_operator_restart_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-operator-restart-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-operator-restart-assert-1",
                scenario_type="operator_restart",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="reconnect_recover",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.OPERATOR_RESTART
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.RECONNECT_RECOVER


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_locked_vault_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-locked-vault-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-locked-vault-assert-1",
                scenario_type="locked_vault",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="fail_closed",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.LOCKED_VAULT
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.FAIL_CLOSED


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_concurrent_state_mutation_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-concurrent-mutation-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-concurrent-mutation-assert-1",
                scenario_type="concurrent_state_mutation",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="serialize_order",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.CONCURRENT_STATE_MUTATION
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.SERIALIZE_ORDER


@pytest.mark.unit
def test_reliability_loader_produces_typed_tasks_with_replay_race_scenario(monkeypatch):
    row = _reliability_row(
        key="reliability-replay-race-001",
        reliability_assertions=[
            _reliability_assertion(
                assertion_id="reliability-replay-race-assert-1",
                scenario_type="replay_race",
                action_type="GOVERNANCE_ACTION",
                expected_behavior="detect_and_reject",
            ),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.scenario_type == ReliabilityScenarioType.REPLAY_RACE
    assert assertion.expected_behavior == ReliabilityExpectedBehavior.DETECT_AND_REJECT


@pytest.mark.unit
def test_reliability_loader_sets_default_category_and_action_class(monkeypatch):
    row = {
        "key": "reliability-defaults-001",
        "description": "Verify default category and action class.",
        "reliability_assertions": [_reliability_assertion()],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "reliability"
    assert task.metadata.expected_action_class == "RELIABILITY"


@pytest.mark.unit
def test_reliability_loader_applies_default_collection_boundary(monkeypatch):
    row = {
        "key": "reliability-defaults-cb-001",
        "description": "Verify default collection boundary.",
        "reliability_assertions": [{
            "assertion_id": "defaults-cb-assert-1",
            "scenario_type": "provider_throttling",
            "action_type": "CHAT_COMPLETION",
            "expected_behavior": "retry_with_backoff",
        }],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.reliability_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


# --- Loader: error paths ---


@pytest.mark.unit
def test_reliability_loader_raises_file_not_found_for_missing_gold_set():
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(ReliabilityLoader(mock_path).load())


@pytest.mark.unit
def test_reliability_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = _reliability_row(key="reliability-blank-001")
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _stub_loader(monkeypatch, content, provenance=_make_provenance(rows=3))

    tasks = list(ReliabilityLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "reliability-blank-001"
    assert tasks[1].id == "reliability-blank-001"


# --- Loader: multiple assertions ---


@pytest.mark.unit
def test_reliability_loader_handles_multiple_assertions_in_one_task(monkeypatch):
    row = _reliability_row(
        key="reliability-multi-001",
        reliability_assertions=[
            _reliability_assertion(assertion_id="multi-assert-1", scenario_type="provider_throttling", action_type="CHAT_COMPLETION", expected_behavior="retry_with_backoff"),
            _reliability_assertion(assertion_id="multi-assert-2", scenario_type="signing_failure", action_type="GOVERNANCE_ACTION", expected_behavior="fail_closed"),
            _reliability_assertion(assertion_id="multi-assert-3", scenario_type="audit_store_unavailable", action_type="GOVERNANCE_ACTION", expected_behavior="fail_closed"),
        ],
    )
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _stub_loader(monkeypatch, content)

    tasks = list(ReliabilityLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.reliability_assertions) == 3
    assert task.metadata.reliability_assertions[0].assertion_id == "multi-assert-1"
    assert task.metadata.reliability_assertions[1].assertion_id == "multi-assert-2"
    assert task.metadata.reliability_assertions[2].assertion_id == "multi-assert-3"
    assert task.metadata.reliability_assertions[0].scenario_type == ReliabilityScenarioType.PROVIDER_THROTTLING
    assert task.metadata.reliability_assertions[1].scenario_type == ReliabilityScenarioType.SIGNING_FAILURE
    assert task.metadata.reliability_assertions[2].scenario_type == ReliabilityScenarioType.AUDIT_STORE_UNAVAILABLE


# --- Loader: suite invariant ---


@pytest.mark.unit
def test_reliability_loader_all_fixture_rows_declare_typed_assertions():
    """Every row in the immutable reliability fixture must declare at least one
    typed reliability assertion with a valid scenario type and expected behavior."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "reliability" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    assert len(rows) == 14
    for row in rows:
        assertions = row.get("reliability_assertions", [])
        assert len(assertions) >= 1, f"row {row['key']} declares no reliability_assertions"
        for a in assertions:
            assert len(a["assertion_id"]) >= 1, (
                f"row {row['key']} assertion has empty assertion_id"
            )
            assert len(a["scenario_type"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty scenario_type"
            )
            assert len(a["action_type"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty action_type"
            )
            assert len(a["expected_behavior"]) >= 1, (
                f"row {row['key']} assertion {a['assertion_id']} has empty expected_behavior"
            )


@pytest.mark.unit
def test_reliability_loader_fixture_rows_cover_all_scenario_types():
    """The fixture covers all twelve declared ReliabilityScenarioType values,
    plus two measured-failure cases."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "reliability" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    scenario_types = set()
    for row in rows:
        for a in row.get("reliability_assertions", []):
            scenario_types.add(a["scenario_type"])
    assert "provider_throttling" in scenario_types
    assert "malformed_structured_output" in scenario_types
    assert "interrupted_stream" in scenario_types
    assert "audit_store_unavailable" in scenario_types
    assert "signing_failure" in scenario_types
    assert "duplicate_event" in scenario_types
    assert "delayed_event" in scenario_types
    assert "operator_disconnect" in scenario_types
    assert "operator_restart" in scenario_types
    assert "locked_vault" in scenario_types
    assert "concurrent_state_mutation" in scenario_types
    assert "replay_race" in scenario_types
    assert len(scenario_types) == 12


@pytest.mark.unit
def test_reliability_loader_fixture_rows_declare_reliability_params():
    """Every fixture row carries scenario_params.reliability_params so the
    CLI observer setup can configure the LocalReliabilitySimulator."""
    from pathlib import Path

    fixture_path = Path(__file__).resolve().parents[1] / "gold_sets" / "reliability" / "input_data.jsonl"
    rows = [json.loads(line) for line in fixture_path.read_text().splitlines() if line.strip()]
    for row in rows:
        params = row.get("scenario_params", {}).get("reliability_params", {})
        assert len(params) >= 1, (
            f"row {row['key']} has no reliability_params in scenario_params"
        )
        for assertion_id, p in params.items():
            assert "observed_behavior" in p, (
                f"row {row['key']} reliability_params for {assertion_id} missing observed_behavior"
            )
            assert "evidence_preserved" in p, (
                f"row {row['key']} reliability_params for {assertion_id} missing evidence_preserved"
            )


# --- Loader: provenance manifest matches fixture ---


@pytest.mark.unit
def test_reliability_fixture_provenance_matches_dataset():
    from pathlib import Path

    fixture_dir = Path(__file__).resolve().parents[1] / "gold_sets" / "reliability"
    data_path = fixture_dir / "input_data.jsonl"
    prov_path = fixture_dir / "provenance.json"

    content = data_path.read_bytes()
    rows = [line for line in content.decode().splitlines() if line.strip()]
    expected_sha = hashlib.sha256(content).hexdigest()

    provenance = json.loads(prov_path.read_text())
    assert provenance["benchmark"] == "reliability"
    assert provenance["output"]["rows"] == len(rows)
    assert provenance["output"]["sha256"] == expected_sha
    assert provenance["partition"] == "development"
    assert provenance["domain_strata"] == ["reliability"]
    assert provenance["source"]["code_path"] == "g8e_evals/benchmarks/reliability/loader.py"


# --- Loader: suite ID ---


@pytest.mark.unit
def test_reliability_loader_suite_id_is_reliability():
    assert ReliabilityLoader.SUITE_ID == "reliability"
