from __future__ import annotations

import json
from pathlib import Path

import pytest
from click.testing import CliRunner

from g8e_evals.cli import main
from g8e_evals.qualification import (
    EvidencePath,
    QualificationBuildRequest,
    QualificationInput,
    build_collection_candidate_qualification,
    content_hash,
    render_qualification_json,
    resolve_qualification_input,
)
from tests.test_qualification_builder import _candidate, _gate, _public_loop, _runtime

pytestmark = pytest.mark.integration


def _write_input(path: Path) -> QualificationInput:
    candidate = _candidate()
    runtime = _runtime(candidate)
    gate = _gate(candidate)
    public_loop = _public_loop(candidate)
    (path.parent / "candidate.json").write_text(candidate.model_dump_json(indent=2) + "\n")
    (path.parent / "runtime.json").write_text(runtime.model_dump_json(indent=2) + "\n")
    (path.parent / "gate.json").write_text(gate.model_dump_json(indent=2) + "\n")
    (path.parent / "public-loop.json").write_text(public_loop.model_dump_json(indent=2) + "\n")
    authority = {"disposition_id": "invalidation", "content_hash": "0" * 64}
    authority["content_hash"] = content_hash(authority)
    (path.parent / "invalidation.json").write_text(
        json.dumps(authority, indent=2, sort_keys=True) + "\n"
    )
    request = QualificationBuildRequest(
        record_id="v2.1.8-collection-candidate-qualification-r6",
        version="v2.1.8",
        generated_at="2026-09-14T12:00:00Z",
        candidate_path="candidate.json",
        runtime_path="runtime.json",
        required_gate_ids=["go_platform"],
        gate_paths=[EvidencePath(name="go_platform", path="gate.json")],
        public_loop_path="public-loop.json",
        authority_paths=[EvidencePath(name="invalidation", path="invalidation.json")],
        prior_qualification_content_hash="f" * 64,
        invalidation_authority_name="invalidation",
        endpoint_class="owner_operated_remote_ollama",
    )
    path.write_text(request.model_dump_json(indent=2) + "\n")
    return resolve_qualification_input(request, path.parent)


def test_qualification_build_writes_only_owner_review_draft(tmp_path: Path) -> None:
    input_path = tmp_path / "input.json"
    output_path = tmp_path / "draft.json"
    _write_input(input_path)

    result = CliRunner().invoke(
        main,
        ["qualification", "build", "--input", str(input_path), "--output", str(output_path)],
    )

    assert result.exit_code == 0, result.output
    assert output_path.read_bytes().endswith(b"\n")
    assert b'"status": "owner_review_required"' in output_path.read_bytes()
    assert b'"model_inventory_status": "not_queried"' in output_path.read_bytes()


def test_qualification_build_check_compares_without_rewriting(tmp_path: Path) -> None:
    input_path = tmp_path / "input.json"
    output_path = tmp_path / "draft.json"
    inputs = _write_input(input_path)
    expected = render_qualification_json(build_collection_candidate_qualification(inputs))
    output_path.write_bytes(expected)

    before = output_path.stat().st_mtime_ns
    result = CliRunner().invoke(
        main,
        ["qualification", "build", "--input", str(input_path), "--check", str(output_path)],
    )

    assert result.exit_code == 0, result.output
    assert output_path.stat().st_mtime_ns == before


def test_qualification_build_refuses_to_overwrite_existing_draft(tmp_path: Path) -> None:
    input_path = tmp_path / "input.json"
    output_path = tmp_path / "draft.json"
    _write_input(input_path)
    output_path.write_text("existing\n")

    result = CliRunner().invoke(
        main,
        ["qualification", "build", "--input", str(input_path), "--output", str(output_path)],
    )

    assert result.exit_code != 0
    assert output_path.read_text() == "existing\n"
