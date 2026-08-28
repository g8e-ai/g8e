# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version.0.

"""
Contract tests for governed document operation payloads.

Enforces three invariants that keep the Python ensemble aligned with the
Go-side DOCUMENT_UPDATE/DOCUMENT_DELETE action types and the
handleDocumentUpdateSync/handleDocumentDeleteSync pubsub handlers:

1. Every payload in the G8eCommandPayload union plus DocumentUpdateRequestPayload
   and DocumentDeleteRequestPayload has a working to_protobuf() method that
   returns the correct protobuf message type.
2. PAYLOAD_TYPE_MAPPING in governance_client.py maps every payload_type
   discriminator to the correct canonical protobuf message name.
3. map_event_type_to_action_type maps all app-level document events to
   DOCUMENT_UPDATE or DOCUMENT_DELETE, matching the Go eventToAction map.
"""

import pytest

from app.constants.action_type_mappings import map_event_type_to_action_type
from app.constants import EventType
from app.models.command_request_payloads import (
    DocumentDeleteRequestPayload,
    DocumentUpdateRequestPayload,
    G8eCommandPayload,
)
from g8e.operator.v1 import operator_pb2
from app.clients.governance_client import PAYLOAD_TYPE_MAPPING
from typing import get_args


pytestmark = [pytest.mark.unit]


class TestDocumentPayloadToProtobuf:
    """Verify to_protobuf() on document payloads produces correct protobuf messages."""

    def test_document_update_to_protobuf_roundtrip(self):
        payload = DocumentUpdateRequestPayload(
            collection="cases",
            document_id="case-1",
            updates={"title": "T", "count": 3, "tags": ["a", "b"]},
            merge=False,
        )
        proto = payload.to_protobuf()
        assert isinstance(proto, operator_pb2.DocumentUpdateRequested)
        assert proto.collection == "cases"
        assert proto.document_id == "case-1"
        assert proto.merge is False
        # Struct fields are accessible via the google.protobuf.struct_pb2 API
        fields = proto.updates.fields
        assert fields["title"].string_value == "T"
        assert fields["count"].number_value == 3.0
        assert [v.string_value for v in fields["tags"].list_value.values] == ["a", "b"]

    def test_document_update_to_protobuf_default_merge(self):
        payload = DocumentUpdateRequestPayload(
            collection="investigations",
            document_id="inv-1",
            updates={"status": "open"},
        )
        proto = payload.to_protobuf()
        assert isinstance(proto, operator_pb2.DocumentUpdateRequested)
        assert proto.merge is True
        assert proto.updates.fields["status"].string_value == "open"

    def test_document_update_to_protobuf_empty_updates(self):
        payload = DocumentUpdateRequestPayload(
            collection="memories",
            document_id="mem-1",
            updates={},
        )
        proto = payload.to_protobuf()
        assert isinstance(proto, operator_pb2.DocumentUpdateRequested)
        assert len(proto.updates.fields) == 0

    def test_document_delete_to_protobuf(self):
        payload = DocumentDeleteRequestPayload(
            collection="cases",
            document_id="case-1",
        )
        proto = payload.to_protobuf()
        assert isinstance(proto, operator_pb2.DocumentDeleteRequested)
        assert proto.collection == "cases"
        assert proto.document_id == "case-1"

    def test_document_update_serializable_to_bytes(self):
        """to_protobuf() output must be serializable for the envelope payload field."""
        payload = DocumentUpdateRequestPayload(
            collection="cases",
            document_id="case-1",
            updates={"title": "T"},
        )
        proto = payload.to_protobuf()
        raw = proto.SerializeToString()
        assert isinstance(raw, bytes) and len(raw) > 0
        # Round-trip parse
        parsed = operator_pb2.DocumentUpdateRequested()
        parsed.ParseFromString(raw)
        assert parsed.collection == "cases"
        assert parsed.document_id == "case-1"

    def test_document_delete_serializable_to_bytes(self):
        payload = DocumentDeleteRequestPayload(collection="cases", document_id="case-1")
        proto = payload.to_protobuf()
        raw = proto.SerializeToString()
        assert isinstance(raw, bytes) and len(raw) > 0
        parsed = operator_pb2.DocumentDeleteRequested()
        parsed.ParseFromString(raw)
        assert parsed.collection == "cases"
        assert parsed.document_id == "case-1"


class TestG8eCommandPayloadToProtobufContract:
    """Every payload in G8eCommandPayload must implement to_protobuf() returning a
    protobuf message whose type matches the canonical PAYLOAD_TYPE_MAPPING entry.
    """

    def test_all_command_payloads_have_to_protobuf(self):
        missing = []
        for cls in get_args(G8eCommandPayload):
            if not hasattr(cls, "to_protobuf") or not callable(getattr(cls, "to_protobuf")):
                missing.append(cls.__name__)
        assert not missing, (
            f"Payloads missing to_protobuf(): {missing}. Every payload in "
            "G8eCommandPayload must implement to_protobuf() so the envelope "
            "builder can serialize it for the governance gauntlet."
        )

    def test_document_payloads_have_to_protobuf(self):
        for cls in (DocumentUpdateRequestPayload, DocumentDeleteRequestPayload):
            assert hasattr(cls, "to_protobuf") and callable(cls.to_protobuf), (
                f"{cls.__name__} must implement to_protobuf()"
            )


class TestPayloadTypeMappingContract:
    """PAYLOAD_TYPE_MAPPING must map every payload_type discriminator to the
    correct canonical protobuf message name.
    """

    @pytest.mark.parametrize(
        "discriminator,expected",
        [
            ("command", "CommandRequested"),
            ("command_cancel", "CommandCancelRequested"),
            ("file_edit", "FileEditRequested"),
            ("fs_list", "FsListRequested"),
            ("fs_grep", "FsGrepRequested"),
            ("fs_read", "FsReadRequested"),
            ("fetch_logs", "FetchLogsRequested"),
            ("fetch_history", "FetchHistoryRequested"),
            ("fetch_file_history", "FetchFileHistoryRequested"),
            ("fetch_file_diff", "FetchFileDiffRequested"),
            ("restore_file", "RestoreFileRequested"),
            ("check_port", "CheckPortRequested"),
            ("heartbeat", "HeartbeatRequested"),
            ("document_update", "DocumentUpdateRequested"),
            ("document_delete", "DocumentDeleteRequested"),
            ("direct_command_audit", "DirectCommandAuditRequested"),
        ],
    )
    def test_payload_type_mapping_canonical(self, discriminator, expected):
        actual = PAYLOAD_TYPE_MAPPING.get(discriminator)
        assert actual == expected, (
            f"PAYLOAD_TYPE_MAPPING[{discriminator!r}] = {actual!r}, expected {expected!r}. "
            "Document mutations must map to DocumentUpdateRequested/DocumentDeleteRequested, "
            "not FileEditRequested or AuditMsgRequested."
        )

    def test_no_investigation_create_mapping(self):
        """investigation_create discriminator must not exist — it was removed when
        InvestigationCreateRequestPayload was consolidated into DocumentUpdateRequestPayload.
        """
        assert "investigation_create" not in PAYLOAD_TYPE_MAPPING, (
            "investigation_create must not appear in PAYLOAD_TYPE_MAPPING. "
            "Investigation creation now uses DocumentUpdateRequestPayload with "
            "payload_type='document_update'."
        )

    def test_no_document_mapping_to_file_edit(self):
        """Document mutations must never map to FileEditRequested."""
        assert PAYLOAD_TYPE_MAPPING.get("document_update") != "FileEditRequested"
        assert PAYLOAD_TYPE_MAPPING.get("document_delete") != "FileEditRequested"


class TestEventTypeToActionTypeContract:
    """map_event_type_to_action_type must map all app-level document events to
    DOCUMENT_UPDATE or DOCUMENT_DELETE, matching the Go eventToAction map in
    internal/constants/mappings.go.
    """

    @pytest.mark.parametrize(
        "event_type,expected",
        [
            (EventType.APP_CASE_CREATED, "DOCUMENT_UPDATE"),
            (EventType.APP_CASE_UPDATED, "DOCUMENT_UPDATE"),
            (EventType.APP_CASE_DELETED, "DOCUMENT_DELETE"),
            (EventType.APP_MEMORY_CREATED, "DOCUMENT_UPDATE"),
            (EventType.APP_MEMORY_UPDATED, "DOCUMENT_UPDATE"),
            (EventType.APP_INVESTIGATION_CREATED, "DOCUMENT_UPDATE"),
            (EventType.APP_INVESTIGATION_UPDATED, "DOCUMENT_UPDATE"),
            (EventType.APP_INVESTIGATION_DELETED, "DOCUMENT_DELETE"),
        ],
    )
    def test_app_document_events_map_to_document_actions(self, event_type, expected):
        actual = map_event_type_to_action_type(event_type)
        assert actual == expected, (
            f"map_event_type_to_action_type({event_type}) = {actual!r}, "
            f"expected {expected!r}. App-level document events must route through "
            "DOCUMENT_UPDATE/DOCUMENT_DELETE to match the Go eventToAction map."
        )
