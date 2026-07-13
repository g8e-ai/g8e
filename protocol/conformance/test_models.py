"""Cross-language protocol model conformance tests.

Validates that Python Pydantic models in the g8e package produce JSON that is
structurally compatible with the canonical model schemas in protocol/models/.

The JSON files in protocol/models/ define field names, types, and required
flags. The Python Pydantic models must produce JSON with matching field names
and compatible types when serialized.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

MODELS_DIR = Path(__file__).parent.parent / "models"


def _load_model_schema(filename: str) -> dict[str, Any]:
    path = MODELS_DIR / filename
    assert path.exists(), f"Model schema not found: {path}"
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def _extract_field_names(schema: dict[str, Any], section: str = "settings") -> set[str]:
    """Extract field names from a model schema section."""
    fields = set()
    if section in schema:
        section_data = schema[section]
        if isinstance(section_data, dict):
            for key, val in section_data.items():
                if isinstance(val, dict) and "type" in val:
                    fields.add(key)
                elif isinstance(val, dict):
                    fields.update(_extract_field_names(val))
    return fields


def _extract_all_field_names(schema: dict[str, Any]) -> set[str]:
    """Recursively extract all field names with a 'type' key from a schema."""
    fields = set()
    if isinstance(schema, dict):
        for key, val in schema.items():
            if isinstance(val, dict) and "type" in val:
                fields.add(key)
            elif isinstance(val, dict):
                fields.update(_extract_all_field_names(val))
    return fields


class TestModelSchemaIntegrity:
    """Verify all model JSON schemas are valid and have expected structure."""

    MODEL_FILES = [
        "account_lock.json",
        "agent_activity_metadata.json",
        "app_policy.json",
        "approval.json",
        "auth_admin_audit.json",
        "bound_session.json",
        "case.json",
        "chat_message.json",
        "cli_session.json",
        "console_audit.json",
        "conversation.json",
        "conversation_message.json",
        "enrollment_token.json",
        "execution_result.json",
        "file_edit.json",
        "fs_grep.json",
        "fs_list.json",
        "heartbeat.json",
        "investigation.json",
        "local_os_user.json",
        "login_audit.json",
        "memory.json",
        "operator_document.json",
        "operator_session.json",
        "operator_usage.json",
        "organization.json",
        "passkey_challenge.json",
        "passkey_credential.json",
        "persona.json",
        "platform_settings.json",
        "reputation_commitment.json",
        "reputation_state.json",
        "request_context.json",
        "revoked_certificate.json",
        "runtime_config.json",
        "security_constraints.json",
        "sse_event_payloads.json",
        "sse_event_wire.json",
        "sse_push_payload.json",
        "stake_resolution.json",
        "task.json",
        "terminal_output.json",
        "tool_results.json",
        "tribunal.json",
        "trusted_signer.json",
        "user.json",
        "user_settings.json",
        "web_session.json",
        "webauthn_response.json",
    ]

    @pytest.mark.parametrize("filename", MODEL_FILES)
    def test_model_file_loads_successfully(self, filename: str):
        data = _load_model_schema(filename)
        assert isinstance(data, dict), f"{filename} must be a JSON object"

    @pytest.mark.parametrize("filename", MODEL_FILES)
    def test_model_file_has_metadata(self, filename: str):
        data = _load_model_schema(filename)
        assert len(data) > 0, f"{filename} is empty"


class TestPlatformSettingsConformance:
    """Verify Python PlatformSettings model matches the JSON schema."""

    def test_platform_settings_fields_match_schema(self):
        from g8e.models import PlatformSettings

        schema = _load_model_schema("platform_settings.json")
        schema_fields = _extract_field_names(schema, "platform_settings")

        py_fields = set(PlatformSettings.model_fields.keys())

        known_metadata = {"created_at", "updated_at"}
        schema_only = schema_fields - py_fields - known_metadata
        assert not schema_only, (
            f"Schema fields not in Python model: {schema_only}"
        )


class TestRequestContextConformance:
    """Verify RequestContext model produces valid JSON round-trips."""

    def test_request_context_serialization_round_trip(self):
        from g8e.models import RequestContext

        ctx = RequestContext(
            source_component="client",
            web_session_id="test-session-123",
            user_id="test-user-456",
        )
        json_str = ctx.model_dump_json()
        parsed = json.loads(json_str)

        assert parsed["source_component"] == "client"
        assert parsed["web_session_id"] == "test-session-123"
        assert parsed["user_id"] == "test-user-456"

    def test_request_context_client_requires_session_and_user(self):
        from g8e.models import RequestContext
        from pydantic import ValidationError

        with pytest.raises(ValidationError):
            RequestContext(source_component="client")

    def test_request_context_non_client_does_not_require_session(self):
        from g8e.models import RequestContext

        ctx = RequestContext(source_component="g8eo")
        assert ctx.source_component == "g8eo"

    def test_request_context_schema_fields_match_model(self):
        from g8e.models import RequestContext

        schema = _load_model_schema("request_context.json")
        schema_fields = _extract_all_field_names(schema.get("request_context", {}))
        py_fields = set(RequestContext.model_fields.keys())
        schema_only = schema_fields - py_fields
        assert not schema_only, f"Schema fields not in Python model: {schema_only}"


class TestBoundOperatorConformance:
    """Verify BoundOperator model round-trips correctly."""

    def test_bound_operator_serialization(self):
        from g8e.models import BoundOperator

        op = BoundOperator(
            operator_id="op-123",
            operator_session_id="sess-456",
        )
        json_str = op.model_dump_json()
        parsed = json.loads(json_str)

        assert parsed["operator_id"] == "op-123"
        assert parsed["operator_session_id"] == "sess-456"

    def test_bound_operator_schema_fields_match_model(self):
        from g8e.models import BoundOperator

        schema = _load_model_schema("request_context.json")
        schema_fields = _extract_all_field_names(schema.get("bound_operator", {}))
        py_fields = set(BoundOperator.model_fields.keys())
        schema_only = schema_fields - py_fields
        assert not schema_only, f"Schema fields not in Python model: {schema_only}"


class TestSerializationParity:
    """Verify that G8eBaseModel serialization follows canonical conventions."""

    def test_exclude_none_on_serialization(self):
        from g8e.models import RequestContext

        ctx = RequestContext(source_component="g8eo")
        dumped = ctx.model_dump()
        assert "web_session_id" not in dumped
        assert "user_id" not in dumped

    def test_extra_fields_ignored(self):
        from g8e.models import RequestContext

        ctx = RequestContext(source_component="g8eo", unknown_field="value")
        dumped = ctx.model_dump()
        assert "unknown_field" not in dumped


class TestUserSettingsConformance:
    """Verify Python G8eeUserSettings model matches the JSON schema."""

    def test_llm_settings_fields_match_schema(self):
        from g8e.models import LLMSettings

        schema = _load_model_schema("user_settings.json")
        schema_fields = _extract_all_field_names(
            schema.get("user_settings", {}).get("settings", {}).get("llm", {})
        )
        py_fields = set(LLMSettings.model_fields.keys())
        py_aliases = {
            f.alias for f in LLMSettings.model_fields.values() if f.alias
        }
        py_all = py_fields | py_aliases
        schema_only = schema_fields - py_all
        assert not schema_only, f"Schema LLM fields not in Python model: {schema_only}"

    def test_search_settings_fields_match_schema(self):
        from g8e.models import SearchSettings

        schema = _load_model_schema("user_settings.json")
        schema_fields = _extract_all_field_names(
            schema.get("user_settings", {}).get("settings", {}).get("search", {})
        )
        py_fields = set(SearchSettings.model_fields.keys())
        py_aliases = {
            f.alias for f in SearchSettings.model_fields.values() if f.alias
        }
        py_all = py_fields | py_aliases
        schema_only = schema_fields - py_all
        assert not schema_only, f"Schema search fields not in Python model: {schema_only}"

    def test_command_validation_settings_fields_match_schema(self):
        from g8e.models import CommandValidationSettings

        schema = _load_model_schema("user_settings.json")
        schema_fields = _extract_all_field_names(
            schema.get("user_settings", {}).get("settings", {}).get("command_validation", {})
        )
        py_fields = set(CommandValidationSettings.model_fields.keys())
        schema_only = schema_fields - py_fields
        assert not schema_only, f"Schema command_validation fields not in Python model: {schema_only}"

    def test_batch_execution_settings_fields_match_schema(self):
        from g8e.models import BatchExecutionSettings

        schema = _load_model_schema("user_settings.json")
        schema_fields = _extract_all_field_names(
            schema.get("user_settings", {}).get("settings", {}).get("batch_execution", {})
        )
        py_fields = set(BatchExecutionSettings.model_fields.keys())
        schema_only = schema_fields - py_fields
        assert not schema_only, f"Schema batch_execution fields not in Python model: {schema_only}"


class TestChatMessageRequestConformance:
    """Verify ChatMessageRequest and related models produce valid round-trips."""

    def test_chat_message_request_round_trip(self):
        from g8e.models import ChatMessageRequest, RequestContext

        req = ChatMessageRequest(
            context=RequestContext(
                source_component="client",
                web_session_id="sess-1",
                user_id="user-1",
            ),
            message="Hello, world",
        )
        parsed = json.loads(req.model_dump_json())
        assert parsed["message"] == "Hello, world"
        assert parsed["context"]["source_component"] == "client"
        assert parsed["context"]["web_session_id"] == "sess-1"

    def test_chat_message_request_with_resource_creation(self):
        from g8e.models import ChatMessageRequest, RequestContext, ResourceCreationRequest

        req = ChatMessageRequest(
            context=RequestContext(
                source_component="client",
                web_session_id="sess-1",
                user_id="user-1",
            ),
            message="Create a case",
            resource_creation=ResourceCreationRequest(create_case=True, case_title="Test Case"),
        )
        parsed = json.loads(req.model_dump_json())
        assert parsed["resource_creation"]["create_case"] is True
        assert parsed["resource_creation"]["case_title"] == "Test Case"

    def test_chat_started_response_round_trip(self):
        from g8e.models import ChatStartedResponse

        resp = ChatStartedResponse(
            success=True,
            case_id="case-1",
            investigation_id="inv-1",
        )
        parsed = json.loads(resp.model_dump_json())
        assert parsed["success"] is True
        assert parsed["case_id"] == "case-1"
        assert parsed["investigation_id"] == "inv-1"

    def test_chat_message_request_schema_fields_match_model(self):
        from g8e.models import ChatMessageRequest

        schema = _load_model_schema("chat_message.json")
        schema_fields = _extract_all_field_names(schema.get("chat_message_request", {}))
        py_fields = set(ChatMessageRequest.model_fields.keys())
        schema_only = schema_fields - py_fields
        assert not schema_only, f"Schema fields not in Python model: {schema_only}"


class TestEventPayloadConformance:
    """Verify SSE event payload models round-trip correctly."""

    def test_ai_processing_stopped_payload_round_trip(self):
        from g8e.models import AiProcessingStoppedPayload

        payload = AiProcessingStoppedPayload(reason="timeout", timestamp="2026-01-01T00:00:00Z")
        parsed = json.loads(payload.model_dump_json())
        assert parsed["reason"] == "timeout"

    def test_ai_tool_lifecycle_payload_round_trip(self):
        from g8e.models import AIToolLifecyclePayload

        payload = AIToolLifecyclePayload(
            tool_name="shell",
            execution_id="exec-1",
            status="running",
        )
        parsed = json.loads(payload.model_dump_json())
        assert parsed["tool_name"] == "shell"
        assert parsed["execution_id"] == "exec-1"
        assert parsed["status"] == "running"

    def test_chat_error_payload_round_trip(self):
        from g8e.models import ChatErrorPayload

        payload = ChatErrorPayload(error="Something went wrong")
        parsed = json.loads(payload.model_dump_json())
        assert parsed["error"] == "Something went wrong"

    def test_chat_processing_started_payload_round_trip(self):
        from g8e.models import ChatProcessingStartedPayload

        payload = ChatProcessingStartedPayload(agent_mode="primary")
        parsed = json.loads(payload.model_dump_json())
        assert parsed["agent_mode"] == "primary"

    def test_chat_response_chunk_payload_round_trip(self):
        from g8e.models import ChatResponseChunkPayload

        payload = ChatResponseChunkPayload(content="chunk text")
        parsed = json.loads(payload.model_dump_json())
        assert parsed["content"] == "chunk text"

    def test_chat_response_complete_payload_round_trip(self):
        from g8e.models import ChatResponseCompletePayload

        payload = ChatResponseCompletePayload(
            content="full response",
            finish_reason="stop",
            has_citations=False,
            grounding_metadata={},
            token_usage={},
            agent_mode="primary",
        )
        parsed = json.loads(payload.model_dump_json())
        assert parsed["content"] == "full response"
        assert parsed["finish_reason"] == "stop"

    def test_chat_retry_payload_round_trip(self):
        from g8e.models import ChatRetryPayload

        payload = ChatRetryPayload(attempt=1, max_attempts=3)
        parsed = json.loads(payload.model_dump_json())
        assert parsed["attempt"] == 1
        assert parsed["max_attempts"] == 3

    def test_chat_thinking_payload_round_trip(self):
        from g8e.models import ChatThinkingPayload

        payload = ChatThinkingPayload(thinking="hmm", action_type="execute_bash")
        parsed = json.loads(payload.model_dump_json())
        assert parsed["thinking"] == "hmm"
        assert parsed["action_type"] == "execute_bash"

    def test_chat_turn_complete_payload_round_trip(self):
        from g8e.models import ChatTurnCompletePayload

        payload = ChatTurnCompletePayload(turn=1)
        parsed = json.loads(payload.model_dump_json())
        assert parsed["turn"] == 1

    def test_triage_clarification_questions_payload_round_trip(self):
        from g8e.models import TriageClarificationQuestionsPayload

        payload = TriageClarificationQuestionsPayload(
            questions=["What?"],
            complexity="low",
            complexity_confidence="high",
            intent="query",
            intent_confidence="high",
            intent_summary="User wants info",
            request_posture="doctrine",
            posture_confidence="high",
        )
        parsed = json.loads(payload.model_dump_json())
        assert parsed["questions"] == ["What?"]
        assert parsed["complexity"] == "low"

    def test_session_event_wire_round_trip(self):
        from g8e.models import SessionEventWire

        event = SessionEventWire(
            user_id="user-1",
            event={"type": "chat_error", "data": {"error": "test"}},
        )
        parsed = json.loads(event.model_dump_json())
        assert parsed["user_id"] == "user-1"
        assert parsed["event"]["type"] == "chat_error"

    def test_background_event_wire_round_trip(self):
        from g8e.models import BackgroundEventWire

        event = BackgroundEventWire(
            user_id="user-1",
            event={"type": "chat_error", "data": {"error": "test"}},
        )
        parsed = json.loads(event.model_dump_json())
        assert parsed["user_id"] == "user-1"
        assert parsed["event"]["type"] == "chat_error"


class TestUserSchemaZeroPII:
    """Verify user.json schema has no PII fields (zero-PII architecture)."""

    def test_no_email_field(self):
        schema = _load_model_schema("user.json")
        all_fields = _extract_all_field_names(schema)
        assert "email" not in all_fields, "user.json must not contain email field (zero-PII)"

    def test_no_name_field(self):
        schema = _load_model_schema("user.json")
        all_fields = _extract_all_field_names(schema)
        assert "name" not in all_fields, "user.json must not contain name field (zero-PII)"

    def test_no_password_hash_field(self):
        schema = _load_model_schema("user.json")
        all_fields = _extract_all_field_names(schema)
        assert "password_hash" not in all_fields, "user.json must not contain password_hash field (zero-PII)"


class TestSchemaCrossReference:
    """Verify all _ref pointers in schemas resolve to defined model sections."""

    def _find_refs(self, obj: Any, path: str = "") -> list[tuple[str, str]]:
        refs: list[tuple[str, str]] = []
        if isinstance(obj, dict):
            for key, val in obj.items():
                if key == "_ref" and isinstance(val, str):
                    refs.append((path, val))
                elif isinstance(val, dict):
                    refs.extend(self._find_refs(val, f"{path}.{key}"))
        return refs

    @pytest.mark.parametrize("filename", TestModelSchemaIntegrity.MODEL_FILES)
    def test_refs_resolve(self, filename: str):
        schema = _load_model_schema(filename)
        refs = self._find_refs(schema)

        for ref_path, ref_value in refs:
            if ref_value.startswith("#"):
                local_section = ref_value.split("#")[-1]
                if local_section:
                    assert local_section in schema, (
                        f"{filename}: _ref '{ref_value}' at {ref_path} "
                        f"points to undefined section '{local_section}'"
                    )
            elif "#" in ref_value:
                target_file, target_section = ref_value.split("#", 1)
                target_path = MODELS_DIR / target_file
                assert target_path.exists(), (
                    f"{filename}: _ref '{ref_value}' at {ref_path} "
                    f"points to missing file '{target_file}'"
                )
                if target_section:
                    with open(target_path, encoding="utf-8") as f:
                        target_schema = json.load(f)
                    assert target_section in target_schema, (
                        f"{filename}: _ref '{ref_value}' at {ref_path} "
                        f"points to undefined section '{target_section}' in '{target_file}'"
                    )
