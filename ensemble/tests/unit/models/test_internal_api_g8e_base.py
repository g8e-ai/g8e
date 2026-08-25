# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 5 — Internal API models sourced from g8e.

Verifies:
- ``ResourceCreationRequest`` is re-exported directly from ``g8e.models.internal_api``
- ``ChatStartedResponse`` is re-exported directly from ``g8e.models.internal_api``
- ``ChatMessageRequest`` subclasses ``g8e.models.internal_api.ChatMessageRequest``
- ``ChatMessageRequest`` also inherits from ``RequestOverrides`` (web search fields)
- g8e base fields are preserved on ``ChatMessageRequest``
- g8ee-specific ``RequestOverrides`` fields are present on ``ChatMessageRequest``
- ``ChatMessageRequest`` overrides ``context`` to use g8ee's ``RequestContext``
- ``ChatMessageRequest`` overrides ``attachments`` to use typed ``AttachmentMetadata``
- Serialization round-trip preserves all fields
"""

import pytest

from g8e.models.internal_api import (
    ResourceCreationRequest as G8eResourceCreationRequest,
    ChatStartedResponse as G8eChatStartedResponse,
    ChatMessageRequest as G8eChatMessageRequest,
)

from app.models.internal_api import (
    ResourceCreationRequest,
    ChatStartedResponse,
    ChatMessageRequest,
    RequestOverrides,
)
from app.models.http_context import RequestContext
from app.models.attachments import AttachmentMetadata
from app.constants import G8EE_COMPONENT

pytestmark = pytest.mark.unit


class TestResourceCreationRequestReExport:
    """Verify ResourceCreationRequest is re-exported from g8e."""

    def test_resource_creation_request_is_g8e(self):
        assert ResourceCreationRequest is G8eResourceCreationRequest

    def test_resource_creation_request_fields(self):
        fields = set(ResourceCreationRequest.model_fields.keys())
        assert "create_case" in fields
        assert "case_title" in fields

    def test_resource_creation_request_defaults(self):
        req = ResourceCreationRequest()
        assert req.create_case is False
        assert req.case_title is None


class TestChatStartedResponseReExport:
    """Verify ChatStartedResponse is re-exported from g8e."""

    def test_chat_started_response_is_g8e(self):
        assert ChatStartedResponse is G8eChatStartedResponse

    def test_chat_started_response_fields(self):
        fields = set(ChatStartedResponse.model_fields.keys())
        assert "success" in fields
        assert "case_id" in fields
        assert "investigation_id" in fields

    def test_chat_started_response_construction(self):
        resp = ChatStartedResponse(success=True, case_id="case-1", investigation_id="inv-1")
        assert resp.success is True
        assert resp.case_id == "case-1"
        assert resp.investigation_id == "inv-1"


class TestChatMessageRequestInheritance:
    """Verify ChatMessageRequest subclasses g8e base and RequestOverrides."""

    def test_chat_message_request_subclasses_g8e(self):
        assert issubclass(ChatMessageRequest, G8eChatMessageRequest)

    def test_chat_message_request_inherits_request_overrides(self):
        assert issubclass(ChatMessageRequest, RequestOverrides)

    def test_chat_message_request_is_not_g8e(self):
        assert ChatMessageRequest is not G8eChatMessageRequest


class TestChatMessageRequestG8eBaseFields:
    """Verify g8e base fields are preserved on ChatMessageRequest."""

    def test_has_message_field(self):
        assert "message" in ChatMessageRequest.model_fields

    def test_has_sentinel_mode_field(self):
        assert "sentinel_mode" in ChatMessageRequest.model_fields

    def test_has_resource_creation_field(self):
        assert "resource_creation" in ChatMessageRequest.model_fields

    def test_has_context_field(self):
        assert "context" in ChatMessageRequest.model_fields

    def test_has_attachments_field(self):
        assert "attachments" in ChatMessageRequest.model_fields

    def test_sentinel_mode_default(self):
        ctx = RequestContext(source_component=G8EE_COMPONENT, user_id="u1")
        req = ChatMessageRequest(context=ctx, message="hello")
        assert req.sentinel_mode is True


class TestChatMessageRequestRequestOverridesFields:
    """Verify RequestOverrides (web search) fields are present on ChatMessageRequest."""

    @pytest.mark.parametrize(
        "field",
        [
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
            "web_search_project",
            "web_search_app",
            "web_search_api_key",
        ],
    )
    def test_field_exists(self, field):
        assert field in ChatMessageRequest.model_fields

    def test_override_fields_default_none(self):
        ctx = RequestContext(source_component=G8EE_COMPONENT, user_id="u1")
        req = ChatMessageRequest(context=ctx, message="hello")
        assert req.web_search_project is None
        assert req.web_search_api_key is None
        assert req.llm_primary_provider is None


class TestChatMessageRequestOverrides:
    """Verify g8ee-specific overrides on ChatMessageRequest."""

    def test_context_uses_g8ee_request_context(self):
        field_info = ChatMessageRequest.model_fields["context"]
        assert field_info.annotation is RequestContext

    def test_attachments_uses_attachment_metadata(self):
        field_info = ChatMessageRequest.model_fields["attachments"]
        # The annotation should be list[AttachmentMetadata] | None
        annotation = field_info.annotation
        assert annotation is not None
        # Check that AttachmentMetadata appears in the annotation's args
        # list[AttachmentMetadata] | None -> Union[list[AttachmentMetadata], None]
        args = getattr(annotation, "__args__", ())
        found = False
        for arg in args:
            inner_args = getattr(arg, "__args__", ())
            if AttachmentMetadata in inner_args:
                found = True
                break
        assert found, "AttachmentMetadata not found in attachments annotation"


class TestChatMessageRequestSerialization:
    """Verify serialization round-trip preserves all fields."""

    def test_round_trip_preserves_fields(self):
        ctx = RequestContext(
            source_component=G8EE_COMPONENT,
            user_id="user-123",
            case_id="case-abc",
            investigation_id="inv-xyz",
        )
        req = ChatMessageRequest(
            context=ctx,
            message="test message",
            sentinel_mode=False,
            llm_primary_provider="openai",
            web_search_project="proj-1",
            resource_creation=ResourceCreationRequest(create_case=True, case_title="Test"),
        )
        dumped = req.model_dump(mode="json")
        restored = ChatMessageRequest.model_validate(dumped)

        assert restored.message == "test message"
        assert restored.sentinel_mode is False
        assert restored.llm_primary_provider == "openai"
        assert restored.web_search_project == "proj-1"
        assert restored.resource_creation is not None
        assert restored.resource_creation.create_case is True
        assert restored.resource_creation.case_title == "Test"

    def test_round_trip_preserves_context_fields(self):
        ctx = RequestContext(
            source_component=G8EE_COMPONENT,
            user_id="user-123",
            operator_id="op-1",
            operator_session_id="op-sess-1",
        )
        req = ChatMessageRequest(context=ctx, message="hello")
        dumped = req.model_dump(mode="json")
        restored = ChatMessageRequest.model_validate(dumped)

        assert restored.context.user_id == "user-123"
        assert restored.context.operator_id == "op-1"
        assert restored.context.operator_session_id == "op-sess-1"
