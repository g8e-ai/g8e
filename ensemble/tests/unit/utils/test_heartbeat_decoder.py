# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import base64
import json

import pytest
from google.protobuf import json_format

from app.utils.result_decoder import decode_and_validate_uap_heartbeat
from g8e.common.v1 import common_pb2
from g8e.operator.v1 import operator_pb2

pytestmark = [pytest.mark.unit]


def _heartbeat_envelope(hostname: str, *, include_payload: bool = True) -> dict[str, object]:
    heartbeat = operator_pb2.HeartbeatResult(
        operator_id="op-1",
        operator_session_id="sess-1",
        timestamp="2026-09-25T12:00:00Z",
        status="automatic",
        event_type="g8e.v1.operator.heartbeat.sent",
        system_identity=operator_pb2.SystemIdentity(
            hostname=hostname,
            os="linux",
            architecture="amd64",
            current_user="root",
        ),
    )
    envelope = common_pb2.GovernanceEnvelope(
        operator_id="op-1",
        operator_session_id="sess-1",
        event_type="g8e.v1.operator.heartbeat.sent",
    )
    if include_payload:
        envelope.payload = heartbeat.SerializeToString()
    envelope_dict = json.loads(json_format.MessageToJson(envelope))
    if include_payload:
        envelope_dict["payload"] = base64.b64encode(heartbeat.SerializeToString()).decode("ascii")
    return envelope_dict


def test_decode_and_validate_uap_heartbeat_reads_binary_payload_hostname():
    payload = decode_and_validate_uap_heartbeat(
        _heartbeat_envelope("worker-1"),
        operator_id="op-1",
        operator_session_id="sess-1",
    )

    assert payload.system_identity.hostname == "worker-1"
    assert payload.system_identity.os == "linux"
