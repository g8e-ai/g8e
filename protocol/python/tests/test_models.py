"""Tests for g8e protocol models: instantiation, validation, serialization."""

import pytest
from pydantic import ValidationError

from g8e.constants import ComponentName
from g8e.models import (
    G8eBaseModel,
    RequestContext,
    BoundOperator,
    PlatformSettings,
    G8eeUserSettings,
    ResourceCreationRequest,
    ChatMessageRequest,
    ChatStartedResponse,
    SessionEventWire,
    BackgroundEventWire,
    ChatErrorPayload,
    ChatResponseChunkPayload,
    ChatTurnCompletePayload,
    AiProcessingStoppedPayload,
    AIToolLifecyclePayload,
)
from g8e.models.settings import LLMSettings, SearchSettings


class TestBoundOperator:
    """Verify BoundOperator model."""

    def test_minimal_instantiation(self):
        op = BoundOperator(operator_id="op-1")
        assert op.operator_id == "op-1"
        assert op.operator_session_id is None
        assert op.status is None

    def test_full_instantiation(self):
        op = BoundOperator(
            operator_id="op-1",
            operator_session_id="sess-1",
            status="active",
        )
        assert op.operator_id == "op-1"
        assert op.operator_session_id == "sess-1"
        assert op.status == "active"

    def test_operator_id_is_required(self):
        with pytest.raises(ValidationError):
            BoundOperator()


class TestRequestContext:
    """Verify RequestContext model and validation."""

    def test_valid_web_session_context(self):
        ctx = RequestContext(
            web_session_id="web-123",
            user_id="user-1",
            source_component=ComponentName.CLIENT,
        )
        assert ctx.web_session_id == "web-123"
        assert ctx.user_id == "user-1"

    def test_valid_cli_session_context(self):
        ctx = RequestContext(
            cli_session_id="cli-456",
            user_id="user-1",
            source_component=ComponentName.CLIENT,
        )
        assert ctx.cli_session_id == "cli-456"

    def test_client_requires_session_id(self):
        with pytest.raises(ValidationError):
            RequestContext(
                user_id="user-1",
                source_component=ComponentName.CLIENT,
            )

    def test_client_requires_user_id(self):
        with pytest.raises(ValidationError):
            RequestContext(
                web_session_id="web-123",
                source_component=ComponentName.CLIENT,
            )

    def test_client_cannot_have_both_sessions(self):
        with pytest.raises(ValidationError):
            RequestContext(
                web_session_id="web-123",
                cli_session_id="cli-456",
                user_id="user-1",
                source_component=ComponentName.CLIENT,
            )

    def test_non_client_does_not_require_session(self):
        ctx = RequestContext(source_component=ComponentName.G8EO)
        assert ctx.source_component == ComponentName.G8EO

    def test_bound_operators_default_empty_list(self):
        ctx = RequestContext(source_component=ComponentName.G8EO)
        assert ctx.bound_operators == []

    def test_serialization_round_trip(self):
        ctx = RequestContext(
            web_session_id="web-123",
            user_id="user-1",
            source_component=ComponentName.CLIENT,
            bound_operators=[
                BoundOperator(operator_id="op-1", status="active"),
            ],
        )
        data = ctx.model_dump(mode="json")
        restored = RequestContext.model_validate(data)
        assert restored.web_session_id == ctx.web_session_id
        assert restored.user_id == ctx.user_id
        assert len(restored.bound_operators) == 1
        assert restored.bound_operators[0].operator_id == "op-1"

    def test_json_serialization_excludes_none(self):
        ctx = RequestContext(source_component=ComponentName.G8EO)
        data = ctx.model_dump(mode="json")
        assert "case_id" not in data
        assert "investigation_id" not in data


class TestPlatformSettings:
    """Verify PlatformSettings model."""

    def test_defaults_all_enabled(self):
        settings = PlatformSettings()
        assert settings.governance_enabled is True
        assert settings.l1_doctrine_enabled is True
        assert settings.l2_consensus_enabled is True
        assert settings.l3_notary_enabled is True
        assert settings.audit_enabled is True
        assert settings.sentinel_enabled is True

    def test_partial_override(self):
        settings = PlatformSettings(governance_enabled=False, sentinel_enabled=False)
        assert settings.governance_enabled is False
        assert settings.sentinel_enabled is False
        assert settings.l1_doctrine_enabled is True


class TestG8eeUserSettings:
    """Verify G8eeUserSettings model."""

    def test_with_llm_settings(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(llm_primary_provider="openai"),
        )
        assert settings.llm.primary_provider == "openai"
        assert isinstance(settings.search, SearchSettings)

    def test_search_defaults(self):
        settings = G8eeUserSettings(llm=LLMSettings())
        assert settings.search.enabled is False
        assert settings.search.location == "global"


class TestChatModels:
    """Verify chat-related models."""

    def test_chat_message_request(self):
        ctx = RequestContext(
            web_session_id="web-1",
            user_id="user-1",
            source_component=ComponentName.CLIENT,
        )
        req = ChatMessageRequest(context=ctx, message="Hello")
        assert req.message == "Hello"
        assert req.sentinel_mode is True

    def test_chat_started_response(self):
        resp = ChatStartedResponse(
            success=True,
            case_id="case-1",
            investigation_id="inv-1",
        )
        assert resp.success is True
        assert resp.case_id == "case-1"

    def test_resource_creation_request_defaults(self):
        req = ResourceCreationRequest()
        assert req.create_case is False
        assert req.case_title is None


class TestEventPayloads:
    """Verify SSE event payload models."""

    def test_chat_error_payload(self):
        payload = ChatErrorPayload(error="Something went wrong")
        assert payload.error == "Something went wrong"

    def test_chat_response_chunk_payload(self):
        payload = ChatResponseChunkPayload(content="chunk text")
        assert payload.content == "chunk text"

    def test_chat_turn_complete_payload(self):
        payload = ChatTurnCompletePayload(turn=3)
        assert payload.turn == 3

    def test_ai_tool_lifecycle_payload(self):
        payload = AIToolLifecyclePayload(
            tool_name="db_read",
            execution_id="exec-1",
            status="completed",
        )
        assert payload.tool_name == "db_read"
        assert payload.execution_id == "exec-1"
        assert payload.status == "completed"

    def test_session_event_wire(self):
        event_body = {"type": "chat.chunk", "data": {"content": "hello"}}
        wire = SessionEventWire(
            user_id="user-1",
            event=event_body,
        )
        assert wire.user_id == "user-1"
        assert wire.event.type == "chat.chunk"

    def test_background_event_wire(self):
        event_body = {"type": "system.alert", "data": {"level": "warning"}}
        wire = BackgroundEventWire(
            user_id="user-1",
            event=event_body,
        )
        assert wire.user_id == "user-1"
        assert wire.event.type == "system.alert"


class TestG8eBaseModel:
    """Verify base model behavior."""

    def test_model_dump_excludes_none_by_default(self):
        op = BoundOperator(operator_id="op-1")
        data = op.model_dump()
        assert "operator_session_id" not in data
        assert "status" not in data

    def test_model_dump_json_excludes_none_by_default(self):
        op = BoundOperator(operator_id="op-1")
        json_str = op.model_dump_json()
        assert "operator_session_id" not in json_str

    def test_extra_fields_ignored(self):
        data = {"operator_id": "op-1", "unknown_field": "value"}
        op = BoundOperator.model_validate(data)
        assert op.operator_id == "op-1"
        assert not hasattr(op, "unknown_field")
