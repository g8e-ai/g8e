from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest
from pydantic import ValidationError

from g8e_evals.qualification import (
    ArtifactDigest,
    CandidateIdentityEvidence,
    ComponentImageIdentity,
    CertificateIdentityEvidence,
    GateResultEvidence,
    PublicLoopEvidence,
    RuntimeIdentityEvidence,
    VersionStampEvidence,
    QualificationInput,
    QualificationStatus,
    build_collection_candidate_qualification,
    render_qualification_json,
)

pytestmark = pytest.mark.unit


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


def _gate(candidate: CandidateIdentityEvidence, gate_id: str = "go_platform") -> GateResultEvidence:
    started_at = datetime(2026, 9, 14, 11, 10, tzinfo=UTC)
    return GateResultEvidence.build(
        gate_id=gate_id,
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
            CertificateIdentityEvidence(
                component="cli",
                certificate_sha256="1" * 64,
                spiffe_ids=["spiffe://g8e.local/cli/owner-1/cli-session-1"],
            ),
            CertificateIdentityEvidence(
                component="remote_operator",
                certificate_sha256="2" * 64,
                spiffe_ids=["spiffe://g8e.local/operator//remote-operator-1/remote-session-1"],
            ),
            CertificateIdentityEvidence(
                component="ensemble",
                certificate_sha256="3" * 64,
                spiffe_ids=["spiffe://g8e.local/app/g8ee", "spiffe://g8e.local/user/owner-1"],
            ),
            CertificateIdentityEvidence(
                component="dashboard",
                certificate_sha256="4" * 64,
                spiffe_ids=["spiffe://g8e.local/app/g8ed", "spiffe://g8e.local/user/owner-1"],
            ),
        ],
        version_stamps=[
            VersionStampEvidence(
                component="host",
                version="v2.1.8",
                source_tree_state_hash=candidate.execution_source_manifest_hash,
            ),
            VersionStampEvidence(
                component="gateway",
                version="v2.1.8",
                source_tree_state_hash=candidate.execution_source_manifest_hash,
            ),
            VersionStampEvidence(
                component="operator",
                version="v2.1.8",
                source_tree_state_hash=candidate.execution_source_manifest_hash,
            ),
        ],
        healthy_components=["gateway", "operator", "ensemble", "dashboard"],
        pending_enrollment_count=0,
    )


def _input(candidate: CandidateIdentityEvidence) -> QualificationInput:
    generated_at = datetime(2026, 9, 14, 12, 0, tzinfo=UTC)
    return QualificationInput(
        record_id="v2.1.8-collection-candidate-qualification-r6",
        version="v2.1.8",
        generated_at=generated_at,
        candidate=candidate,
        runtime=_runtime(candidate),
        required_gate_ids=["go_platform"],
        gates=[_gate(candidate)],
        public_loop=_public_loop(candidate),
        authority_bindings=[
            ArtifactDigest(
                name="invalidation",
                path="live-packet/invalidation.json",
                content_sha256="d" * 64,
                file_sha256="e" * 64,
            )
        ],
        prior_qualification_content_hash="f" * 64,
        invalidation_content_hash="d" * 64,
        endpoint_class="owner_operated_remote_ollama",
    )


def test_artifact_digest_distinguishes_content_and_file_hashes() -> None:
    digest = ArtifactDigest(
        name="fixture",
        path="artifact.json",
        content_sha256="1" * 64,
        file_sha256="2" * 64,
    )

    assert digest.content_sha256 != digest.file_sha256


def test_qualification_builder_is_byte_deterministic_and_draft_only() -> None:
    candidate = _candidate()

    first = build_collection_candidate_qualification(_input(candidate))
    second = build_collection_candidate_qualification(_input(candidate))
    rendered = render_qualification_json(first)

    assert first == second
    assert rendered == render_qualification_json(second)
    assert first.status == QualificationStatus.OWNER_REVIEW_REQUIRED
    assert first.owner_approval_required is True
    assert first.live_operation_lease_status == "no_active_lease"
    assert "approval" not in type(first).model_fields
    assert "lease" not in type(first).model_fields


def test_qualification_rejects_failed_or_missing_required_gate() -> None:
    candidate = _candidate()
    valid = _input(candidate)
    failed_fields = _gate(candidate).model_dump(exclude={"content_hash", "result"})
    failed_fields["exit_code"] = 1
    failed = GateResultEvidence.build(**failed_fields)

    with pytest.raises(ValidationError, match="passed"):
        QualificationInput.model_validate(valid.model_dump() | {"gates": [failed]})
    with pytest.raises(ValidationError, match="gates"):
        QualificationInput.model_validate(valid.model_dump() | {"gates": []})


def test_qualification_rejects_gate_or_public_loop_from_other_candidate() -> None:
    candidate = _candidate()
    valid = _input(candidate)
    other_hash = "0" * 64

    mismatched_gate = _gate(candidate).model_dump(exclude={"content_hash"})
    mismatched_gate["candidate_content_hash"] = other_hash
    mismatched_loop = _public_loop(candidate).model_dump(exclude={"content_hash"})
    mismatched_loop["candidate_content_hash"] = other_hash

    with pytest.raises(ValidationError, match="gate candidate"):
        QualificationInput.model_validate(
            valid.model_dump() | {"gates": [GateResultEvidence.build(**mismatched_gate)]}
        )
    with pytest.raises(ValidationError, match="public loop candidate"):
        QualificationInput.model_validate(
            valid.model_dump() | {"public_loop": PublicLoopEvidence.build(**mismatched_loop)}
        )


def test_qualification_contract_rejects_secret_bearing_fields() -> None:
    candidate = _candidate()

    with pytest.raises(ValidationError, match="extra_forbidden"):
        QualificationInput.model_validate(_input(candidate).model_dump() | {"ingest_token": "secret"})


def test_qualification_rejects_stale_gate_evidence() -> None:
    candidate = _candidate()
    valid = _input(candidate)
    stale_fields = _gate(candidate).model_dump(exclude={"content_hash", "result"})
    stale_fields["started_at"] = datetime(2026, 9, 14, 10, 0, tzinfo=UTC)
    stale_fields["completed_at"] = datetime(2026, 9, 14, 10, 1, tzinfo=UTC)

    with pytest.raises(ValidationError, match="stale"):
        QualificationInput.model_validate(
            valid.model_dump() | {"gates": [GateResultEvidence.build(**stale_fields)]}
        )


def test_runtime_identity_rejects_certificate_or_source_stamp_mismatch() -> None:
    candidate = _candidate()
    runtime = _runtime(candidate)
    certificates = runtime.certificates.copy()
    certificates[2] = certificates[2].model_copy(
        update={"spiffe_ids": ["spiffe://g8e.local/app/g8ee", "spiffe://g8e.local/user/other-owner"]}
    )
    stamps = runtime.version_stamps.copy()
    stamps[1] = stamps[1].model_copy(update={"source_tree_state_hash": "0" * 64})
    mismatched_runtime = runtime.model_dump(exclude={"content_hash"})
    mismatched_runtime["version_stamps"] = stamps

    with pytest.raises(ValidationError, match="certificate SPIFFE"):
        RuntimeIdentityEvidence.model_validate(runtime.model_dump() | {"certificates": certificates})
    with pytest.raises(ValidationError, match="source stamp"):
        QualificationInput.model_validate(
            _input(candidate).model_dump()
            | {"runtime": RuntimeIdentityEvidence.build(**mismatched_runtime)}
        )


def test_public_loop_requires_explicit_zero_inference() -> None:
    candidate = _candidate()
    valid = _public_loop(candidate)

    with pytest.raises(ValidationError, match="inference_invocations"):
        PublicLoopEvidence.model_validate(valid.model_dump() | {"inference_invocations": 1})
