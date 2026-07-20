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
    LLMOverrides,
    ChatMessageRequest,
    ChatStartedResponse,
    SessionEventWire,
    BackgroundEventWire,
    ChatErrorPayload,
    ChatResponseChunkPayload,
    ChatTurnCompletePayload,
    AiProcessingStoppedPayload,
    AIToolLifecyclePayload,
    TriageClarificationQuestionsPayload,
    GovernanceEnvelope,
    GovernanceMetadata,
    GovernanceL1,
    GovernanceL2,
    GovernanceL2Vote,
    GovernanceL3,
    GovernanceL3Proof,
    compute_transaction_hash,
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
        event_body = {"type": "g8e.v1.ai.llm.chat.iteration.text.chunk.received", "data": {"content": "hello"}}
        wire = SessionEventWire(
            user_id="user-1",
            event=event_body,
        )
        assert wire.user_id == "user-1"
        assert wire.event.type == "g8e.v1.ai.llm.chat.iteration.text.chunk.received"

    def test_background_event_wire(self):
        event_body = {"type": "g8e.v1.operator.command.requested", "data": {"level": "warning"}}
        wire = BackgroundEventWire(
            user_id="user-1",
            event=event_body,
        )
        assert wire.user_id == "user-1"
        assert wire.event.type == "g8e.v1.operator.command.requested"

    def test_sse_event_body_rejects_invalid_event_type(self):
        with pytest.raises(ValidationError):
            SessionEventWire(
                user_id="user-1",
                event={"type": "not.a.real.event", "data": {}},
            )


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


class TestTriageClarificationPayload:
    """Verify TriageClarificationQuestionsPayload optional fields."""

    def test_minimal_with_only_questions(self):
        payload = TriageClarificationQuestionsPayload(questions=["What happened?"])
        assert payload.questions == ["What happened?"]
        assert payload.complexity is None
        assert payload.intent is None

    def test_full_instantiation(self):
        payload = TriageClarificationQuestionsPayload(
            questions=["Q1", "Q2"],
            complexity="high",
            complexity_confidence="0.9",
            intent="discovery",
            intent_confidence="0.8",
            intent_summary="testing",
            request_posture="active",
            posture_confidence="0.7",
        )
        assert payload.complexity == "high"
        assert payload.intent == "discovery"


class TestLLMOverrides:
    """Verify LLMOverrides mixin and ChatMessageRequest inheritance."""

    def test_llm_overrides_standalone(self):
        overrides = LLMOverrides(llm_primary_provider="openai")
        assert overrides.llm_primary_provider == "openai"
        assert overrides.llm_assistant_provider is None

    def test_chat_message_request_inherits_overrides(self):
        ctx = RequestContext(
            web_session_id="web-1",
            user_id="user-1",
            source_component=ComponentName.CLIENT,
        )
        req = ChatMessageRequest(
            context=ctx,
            message="Hello",
            llm_primary_provider="anthropic",
            llm_primary_model="claude-3",
        )
        assert req.llm_primary_provider == "anthropic"
        assert req.llm_primary_model == "claude-3"
        assert req.llm_lite_provider is None

    def test_llm_overrides_all_fields_present(self):
        overrides = LLMOverrides(
            llm_primary_provider="a",
            llm_assistant_provider="b",
            llm_lite_provider="c",
            llm_primary_model="d",
            llm_assistant_model="e",
            llm_lite_model="f",
            llm_primary_api_key="k1",
            llm_primary_endpoint="u1",
            llm_assistant_api_key="k2",
            llm_assistant_endpoint="u2",
            llm_lite_api_key="k3",
            llm_lite_endpoint="u3",
        )
        for field in (
            "llm_primary_provider",
            "llm_assistant_provider",
            "llm_lite_provider",
            "llm_primary_model",
            "llm_assistant_model",
            "llm_lite_model",
            "llm_primary_api_key",
            "llm_primary_endpoint",
            "llm_assistant_api_key",
            "llm_assistant_endpoint",
            "llm_lite_api_key",
            "llm_lite_endpoint",
        ):
            assert getattr(overrides, field) is not None


class TestEventFactoryMethods:
    """Verify factory methods on SessionEventWire and BackgroundEventWire."""

    def test_session_event_wire_from_session_event(self):
        wire = SessionEventWire.from_session_event(
            "g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            {"content": "hello"},
            web_session_id="web-1",
            user_id="user-1",
        )
        assert wire.user_id == "user-1"
        assert wire.web_session_id == "web-1"
        assert wire.event.type == "g8e.v1.ai.llm.chat.iteration.text.chunk.received"
        assert wire.event.data == {"content": "hello"}

    def test_session_event_wire_from_session_event_round_trip(self):
        wire = SessionEventWire.from_session_event(
            "g8e.v1.ai.llm.chat.iteration.text.completed",
            {"content": "done", "finish_reason": "stop"},
            cli_session_id="cli-1",
            user_id="user-2",
        )
        data = wire.model_dump(mode="json")
        restored = SessionEventWire.model_validate(data)
        assert restored.event.type == "g8e.v1.ai.llm.chat.iteration.text.completed"
        assert restored.event.data["finish_reason"] == "stop"
        assert restored.cli_session_id == "cli-1"

    def test_background_event_wire_from_background_event(self):
        wire = BackgroundEventWire.from_background_event(
            "g8e.v1.operator.command.requested",
            {"level": "warning"},
            user_id="user-1",
        )
        assert wire.user_id == "user-1"
        assert wire.event.type == "g8e.v1.operator.command.requested"
        assert wire.event.data == {"level": "warning"}


class TestGovernanceEnvelope:
    """Verify GovernanceEnvelope model and compute_transaction_hash."""

    def test_compute_transaction_hash_deterministic(self):
        kwargs = dict(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="abc123",
            nonce="nonce-1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={"command": "ls", "working_directory": "/tmp"},
            requestor_user_id="user-1",
            acting_app_id="app-1",
        )
        hash1 = compute_transaction_hash(**kwargs)
        hash2 = compute_transaction_hash(**kwargs)
        assert hash1 == hash2
        assert len(hash1) == 64

    def test_compute_transaction_hash_changes_with_fields(self):
        base_kwargs = dict(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="abc123",
            nonce="nonce-1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={"command": "ls"},
            requestor_user_id="user-1",
            acting_app_id="app-1",
        )
        hash1 = compute_transaction_hash(**base_kwargs)
        modified = {**base_kwargs, "action_type": "FILE_EDIT"}
        hash2 = compute_transaction_hash(**modified)
        assert hash1 != hash2

    def test_compute_transaction_hash_intent_ordering(self):
        kwargs1 = dict(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="abc",
            nonce="n1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={"a": "1", "b": "2"},
        )
        kwargs2 = dict(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="abc",
            nonce="n1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={"b": "2", "a": "1"},
        )
        assert compute_transaction_hash(**kwargs1) == compute_transaction_hash(**kwargs2)

    def test_compute_transaction_hash_optional_fields(self):
        hash_with_none = compute_transaction_hash(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="abc",
            nonce="n1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={},
        )
        hash_with_empty = compute_transaction_hash(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="abc",
            nonce="n1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={},
            requestor_user_id="",
            acting_app_id="",
        )
        assert hash_with_none == hash_with_empty

    def test_governance_envelope_serialization_round_trip(self):
        tx_hash = compute_transaction_hash(
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            payload="dGVzdA==",
            state_merkle_root="root",
            nonce="nonce-1",
            expires_at="2026-01-01T00:00:00Z",
            intent_data={"command": "ls"},
            requestor_user_id="user-1",
            acting_app_id="app-1",
        )
        envelope = GovernanceEnvelope(
            id=tx_hash,
            timestamp="2026-01-01T00:00:00Z",
            expires_at="2026-01-01T00:00:00Z",
            source_component="COMPONENT_CLIENT",
            event_type="g8e.v1.operator.command.requested",
            payload="dGVzdA==",
            intent_data={"command": "ls"},
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            state_merkle_root="root",
            nonce="nonce-1",
            transaction_hash=tx_hash,
            requestor_user_id="user-1",
            acting_app_id="app-1",
        )
        data = envelope.model_dump(mode="json")
        restored = GovernanceEnvelope.model_validate(data)
        assert restored.id == tx_hash
        assert restored.action_type == "EXECUTE_BASH"
        assert restored.governance.l1.validated is False

    def test_governance_envelope_with_full_governance(self):
        envelope = GovernanceEnvelope(
            id="hash-1",
            timestamp="2026-01-01T00:00:00Z",
            expires_at="2026-01-01T01:00:00Z",
            source_component="COMPONENT_CLIENT",
            event_type="g8e.v1.operator.command.requested",
            payload="dGVzdA==",
            action_type="EXECUTE_BASH",
            target_resource="/tmp",
            state_merkle_root="root",
            nonce="n1",
            governance=GovernanceMetadata(
                l1=GovernanceL1(validated=True, violations=["policy_violation"]),
                l2=GovernanceL2(
                    tribunal_id="tribunal-1",
                    votes=[
                        GovernanceL2Vote(
                            signer_key_id="signer-1",
                            consensus_signature="sig-1",
                            decision=True,
                        ),
                    ],
                ),
                l3=GovernanceL3(
                    proof=GovernanceL3Proof(
                        client_data_json="data",
                        authenticator_data="auth",
                        signature="sig",
                        credential_id="cred-1",
                    ),
                ),
            ),
        )
        assert envelope.governance.l1.validated is True
        assert envelope.governance.l1.violations == ["policy_violation"]
        assert envelope.governance.l2.tribunal_id == "tribunal-1"
        assert len(envelope.governance.l2.votes) == 1
        assert envelope.governance.l2.votes[0].signer_key_id == "signer-1"
        assert envelope.governance.l2.votes[0].decision is True
        assert envelope.governance.l3.proof.signature == "sig"
        assert envelope.governance.l3.proof.credential_id == "cred-1"
