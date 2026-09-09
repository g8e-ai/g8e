"""Tests for the observe producer request and response models.

These models are the mTLS-internal wire types the g8ee ensemble sends to the
gateway producer endpoints. They use typed lifecycle enums, reject unknown
fields (extra="forbid"), and carry SSE routing targets (exactly one of
web_session_id or cli_session_id). The gateway derives user_id from the mTLS
peer certificate, never from the request body.
"""

from datetime import UTC, datetime

import pytest
from pydantic import ValidationError

from g8e.models import (
    ObserveProducerAgentStateRequest,
    ObserveProducerRunStateRequest,
    ObserveProducerResponse,
)


SCHEMA_VERSION = "1.0.0"
OBSERVED_AT = datetime(2026, 9, 9, 12, 0, 0, tzinfo=UTC)


class TestObserveProducerAgentStateRequest:
    """Verify the agent producer request model."""

    def test_valid_web_routing(self):
        req = ObserveProducerAgentStateRequest(
            schema_version=SCHEMA_VERSION,
            agent_id="triage",
            display_name="Triage",
            role="triage",
            status="running",
            observed_at=OBSERVED_AT,
            web_session_id="web-session-abc",
        )
        assert req.agent_id == "triage"
        assert req.web_session_id == "web-session-abc"
        assert req.cli_session_id is None

    def test_valid_cli_routing(self):
        req = ObserveProducerAgentStateRequest(
            schema_version=SCHEMA_VERSION,
            agent_id="triage",
            display_name="Triage",
            role="triage",
            status="running",
            observed_at=OBSERVED_AT,
            cli_session_id="cli-session-xyz",
        )
        assert req.cli_session_id == "cli-session-xyz"
        assert req.web_session_id is None

    def test_invalid_status_rejected(self):
        with pytest.raises(ValidationError):
            ObserveProducerAgentStateRequest(
                schema_version=SCHEMA_VERSION,
                agent_id="triage",
                display_name="Triage",
                role="triage",
                status="bogus",
                observed_at=OBSERVED_AT,
                web_session_id="web-1",
            )

    def test_unknown_fields_rejected(self):
        with pytest.raises(ValidationError):
            ObserveProducerAgentStateRequest(
                schema_version=SCHEMA_VERSION,
                agent_id="triage",
                display_name="Triage",
                role="triage",
                status="running",
                observed_at=OBSERVED_AT,
                web_session_id="web-1",
                evil="no",
            )

    def test_missing_routing_accepted_at_protocol_boundary(self):
        """The protocol model accepts missing routing; the gateway enforces
        exactly-one routing authoritatively at the service boundary."""
        req = ObserveProducerAgentStateRequest(
            schema_version=SCHEMA_VERSION,
            agent_id="triage",
            display_name="Triage",
            role="triage",
            status="running",
            observed_at=OBSERVED_AT,
        )
        assert req.web_session_id is None
        assert req.cli_session_id is None

    def test_utc_timestamp_accepted(self):
        req = ObserveProducerAgentStateRequest(
            schema_version=SCHEMA_VERSION,
            agent_id="triage",
            display_name="Triage",
            role="triage",
            status="running",
            observed_at=OBSERVED_AT,
            web_session_id="web-1",
        )
        assert req.observed_at.tzinfo is not None

    def test_no_user_id_field(self):
        """The producer request must not carry user_id — the gateway derives
        it from the mTLS peer certificate."""
        req = ObserveProducerAgentStateRequest(
            schema_version=SCHEMA_VERSION,
            agent_id="triage",
            display_name="Triage",
            role="triage",
            status="running",
            observed_at=OBSERVED_AT,
            web_session_id="web-1",
        )
        dumped = req.model_dump_json()
        assert "user_id" not in dumped


class TestObserveProducerRunStateRequest:
    """Verify the run producer request model."""

    def test_valid_web_routing(self):
        req = ObserveProducerRunStateRequest(
            schema_version=SCHEMA_VERSION,
            run_id="run-1",
            run_kind="investigation",
            display_name="Investigation",
            status="running",
            completed_tasks=1,
            total_tasks=5,
            observed_at=OBSERVED_AT,
            web_session_id="web-1",
        )
        assert req.run_id == "run-1"
        assert req.run_kind == "investigation"

    def test_valid_cli_routing(self):
        req = ObserveProducerRunStateRequest(
            schema_version=SCHEMA_VERSION,
            run_id="run-1",
            run_kind="investigation",
            display_name="Investigation",
            status="running",
            completed_tasks=1,
            total_tasks=5,
            observed_at=OBSERVED_AT,
            cli_session_id="cli-1",
        )
        assert req.cli_session_id == "cli-1"

    def test_invalid_run_kind_rejected(self):
        with pytest.raises(ValidationError):
            ObserveProducerRunStateRequest(
                schema_version=SCHEMA_VERSION,
                run_id="run-1",
                run_kind="bogus",
                display_name="Investigation",
                status="running",
                completed_tasks=1,
                total_tasks=5,
                observed_at=OBSERVED_AT,
                web_session_id="web-1",
            )

    def test_invalid_status_rejected(self):
        with pytest.raises(ValidationError):
            ObserveProducerRunStateRequest(
                schema_version=SCHEMA_VERSION,
                run_id="run-1",
                run_kind="investigation",
                display_name="Investigation",
                status="bogus",
                completed_tasks=1,
                total_tasks=5,
                observed_at=OBSERVED_AT,
                web_session_id="web-1",
            )

    def test_unknown_fields_rejected(self):
        with pytest.raises(ValidationError):
            ObserveProducerRunStateRequest(
                schema_version=SCHEMA_VERSION,
                run_id="run-1",
                run_kind="investigation",
                display_name="Investigation",
                status="running",
                completed_tasks=1,
                total_tasks=5,
                observed_at=OBSERVED_AT,
                web_session_id="web-1",
                evil="no",
            )

    def test_utc_timestamps_accepted(self):
        started = datetime(2026, 9, 9, 11, 0, 0, tzinfo=UTC)
        req = ObserveProducerRunStateRequest(
            schema_version=SCHEMA_VERSION,
            run_id="run-1",
            run_kind="investigation",
            display_name="Investigation",
            status="running",
            completed_tasks=1,
            total_tasks=5,
            started_at=started,
            observed_at=OBSERVED_AT,
            web_session_id="web-1",
        )
        assert req.started_at.tzinfo is not None

    def test_no_user_id_field(self):
        req = ObserveProducerRunStateRequest(
            schema_version=SCHEMA_VERSION,
            run_id="run-1",
            run_kind="investigation",
            display_name="Investigation",
            status="running",
            completed_tasks=1,
            total_tasks=5,
            observed_at=OBSERVED_AT,
            web_session_id="web-1",
        )
        dumped = req.model_dump_json()
        assert "user_id" not in dumped


class TestObserveProducerResponse:
    """Verify the producer response model."""

    def test_accepted_true(self):
        resp = ObserveProducerResponse(accepted=True)
        assert resp.accepted is True
        dumped = resp.model_dump_json()
        assert '"accepted":true' in dumped

    def test_accepted_default_false(self):
        resp = ObserveProducerResponse()
        assert resp.accepted is False

    def test_unknown_fields_rejected(self):
        with pytest.raises(ValidationError):
            ObserveProducerResponse(accepted=True, evil="no")

    def test_no_user_id_or_record_fields(self):
        resp = ObserveProducerResponse(accepted=True)
        dumped = resp.model_dump_json()
        assert "user_id" not in dumped
        assert "agent_id" not in dumped
        assert "run_id" not in dumped
