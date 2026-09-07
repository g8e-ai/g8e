# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""P1-3 regression tests for executable-code provenance binding.

These tests prove that ``validate_provenance`` enforces executable-code
provenance: the declared ``code_path`` is resolved under a trusted root
with traversal protection, its SHA-256 digest is verified against
``code_sha256``, the provenance benchmark matches the loader's
``SUITE_ID``, and unknown fields are rejected by ``extra="forbid"``.

The tests cover both the synthetic-suite provenance module
(``g8e_evals.benchmarks.privacy.provenance``) and the IFEval provenance
module (``g8e_evals.benchmarks.ifeval.provenance``).
"""

from __future__ import annotations

import hashlib
from pathlib import Path

import pytest

from g8e_evals.benchmarks.ifeval.provenance import (
    DatasetProvenance,
    validate_provenance as validate_ifeval_provenance,
)
from g8e_evals.benchmarks.privacy.provenance import (
    SyntheticSuiteProvenance,
    _resolve_under_root,
    validate_provenance as validate_synthetic_provenance,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_synthetic_provenance(
    *,
    benchmark: str = "privacy_token_lifecycle",
    code_path: str = "g8e_evals/benchmarks/privacy/loader.py",
    code_sha256: str = "0" * 64,
    extra_field: bool = False,
) -> SyntheticSuiteProvenance:
    data = {
        "schema_version": 1,
        "benchmark": benchmark,
        "source": {
            "repository": "https://example.com/repo",
            "revision": "test",
            "license_spdx": "Apache-2.0",
            "code_path": code_path,
            "code_sha256": code_sha256,
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "a" * 64,
        },
        "partition": "development",
        "domain_strata": ["privacy"],
    }
    if extra_field:
        data["unexpected_extra"] = "should-be-rejected"
    return SyntheticSuiteProvenance.model_validate(data)


def _make_ifeval_provenance(
    *,
    benchmark: str = "ifeval_subset",
    code_path: str = "g8e_evals/benchmarks/ifeval/import_subset.py",
    code_sha256: str = "0" * 64,
    fixture_path: str = "g8e_evals/benchmarks/ifeval/import_subset.py",
    fixture_sha256: str = "0" * 64,
    extra_field: bool = False,
) -> DatasetProvenance:
    data = {
        "schema_version": 1,
        "benchmark": benchmark,
        "source": {
            "url": "https://example.com",
            "revision": "rev",
            "license_spdx": "Apache-2.0",
            "license_url": "https://example.com",
            "sha256": "0" * 64,
        },
        "selected_keys": [1001],
        "transformation": {
            "description": "stub",
            "code_path": code_path,
            "code_sha256": code_sha256,
            "fixture_path": fixture_path,
            "fixture_sha256": fixture_sha256,
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "0" * 64,
        },
        "partition": "development",
        "domain_strata": ["utility"],
    }
    if extra_field:
        data["unexpected_extra"] = "should-be-rejected"
    return DatasetProvenance.model_validate(data)


def _write_code_file(root: Path, rel_path: str, content: bytes = b"# test code\n") -> str:
    """Write a file under ``root`` at ``rel_path`` and return its SHA-256."""
    target = root / rel_path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(content)
    return hashlib.sha256(content).hexdigest()


# ---------------------------------------------------------------------------
# Code-tampering: declared code_sha256 does not match the actual file digest
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_synthetic_validate_provenance_rejects_code_tampering(tmp_path: Path):
    """A declared code_sha256 that does not match the actual file digest is rejected."""
    code_rel = "g8e_evals/benchmarks/privacy/loader.py"
    _write_code_file(tmp_path, code_rel, b"# real code\n")
    wrong_sha = "b" * 64
    provenance = _make_synthetic_provenance(code_path=code_rel, code_sha256=wrong_sha)

    with pytest.raises(ValueError, match="code_sha256 mismatch"):
        validate_synthetic_provenance(
            provenance,
            suite_id="privacy_token_lifecycle",
            trusted_root=tmp_path,
        )


@pytest.mark.unit
def test_synthetic_validate_provenance_accepts_correct_code_digest(tmp_path: Path):
    """A declared code_sha256 that matches the actual file digest is accepted."""
    code_rel = "g8e_evals/benchmarks/privacy/loader.py"
    actual_sha = _write_code_file(tmp_path, code_rel, b"# real code\n")
    provenance = _make_synthetic_provenance(code_path=code_rel, code_sha256=actual_sha)

    validate_synthetic_provenance(
        provenance,
        suite_id="privacy_token_lifecycle",
        trusted_root=tmp_path,
    )


@pytest.mark.unit
def test_ifeval_validate_provenance_rejects_code_tampering(tmp_path: Path):
    """A declared transformation code_sha256 that does not match the actual file digest is rejected."""
    code_rel = "g8e_evals/benchmarks/ifeval/import_subset.py"
    fixture_rel = "g8e_evals/benchmarks/ifeval/fixture.py"
    _write_code_file(tmp_path, code_rel, b"# real code\n")
    _write_code_file(tmp_path, fixture_rel, b"# real fixture\n")
    wrong_sha = "b" * 64
    provenance = _make_ifeval_provenance(
        code_path=code_rel,
        code_sha256=wrong_sha,
        fixture_path=fixture_rel,
        fixture_sha256=hashlib.sha256(b"# real fixture\n").hexdigest(),
    )

    with pytest.raises(ValueError, match="sha256 mismatch"):
        validate_ifeval_provenance(provenance, trusted_root=tmp_path)


@pytest.mark.unit
def test_ifeval_validate_provenance_rejects_fixture_tampering(tmp_path: Path):
    """A declared fixture_sha256 that does not match the actual file digest is rejected."""
    code_rel = "g8e_evals/benchmarks/ifeval/import_subset.py"
    fixture_rel = "g8e_evals/benchmarks/ifeval/fixture.py"
    code_sha = _write_code_file(tmp_path, code_rel, b"# real code\n")
    _write_code_file(tmp_path, fixture_rel, b"# real fixture\n")
    wrong_fixture_sha = "c" * 64
    provenance = _make_ifeval_provenance(
        code_path=code_rel,
        code_sha256=code_sha,
        fixture_path=fixture_rel,
        fixture_sha256=wrong_fixture_sha,
    )

    with pytest.raises(ValueError, match="sha256 mismatch"):
        validate_ifeval_provenance(provenance, trusted_root=tmp_path)


@pytest.mark.unit
def test_synthetic_validate_provenance_rejects_missing_code_file(tmp_path: Path):
    """A code_path that does not exist under the trusted root is rejected."""
    provenance = _make_synthetic_provenance(
        code_path="g8e_evals/benchmarks/privacy/nonexistent.py",
        code_sha256="0" * 64,
    )

    with pytest.raises(ValueError, match="code_path does not exist"):
        validate_synthetic_provenance(
            provenance,
            suite_id="privacy_token_lifecycle",
            trusted_root=tmp_path,
        )


# ---------------------------------------------------------------------------
# Path-traversal: code_path with .. or absolute paths is rejected
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_resolve_under_root_rejects_absolute_path():
    """An absolute code_path is rejected by _resolve_under_root."""
    with pytest.raises(ValueError, match="absolute"):
        _resolve_under_root("/etc/passwd", Path("/tmp"))


@pytest.mark.unit
def test_resolve_under_root_rejects_null_bytes():
    """A code_path containing null bytes is rejected."""
    with pytest.raises(ValueError, match="null bytes"):
        _resolve_under_root("foo\x00bar", Path("/tmp"))


@pytest.mark.unit
def test_resolve_under_root_rejects_traversal_sequences():
    """A code_path containing ``..`` path components is rejected."""
    with pytest.raises(ValueError, match="traversal"):
        _resolve_under_root("../../etc/passwd", Path("/tmp"))


@pytest.mark.unit
def test_resolve_under_root_rejects_empty_string():
    """An empty code_path is rejected."""
    with pytest.raises(ValueError, match="empty"):
        _resolve_under_root("", Path("/tmp"))


@pytest.mark.unit
def test_resolve_under_root_rejects_path_escaping_root(tmp_path: Path):
    """A code_path that resolves outside the trusted root via symlink is rejected.

    The ``..`` traversal check catches paths with explicit ``..``
    components.  A symlink-based escape is caught by the
    ``relative_to`` check after resolution.
    """
    sub = tmp_path / "sub"
    sub.mkdir()
    # Create a symlink inside sub that points outside sub
    link = sub / "escape"
    outside_target = tmp_path / "outside_target"
    outside_target.write_text("outside")
    link.symlink_to(outside_target)

    with pytest.raises(ValueError, match="outside trusted root"):
        _resolve_under_root("escape", sub)


@pytest.mark.unit
def test_synthetic_validate_provenance_rejects_traversal_code_path(tmp_path: Path):
    """validate_provenance rejects a code_path with traversal sequences."""
    provenance = _make_synthetic_provenance(
        code_path="../../etc/passwd",
        code_sha256="0" * 64,
    )

    with pytest.raises(ValueError, match="traversal"):
        validate_synthetic_provenance(
            provenance,
            suite_id="privacy_token_lifecycle",
            trusted_root=tmp_path,
        )


@pytest.mark.unit
def test_synthetic_validate_provenance_rejects_absolute_code_path(tmp_path: Path):
    """validate_provenance rejects an absolute code_path."""
    provenance = _make_synthetic_provenance(
        code_path="/etc/passwd",
        code_sha256="0" * 64,
    )

    with pytest.raises(ValueError, match="absolute"):
        validate_synthetic_provenance(
            provenance,
            suite_id="privacy_token_lifecycle",
            trusted_root=tmp_path,
        )


# ---------------------------------------------------------------------------
# Suite-substitution: provenance benchmark does not match the loader's SUITE_ID
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_synthetic_validate_provenance_rejects_suite_substitution(tmp_path: Path):
    """A provenance benchmark that does not match the loader's SUITE_ID is rejected."""
    code_rel = "g8e_evals/benchmarks/privacy/loader.py"
    actual_sha = _write_code_file(tmp_path, code_rel, b"# real code\n")
    provenance = _make_synthetic_provenance(
        benchmark="wrong_suite",
        code_path=code_rel,
        code_sha256=actual_sha,
    )

    with pytest.raises(ValueError, match="benchmark mismatch"):
        validate_synthetic_provenance(
            provenance,
            suite_id="privacy_token_lifecycle",
            trusted_root=tmp_path,
        )


@pytest.mark.unit
def test_ifeval_validate_provenance_rejects_suite_substitution(tmp_path: Path):
    """An IFEval provenance benchmark that does not match the suite_id is rejected."""
    code_rel = "g8e_evals/benchmarks/ifeval/import_subset.py"
    actual_sha = _write_code_file(tmp_path, code_rel, b"# real code\n")
    provenance = _make_ifeval_provenance(
        benchmark="wrong_suite",
        code_path=code_rel,
        code_sha256=actual_sha,
        fixture_path=code_rel,
        fixture_sha256=actual_sha,
    )

    with pytest.raises(ValueError, match="benchmark mismatch"):
        validate_ifeval_provenance(
            provenance,
            suite_id="ifeval_subset",
            trusted_root=tmp_path,
        )


# ---------------------------------------------------------------------------
# Unknown-field: provenance JSON with extra fields is rejected by extra="forbid"
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_synthetic_provenance_model_rejects_unknown_top_level_field():
    """SyntheticSuiteProvenance rejects unknown top-level fields at parse time."""
    with pytest.raises(Exception, match="extra"):
        _make_synthetic_provenance(extra_field=True)


@pytest.mark.unit
def test_synthetic_provenance_model_rejects_unknown_source_field():
    """SyntheticSuiteSource rejects unknown fields at parse time."""
    data = {
        "schema_version": 1,
        "benchmark": "privacy_token_lifecycle",
        "source": {
            "repository": "https://example.com/repo",
            "revision": "test",
            "license_spdx": "Apache-2.0",
            "code_path": "g8e_evals/benchmarks/privacy/loader.py",
            "code_sha256": "0" * 64,
            "unexpected_source_field": "rejected",
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "a" * 64,
        },
        "partition": "development",
        "domain_strata": ["privacy"],
    }
    with pytest.raises(Exception, match="extra"):
        SyntheticSuiteProvenance.model_validate(data)


@pytest.mark.unit
def test_synthetic_provenance_model_rejects_unknown_output_field():
    """SyntheticSuiteOutput rejects unknown fields at parse time."""
    data = {
        "schema_version": 1,
        "benchmark": "privacy_token_lifecycle",
        "source": {
            "repository": "https://example.com/repo",
            "revision": "test",
            "license_spdx": "Apache-2.0",
            "code_path": "g8e_evals/benchmarks/privacy/loader.py",
            "code_sha256": "0" * 64,
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "a" * 64,
            "unexpected_output_field": "rejected",
        },
        "partition": "development",
        "domain_strata": ["privacy"],
    }
    with pytest.raises(Exception, match="extra"):
        SyntheticSuiteProvenance.model_validate(data)


@pytest.mark.unit
def test_ifeval_provenance_model_rejects_unknown_top_level_field():
    """DatasetProvenance rejects unknown top-level fields at parse time."""
    with pytest.raises(Exception, match="extra"):
        _make_ifeval_provenance(extra_field=True)


@pytest.mark.unit
def test_ifeval_provenance_model_rejects_unknown_transformation_field():
    """DatasetTransformation rejects unknown fields at parse time."""
    data = {
        "schema_version": 1,
        "benchmark": "ifeval_subset",
        "source": {
            "url": "https://example.com",
            "revision": "rev",
            "license_spdx": "Apache-2.0",
            "license_url": "https://example.com",
            "sha256": "0" * 64,
        },
        "selected_keys": [1001],
        "transformation": {
            "description": "stub",
            "code_path": "g8e_evals/benchmarks/ifeval/import_subset.py",
            "code_sha256": "0" * 64,
            "fixture_path": "g8e_evals/benchmarks/ifeval/import_subset.py",
            "fixture_sha256": "0" * 64,
            "unexpected_field": "rejected",
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "0" * 64,
        },
        "partition": "development",
        "domain_strata": ["utility"],
    }
    with pytest.raises(Exception, match="extra"):
        DatasetProvenance.model_validate(data)


# ---------------------------------------------------------------------------
# SHA-256 pattern validation
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_synthetic_provenance_rejects_malformed_code_sha256():
    """A code_sha256 that is not a 64-character lowercase hex digest is rejected at parse time."""
    data = {
        "schema_version": 1,
        "benchmark": "privacy_token_lifecycle",
        "source": {
            "repository": "https://example.com/repo",
            "revision": "test",
            "license_spdx": "Apache-2.0",
            "code_path": "g8e_evals/benchmarks/privacy/loader.py",
            "code_sha256": "not-a-sha256",
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "a" * 64,
        },
        "partition": "development",
        "domain_strata": ["privacy"],
    }
    with pytest.raises(Exception, match="64-character lowercase hex"):
        SyntheticSuiteProvenance.model_validate(data)


@pytest.mark.unit
def test_ifeval_provenance_rejects_malformed_sha256():
    """A source sha256 that is not a 64-character lowercase hex digest is rejected at parse time."""
    data = {
        "schema_version": 1,
        "benchmark": "ifeval_subset",
        "source": {
            "url": "https://example.com",
            "revision": "rev",
            "license_spdx": "Apache-2.0",
            "license_url": "https://example.com",
            "sha256": "too-short",
        },
        "selected_keys": [1001],
        "transformation": {
            "description": "stub",
            "code_path": "g8e_evals/benchmarks/ifeval/import_subset.py",
            "code_sha256": "0" * 64,
            "fixture_path": "g8e_evals/benchmarks/ifeval/import_subset.py",
            "fixture_sha256": "0" * 64,
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "0" * 64,
        },
        "partition": "development",
        "domain_strata": ["utility"],
    }
    with pytest.raises(Exception, match="64-character lowercase hex"):
        DatasetProvenance.model_validate(data)
