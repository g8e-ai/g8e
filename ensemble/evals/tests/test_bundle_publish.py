# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the eval bundle publication module (Packet 6 Step 8.5).

Verifies request construction from a verified bundle, disclosure policy
(public-only artifacts), verification failure rejection, routing ID
pass-through, mTLS client behavior, HTTP response/error handling, and
cross-language contract coverage for the typed publication request.
"""

from __future__ import annotations

import base64
import hashlib
import json
from datetime import UTC, datetime
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock

import pytest
from pydantic import ValidationError

pytestmark = pytest.mark.integration

from g8e.constants import API_PATHS, CLI_SESSION_ID_HEADER
from g8e.models.observe_api import (
    ObserveProducerEvalPublicationRequest,
    ObserveProducerResponse,
)
from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle import (
    EvalSigningKey,
    EvalTrustStore,
    EvalTrustedKey,
    PrivacyClass,
    PublicationError,
    build_publication_request,
    produce_bundle,
    publish_bundle,
    verify_bundle,
)
from g8e_evals.bundle.manifest import BundleArtifactEntry
from g8e_evals.bundle.publish import (
    _build_downloads,
    _build_manifest_wire,
    _build_metric_summaries,
    _build_verification_report_wire,
    _read_bundle_analysis,
    _read_bundle_manifest,
    _read_run_manifest,
)
from g8e_evals.bundle.signing import EVAL_SIGNING_ALGORITHM, EVAL_TRUST_SCOPE
from g8e_evals.bundle.verify import VerificationReport
from g8e_evals.schema import (
    ActionReceipt,
    AttemptRecord,
    MetricObservation,
    ReceiptObservation,
    RunManifest,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.arms import Arm


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


# ---------------------------------------------------------------------------
# Minimal report directory builder
# ---------------------------------------------------------------------------


def _minimal_task() -> TaskDefinition:
    return TaskDefinition(
        task_id="task-1",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        prompt_hash="0" * 64,
    )


def _minimal_attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
        started_at=_TS,
        ended_at=_TS,
    )


def _minimal_metric() -> MetricObservation:
    return MetricObservation(
        metric_id="receipt_integrity",
        metric_version="1.0.0",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
        value=1.0,
        unit="boolean",
        verification_status=VerificationStatus.VERIFIED,
    )


def _minimal_receipt() -> ReceiptObservation:
    receipt = ActionReceipt(
        transaction_id="tx-1",
        transaction_hash="hash-1",
    )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="TEST_ACTION",
    )
    return ReceiptObservation(
        receipt_id="receipt-1",
        attempt_id="attempt-1",
        run_id="run-1",
        transaction_id="tx-1",
        action_type="TEST_ACTION",
        primary=True,
        action_receipt=receipt,
    )


def _minimal_stage() -> StageObservation:
    return StageObservation(
        stage_id="stage-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        kind=StageKind.PROTOCOL_L2,
        decision="pass",
    )


def _build_report_dir(report_dir: Path) -> None:
    """Build a minimal valid report directory with analysis artifacts."""
    report_dir.mkdir(parents=True, exist_ok=True)

    task = _minimal_task()
    attempt = _minimal_attempt()
    metric = _minimal_metric()
    receipt = _minimal_receipt()
    stage = _minimal_stage()

    # Write a typed run manifest with suite_id and suite_version.
    run_manifest = RunManifest(
        run_id="run-1",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        created_at=_TS,
    )
    (report_dir / evals_constants.MANIFEST_JSON).write_text(canonical_model_json(run_manifest))
    (report_dir / evals_constants.TASKS_JSONL).write_text(canonical_model_json(task) + "\n")
    (report_dir / evals_constants.ATTEMPTS_JSONL).write_text(canonical_model_json(attempt) + "\n")
    (report_dir / evals_constants.METRICS_JSONL).write_text(canonical_model_json(metric) + "\n")
    (report_dir / evals_constants.RECEIPTS_JSONL).write_text(canonical_model_json(receipt) + "\n")
    (report_dir / evals_constants.STAGES_JSONL).write_text(canonical_model_json(stage) + "\n")

    # Build and write analysis artifacts.
    analysis_input = AnalysisInputRecord(
        run_id="run-1",
        release_version="v2.1.8",
        tasks=[task],
        attempts=[attempt],
        metric_observations=[metric],
        receipts=[receipt],
        stages=[stage],
    )
    (report_dir / evals_constants.ANALYSIS_INPUT_JSON).write_text(
        canonical_model_json(analysis_input)
    )

    from g8e_evals.analysis import render_cli, render_html, render_markdown
    analysis = compute_canonical_analysis_from_record(analysis_input)
    (report_dir / evals_constants.ANALYSIS_JSON).write_text(canonical_model_json(analysis))
    (report_dir / evals_constants.ANALYSIS_MD).write_text(render_markdown(analysis))
    (report_dir / evals_constants.ANALYSIS_HTML).write_text(render_html(analysis))
    (report_dir / evals_constants.ANALYSIS_TXT).write_text(render_cli(analysis))


def _produce_valid_bundle(tmp_path: Path, signing_key: EvalSigningKey | None = None) -> Path:
    """Produce a valid bundle from a minimal report directory.

    Defaults to a deterministic signing key so the bundle is signed unless
    the caller explicitly passes ``signing_key=None`` with ``diagnostic=True``.
    """
    report_dir = tmp_path / "report"
    bundle_dir = tmp_path / "bundle"
    _build_report_dir(report_dir)
    if signing_key is None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
    produce_bundle(
        report_dir=report_dir,
        bundle_dir=bundle_dir,
        bundle_id="bundle-1",
        signing_key=signing_key,
        created_at=_TS,
    )
    return bundle_dir


def _produce_signed_bundle(tmp_path: Path) -> tuple[Path, EvalTrustStore]:
    """Produce a signed bundle and return the bundle dir plus its trust store."""
    signing_key = EvalSigningKey.generate()
    bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
    return bundle_dir, _trust_store(signing_key)


def _verify_bundle(tmp_path: Path) -> tuple[Path, VerificationReport]:
    """Produce a signed bundle, verify it, and return the dir plus report."""
    bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
    report = verify_bundle(bundle_dir, trust_store=trust_store)
    assert report.ok, f"Bundle should verify: {[f.message for f in report.failures]}"
    return bundle_dir, report


def _trusted_key(signing_key: EvalSigningKey) -> EvalTrustedKey:
    return EvalTrustedKey(
        key_id=signing_key.key_id,
        public_key_hex=signing_key.public_key_hex,
        algorithm=EVAL_SIGNING_ALGORITHM,
        scope=EVAL_TRUST_SCOPE,
        valid_from=_TS,
        valid_until=_TS_PLUS,
        revoked=False,
        source="protocol-owned-test-metadata",
    )


def _trust_store(signing_key: EvalSigningKey) -> EvalTrustStore:
    return EvalTrustStore(keys=[_trusted_key(signing_key)])


def _fake_auth_context() -> MagicMock:
    """Build a fake AuthContext with a mock async client."""
    auth_ctx = MagicMock()
    auth_ctx.auth_headers.return_value = {"Content-Type": "application/json"}
    auth_ctx.make_async_client.return_value.__aenter__ = AsyncMock()
    auth_ctx.make_async_client.return_value.__aexit__ = AsyncMock(return_value=None)
    return auth_ctx


def _mock_response(status_code: int, body: str) -> MagicMock:
    resp = MagicMock()
    resp.status_code = status_code
    resp.text = body
    return resp


# ---------------------------------------------------------------------------
# build_publication_request tests
# ---------------------------------------------------------------------------


class TestBuildPublicationRequest:
    """build_publication_request constructs a typed request from a verified bundle."""

    def test_builds_typed_request_from_verified_bundle(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        report = verify_bundle(bundle_dir, trust_store=trust_store)
        assert report.ok, f"Bundle should verify: {[f.message for f in report.failures]}"

        manifest = _read_bundle_manifest(bundle_dir)
        analysis = _read_bundle_analysis(bundle_dir)
        run_manifest = _read_run_manifest(bundle_dir)

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )

        assert isinstance(request, ObserveProducerEvalPublicationRequest)
        assert request.schema_version == "1.0.0"
        assert request.bundle_id == "bundle-1"
        assert request.run_id == "run-1"
        assert request.release_version == "v2.1.8"
        assert request.suite_id == "ifeval_subset"
        assert request.suite_version == "1.0.0"
        assert request.arm_ids == [Arm.DOCTRINE.value]
        assert request.campaign_id == ""
        assert request.model_cohort_ids == []
        assert request.assignment_count == 0
        assert request.receipt_count == analysis.input_summary.receipt_count
        assert request.assigned_tasks == analysis.input_summary.task_count
        assert request.terminal_attempts == analysis.input_summary.attempt_count
        assert len(request.metrics) == len(analysis.metric_results)
        assert request.verification_report.ok is True
        assert request.bundle_manifest.bundle_id == "bundle-1"
        assert request.web_session_id == "ws-1"
        assert request.cli_session_id is None

    def test_downloads_contain_only_public_artifacts(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)

        downloads = _build_downloads(bundle_dir, manifest, "run-1")

        # Every download must be public_safe.
        for dl in downloads:
            assert dl.privacy_classification == "public_safe"
        # No restricted or internal artifacts appear.
        privacy_classes = {a.privacy_class for a in manifest.artifacts}
        assert PrivacyClass.PUBLIC in privacy_classes
        # All public artifacts in the manifest appear in downloads.
        public_artifacts = [a for a in manifest.artifacts if a.privacy_class == PrivacyClass.PUBLIC]
        assert len(downloads) == len(public_artifacts)

    def test_download_content_is_base64_of_public_artifact_bytes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)

        downloads = _build_downloads(bundle_dir, manifest, "run-1")

        for dl in downloads:
            artifact_path = bundle_dir / dl.artifact_id
            expected_bytes = artifact_path.read_bytes()
            decoded = base64.b64decode(dl.content)
            assert decoded == expected_bytes
            assert dl.byte_size == len(expected_bytes)
            assert dl.sha256 == _sha256(expected_bytes)
            assert dl.source_run_id == "run-1"

    def test_restricted_artifacts_excluded_from_downloads(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)

        # Confirm the bundle has restricted artifacts (evidence-index).
        restricted = [a for a in manifest.artifacts if a.privacy_class == PrivacyClass.RESTRICTED]
        # The minimal bundle may not have restricted artifacts; add a synthetic one.
        if not restricted:
            # Create a fake restricted artifact entry in the manifest for this test.
            restricted_entry = BundleArtifactEntry(
                path="evidence-index.jsonl",
                media_type="application/x-jsonlines",
                privacy_class=PrivacyClass.RESTRICTED,
                sha256="a" * 64,
                byte_length=10,
                artifact_type=manifest.artifacts[0].artifact_type,
                record_count=1,
            )
            manifest = manifest.model_copy(update={"artifacts": [*list(manifest.artifacts), restricted_entry]})

        downloads = _build_downloads(bundle_dir, manifest, "run-1")
        for dl in downloads:
            assert dl.privacy_classification == "public_safe"
            assert "restricted" not in dl.artifact_id.lower() or dl.privacy_classification != "restricted"

    def test_routing_ids_passed_through(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        manifest = _read_bundle_manifest(bundle_dir)
        analysis = _read_bundle_analysis(bundle_dir)
        run_manifest = _read_run_manifest(bundle_dir)

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id=None, cli_session_id="cs-1",
        )
        assert request.web_session_id is None
        assert request.cli_session_id == "cs-1"

    def test_metric_summaries_match_analysis(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        analysis = _read_bundle_analysis(bundle_dir)

        summaries = _build_metric_summaries(analysis)

        assert len(summaries) == len(analysis.metric_results)
        for summary, result in zip(summaries, analysis.metric_results, strict=True):
            assert summary.metric_id == result.metric_id
            assert summary.metric_version == result.metric_version
            assert summary.model_cohort_id == result.model_cohort_id
            assert summary.arm_id == result.arm_id
            assert summary.value == result.value
            assert summary.unit == result.unit
            assert summary.eligible == result.eligible_count
            assert summary.denominator == result.denominator
            assert summary.verification_status in ("verified", "projection_validated", "receipt_verification_not_applicable")

    def test_verification_report_wire_preserves_ok_and_layers(self, tmp_path: Path) -> None:
        _, report = _verify_bundle(tmp_path)

        wire = _build_verification_report_wire(report)

        assert wire.ok is True
        assert wire.bundle_id == report.bundle_id
        assert wire.run_id == report.run_id
        assert wire.release_version == report.release_version
        assert len(wire.layers) == len(report.layers)
        assert all(lr.passed for lr in wire.layers)

    def test_manifest_wire_preserves_artifacts_and_hashes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)

        wire = _build_manifest_wire(manifest)

        assert wire.bundle_id == manifest.bundle_id
        assert wire.run_id == manifest.run_id
        assert len(wire.artifacts) == len(manifest.artifacts)
        for wa, a in zip(wire.artifacts, manifest.artifacts, strict=True):
            assert wa.path == a.path
            assert wa.privacy_class == a.privacy_class.value
            assert wa.sha256 == a.sha256
            assert wa.byte_length == a.byte_length

    def test_request_has_no_user_id_field(self, tmp_path: Path) -> None:
        """The publication request must not carry a user_id field."""
        bundle_dir, report = _verify_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)
        analysis = _read_bundle_analysis(bundle_dir)
        run_manifest = _read_run_manifest(bundle_dir)

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )
        dumped = request.model_dump(mode="json")
        assert "user_id" not in dumped

    def test_request_serializes_to_canonical_json(self, tmp_path: Path) -> None:
        """The request serializes to valid canonical JSON with expected field names."""
        bundle_dir, report = _verify_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)
        analysis = _read_bundle_analysis(bundle_dir)
        run_manifest = _read_run_manifest(bundle_dir)

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )
        body = canonical_model_json(request)
        parsed = json.loads(body)
        # Check key wire field names match the Go struct JSON tags.
        assert "schema_version" in parsed
        assert "bundle_id" in parsed
        assert "run_id" in parsed
        assert "release_version" in parsed
        assert "suite_id" in parsed
        assert "suite_version" in parsed
        assert "campaign_id" in parsed
        assert "arm_ids" in parsed
        assert "model_cohort_ids" in parsed
        assert "assignment_count" in parsed
        assert "receipt_count" in parsed
        assert "assigned_tasks" in parsed
        assert "terminal_attempts" in parsed
        assert "metrics" in parsed
        assert "verification_report" in parsed
        assert "bundle_manifest" in parsed
        assert "downloads" in parsed
        assert "web_session_id" in parsed


# ---------------------------------------------------------------------------
# publish_bundle tests
# ---------------------------------------------------------------------------


class TestPublishBundle:
    """publish_bundle verifies, builds, and transmits the publication request."""

    async def test_verified_bundle_success(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        response_body = canonical_model_json(ObserveProducerResponse(accepted=True))
        auth_ctx.make_async_client.return_value.__aenter__.return_value = AsyncMock(
            post=AsyncMock(return_value=_mock_response(200, response_body))
        )

        result = await publish_bundle(
            bundle_dir=bundle_dir,
            gateway_url="https://localhost:8443",
            auth_context=auth_ctx,
            web_session_id="ws-1",
            cli_session_id=None,
            trust_store=trust_store,
        )

        assert isinstance(result, ObserveProducerResponse)
        assert result.accepted is True

    async def test_failed_verification_rejected(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        # Corrupt a file to make verification fail.
        (bundle_dir / evals_constants.TASKS_JSONL).write_text("corrupted")

        auth_ctx = _fake_auth_context()

        with pytest.raises(PublicationError, match="verification failed"):
            await publish_bundle(
                bundle_dir=bundle_dir,
                gateway_url="https://localhost:8443",
                auth_context=auth_ctx,
                web_session_id="ws-1",
                cli_session_id=None,
                trust_store=trust_store,
            )

    async def test_gateway_rejection_raises_publication_error(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        auth_ctx.make_async_client.return_value.__aenter__.return_value = AsyncMock(
            post=AsyncMock(return_value=_mock_response(400, "bad request"))
        )

        with pytest.raises(PublicationError, match="HTTP 400"):
            await publish_bundle(
                bundle_dir=bundle_dir,
                gateway_url="https://localhost:8443",
                auth_context=auth_ctx,
                web_session_id="ws-1",
                cli_session_id=None,
                trust_store=trust_store,
            )

    async def test_gateway_500_raises_publication_error(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        auth_ctx.make_async_client.return_value.__aenter__.return_value = AsyncMock(
            post=AsyncMock(return_value=_mock_response(500, "internal error"))
        )

        with pytest.raises(PublicationError, match="HTTP 500"):
            await publish_bundle(
                bundle_dir=bundle_dir,
                gateway_url="https://localhost:8443",
                auth_context=auth_ctx,
                web_session_id="ws-1",
                cli_session_id=None,
                trust_store=trust_store,
            )

    async def test_uses_correct_gateway_path(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        response_body = canonical_model_json(ObserveProducerResponse(accepted=True))
        mock_client = AsyncMock(
            post=AsyncMock(return_value=_mock_response(200, response_body))
        )
        auth_ctx.make_async_client.return_value.__aenter__.return_value = mock_client

        await publish_bundle(
            bundle_dir=bundle_dir,
            gateway_url="https://localhost:8443",
            auth_context=auth_ctx,
            web_session_id="ws-1",
            cli_session_id=None,
            trust_store=trust_store,
        )

        call_args = mock_client.post.call_args
        expected_path = API_PATHS["observe_producer_eval_publication"]
        expected_url = f"https://localhost:8443{expected_path}"
        assert call_args.kwargs.get("url", call_args.args[0] if call_args.args else None) == expected_url

    async def test_cli_session_id_header_added(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        response_body = canonical_model_json(ObserveProducerResponse(accepted=True))
        mock_client = AsyncMock(
            post=AsyncMock(return_value=_mock_response(200, response_body))
        )
        auth_ctx.make_async_client.return_value.__aenter__.return_value = mock_client

        await publish_bundle(
            bundle_dir=bundle_dir,
            gateway_url="https://localhost:8443",
            auth_context=auth_ctx,
            web_session_id=None,
            cli_session_id="cs-1",
            trust_store=trust_store,
        )

        call_args = mock_client.post.call_args
        headers = call_args.kwargs.get("headers", {})
        assert headers.get(CLI_SESSION_ID_HEADER) == "cs-1"

    async def test_no_cli_session_id_header_when_absent(self, tmp_path: Path) -> None:
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        response_body = canonical_model_json(ObserveProducerResponse(accepted=True))
        mock_client = AsyncMock(
            post=AsyncMock(return_value=_mock_response(200, response_body))
        )
        auth_ctx.make_async_client.return_value.__aenter__.return_value = mock_client

        await publish_bundle(
            bundle_dir=bundle_dir,
            gateway_url="https://localhost:8443",
            auth_context=auth_ctx,
            web_session_id="ws-1",
            cli_session_id=None,
            trust_store=trust_store,
        )

        call_args = mock_client.post.call_args
        headers = call_args.kwargs.get("headers", {})
        assert CLI_SESSION_ID_HEADER not in headers

    async def test_uses_mtls_client_from_auth_context(self, tmp_path: Path) -> None:
        """publish_bundle must use auth_context.make_async_client for mTLS."""
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        response_body = canonical_model_json(ObserveProducerResponse(accepted=True))
        auth_ctx.make_async_client.return_value.__aenter__.return_value = AsyncMock(
            post=AsyncMock(return_value=_mock_response(200, response_body))
        )

        await publish_bundle(
            bundle_dir=bundle_dir,
            gateway_url="https://localhost:8443",
            auth_context=auth_ctx,
            web_session_id="ws-1",
            cli_session_id=None,
            trust_store=trust_store,
        )

        auth_ctx.make_async_client.assert_called_once()

    async def test_request_body_is_canonical_json(self, tmp_path: Path) -> None:
        """The POST body is canonical JSON of the typed request."""
        bundle_dir, trust_store = _produce_signed_bundle(tmp_path)
        auth_ctx = _fake_auth_context()
        response_body = canonical_model_json(ObserveProducerResponse(accepted=True))
        mock_client = AsyncMock(
            post=AsyncMock(return_value=_mock_response(200, response_body))
        )
        auth_ctx.make_async_client.return_value.__aenter__.return_value = mock_client

        await publish_bundle(
            bundle_dir=bundle_dir,
            gateway_url="https://localhost:8443",
            auth_context=auth_ctx,
            web_session_id="ws-1",
            cli_session_id=None,
            trust_store=trust_store,
        )

        call_args = mock_client.post.call_args
        body = call_args.kwargs.get("content", "")
        parsed = json.loads(body)
        assert parsed["bundle_id"] == "bundle-1"
        assert parsed["run_id"] == "run-1"
        assert parsed["web_session_id"] == "ws-1"


# ---------------------------------------------------------------------------
# Cross-language contract coverage
# ---------------------------------------------------------------------------


class TestCrossLanguageContract:
    """Python and Go ObserveProducerEvalPublicationRequest field names and types match."""

    def test_python_request_field_names_match_go_json_tags(self, tmp_path: Path) -> None:
        """The Python model field names match the Go struct JSON tags byte-for-byte."""
        bundle_dir, report = _verify_bundle(tmp_path)
        manifest = _read_bundle_manifest(bundle_dir)
        analysis = _read_bundle_analysis(bundle_dir)
        run_manifest = _read_run_manifest(bundle_dir)

        request = build_publication_request(
            bundle_dir, manifest, analysis, run_manifest, report,
            web_session_id="ws-1", cli_session_id=None,
        )
        dumped = request.model_dump(mode="json", by_alias=True, exclude_none=False)

        # These are the exact JSON field names from the Go struct tags in
        # internal/models/eval_bundle.go ObserveProducerEvalPublicationRequest
        # after the campaign-aware evolution: arm_id -> arm_ids, model_id and
        # model_provider removed, campaign_id/model_cohort_ids/assignment_count
        # added.
        expected_fields = {
            "schema_version", "bundle_id", "run_id", "release_version",
            "suite_id", "suite_version",
            "campaign_id", "arm_ids", "model_cohort_ids", "assignment_count",
            "receipt_count", "assigned_tasks", "terminal_attempts",
            "metrics", "verification_report", "bundle_manifest",
            "downloads", "web_session_id", "cli_session_id",
        }
        assert set(dumped.keys()) == expected_fields

    def test_python_response_field_names_match_go_json_tags(self) -> None:
        """The Python response model field names match the Go struct JSON tags."""
        response = ObserveProducerResponse(accepted=True)
        dumped = response.model_dump(mode="json", by_alias=True)
        assert set(dumped.keys()) == {"accepted"}

    def test_python_request_rejects_unknown_fields(self) -> None:
        """The Python model enforces extra='forbid' for unknown field rejection."""
        valid_payload = {
            "schema_version": "1.0.0",
            "bundle_id": "b",
            "run_id": "r",
            "release_version": "v",
            "suite_id": "s",
            "suite_version": "1.0.0",
            "campaign_id": "",
            "arm_ids": ["a"],
            "model_cohort_ids": [],
            "assignment_count": 0,
            "receipt_count": 0,
            "assigned_tasks": 0,
            "terminal_attempts": 0,
            "metrics": [],
            "verification_report": {
                "schema_version": "1.0.0",
                "bundle_id": "b",
                "run_id": "r",
                "release_version": "v",
                "verified_at": "2026-01-01T00:00:00Z",
                "ok": True,
                "layers": [],
            },
            "bundle_manifest": {
                "schema_version": "1.0.0",
                "bundle_id": "b",
                "run_id": "r",
                "release_version": "v",
                "created_at": "2026-01-01T00:00:00Z",
                "artifacts": [],
            },
            "downloads": [],
            "web_session_id": "ws-1",
        }
        payload_with_unknown = {**valid_payload, "unknown_field": "bad"}
        with pytest.raises(ValidationError):
            ObserveProducerEvalPublicationRequest.model_validate(payload_with_unknown)

    def test_python_request_rejects_user_id(self) -> None:
        """The request model must not accept a user_id field."""
        valid_payload = {
            "schema_version": "1.0.0",
            "bundle_id": "b",
            "run_id": "r",
            "release_version": "v",
            "suite_id": "s",
            "suite_version": "1.0.0",
            "campaign_id": "",
            "arm_ids": ["a"],
            "model_cohort_ids": [],
            "assignment_count": 0,
            "receipt_count": 0,
            "assigned_tasks": 0,
            "terminal_attempts": 0,
            "metrics": [],
            "verification_report": {
                "schema_version": "1.0.0",
                "bundle_id": "b",
                "run_id": "r",
                "release_version": "v",
                "verified_at": "2026-01-01T00:00:00Z",
                "ok": True,
                "layers": [],
            },
            "bundle_manifest": {
                "schema_version": "1.0.0",
                "bundle_id": "b",
                "run_id": "r",
                "release_version": "v",
                "created_at": "2026-01-01T00:00Z",
                "artifacts": [],
            },
            "downloads": [],
        }
        payload_with_user_id = {**valid_payload, "user_id": "should-be-rejected"}
        with pytest.raises(ValidationError):
            ObserveProducerEvalPublicationRequest.model_validate(payload_with_user_id)
