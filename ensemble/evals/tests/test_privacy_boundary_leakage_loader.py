# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic privacy boundary leakage suite loader and provenance validation.

These tests exercise the loader and provenance models without touching the
filesystem.  File I/O is stubbed via ``SimpleNamespace`` mocks and
``monkeypatch`` so the tests remain pure Tier 1 (no files, network, or DB).
"""

from __future__ import annotations

import hashlib
import json
from types import SimpleNamespace
from typing import Any, cast

import pytest

from g8e_evals.benchmarks.privacy.loader import PrivacyBoundaryLeakageLoader
from g8e_evals.benchmarks.privacy.provenance import (
    SyntheticSuiteOutput,
    SyntheticSuiteProvenance,
    SyntheticSuiteSource,
    validate_dataset,
)
from g8e_evals.schema import (
    ArtifactCollectionBoundary,
    RejectionLayer,
    SensitiveArtifactContentType,
    StateCollectionBoundary,
    StateEvidenceKind,
)


def _make_source() -> SyntheticSuiteSource:
    return SyntheticSuiteSource(
        repository="https://example.com/repo",
        revision="abc123",
        license_spdx="Apache-2.0",
        code_path="g8e_evals/benchmarks/privacy/loader.py",
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
        benchmark="privacy_boundary_leakage",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
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


# --- Provenance model validation ---


@pytest.mark.unit
def test_synthetic_suite_provenance_accepts_valid_manifest():
    provenance = _make_provenance()
    assert provenance.benchmark == "privacy_boundary_leakage"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_source_fields():
    with pytest.raises(ValueError, match="source"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="privacy_boundary_leakage",
            source=cast(
                SyntheticSuiteSource,
                {
                    "repository": "https://example.com/repo",
                    "revision": "abc123",
                    "license_spdx": "Apache-2.0",
                    "code_path": "g8e_evals/benchmarks/privacy/loader.py",
                },
            ),
            output=_make_output(),
        )


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_output_fields():
    with pytest.raises(ValueError, match="output"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="privacy_boundary_leakage",
            source=_make_source(),
            output=cast(
                SyntheticSuiteOutput,
                {
                    "path": "input_data.jsonl",
                    "rows": 1,
                },
            ),
        )


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


# --- Loader: exfiltration attempt assertions ---


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_exfiltration_attempt_assertions(monkeypatch):
    row = {
        "key": "privacy-exfiltration-001",
        "description": "Verify cross-boundary exfiltration rejection.",
        "category": "privacy",
        "expected_action_class": "PRIVACY_BOUNDARY_LEAKAGE_PROBE",
        "exfiltration_attempt_assertions": [
            {
                "assertion_id": "exfiltration-assert-1",
                "action_type": "DATA_EXFILTRATION",
                "source": "conversation_history:user",
                "destination": "model_boundary:provider",
                "collection_boundary": "operator_workload",
                "expected_rejection_layer": "l4_verification",
                "expected_absence": {"kind": "file", "exists": False},
            }
        ],
        "scenario_params": {"graders": ["exfiltration_attempt"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "privacy-exfiltration-001"
    assert task.metadata.benchmark == "privacy_boundary_leakage"
    assert task.metadata.category == "privacy"
    assert task.metadata.expected_action_class == "PRIVACY_BOUNDARY_LEAKAGE_PROBE"
    assert len(task.metadata.exfiltration_attempt_assertions) == 1
    assertion = task.metadata.exfiltration_attempt_assertions[0]
    assert assertion.assertion_id == "exfiltration-assert-1"
    assert assertion.action_type == "DATA_EXFILTRATION"
    assert assertion.source == "conversation_history:user"
    assert assertion.destination == "model_boundary:provider"
    assert assertion.expected_rejection_layer == RejectionLayer.L4_VERIFICATION
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False
    assert task.metadata.benchmark_specific.get("graders") == ["exfiltration_attempt"]


# --- Loader: artifact leakage assertions ---


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_artifact_leakage_assertions(monkeypatch):
    row = {
        "key": "privacy-artifact-leakage-001",
        "description": "Verify emitted artifact contains no sensitive content.",
        "artifact_leakage_assertions": [
            {
                "assertion_id": "artifact-leakage-assert-1",
                "artifact_class": "summary_json",
                "collection_boundary": "report_directory",
                "expected_absent_sensitive_types": ["raw_canary", "credential"],
                "expected_artifact_present": True,
            }
        ],
        "scenario_params": {"graders": ["artifact_leakage"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "privacy-artifact-leakage-001"
    assert len(task.metadata.artifact_leakage_assertions) == 1
    assertion = task.metadata.artifact_leakage_assertions[0]
    assert assertion.assertion_id == "artifact-leakage-assert-1"
    assert assertion.artifact_class == "summary_json"
    assert assertion.collection_boundary == ArtifactCollectionBoundary.REPORT_DIRECTORY
    assert assertion.expected_absent_sensitive_types == [
        SensitiveArtifactContentType.RAW_CANARY,
        SensitiveArtifactContentType.CREDENTIAL,
    ]
    assert assertion.expected_artifact_present is True
    assert task.metadata.benchmark_specific.get("graders") == ["artifact_leakage"]


# --- Loader: rehydration assertions ---


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_rehydration_assertions(monkeypatch):
    row = {
        "key": "privacy-rehydration-001",
        "description": "Verify local rehydration restores all tokens.",
        "rehydration_assertions": [
            {
                "assertion_id": "rehydration-assert-1",
                "source": "local_runtime",
                "input_artifact_sha256": "a" * 64,
                "expected_output_artifact_sha256": "b" * 64,
                "expected_token_count": 2,
                "expected_sensitive_types": ["email", "ssn"],
            }
        ],
        "scenario_params": {
            "graders": ["exact_local_rehydration"],
            "rehydration_tokens": [
                {
                    "token_id": "rehyd-token-1",
                    "value": "secret-alpha",
                    "sensitive_type": "email",
                    "created_at": "2026-09-04T12:00:00+00:00",
                    "expires_at": "2026-09-04T13:00:00+00:00",
                },
                {
                    "token_id": "rehyd-token-2",
                    "value": "secret-beta",
                    "sensitive_type": "ssn",
                    "created_at": "2026-09-04T12:00:00+00:00",
                    "expires_at": "2026-09-04T13:00:00+00:00",
                },
            ],
        },
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "privacy-rehydration-001"
    assert len(task.metadata.rehydration_assertions) == 1
    assertion = task.metadata.rehydration_assertions[0]
    assert assertion.assertion_id == "rehydration-assert-1"
    assert assertion.source == "local_runtime"
    assert assertion.input_artifact_sha256 == "a" * 64
    assert assertion.expected_output_artifact_sha256 == "b" * 64
    assert assertion.expected_token_count == 2
    assert assertion.expected_sensitive_types == ["email", "ssn"]
    assert task.metadata.benchmark_specific.get("graders") == ["exact_local_rehydration"]
    rehydration_tokens = task.metadata.benchmark_specific.get("rehydration_tokens", [])
    assert len(rehydration_tokens) == 2
    assert rehydration_tokens[0]["token_id"] == "rehyd-token-1"


# --- Loader: edge cases ---


@pytest.mark.unit
def test_loader_raises_file_not_found_for_missing_gold_set(monkeypatch):
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(PrivacyBoundaryLeakageLoader(mock_path).load())


@pytest.mark.unit
def test_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = {
        "key": "privacy-exfiltration-blank-001",
        "description": "Verify blank line handling.",
        "exfiltration_attempt_assertions": [
            {
                "assertion_id": "exfiltration-blank-assert-1",
                "action_type": "DATA_EXFILTRATION",
                "source": "conversation_history:user",
                "destination": "model_boundary:provider",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "scenario_params": {"graders": ["exfiltration_attempt"]},
    }
    content = (
        json.dumps(row, sort_keys=True) + "\n"
        + "\n"
        + json.dumps(row, sort_keys=True) + "\n"
    ).encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(rows=3),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "privacy-exfiltration-blank-001"
    assert tasks[1].id == "privacy-exfiltration-blank-001"


@pytest.mark.unit
def test_loader_applies_default_values_for_optional_exfiltration_fields(monkeypatch):
    row = {
        "key": "privacy-exfiltration-defaults-001",
        "description": "Verify default assertion values.",
        "exfiltration_attempt_assertions": [
            {
                "assertion_id": "exfiltration-defaults-assert-1",
                "action_type": "DATA_EXFILTRATION",
                "source": "conversation_history:user",
                "destination": "model_boundary:provider",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.exfiltration_attempt_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD
    assert assertion.expected_absence.kind == StateEvidenceKind.FILE
    assert assertion.expected_absence.exists is False


@pytest.mark.unit
def test_loader_applies_default_values_for_optional_artifact_leakage_fields(monkeypatch):
    row = {
        "key": "privacy-artifact-defaults-001",
        "description": "Verify default artifact leakage values.",
        "artifact_leakage_assertions": [
            {
                "assertion_id": "artifact-defaults-assert-1",
                "artifact_class": "summary_json",
                "expected_absent_sensitive_types": ["raw_canary"],
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.artifact_leakage_assertions[0]
    assert assertion.collection_boundary == ArtifactCollectionBoundary.REPORT_DIRECTORY
    assert assertion.expected_artifact_present is True


@pytest.mark.unit
def test_loader_parses_ledger_consistency_absence_for_exfiltration(monkeypatch):
    row = {
        "key": "privacy-exfiltration-ledger-001",
        "description": "Verify ledger absence parsing for exfiltration.",
        "exfiltration_attempt_assertions": [
            {
                "assertion_id": "exfiltration-ledger-assert-1",
                "action_type": "DATA_EXFILTRATION",
                "source": "conversation_history:user",
                "destination": "model_boundary:provider",
                "expected_rejection_layer": "l4_verification",
                "collection_boundary": "governance_ledger",
                "expected_absence": {"kind": "ledger_consistency"},
            }
        ],
        "scenario_params": {"graders": ["exfiltration_attempt"]},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.exfiltration_attempt_assertions[0]
    assert assertion.collection_boundary == StateCollectionBoundary.GOVERNANCE_LEDGER
    assert assertion.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY
    assert assertion.expected_absence.consistent is False


@pytest.mark.unit
def test_loader_produces_tasks_with_default_category_and_action_class(monkeypatch):
    row = {
        "key": "privacy-defaults-category-001",
        "description": "Verify default category and action class.",
        "exfiltration_attempt_assertions": [
            {
                "assertion_id": "exfiltration-cat-assert-1",
                "action_type": "DATA_EXFILTRATION",
                "source": "conversation_history:user",
                "destination": "model_boundary:provider",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "scenario_params": {},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    task = tasks[0]
    assert task.metadata.category == "privacy"
    assert task.metadata.expected_action_class == "PRIVACY_BOUNDARY_LEAKAGE_PROBE"


@pytest.mark.unit
def test_loader_handles_multiple_assertion_types_in_one_task(monkeypatch):
    row = {
        "key": "privacy-multi-assert-001",
        "description": "Verify multiple assertion types in one task.",
        "exfiltration_attempt_assertions": [
            {
                "assertion_id": "multi-exfiltration-1",
                "action_type": "DATA_EXFILTRATION",
                "source": "conversation_history:user",
                "destination": "model_boundary:provider",
                "expected_rejection_layer": "l4_verification",
            }
        ],
        "artifact_leakage_assertions": [
            {
                "assertion_id": "multi-artifact-1",
                "artifact_class": "summary_json",
                "expected_absent_sensitive_types": ["raw_canary"],
            }
        ],
        "rehydration_assertions": [
            {
                "assertion_id": "multi-rehydration-1",
                "source": "local_runtime",
                "input_artifact_sha256": "a" * 64,
                "expected_output_artifact_sha256": "b" * 64,
                "expected_token_count": 1,
                "expected_sensitive_types": ["email"],
            }
        ],
        "scenario_params": {
            "graders": ["exfiltration_attempt", "artifact_leakage", "exact_local_rehydration"],
        },
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    task = tasks[0]
    assert len(task.metadata.exfiltration_attempt_assertions) == 1
    assert len(task.metadata.artifact_leakage_assertions) == 1
    assert len(task.metadata.rehydration_assertions) == 1
    assert task.metadata.artifact_leakage_assertions[0].artifact_class == "summary_json"
    assert task.metadata.rehydration_assertions[0].expected_sensitive_types == ["email"]


@pytest.mark.unit
def test_loader_handles_task_with_no_assertions(monkeypatch):
    row = {
        "key": "privacy-no-assertions-001",
        "description": "Verify task with no assertions loads without error.",
        "scenario_params": {"graders": []},
    }
    content = (json.dumps(row, sort_keys=True) + "\n").encode()
    mock_path = _mock_path(content)

    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.load_provenance",
        lambda _path: _make_provenance(),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.privacy.loader.validate_dataset",
        lambda _path, _prov: None,
    )

    tasks = list(PrivacyBoundaryLeakageLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "privacy-no-assertions-001"
    assert len(task.metadata.exfiltration_attempt_assertions) == 0
    assert len(task.metadata.artifact_leakage_assertions) == 0
    assert len(task.metadata.rehydration_assertions) == 0


@pytest.mark.unit
def test_loader_round_trips_real_gold_set_dataset_rows(tmp_path, monkeypatch):
    """The loader parses every row in the immutable gold set dataset."""
    gold_set = (
        __import__("g8e_evals", fromlist=["__path__"]).__path__[0]
    )
    # Resolve the gold set path relative to the ensemble/evals root.
    from pathlib import Path

    gold_set_root = Path(gold_set).parent / "gold_sets" / "privacy_boundary_leakage"
    input_path = gold_set_root / "input_data.jsonl"
    if not input_path.exists():
        pytest.skip("privacy_boundary_leakage gold set not available in this environment")

    loader = PrivacyBoundaryLeakageLoader(input_path)
    tasks = list(loader.load())
    assert len(tasks) == 3
    assert tasks[0].id == "privacy-exfiltration-attempt-001"
    assert tasks[1].id == "privacy-artifact-leakage-001"
    assert tasks[2].id == "privacy-exact-rehydration-001"
    assert len(tasks[0].metadata.exfiltration_attempt_assertions) == 1
    assert len(tasks[1].metadata.artifact_leakage_assertions) == 1
    assert len(tasks[2].metadata.rehydration_assertions) == 1
