from __future__ import annotations

import hashlib
import json
import sys
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID
from click.testing import CliRunner

from g8e_evals.cli import main
from g8e_evals.qualification import (
    ArtifactDigest,
    CandidateIdentityEvidence,
    CertificateIdentityEvidence,
    ComponentImageIdentity,
    EvidencePath,
    GateResultEvidence,
    PublicLoopEvidence,
    QualificationBuildRequest,
    QualificationInput,
    RuntimeCollectionRequest,
    RuntimeIdentityEvidence,
    SourceManifestAuthority,
    SourceManifestResult,
    VersionStampEvidence,
    build_collection_candidate_qualification,
    content_hash,
    render_qualification_json,
    resolve_qualification_input,
)
pytestmark = pytest.mark.integration


def _certificate(common_name: str, uris: list[str]) -> bytes:
    key = ec.generate_private_key(ec.SECP256R1())
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])
    now = datetime(2026, 9, 14, tzinfo=UTC)
    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=1))
    )
    if uris:
        builder = builder.add_extension(
            x509.SubjectAlternativeName(
                [x509.UniformResourceIdentifier(uri) for uri in uris]
            ),
            critical=False,
        )
    certificate = builder.sign(key, hashes.SHA256())
    return certificate.public_bytes(serialization.Encoding.PEM)


def _candidate() -> CandidateIdentityEvidence:
    return CandidateIdentityEvidence.build(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="3" * 64,
        images=[
            ComponentImageIdentity(components=["gateway", "operator"], image_id="sha256:" + "4" * 64),
            ComponentImageIdentity(components=["ensemble"], image_id="sha256:" + "5" * 64),
            ComponentImageIdentity(components=["dashboard"], image_id="sha256:" + "6" * 64),
        ],
    )


def _runtime(candidate: CandidateIdentityEvidence) -> RuntimeIdentityEvidence:
    return RuntimeIdentityEvidence.build(
        candidate_content_hash=candidate.content_hash,
        build_id="unavailable",
        build_time=datetime(2026, 9, 14, 11, 0, tzinfo=UTC),
        source_revision="unavailable",
        pki_root_sha256="e" * 64,
        owner_id="owner-1",
        cli_session_id="cli-session-1",
        embedded_operator_id="embedded-operator",
        embedded_operator_session_id="embedded-session-1",
        remote_operator_id="remote-operator-1",
        remote_operator_session_id="remote-session-1",
        certificates=[
            CertificateIdentityEvidence(component="cli", certificate_sha256="1" * 64, spiffe_ids=["spiffe://g8e.local/cli/owner-1/cli-session-1"]),
            CertificateIdentityEvidence(component="remote_operator", certificate_sha256="2" * 64, spiffe_ids=["spiffe://g8e.local/operator//remote-operator-1/remote-session-1"]),
            CertificateIdentityEvidence(component="ensemble", certificate_sha256="3" * 64, spiffe_ids=["spiffe://g8e.local/app/g8ee", "spiffe://g8e.local/user/owner-1"]),
            CertificateIdentityEvidence(component="dashboard", certificate_sha256="4" * 64, spiffe_ids=["spiffe://g8e.local/app/g8ed", "spiffe://g8e.local/user/owner-1"]),
        ],
        version_stamps=[
            VersionStampEvidence(component="host", version="v2.1.8", source_tree_state_hash=candidate.execution_source_manifest_hash),
            VersionStampEvidence(component="gateway", version="v2.1.8", source_tree_state_hash=candidate.execution_source_manifest_hash),
            VersionStampEvidence(component="operator", version="v2.1.8", source_tree_state_hash=candidate.execution_source_manifest_hash),
        ],
        healthy_components=["gateway", "operator", "ensemble", "dashboard"],
        pending_enrollment_count=0,
    )


def _gate(candidate: CandidateIdentityEvidence) -> GateResultEvidence:
    started_at = datetime(2026, 9, 14, 11, 10, tzinfo=UTC)
    return GateResultEvidence.build(
        gate_id="go_platform",
        candidate_content_hash=candidate.content_hash,
        command=["./g8e", "test", "unit"],
        started_at=started_at,
        completed_at=started_at + timedelta(minutes=1),
        exit_code=0,
        stdout_sha256="7" * 64,
        stderr_sha256="8" * 64,
        tool_versions={"g8e": "v2.1.8"},
        skipped=[],
    )


def _public_loop(candidate: CandidateIdentityEvidence) -> PublicLoopEvidence:
    return PublicLoopEvidence.build(
        candidate_content_hash=candidate.content_hash,
        source_id="qualification-fixture",
        first_sequence=1,
        high_water_sequence=5,
        batch_count=5,
        feed_chain_hash="9" * 64,
        proof_artifact_sha256="a" * 64,
        proof_artifact_bytes=67,
        retry_count=1,
        old_key_id="b" * 64,
        replacement_key_id="c" * 64,
        old_key_revoked=True,
        replacement_key_accepted_after_restart=True,
        mirror_restart_recovered=True,
        inference_invocations=0,
        generated_at=datetime(2026, 9, 14, 11, 30, tzinfo=UTC),
    )


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
        generated_at=datetime(2026, 9, 14, 12, 0, tzinfo=UTC),
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


def test_qualification_hash_source_uses_explicit_manifest_and_excludes(tmp_path: Path) -> None:
    source_root = tmp_path / "source"
    source_root.mkdir()
    (source_root / "included.txt").write_text("included\n")
    cache = source_root / "__pycache__"
    cache.mkdir()
    (cache / "ignored.pyc").write_bytes(b"ignored")
    authority_fields = {
        "authority_id": "full-source-v1",
        "schema_version": "1.0.0",
        "scope": "full_source",
        "mode": "manifest",
        "entries": ["included.txt", "__pycache__"],
        "excludes": ["__pycache__"],
        "owner_approved_at": "2026-09-14T12:00:00Z",
    }
    authority = SourceManifestAuthority(
        **authority_fields, content_hash=content_hash(authority_fields)
    )
    authority_path = tmp_path / "authority.json"
    output_path = tmp_path / "source-result.json"
    authority_path.write_text(authority.model_dump_json(indent=2) + "\n")

    result = CliRunner().invoke(
        main,
        [
            "qualification",
            "hash-source",
            "--authority",
            str(authority_path),
            "--source-root",
            str(source_root),
            "--authority-record-path",
            "authorities/full-source.json",
            "--output",
            str(output_path),
        ],
    )

    assert result.exit_code == 0, result.output
    source_result = SourceManifestResult.model_validate_json(output_path.read_bytes())
    expected_file_hash = hashlib.sha256(b"included\n").hexdigest()
    expected_tree_hash = hashlib.sha256(
        b"included.txt\0" + expected_file_hash.encode() + b"\0"
    ).hexdigest()
    assert source_result.source_tree_hash == expected_tree_hash


def test_qualification_candidate_computes_binary_hash_and_named_images(tmp_path: Path) -> None:
    output_path = tmp_path / "candidate.json"
    binary_path = Path(sys.executable).resolve(strict=True)
    authority = ArtifactDigest(
        name="manifest.json",
        path="manifest.json",
        content_sha256="a" * 64,
        file_sha256="b" * 64,
    )
    full_source = SourceManifestResult.build(
        scope="full_source", authority=authority, source_tree_hash="1" * 64
    )
    execution_source = SourceManifestResult.build(
        scope="execution_source", authority=authority, source_tree_hash="2" * 64
    )
    full_source_path = tmp_path / "full-source.json"
    execution_source_path = tmp_path / "execution-source.json"
    full_source_path.write_text(full_source.model_dump_json(indent=2) + "\n")
    execution_source_path.write_text(execution_source.model_dump_json(indent=2) + "\n")

    result = CliRunner().invoke(
        main,
        [
            "qualification",
            "candidate",
            "--full-source",
            str(full_source_path),
            "--execution-source",
            str(execution_source_path),
            "--binary",
            str(binary_path),
            "--image",
            "gateway,operator=sha256:" + "3" * 64,
            "--image",
            "ensemble=sha256:" + "4" * 64,
            "--image",
            "dashboard=sha256:" + "5" * 64,
            "--output",
            str(output_path),
        ],
    )

    assert result.exit_code == 0, result.output
    candidate = CandidateIdentityEvidence.model_validate_json(output_path.read_bytes())
    assert candidate.binary_sha256 == hashlib.sha256(binary_path.read_bytes()).hexdigest()
    assert candidate.images[0].components == ["gateway", "operator"]


def test_qualification_collect_runtime_computes_public_certificate_and_version_evidence(
    tmp_path: Path,
) -> None:
    candidate = _candidate()
    (tmp_path / "candidate.json").write_text(candidate.model_dump_json(indent=2) + "\n")
    certificate_uris = {
        "cli": ["spiffe://g8e.local/cli/owner-1/cli-session-1"],
        "remote_operator": [
            "spiffe://g8e.local/operator//remote-operator-1/remote-session-1"
        ],
        "ensemble": [
            "spiffe://g8e.local/app/g8ee",
            "spiffe://g8e.local/user/owner-1",
        ],
        "dashboard": [
            "spiffe://g8e.local/app/g8ed",
            "spiffe://g8e.local/user/owner-1",
        ],
    }
    certificate_paths: list[EvidencePath] = []
    for component, uris in certificate_uris.items():
        filename = component + ".pem"
        (tmp_path / filename).write_bytes(_certificate(component, uris))
        certificate_paths.append(EvidencePath(name=component, path=filename))
    (tmp_path / "root.pem").write_bytes(_certificate("root", []))
    version_paths: list[EvidencePath] = []
    for component in ("host", "gateway", "operator"):
        filename = component + "-version.json"
        (tmp_path / filename).write_text(
            json.dumps(
                {
                    "version": "v2.1.8",
                    "build_id": "unavailable",
                    "build_time": "2026-09-14T11:00:00Z",
                    "source_revision": "unavailable",
                    "source_tree_state_hash": candidate.execution_source_manifest_hash,
                }
            )
        )
        version_paths.append(EvidencePath(name=component, path=filename))
    request = RuntimeCollectionRequest(
        version="v2.1.8",
        candidate_path="candidate.json",
        pki_root_path="root.pem",
        certificate_paths=certificate_paths,
        version_paths=version_paths,
        owner_id="owner-1",
        cli_session_id="cli-session-1",
        embedded_operator_id="embedded-operator",
        embedded_operator_session_id="embedded-session-1",
        remote_operator_id="remote-operator-1",
        remote_operator_session_id="remote-session-1",
        healthy_components=["gateway", "operator", "ensemble", "dashboard"],
        pending_enrollment_count=0,
    )
    request_path = tmp_path / "runtime-request.json"
    output_path = tmp_path / "runtime.json"
    request_path.write_text(request.model_dump_json(indent=2) + "\n")

    result = CliRunner().invoke(
        main,
        [
            "qualification",
            "collect-runtime",
            "--request",
            str(request_path),
            "--output",
            str(output_path),
        ],
    )

    assert result.exit_code == 0, result.output
    runtime = RuntimeIdentityEvidence.model_validate_json(output_path.read_bytes())
    assert runtime.candidate_content_hash == candidate.content_hash
    assert runtime.pending_enrollment_count == 0
    assert [certificate.component for certificate in runtime.certificates] == list(
        certificate_uris
    )


def test_qualification_run_gate_emits_machine_readable_candidate_bound_result(tmp_path: Path) -> None:
    candidate = _candidate()
    candidate_path = tmp_path / "candidate.json"
    output_path = tmp_path / "gate.json"
    candidate_path.write_text(candidate.model_dump_json(indent=2) + "\n")

    result = CliRunner().invoke(
        main,
        [
            "qualification",
            "run-gate",
            "--candidate",
            str(candidate_path),
            "--gate-id",
            "fixture_gate",
            "--tool-version",
            "python=" + sys.version.split()[0],
            "--output",
            str(output_path),
            "--",
            sys.executable,
            "-c",
            "print('passed')",
        ],
    )

    assert result.exit_code == 0, result.output
    evidence = GateResultEvidence.model_validate_json(output_path.read_bytes())
    assert evidence.candidate_content_hash == candidate.content_hash
    assert evidence.command == [sys.executable, "-c", "print('passed')"]
    assert evidence.exit_code == 0
    assert evidence.result == "passed"


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
