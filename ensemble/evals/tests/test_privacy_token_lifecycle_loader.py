# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the synthetic privacy token lifecycle suite loader and provenance validation.

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

from g8e_evals.benchmarks.privacy.loader import PrivacyTokenLifecycleLoader
from g8e_evals.benchmarks.privacy.provenance import (
    SyntheticSuiteOutput,
    SyntheticSuiteProvenance,
    SyntheticSuiteSource,
    validate_dataset,
    validate_provenance,
)
from g8e_evals.schema import (
    StateCollectionBoundary,
    TokenPersistenceFailureOutcome,
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
        benchmark="privacy_token_lifecycle",
        source=_make_source(),
        output=_make_output(path=path, rows=rows, sha256=sha256),
        partition="development",
        domain_strata=["privacy"],
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
    assert provenance.benchmark == "privacy_token_lifecycle"
    assert provenance.schema_version == 1
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.output.rows == 1


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_source_fields():
    with pytest.raises(ValueError, match="source"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="privacy_token_lifecycle",
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
            partition="development",
            domain_strata=["privacy"],
        )


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_output_fields():
    with pytest.raises(ValueError, match="output"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="privacy_token_lifecycle",
            source=_make_source(),
            output=cast(
                SyntheticSuiteOutput,
                {
                    "path": "input_data.jsonl",
                    "rows": 1,
                },
            ),
            partition="development",
            domain_strata=["privacy"],
        )


@pytest.mark.unit
def test_synthetic_suite_provenance_records_partition_and_domain_strata():
    provenance = _make_provenance()
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["privacy"]


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_partition():
    base = _make_provenance().model_dump()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_missing_domain_strata():
    base = _make_provenance().model_dump()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance.model_validate(base)


@pytest.mark.unit
def test_synthetic_suite_provenance_rejects_empty_domain_strata_entry():
    with pytest.raises(ValueError, match="domain_strata"):
        SyntheticSuiteProvenance(
            schema_version=1,
            benchmark="privacy_token_lifecycle",
            source=_make_source(),
            output=_make_output(),
            partition="development",
            domain_strata=["privacy", ""],
        )


# --- validate_provenance ---


@pytest.mark.unit
def test_validate_provenance_accepts_complete_manifest():
    validate_provenance(_make_provenance())


@pytest.mark.unit
def test_validate_provenance_rejects_zero_schema_version():
    provenance = _make_provenance()
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance)


@pytest.mark.unit
def test_validate_provenance_rejects_empty_partition():
    provenance = _make_provenance()
    provenance.partition = ""
    with pytest.raises(ValueError, match="partition"):
        validate_provenance(provenance)


@pytest.mark.unit
def test_validate_provenance_rejects_empty_domain_strata():
    provenance = _make_provenance()
    provenance.domain_strata = []
    with pytest.raises(ValueError, match="domain_strata"):
        validate_provenance(provenance)


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


# --- Loader ---


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_persistence_assertions(monkeypatch):
    row = {
        "key": "task-persist-001",
        "description": "Verify token store persistence.",
        "category": "privacy",
        "expected_action_class": "TOKEN_STORE_PRIVACY_PROBE",
        "token_store_persistence_assertions": [
            {
                "assertion_id": "persist-assert-1",
                "collection_boundary": "encrypted_token_store",
                "expected_encryption_at_rest": True,
                "expected_fail_closed_on_lock": True,
                "expected_persistence_across_restart": True,
                "expected_ttl_seconds": 3600,
                "expected_restored_token_count": 2,
            }
        ],
        "scenario_params": {"tokens": [], "graders": ["token_store_persistence"]},
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

    tasks = list(PrivacyTokenLifecycleLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "task-persist-001"
    assert task.metadata.benchmark == "privacy_token_lifecycle"
    assert task.metadata.category == "privacy"
    assert task.metadata.expected_action_class == "TOKEN_STORE_PRIVACY_PROBE"
    assert len(task.metadata.token_store_persistence_assertions) == 1
    assertion = task.metadata.token_store_persistence_assertions[0]
    assert assertion.assertion_id == "persist-assert-1"
    assert assertion.collection_boundary == StateCollectionBoundary.ENCRYPTED_TOKEN_STORE
    assert assertion.expected_encryption_at_rest is True
    assert assertion.expected_fail_closed_on_lock is True
    assert assertion.expected_persistence_across_restart is True
    assert assertion.expected_restored_token_count == 2
    assert task.metadata.benchmark_specific.get("graders") == ["token_store_persistence"]


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_ttl_expiry_assertions(monkeypatch):
    row = {
        "key": "task-ttl-001",
        "description": "Verify token TTL expiry.",
        "token_ttl_expiry_assertions": [
            {
                "assertion_id": "ttl-assert-1",
                "collection_boundary": "encrypted_token_store",
                "expected_ttl_seconds": 60,
                "expected_visible_before_expiry": True,
                "expected_invisible_after_expiry": True,
                "expected_expiry_tolerance_seconds": 1,
            }
        ],
        "scenario_params": {"graders": ["token_ttl_expiry"]},
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

    tasks = list(PrivacyTokenLifecycleLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "task-ttl-001"
    assert len(task.metadata.token_ttl_expiry_assertions) == 1
    assertion = task.metadata.token_ttl_expiry_assertions[0]
    assert assertion.assertion_id == "ttl-assert-1"
    assert assertion.expected_ttl_seconds == 60
    assert assertion.expected_expiry_tolerance_seconds == 1


@pytest.mark.unit
def test_loader_produces_typed_tasks_with_persistence_failure_assertions(monkeypatch):
    row = {
        "key": "task-fail-001",
        "description": "Verify token persistence failure.",
        "token_persistence_failure_assertions": [
            {
                "assertion_id": "fail-assert-1",
                "collection_boundary": "encrypted_token_store",
                "expected_fail_closed": True,
                "expected_rollback": True,
                "expected_no_sensitive_leak": True,
                "expected_no_unsafe_continuation": True,
                "expected_failure_outcome": "storage_failure",
            }
        ],
        "scenario_params": {"graders": ["token_persistence_failure"]},
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

    tasks = list(PrivacyTokenLifecycleLoader(mock_path).load())
    assert len(tasks) == 1
    task = tasks[0]
    assert task.id == "task-fail-001"
    assert len(task.metadata.token_persistence_failure_assertions) == 1
    assertion = task.metadata.token_persistence_failure_assertions[0]
    assert assertion.assertion_id == "fail-assert-1"
    assert assertion.expected_fail_closed is True
    assert assertion.expected_rollback is True
    assert assertion.expected_failure_outcome == TokenPersistenceFailureOutcome.STORAGE_FAILURE


@pytest.mark.unit
def test_loader_raises_file_not_found_for_missing_gold_set(monkeypatch):
    mock_path: Any = SimpleNamespace(
        name="input_data.jsonl",
        exists=lambda: False,
    )
    with pytest.raises(FileNotFoundError, match="gold set not found"):
        list(PrivacyTokenLifecycleLoader(mock_path).load())


@pytest.mark.unit
def test_loader_skips_blank_lines_in_dataset(monkeypatch):
    row = {
        "key": "task-blank-001",
        "description": "Verify blank line handling.",
        "token_store_persistence_assertions": [
            {
                "assertion_id": "blank-assert-1",
                "collection_boundary": "encrypted_token_store",
                "expected_ttl_seconds": 60,
                "expected_restored_token_count": 1,
            }
        ],
        "scenario_params": {"graders": ["token_store_persistence"]},
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

    tasks = list(PrivacyTokenLifecycleLoader(mock_path).load())
    assert len(tasks) == 2
    assert tasks[0].id == "task-blank-001"
    assert tasks[1].id == "task-blank-001"


@pytest.mark.unit
def test_loader_applies_default_values_for_optional_assertion_fields(monkeypatch):
    row = {
        "key": "task-defaults-001",
        "description": "Verify default assertion values.",
        "token_store_persistence_assertions": [
            {
                "assertion_id": "defaults-assert-1",
                "expected_ttl_seconds": 300,
                "expected_restored_token_count": 1,
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

    tasks = list(PrivacyTokenLifecycleLoader(mock_path).load())
    task = tasks[0]
    assertion = task.metadata.token_store_persistence_assertions[0]
    assert assertion.expected_encryption_at_rest is True
    assert assertion.expected_fail_closed_on_lock is True
    assert assertion.expected_persistence_across_restart is True
    assert assertion.collection_boundary == StateCollectionBoundary.ENCRYPTED_TOKEN_STORE
